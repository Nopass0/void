package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/pgimport"
)

// ImportHandler handles long-running data import operations.
type ImportHandler struct {
	store *engine.Store
}

// NewImportHandler creates a new import handler.
func NewImportHandler(store *engine.Store) *ImportHandler {
	return &ImportHandler{store: store}
}

type postgresImportRequest struct {
	SourceURL      string `json:"source_url"`
	TargetDatabase string `json:"target_database"`
	SourceSchema   string `json:"source_schema"`
	DropExisting   bool   `json:"drop_existing"`
}

// ImportPostgres imports a PostgreSQL schema and its data into VoidDB.
func (h *ImportHandler) ImportPostgres(w http.ResponseWriter, r *http.Request) {
	var req postgresImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid PostgreSQL import JSON")
		return
	}
	if req.SourceURL == "" {
		writeError(w, http.StatusBadRequest, "source_url is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	result, err := pgimport.ImportURL(ctx, h.store, pgimport.Options{
		SourceURL:      req.SourceURL,
		TargetDatabase: req.TargetDatabase,
		SourceSchema:   req.SourceSchema,
		DropExisting:   req.DropExisting,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
