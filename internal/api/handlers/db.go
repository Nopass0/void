// Package handlers – db.go implements the document-store REST API.
//
// Routes (all under /v1):
//
//	GET    /databases                        – list databases
//	POST   /databases                        – create database
//	DELETE /databases/{db}                   – drop database
//	GET    /databases/{db}/collections       – list collections
//	POST   /databases/{db}/collections       – create collection
//	DELETE /databases/{db}/collections/{col} – drop collection
//	POST   /databases/{db}/{col}/query       – query documents
//	GET    /databases/{db}/{col}/{id}        – get document by ID
//	POST   /databases/{db}/{col}             – insert document
//	PUT    /databases/{db}/{col}/{id}        – replace document
//	PATCH  /databases/{db}/{col}/{id}        – partial update
//	DELETE /databases/{db}/{col}/{id}        – delete document
//	GET    /databases/{db}/{col}/count       – count documents
//	GET    /stats                            – engine stats
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/engine/types"
)

// DBHandler handles all document-store HTTP requests.
type DBHandler struct {
	store *engine.Store
}

// NewDBHandler creates a DBHandler backed by store.
func NewDBHandler(store *engine.Store) *DBHandler {
	return &DBHandler{store: store}
}

// --- Database endpoints ------------------------------------------------------

// ListDatabases handles GET /v1/databases.
func (h *DBHandler) ListDatabases(w http.ResponseWriter, r *http.Request) {
	dbs := h.store.ListDatabases()
	writeJSON(w, http.StatusOK, map[string]interface{}{"databases": dbs})
}

// CreateDatabase handles POST /v1/databases.
func (h *DBHandler) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	h.store.DB(req.Name) // lazy-create
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

// --- Collection endpoints ----------------------------------------------------

// ListCollections handles GET /v1/databases/{db}/collections.
func (h *DBHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	cols := h.store.ListCollections(dbName)
	writeJSON(w, http.StatusOK, map[string]interface{}{"collections": cols})
}

// CreateCollection handles POST /v1/databases/{db}/collections.
func (h *DBHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	h.store.DB(dbName).Collection(req.Name)
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

// --- Document endpoints ------------------------------------------------------

// InsertDocument handles POST /v1/databases/{db}/{col}.
func (h *DBHandler) InsertDocument(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	doc := &types.Document{Fields: make(map[string]types.Value, len(raw))}
	if id, ok := raw["_id"].(string); ok {
		doc.ID = id
		delete(raw, "_id")
	}
	for k, v := range raw {
		doc.Fields[k] = jsonToValue(v)
	}
	id, err := col.Insert(doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"_id": id})
}

// GetDocument handles GET /v1/databases/{db}/{col}/{id}.
func (h *DBHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	id := mux.Vars(r)["id"]
	doc, err := col.FindByID(id)
	if err == engine.ErrNotFound {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docToMap(doc))
}

// ReplaceDocument handles PUT /v1/databases/{db}/{col}/{id}.
func (h *DBHandler) ReplaceDocument(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	id := mux.Vars(r)["id"]
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	doc := &types.Document{ID: id, Fields: make(map[string]types.Value, len(raw))}
	for k, v := range raw {
		if k == "_id" {
			continue
		}
		doc.Fields[k] = jsonToValue(v)
	}
	if err := col.Update(id, doc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"_id": id})
}

// PatchDocument handles PATCH /v1/databases/{db}/{col}/{id}.
// Only fields present in the body are updated; others remain unchanged.
func (h *DBHandler) PatchDocument(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	id := mux.Vars(r)["id"]

	existing, err := col.FindByID(id)
	if err == engine.ErrNotFound {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	for k, v := range patch {
		if k == "_id" {
			continue
		}
		existing.Fields[k] = jsonToValue(v)
	}
	if err := col.Update(id, existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docToMap(existing))
}

// DeleteDocument handles DELETE /v1/databases/{db}/{col}/{id}.
func (h *DBHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	id := mux.Vars(r)["id"]
	if err := col.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// QueryDocuments handles POST /v1/databases/{db}/{col}/query.
// Body is a JSON query specification.
func (h *DBHandler) QueryDocuments(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	var spec querySpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid query JSON")
		return
	}

	q := engine.NewQuery()
	for _, f := range spec.Where {
		q = q.Where(f.Field, engine.Op(f.Op), jsonToValue(f.Value))
	}
	for _, s := range spec.OrderBy {
		dir := engine.Asc
		if s.Dir == "desc" {
			dir = engine.Desc
		}
		q = q.OrderBy(s.Field, dir)
	}
	if spec.Limit > 0 {
		q = q.Limit(spec.Limit)
	}
	if spec.Skip > 0 {
		q = q.Skip(spec.Skip)
	}

	docs, err := col.Find(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]map[string]interface{}, len(docs))
	for i, d := range docs {
		out[i] = docToMap(d)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": out,
		"count":   len(out),
	})
}

// CountDocuments handles GET /v1/databases/{db}/{col}/count.
func (h *DBHandler) CountDocuments(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	n, err := col.Count(nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": n})
}

// Stats handles GET /v1/stats.
func (h *DBHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.Engine().Stats())
}

// --- helpers -----------------------------------------------------------------

// collection extracts the Collection from mux route variables.
func (h *DBHandler) collection(r *http.Request) *engine.Collection {
	vars := mux.Vars(r)
	return h.store.DB(vars["db"]).Collection(vars["col"])
}

// querySpec is the JSON shape for POST /query.
type querySpec struct {
	Where []struct {
		Field string      `json:"field"`
		Op    string      `json:"op"`
		Value interface{} `json:"value"`
	} `json:"where"`
	OrderBy []struct {
		Field string `json:"field"`
		Dir   string `json:"dir"`
	} `json:"order_by"`
	Limit int `json:"limit"`
	Skip  int `json:"skip"`
}

// docToMap converts a Document to a JSON-serialisable map including the _id.
func docToMap(doc *types.Document) map[string]interface{} {
	m := make(map[string]interface{}, len(doc.Fields)+1)
	m["_id"] = doc.ID
	for k, v := range doc.Fields {
		m[k] = valueToJSON(v)
	}
	return m
}

// valueToJSON converts a types.Value to a JSON-friendly Go value.
func valueToJSON(v types.Value) interface{} {
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
			out[i] = valueToJSON(item)
		}
		return out
	case types.TypeObject:
		obj := v.ObjectVal()
		out := make(map[string]interface{}, len(obj))
		for k, val := range obj {
			out[k] = valueToJSON(val)
		}
		return out
	case types.TypeBlob:
		bucket, key := v.BlobRef()
		return map[string]string{"_blob_bucket": bucket, "_blob_key": key}
	}
	return nil
}

// jsonToValue converts a JSON-decoded interface{} to a types.Value.
func jsonToValue(v interface{}) types.Value {
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
			arr[i] = jsonToValue(item)
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
			obj[k] = jsonToValue(item)
		}
		return types.Object(obj)
	}
	return types.Null()
}

// writeJSON serialises v to JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
