// Package handlers – backup/restore HTTP endpoints.
//
// POST /v1/backup          – export all (or selected) databases to a .void archive.
// POST /v1/backup/restore  – import a previously exported .void archive.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/voiddb/void/internal/backup"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/engine/types"
)

// BackupHandler handles backup and restore requests.
type BackupHandler struct {
	store         *engine.Store
	serverVersion string
}

// NewBackupHandler creates a BackupHandler.
// serverVersion is embedded in every archive manifest.
func NewBackupHandler(store *engine.Store, serverVersion string) *BackupHandler {
	return &BackupHandler{store: store, serverVersion: serverVersion}
}

// backupRequest is the optional JSON body for POST /v1/backup.
type backupRequest struct {
	// Databases to include. Empty slice or missing field = export all.
	Databases []string `json:"databases"`
}

// Export streams a .void archive to the HTTP response body.
//
//	POST /v1/backup
//	Content-Type: application/json  (optional)
//	{ "databases": ["mydb"] }       (optional – omit to export everything)
//
// Response: application/octet-stream, Content-Disposition: attachment.
func (h *BackupHandler) Export(w http.ResponseWriter, r *http.Request) {
	var req backupRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
	}

	// Resolve database list.
	allDBs := h.store.ListDatabases()
	dbNames := allDBs
	if len(req.Databases) > 0 {
		keep := make(map[string]struct{}, len(req.Databases))
		for _, d := range req.Databases {
			keep[d] = struct{}{}
		}
		filtered := make([]string, 0, len(req.Databases))
		for _, d := range allDBs {
			if _, ok := keep[d]; ok {
				filtered = append(filtered, d)
			}
		}
		dbNames = filtered
	}

	// Stream archive directly into the response body.
	ts := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("voiddb_backup_%s.void", ts)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Void-Backup-Timestamp", ts)

	bw := backup.NewWriter(w, h.serverVersion)

	for _, dbName := range dbNames {
		bw.AddDatabase(dbName)
		db := h.store.DB(dbName)

		for _, colName := range h.store.ListCollections(dbName) {
			col := db.Collection(colName)
			docs, err := col.Find(nil)
			if err != nil {
				continue // best-effort: skip unreadable collections
			}

			raw := make([]map[string]interface{}, 0, len(docs))
			for _, doc := range docs {
				m := make(map[string]interface{}, len(doc.Fields)+1)
				m["_id"] = doc.ID
				for k, v := range doc.Fields {
					m[k] = valueToJSONInterface(v)
				}
				raw = append(raw, m)
			}

			if err := bw.WriteCollection(dbName, colName, raw); err != nil {
				// Headers already sent; can't change status.
				return
			}
		}
	}

	_ = bw.Close()
}

// Restore reads a .void archive from the request body and re-inserts documents.
//
//	POST /v1/backup/restore
//	Content-Type: application/octet-stream
//	<raw .void archive body>
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, `{"error":"empty body"}`, http.StatusBadRequest)
		return
	}

	br, err := backup.NewReader(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"not a valid .void archive: %s"}`, err),
			http.StatusBadRequest)
		return
	}
	defer br.Close()

	type colResult struct {
		DB         string `json:"db"`
		Collection string `json:"collection"`
		Imported   int    `json:"imported"`
	}
	var results []colResult

	for {
		name, data, entryErr := br.ReadEntry()
		if entryErr != nil {
			break // io.EOF or real error
		}
		if name == "manifest.json" || len(data) == 0 {
			continue
		}

		colName, dbName, rawDocs, ok := parseNDJSON(data)
		if !ok {
			continue
		}

		col := h.store.DB(dbName).Collection(colName)
		imported := 0
		for _, rawDoc := range rawDocs {
			doc := rawDocToDocument(rawDoc)
			if _, err := col.Insert(doc); err == nil {
				imported++
			}
		}
		results = append(results, colResult{DB: dbName, Collection: colName, Imported: imported})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"restored":  results,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- helpers -----------------------------------------------------------------

// valueToJSONInterface converts a types.Value to a JSON-serialisable Go value.
func valueToJSONInterface(v types.Value) interface{} {
	switch v.Type() {
	case types.TypeNull:
		return nil
	case types.TypeString:
		return v.StringVal()
	case types.TypeNumber:
		return v.NumberVal()
	case types.TypeBoolean:
		return v.BoolVal()
	case types.TypeArray:
		arr := v.ArrayVal()
		out := make([]interface{}, len(arr))
		for i, item := range arr {
			out[i] = valueToJSONInterface(item)
		}
		return out
	case types.TypeObject:
		obj := v.ObjectVal()
		out := make(map[string]interface{}, len(obj))
		for k, val := range obj {
			out[k] = valueToJSONInterface(val)
		}
		return out
	case types.TypeBlob:
		bucket, key := v.BlobRef()
		return map[string]string{"_blob_bucket": bucket, "_blob_key": key}
	}
	return nil
}

// rawDocToDocument converts a map[string]interface{} (from NDJSON) to a
// types.Document, preserving the _id field.
func rawDocToDocument(raw map[string]interface{}) *types.Document {
	doc := &types.Document{Fields: make(map[string]types.Value, len(raw))}
	if id, ok := raw["_id"].(string); ok {
		doc.ID = id
	}
	for k, v := range raw {
		if k == "_id" {
			continue
		}
		doc.Fields[k] = jsonInterfaceToValue(v)
	}
	return doc
}

// jsonInterfaceToValue converts a JSON-decoded interface{} to types.Value.
func jsonInterfaceToValue(v interface{}) types.Value {
	if v == nil {
		return types.Null()
	}
	switch val := v.(type) {
	case string:
		return types.String(val)
	case float64:
		return types.Number(val)
	case bool:
		return types.Boolean(val)
	case []interface{}:
		arr := make([]types.Value, len(val))
		for i, item := range val {
			arr[i] = jsonInterfaceToValue(item)
		}
		return types.Array(arr)
	case map[string]interface{}:
		if bucket, ok := val["_blob_bucket"].(string); ok {
			if key, ok2 := val["_blob_key"].(string); ok2 {
				return types.Blob(bucket, key)
			}
		}
		obj := make(map[string]types.Value, len(val))
		for k, item := range val {
			obj[k] = jsonInterfaceToValue(item)
		}
		return types.Object(obj)
	}
	return types.Null()
}

// parseNDJSON reads a .ndjson entry and returns collection name, database name,
// and parsed documents. Returns ok=false for non-collection entries.
func parseNDJSON(data []byte) (col, db string, docs []map[string]interface{}, ok bool) {
	lines := splitNDJSONLines(data)
	if len(lines) == 0 {
		return "", "", nil, false
	}
	var hdr backup.CollectionHeader
	if err := json.Unmarshal(lines[0], &hdr); err != nil || hdr.Collection == "" {
		return "", "", nil, false
	}
	docs = make([]map[string]interface{}, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		var doc map[string]interface{}
		if err := json.Unmarshal(line, &doc); err == nil {
			docs = append(docs, doc)
		}
	}
	return hdr.Collection, hdr.Database, docs, true
}

// splitNDJSONLines splits byte slice on newlines, skipping blank lines.
func splitNDJSONLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
