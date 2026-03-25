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
	"strings"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/engine/types"
	"github.com/voiddb/void/internal/schemafile"
	"go.uber.org/zap"
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
	if _, err := h.store.CreateDatabase(req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	zap.L().Info("database created", zap.String("database", req.Name))
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

// DeleteDatabase handles DELETE /v1/databases/{db}.
func (h *DBHandler) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	if dbName == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	if err := h.store.DropDatabase(dbName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	zap.L().Info("database dropped", zap.String("database", dbName))
	w.WriteHeader(http.StatusNoContent)
}

// Realtime handles GET /v1/databases/{db}/realtime (SSE).
func (h *DBHandler) Realtime(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	zap.L().Info("realtime stream opened", zap.String("database", dbName), zap.String("remote_addr", r.RemoteAddr))

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan engine.Event, 64)
	h.store.Hub().Subscribe(ch)
	defer func() {
		h.store.Hub().Unsubscribe(ch)
		zap.L().Info("realtime stream closed", zap.String("database", dbName), zap.String("remote_addr", r.RemoteAddr))
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if ev.Database == dbName {
				data, _ := json.Marshal(ev)
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(data)
				_, _ = w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	}
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
	if _, err := h.store.CreateCollection(dbName, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	zap.L().Info("collection created", zap.String("database", dbName), zap.String("collection", req.Name))
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

// DeleteCollection handles DELETE /v1/databases/{db}/collections/{col}.
func (h *DBHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["db"]
	colName := vars["col"]
	if dbName == "" || colName == "" {
		writeError(w, http.StatusBadRequest, "database and collection are required")
		return
	}
	if err := h.store.DropCollection(dbName, colName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	zap.L().Info("collection dropped", zap.String("database", dbName), zap.String("collection", colName))
	w.WriteHeader(http.StatusNoContent)
}

// --- Schema endpoints --------------------------------------------------------

// GetSchema handles GET /v1/databases/{db}/{col}/schema.
func (h *DBHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	writeJSON(w, http.StatusOK, col.Schema())
}

// SetSchema handles PUT /v1/databases/{db}/{col}/schema.
func (h *DBHandler) SetSchema(w http.ResponseWriter, r *http.Request) {
	col := h.collection(r)
	var schema engine.Schema
	if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
		writeError(w, http.StatusBadRequest, "invalid schema JSON")
		return
	}
	if err := col.SetSchema(&schema); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

// ExportDatabaseSchema renders a Prisma-like schema file for one database.
func (h *DBHandler) ExportDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	project := &schemafile.Project{}
	usedNames := make(map[string]int)

	for _, colName := range h.store.ListCollections(dbName) {
		schema := h.store.DB(dbName).Collection(colName).Schema()
		if schema == nil {
			continue
		}
		copySchema := *schema.Normalize()
		copySchema.Database = dbName
		copySchema.Collection = colName
		copySchema.Model = uniqueModelNameForExport(defaultModelNameForExport(dbName, colName), usedNames)
		project.Models = append(project.Models, schemafile.Model{
			Name:   copySchema.Model,
			Schema: &copySchema,
		})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(schemafile.Render(project)))
}

// ExportCollectionSchema renders a Prisma-like schema block for one collection.
func (h *DBHandler) ExportCollectionSchema(w http.ResponseWriter, r *http.Request) {
	dbName := mux.Vars(r)["db"]
	colName := mux.Vars(r)["col"]
	schema := h.store.DB(dbName).Collection(colName).Schema()
	if schema == nil {
		writeError(w, http.StatusNotFound, "schema not found")
		return
	}

	copySchema := *schema.Normalize()
	copySchema.Database = dbName
	copySchema.Collection = colName
	if copySchema.Model == "" {
		copySchema.Model = defaultModelNameForExport(dbName, colName)
	}

	project := &schemafile.Project{
		Models: []schemafile.Model{{
			Name:   copySchema.Model,
			Schema: &copySchema,
		}},
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(schemafile.Render(project)))
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
	zap.L().Info("document inserted", zap.String("database", mux.Vars(r)["db"]), zap.String("collection", mux.Vars(r)["col"]), zap.String("id", id))
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
	zap.L().Info("document replaced", zap.String("database", mux.Vars(r)["db"]), zap.String("collection", mux.Vars(r)["col"]), zap.String("id", id))
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
	zap.L().Info("document patched", zap.String("database", mux.Vars(r)["db"]), zap.String("collection", mux.Vars(r)["col"]), zap.String("id", id))
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
	zap.L().Info("document deleted", zap.String("database", mux.Vars(r)["db"]), zap.String("collection", mux.Vars(r)["col"]), zap.String("id", id))
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
	if spec.Where != nil {
		q = q.WhereNode(parseQueryNode(*spec.Where))
	}

	for _, inc := range spec.Include {
		q = q.Include(engine.JoinSpec{
			As:         inc.As,
			Relation:   inc.Relation,
			TargetCol:  inc.TargetCol,
			LocalKey:   inc.LocalKey,
			ForeignKey: inc.ForeignKey,
		})
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

// querySpecNode allows nested AND/OR arrays or direct leaf filters.
type querySpecNode struct {
	AND   []querySpecNode `json:"AND"`
	OR    []querySpecNode `json:"OR"`
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value interface{}     `json:"value"`
}

// joinSpecJSON parses HTTP include relations.
type joinSpecJSON struct {
	As         string `json:"as"`
	Relation   string `json:"relation"`
	TargetCol  string `json:"target_col"`
	LocalKey   string `json:"local_key"`
	ForeignKey string `json:"foreign_key"`
}

// querySpec is the JSON shape for POST /query.
type querySpec struct {
	Where   *querySpecNode `json:"where"`
	OrderBy []struct {
		Field string `json:"field"`
		Dir   string `json:"dir"`
	} `json:"order_by"`
	Include []joinSpecJSON `json:"include"`
	Limit   int            `json:"limit"`
	Skip    int            `json:"skip"`
}

// parseQueryNode recursively builds a tree of engine.Predicate.
func parseQueryNode(node querySpecNode) engine.Predicate {
	if len(node.AND) > 0 {
		var children []engine.Predicate
		for _, c := range node.AND {
			children = append(children, parseQueryNode(c))
		}
		return engine.Predicate{IsLogic: true, LogicOp: "AND", Children: children}
	}
	if len(node.OR) > 0 {
		var children []engine.Predicate
		for _, c := range node.OR {
			children = append(children, parseQueryNode(c))
		}
		return engine.Predicate{IsLogic: true, LogicOp: "OR", Children: children}
	}

	// Leaf/field node
	p := engine.Predicate{
		IsLogic: false,
		Field:   node.Field,
		Op:      engine.Op(node.Op),
		Value:   jsonToValue(node.Value),
	}
	// For "in" operator, Value might be parsed as an array. Normalize to List.
	if p.Op == engine.In && p.Value.Type() == types.TypeArray {
		p.List = p.Value.ArrayVal()
	}
	return p
}

// docToMap converts a Document to a JSON-serialisable map including the _id.
func docToMap(doc *types.Document) map[string]interface{} {
	m := make(map[string]interface{}, len(doc.Fields)+1)
	m["_id"] = doc.ID
	for k, v := range doc.Fields {
		if k == "_id" {
			continue
		}
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

func defaultModelNameForExport(dbName, colName string) string {
	base := toPascalForExport(colName)
	if base == "" {
		base = "Model"
	}
	if dbName == "" || dbName == "default" {
		return base
	}
	return toPascalForExport(dbName) + base
}

func uniqueModelNameForExport(name string, used map[string]int) string {
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	return name + strconvItoaForExport(used[name])
}

func toPascalForExport(value string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			if upperNext {
				b.WriteString(strings.ToUpper(string(r)))
				upperNext = false
			} else {
				b.WriteRune(r)
			}
		default:
			upperNext = true
		}
	}
	return b.String()
}

func strconvItoaForExport(v int) string {
	if v == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for v > 0 {
		pos--
		digits[pos] = byte('0' + (v % 10))
		v /= 10
	}
	return string(digits[pos:])
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
