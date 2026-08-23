package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// 内嵌 Web UI：配置页由本进程直接提供，保存后热更新，无需重启。

func startServer(basePort uint16) (uint16, error) {
	var ln net.Listener
	port := int(basePort)
	for i := 0; i < 50; i++ {
		p := int(basePort) + i
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			ln = l
			port = p
			break
		}
	}
	if ln == nil {
		return 0, fmt.Errorf("监听 127.0.0.1:%d 起的连续端口均失败", basePort)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/pause", handlePause)

	httpSrv = &http.Server{Handler: mux}
	go httpSrv.Serve(ln)
	logf("Web 配置界面: http://127.0.0.1:%d/", port)
	return uint16(port), nil
}

func stopServer() {
	if httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, format string, args ...any) {
	writeJSON(w, code, map[string]any{"error": fmt.Sprintf(format, args...)})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		engineMu.Lock()
		cfg := appConfig
		engineMu.Unlock()
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPost, http.MethodPut:
		body := http.MaxBytesReader(w, r.Body, 1<<20)
		data, err := io.ReadAll(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "读取请求失败: %v", err)
			return
		}
		newCfg, err := parseConfigBytes(data)
		if err != nil {
			writeError(w, http.StatusBadRequest, "配置格式错误: %v", err)
			return
		}
		compiled, err := newCfg.Compile()
		if err != nil {
			writeError(w, http.StatusBadRequest, "配置校验失败: %v", err)
			return
		}

		engineMu.Lock()
		if err := saveConfigFile(cfgPath, newCfg); err != nil {
			engineMu.Unlock()
			writeError(w, http.StatusInternalServerError, "写入配置文件失败: %v", err)
			return
		}
		fx := eng.Reload(compiled)
		sendEffects(fx) // 释放热更新前按住的合成键
		appConfig = newCfg
		engineMu.Unlock()
		logf("配置已热更新 (%s)", cfgPath)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func statusJSON() map[string]any {
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

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusJSON())
}

func handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "请用 POST")
		return
	}
	var body struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求体应为 {\"paused\": true/false}")
		return
	}
	pausedFlag.Store(body.Paused)
	// 让主线程同步托盘提示
	if t := mainTID.Load(); t != 0 {
		pPostThreadMessageW.Call(t, wmAppTrayCmd, cmdRefreshTip, 0)
	}
	if body.Paused {
		logf("按键映射已暂停（来自 Web UI）")
	} else {
		logf("按键映射已恢复（来自 Web UI）")
	}
	writeJSON(w, http.StatusOK, statusJSON())
}
