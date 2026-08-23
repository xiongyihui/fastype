package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

// Handler 由应用层实现，把 HTTP API 委托给真正的状态持有者（main）。
type Handler interface {
	// Config 返回当前配置（JSON 序列化后作为 GET /api/config 响应）。
	Config() any
	// SaveConfigJSON 校验、持久化并热更新一份新的配置 JSON。
	SaveConfigJSON(data []byte) error
	// Status 返回 GET /api/status 的状态对象。
	Status() map[string]any
	// SetPaused 切换暂停状态并返回新的状态对象。
	SetPaused(paused bool) map[string]any
}

var httpSrv *http.Server

// Start 在 basePort 起的本机回环端口上提供 Web UI，端口被占用时自动 +1 递增。
func Start(basePort uint16, h Handler) (uint16, error) {
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { handleIndex(w, r) })
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) { handleConfig(w, r, h) })
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.Status())
	})
	mux.HandleFunc("/api/pause", func(w http.ResponseWriter, r *http.Request) { handlePause(w, r, h) })

	httpSrv = &http.Server{Handler: mux}
	go httpSrv.Serve(ln)
	log.Printf("Web 配置界面: http://127.0.0.1:%d/", port)
	return uint16(port), nil
}

// Stop 关闭 Web UI 服务。
func Stop() {
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

func handleConfig(w http.ResponseWriter, r *http.Request, h Handler) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.Config())
	case http.MethodPost, http.MethodPut:
		body := http.MaxBytesReader(w, r.Body, 1<<20)
		data, err := io.ReadAll(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "读取请求失败: %v", err)
			return
		}
		if err := h.SaveConfigJSON(data); err != nil {
			writeError(w, http.StatusBadRequest, "%v", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func handlePause(w http.ResponseWriter, r *http.Request, h Handler) {
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
	writeJSON(w, http.StatusOK, h.SetPaused(body.Paused))
}
