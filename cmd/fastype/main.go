//go:build windows && amd64

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"fastype/internal/config"
	"fastype/internal/engine"
	"fastype/internal/ui"
)

var version = "0.1.0"

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
	mainTID    atomic.Uintptr
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
	fx := eng.Reload(compiled)
	sendEffects(fx) // 释放热更新前按住的合成键
	appConfig = newCfg
	logf("配置已热更新 (%s)", cfgPath)
	return nil
}

func (apiHandler) Status() map[string]any {
	engineMu.Lock()
	defer engineMu.Unlock()
	return map[string]any{
		"running":  !pausedFlag.Load(),
		"paused":   pausedFlag.Load(),
		"port":     appConfig.Port,
		"layers":   len(appConfig.Layers),
		"dry_run":  dryRun,
		"version":  version,
		"config":   cfgPath,
	}
}

func (apiHandler) SetPaused(paused bool) map[string]any {
	pausedFlag.Store(paused)
	// 让主线程同步托盘提示
	if t := mainTID.Load(); t != 0 {
		pPostThreadMessageW.Call(t, wmAppTrayCmd, cmdRefreshTip, 0)
	}
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
// 命令行 > FASTYPE_CONFIG 环境变量 > 当前目录 > 程序所在目录 > %APPDATA%\fastype
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
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.json")
		if fileExists(p) {
			return p
		}
	}
	if p := "config.json"; dirWritable(filepath.Dir(p)) {
		return p
	}
	if exe, err := os.Executable(); err == nil && dirWritable(filepath.Dir(exe)) {
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "fastype", "config.json")
	}
	return "config.json"
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
  --config 路径   指定配置文件 (默认: ./config.json, 其次 exe 目录, 最后 %APPDATA%\fastype)
  --dry-run       只记录判定结果，不真正拦截/注入按键（用于调试）
  --version       显示版本

环境变量:
  FASTYPE_DEBUG=1    输出调试日志（按键判定过程）
  FASTYPE_DRY_RUN=1  等同 --dry-run
  FASTYPE_CONFIG     指定配置文件路径

启动后托盘图标右键可 打开配置页面 / 暂停 / 退出。`)
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

func main() {
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
			return
		case "--help", "-h":
			usage()
			return
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

	cfgPath = resolveConfigPath(explicitCfg)
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
	compiled, err := cfg.Compile()
	if err != nil {
		fatalf("配置 %s 无效: %v", cfgPath, err)
	}

	eng = engine.NewEngine(compiled)
	eng.Logf = logf
	appConfig = cfg

	if alreadyRunning(cfg.Port) {
		fatalf("fastype 已在运行（端口 %d）。请先从托盘菜单退出旧实例，或访问 http://127.0.0.1:%d/", cfg.Port, cfg.Port)
	}

	actualPort, err := ui.Start(cfg.Port, apiHandler{})
	if err != nil {
		fatalf("%v", err)
	}
	cfg.Port = actualPort // 端口被占用时自动 +1 递增后的实际端口

	fmt.Printf("fastype %s 已启动  配置: %s  界面: http://127.0.0.1:%d/\n", version, cfgPath, actualPort)
	if dryRun {
		fmt.Println("【dry-run 模式】只记录判定，不拦截按键")
	}
	fmt.Println("托盘图标右键: 打开配置 / 暂停 / 退出；Ctrl+C 退出")

	handler := syscall.NewCallback(consoleCtrlHandler)
	pSetConsoleCtrlHandler.Call(handler, 1)

	runHookLoop() // 阻塞直到退出

	// 收尾：释放可能按住的合成键，避免退出后修饰键卡住
	engineMu.Lock()
	fx := eng.ReleaseAll()
	engineMu.Unlock()
	if !dryRun {
		sendEffects(fx)
	}
	ui.Stop()
	logf("fastype 已退出")
}
