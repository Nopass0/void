package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/voiddb/void/internal/logs"
	"go.uber.org/zap"
)

// LogsHandler handles the /v1/logs endpoint.
type LogsHandler struct{}

// NewLogsHandler returns a new LogsHandler.
func NewLogsHandler() *LogsHandler {
	return &LogsHandler{}
}

// Get handles GET /v1/logs.
func (h *LogsHandler) Get(w http.ResponseWriter, r *http.Request) {
	limit := -1
	skip := 0
	
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	if s := r.URL.Query().Get("skip"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			skip = v
		}
	}

	data := logs.GlobalRing.Get(limit, skip)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs": data,
		"count": logs.GlobalRing.Len(),
	})
}

// Realtime handles GET /v1/logs/realtime.
func (h *LogsHandler) Realtime(w http.ResponseWriter, r *http.Request) {
	zap.L().Info("log stream opened", zap.String("remote_addr", r.RemoteAddr))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := logs.Subscribe()
	defer func() {
		logs.Unsubscribe(ch)
		zap.L().Info("log stream closed", zap.String("remote_addr", r.RemoteAddr))
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
	}
}
