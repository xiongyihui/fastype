package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"fastype/internal/config"
	"fastype/internal/engine"
)

var version = "0.3.0"

// debugDefault 由构建注入：fastype-debug.exe 默认打开调试日志。
var debugDefault = "0"

var (
	eng       *engine.Engine
	engineMu  sync.Mutex // 保护 eng / appConfig / cfgPath
	appConfig *config.Config
	cfgPath   string

	pausedFlag atomic.Bool
	dryRun     bool
	debugOn    bool
)

// apiHandler 实现 internal/ui 的 HTTP API，把请求转交给本包持有的状态。
type apiHandler struct{}

func (apiHandler) Config() any {
	engineMu.Lock()
	defer engineMu.Unlock()
	return appConfig
}

func (apiHandler) SaveConfigJSON(data []byte) error {
	newCfg, err := config.ParseBytes(data)
	if err != nil {
		return fmt.Errorf("配置格式错误: %w", err)
	}
	compiled, err := newCfg.Compile()
	if err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}
	engineMu.Lock()
	defer engineMu.Unlock()
	if err := config.SaveFile(cfgPath, newCfg); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	fx := eng.Reload(compiled) // 释放热更新前按住的合成键
	sendEffects(fx)
	appConfig = newCfg
	logf("配置已热更新 (%s)", cfgPath)
	return nil
}

func (apiHandler) Status() map[string]any {
	engineMu.Lock()
	defer engineMu.Unlock()
	return map[string]any{
		"running":    !pausedFlag.Load(),
		"paused":     pausedFlag.Load(),
		"hook":       hookActive(),
		"trusted":    hookTrusted(),
		"hook_error": hookError(),
		"port":       appConfig.Port,
		"layers":     len(appConfig.Layers),
		"dry_run":    dryRun,
		"version":    version,
		"config":     cfgPath,
		"platform":   runtime.GOOS,
	}
}

func (apiHandler) SetPaused(paused bool) map[string]any {
	pausedFlag.Store(paused)
	trayStateRefresh() // 各平台同步托盘/菜单栏提示
	if paused {
		logf("按键映射已暂停（来自 Web UI）")
	} else {
		logf("按键映射已恢复（来自 Web UI）")
	}
	return apiHandler{}.Status()
}

func logf(format string, args ...any) {
	if debugOn {
		log.Printf(format, args...)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fastype: "+format+"\n", args...)
	os.Exit(1)
}

func configURL() string {
	engineMu.Lock()
	defer engineMu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d/", appConfig.Port)
}

// resolveConfigPath 按优先级定位配置文件：
// 命令行 > FASTYPE_CONFIG 环境变量 > 当前目录 > 程序所在目录 > 平台配置目录
// （.app bundle 内的可执行文件跳过程序目录，避免把配置写进安装包）。
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if p := os.Getenv("FASTYPE_CONFIG"); p != "" {
		return p
	}
	if p := "config.json"; fileExists(p) {
		return p
	}
	exe, exeErr := os.Executable()
	if exeErr == nil && !insideAppBundle(exe) {
		p := filepath.Join(filepath.Dir(exe), "config.json")
		if fileExists(p) {
			return p
		}
	}
	if p := "config.json"; dirWritable(filepath.Dir(p)) {
		return p
	}
	if exeErr == nil && !insideAppBundle(exe) && dirWritable(filepath.Dir(exe)) {
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	if dir, ok := osFallbackConfigDir(); ok {
		return filepath.Join(dir, "config.json")
	}
	return "config.json"
}

// insideAppBundle 判断可执行文件是否位于 macOS .app bundle 内。
func insideAppBundle(exe string) bool {
	marker := string(filepath.Separator) + "Contents" + string(filepath.Separator) + "MacOS" + string(filepath.Separator)
	return strings.Contains(exe, marker) && strings.Contains(exe, ".app"+string(filepath.Separator))
}

// parseCLI 解析命令行与环境变量，设置 dryRun / debugOn，返回显式指定的配置路径。
func parseCLI() string {
	args := os.Args[1:]
	explicitCfg := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 < len(args) {
				i++
				explicitCfg = args[i]
			}
		case "--dry-run":
			dryRun = true
		case "--version", "-v":
			fmt.Println("fastype", version)
			os.Exit(0)
		case "--help", "-h":
			usage()
			os.Exit(0)
		default:
			fatalf("未知参数 %q（--help 查看用法）", args[i])
		}
	}
	if os.Getenv("FASTYPE_DRY_RUN") == "1" {
		dryRun = true
	}
	if debugDefault == "1" {
		debugOn = true
	}
	if os.Getenv("FASTYPE_DEBUG") == "1" {
		debugOn = true
		log.SetFlags(log.Ltime | log.Lmicroseconds)
	}
	if dryRun {
		debugOn = true
	}
	return explicitCfg
}

// loadOrCreateConfig 定位并加载配置文件，不存在时生成平台默认配置，
// 同时完成编译校验。失败直接退出。
func loadOrCreateConfig(explicitPath string) (*config.Config, *engine.Compiled) {
	cfgPath = resolveConfigPath(explicitPath)
	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		fatalf("%v", err)
	}
	if cfg == nil {
		cfg, err = config.ParseBytes(config.DefaultJSON())
		if err != nil {
			fatalf("内置默认配置错误: %v", err)
		}
		if err := config.SaveFile(cfgPath, cfg); err != nil {
			fatalf("创建默认配置 %s 失败: %v", cfgPath, err)
		}
		fmt.Printf("已生成默认配置: %s\n", cfgPath)
	}
	if cfg.Port == 0 {
		cfg.Port = 8765
	}
	compiled, err := cfg.Compile()
	if err != nil {
		fatalf("配置 %s 无效: %v", cfgPath, err)
	}
	return cfg, compiled
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dirWritable(dir string) bool {
	if !fileExists(dir) {
		return false
	}
	probe := filepath.Join(dir, ".fastype-write-test")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		return false
	}
	os.Remove(probe)
	return true
}

func usage() {
	fmt.Println(`fastype - 键盘分层映射，让普通键盘学会长按与分层

用法:
  fastype [--config 路径] [--dry-run]

选项:
  --config 路径   指定配置文件
                  (默认: ./config.json, 其次程序目录, 最后
                   Windows: %APPDATA%\fastype  macOS: ~/Library/Application Support/fastype)
  --dry-run       只记录判定结果，不真正拦截/注入按键（用于调试）
  --version       显示版本
  --help          显示本帮助

环境变量:
  FASTYPE_DEBUG=1    输出调试日志（按键判定过程）
  FASTYPE_DRY_RUN=1  等同 --dry-run
  FASTYPE_CONFIG     指定配置文件路径

启动后托盘/菜单栏图标右键可 打开配置页面 / 暂停 / 退出。`)
}

// alreadyRunning 探测目标端口上是否已有 fastype 实例，避免双开钩子互相干扰。
func alreadyRunning(port uint16) bool {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	var s struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(body, &s) == nil && s.Version != ""
}
