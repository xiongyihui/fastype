package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInsideAppBundle(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/Applications/Fastype.app/Contents/MacOS/fastype", true},
		{"~/Applications/Fastype.app/Contents/MacOS/fastype", true}, // ~ 展开后仍含 .app/ 与 Contents/MacOS/
		{"/usr/local/bin/fastype", false},
		{"/tmp/build/Fastype.app/Contents/MacOS/fastype/", true},
		{"/x/Contents/MacOS/y", false},           // 无 .app
		{"/x/Fastype.app/Contents/bin/y", false}, // 无 Contents/MacOS 段
	}
	for _, c := range cases {
		if got := insideAppBundle(c.path); got != c.want {
			t.Fatalf("insideAppBundle(%q) = %v，期望 %v", c.path, got, c.want)
		}
	}
}

// resolveConfigPath 优先级：命令行 > FASTYPE_CONFIG > 当前目录 > …
func TestResolveConfigPathPrecedence(t *testing.T) {
	if p := resolveConfigPath("/explicit/config.json"); p != "/explicit/config.json" {
		t.Fatalf("显式路径应原样返回: %q", p)
	}

	t.Setenv("FASTYPE_CONFIG", "/env/config.json")
	if p := resolveConfigPath(""); p != "/env/config.json" {
		t.Fatalf("环境变量配置应生效: %q", p)
	}
	if p := resolveConfigPath("/explicit/config.json"); p != "/explicit/config.json" {
		t.Fatalf("显式路径应优先于环境变量: %q", p)
	}

	// 当前目录存在 config.json 时优先于后续回退路径
	t.Setenv("FASTYPE_CONFIG", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreCwd(t, dir)
	if p := resolveConfigPath(""); p != "config.json" {
		t.Fatalf("当前目录的 config.json 应被选中: %q", p)
	}
}

// restoreCwd 切换工作目录并在测试结束后还原。
func restoreCwd(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestFileExistsAndDirWritable(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Fatal("目录应存在")
	}
	if fileExists(filepath.Join(dir, "nope")) {
		t.Fatal("不存在的路径应返回 false")
	}
	if !dirWritable(dir) {
		t.Fatal("临时目录应可写")
	}
	if dirWritable(filepath.Join(dir, "nope")) {
		t.Fatal("不存在的目录应不可写")
	}
	// 只读文件不影响目录可写性
	p := filepath.Join(dir, "ro.txt")
	os.WriteFile(p, nil, 0o444)
	if !fileExists(p) {
		t.Fatal("只读文件应存在")
	}
}

func TestParseCLISafeFlags(t *testing.T) {
	reset := func() {
		dryRun, debugOn = false, false
		os.Args = []string{"fastype"}
	}
	defer reset()

	os.Args = []string{"fastype", "--config", "a.json"}
	if p := parseCLI(); p != "a.json" {
		t.Fatalf("--config 应返回路径: %q", p)
	}
	reset()

	os.Args = []string{"fastype", "-c", "b.json"}
	if p := parseCLI(); p != "b.json" {
		t.Fatalf("-c 应返回路径: %q", p)
	}
	reset()

	os.Args = []string{"fastype", "--dry-run"}
	parseCLI()
	if !dryRun {
		t.Fatal("--dry-run 应生效")
	}
	reset()

	t.Setenv("FASTYPE_DRY_RUN", "1")
	parseCLI()
	if !dryRun {
		t.Fatal("FASTYPE_DRY_RUN=1 应等同 --dry-run")
	}
	reset()

	t.Setenv("FASTYPE_DEBUG", "1")
	parseCLI()
	if !debugOn {
		t.Fatal("FASTYPE_DEBUG=1 应打开调试日志")
	}
	reset()
}

// alreadyRunning 通过 /api/status 的 version 字段识别同端口的本应用实例。
func TestAlreadyRunning(t *testing.T) {
	// 模拟一个已在运行的 fastype
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			fmt.Fprint(w, `{"version":"0.4.1"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	if !alreadyRunning(port) {
		t.Fatal("同版本实例应被识别为已运行")
	}

	// 响应里没有 version 字段的不是本应用
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"hello":"world"}`)
	}))
	defer srv2.Close()
	_, portStr2, _ := net.SplitHostPort(srv2.Listener.Addr().String())
	var port2 uint16
	fmt.Sscanf(portStr2, "%d", &port2)
	if alreadyRunning(port2) {
		t.Fatal("无关服务不应被识别为已运行")
	}

	// 没人监听的端口
	if alreadyRunning(1) {
		t.Fatal("空闲端口不应被识别为已运行")
	}
}
