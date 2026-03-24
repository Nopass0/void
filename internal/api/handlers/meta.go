package handlers

import (
	_ "embed"
	"net/http"
)

var (
	//go:embed assets/skill.md
	skillDoc string
)

// MetaHandler serves small metadata documents useful for tooling and agents.
type MetaHandler struct{}

// NewMetaHandler creates a new MetaHandler.
func NewMetaHandler() *MetaHandler { return &MetaHandler{} }

// Skill serves the AI-agent guide as markdown.
func (h *MetaHandler) Skill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(skillDoc))
}
