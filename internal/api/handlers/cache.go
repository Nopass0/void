package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/kvcache"
)

// CacheHandler handles /v1/cache HTTP endpoints.
type CacheHandler struct {
	cache *kvcache.Cache
}

// NewCacheHandler creates a new CacheHandler.
func NewCacheHandler(c *kvcache.Cache) *CacheHandler {
	return &CacheHandler{cache: c}
}

// Get handles GET /v1/cache/{key}.
func (h *CacheHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	val := h.cache.Get(key)
	if val == nil {
		writeError(w, http.StatusNotFound, "key not found or expired")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(val)
}

// Set handles POST /v1/cache/{key}. Body is the raw value.
func (h *CacheHandler) Set(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	
	var ttl time.Duration
	if ttlStr := r.URL.Query().Get("ttl"); ttlStr != "" {
		d, err := time.ParseDuration(ttlStr)
		if err == nil {
			ttl = d
		}
	}
	
	h.cache.Set(key, body, ttl)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete handles DELETE /v1/cache/{key}.
func (h *CacheHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	h.cache.Delete(key)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
