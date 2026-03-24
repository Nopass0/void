// Package engine – collection.go exposes a document-oriented API built on top
// of the raw key/value Engine.  Each Collection corresponds to a namespace in
// the key space and stores VoidDB Documents as serialised binary values.
package engine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/voiddb/void/internal/engine/types"
)

// Collection is a named set of Documents stored in the Engine.
// It is safe for concurrent use.
type Collection struct {
	mu   sync.RWMutex
	name string
	eng  *Engine
}

// newCollection returns a Collection backed by eng with the given name.
func newCollection(eng *Engine, name string) *Collection {
	return &Collection{name: name, eng: eng}
}

// Name returns the collection name.
func (c *Collection) Name() string { return c.name }

// Insert creates a new Document, generating a UUID if ID is empty.
// Returns the assigned document ID.
func (c *Collection) Insert(doc *types.Document) (string, error) {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	data, err := marshalDoc(doc)
	if err != nil {
		return "", fmt.Errorf("collection %s: marshal: %w", c.name, err)
	}
	if err := c.eng.Put(c.name, []byte(doc.ID), data); err != nil {
		return "", fmt.Errorf("collection %s: put: %w", c.name, err)
	}
	return doc.ID, nil
}

// Update replaces the document with the given ID.
// Returns ErrNotFound if the ID does not exist.
func (c *Collection) Update(id string, doc *types.Document) error {
	doc.ID = id
	data, err := marshalDoc(doc)
	if err != nil {
		return fmt.Errorf("collection %s: marshal: %w", c.name, err)
	}
	return c.eng.Put(c.name, []byte(id), data)
}

// Delete removes the document with the given ID.
func (c *Collection) Delete(id string) error {
	return c.eng.Delete(c.name, []byte(id))
}

// FindByID retrieves a single document by its primary key.
func (c *Collection) FindByID(id string) (*types.Document, error) {
	data, found, err := c.eng.Get(c.name, []byte(id))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return unmarshalDoc(data)
}

// Find executes a Query and returns all matching documents.
func (c *Collection) Find(q *Query) ([]*types.Document, error) {
	var results []*types.Document
	scanErr := c.eng.Scan(c.name, func(key, value []byte) bool {
		doc, err := unmarshalDoc(value)
		if err != nil {
			return true // skip corrupt records
		}
		if q == nil || q.matches(doc) {
			results = append(results, doc)
		}
		return true
	})
	if scanErr != nil {
		return nil, scanErr
	}
	// Apply sort + pagination.
	if q != nil {
		results = q.applySort(results)
		results = q.applyPagination(results)
	}
	return results, nil
}

// Count returns the number of documents matching q (nil = all).
func (c *Collection) Count(q *Query) (int64, error) {
	var n int64
	err := c.eng.Scan(c.name, func(key, value []byte) bool {
		if q == nil {
			n++
			return true
		}
		doc, err := unmarshalDoc(value)
		if err != nil {
			return true
		}
		if q.matches(doc) {
			n++
		}
		return true
	})
	return n, err
}

// --- serialisation -----------------------------------------------------------

// marshalDoc serialises a Document to bytes.
// Format: ID(len4+bytes) + JSON fields.
func marshalDoc(doc *types.Document) ([]byte, error) {
	// We use JSON for the field map so it is human-inspectable and flexible.
	// The hot path (raw binary types) is handled by the types package for wire.
	jsonMap := make(map[string]interface{}, len(doc.Fields))
	for k, v := range doc.Fields {
		jsonMap[k] = valueToInterface(v)
	}
	fieldsJSON, err := json.Marshal(jsonMap)
	if err != nil {
		return nil, err
	}
	idBytes := []byte(doc.ID)
	buf := make([]byte, 4+len(idBytes)+len(fieldsJSON))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(idBytes)))
	copy(buf[4:], idBytes)
	copy(buf[4+len(idBytes):], fieldsJSON)
	return buf, nil
}

// unmarshalDoc deserialises a Document from bytes.
func unmarshalDoc(data []byte) (*types.Document, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("collection: document too short")
	}
	idLen := int(binary.LittleEndian.Uint32(data[0:4]))
	if len(data) < 4+idLen {
		return nil, fmt.Errorf("collection: truncated id")
	}
	id := string(data[4 : 4+idLen])
	fieldsJSON := data[4+idLen:]
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(fieldsJSON, &jsonMap); err != nil {
		return nil, fmt.Errorf("collection: unmarshal fields: %w", err)
	}
	doc := &types.Document{ID: id, Fields: make(map[string]types.Value, len(jsonMap))}
	for k, v := range jsonMap {
		doc.Fields[k] = interfaceToValue(v)
	}
	return doc, nil
}

// valueToInterface converts a types.Value to a JSON-serialisable Go value.
func valueToInterface(v types.Value) interface{} {
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
			out[i] = valueToInterface(item)
		}
		return out
	case types.TypeObject:
		obj := v.ObjectVal()
		out := make(map[string]interface{}, len(obj))
		for k, val := range obj {
			out[k] = valueToInterface(val)
		}
		return out
	case types.TypeBlob:
		bucket, key := v.BlobRef()
		return map[string]string{"_blob_bucket": bucket, "_blob_key": key}
	}
	return nil
}

// interfaceToValue converts a JSON-decoded Go value to a types.Value.
func interfaceToValue(v interface{}) types.Value {
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
			arr[i] = interfaceToValue(item)
		}
		return types.Array(arr)
	case map[string]interface{}:
		// Check for blob reference.
		if bucket, ok := val["_blob_bucket"].(string); ok {
			if key, ok2 := val["_blob_key"].(string); ok2 {
				return types.Blob(bucket, key)
			}
		}
		obj := make(map[string]types.Value, len(val))
		for k, item := range val {
			obj[k] = interfaceToValue(item)
		}
		return types.Object(obj)
	}
	return types.Null()
}

// ErrNotFound is returned when a document ID does not exist.
var ErrNotFound = fmt.Errorf("voiddb: document not found")

// --- Database ----------------------------------------------------------------

// Database groups multiple Collections under one logical database namespace.
type Database struct {
	mu          sync.RWMutex
	name        string
	eng         *Engine
	collections map[string]*Collection
}

// newDatabase creates a Database backed by eng.
func newDatabase(eng *Engine, name string) *Database {
	return &Database{
		name:        name,
		eng:         eng,
		collections: make(map[string]*Collection),
	}
}

// Collection returns (or creates) the named Collection within this database.
func (db *Database) Collection(name string) *Collection {
	db.mu.Lock()
	defer db.mu.Unlock()
	ns := db.name + "/" + name
	if c, ok := db.collections[ns]; ok {
		return c
	}
	c := newCollection(db.eng, ns)
	db.collections[ns] = c
	return c
}

// Name returns the database name.
func (db *Database) Name() string { return db.name }

// --- Store -------------------------------------------------------------------

// Store is the top-level entry point that manages multiple databases.
type Store struct {
	mu        sync.RWMutex
	eng       *Engine
	databases map[string]*Database
}

// NewStore wraps an Engine in a document-store interface.
func NewStore(eng *Engine) *Store {
	return &Store{eng: eng, databases: make(map[string]*Database)}
}

// DB returns (or lazily creates) the named database.
func (s *Store) DB(name string) *Database {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.databases[name]; ok {
		return db
	}
	db := newDatabase(s.eng, name)
	s.databases[name] = db
	return db
}

// ListDatabases returns the names of all known databases
// (those that have at least one key in the engine).
func (s *Store) ListDatabases() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.databases))
	for name := range s.databases {
		out = append(out, name)
	}
	return out
}

// ListCollections returns collection names within a database.
func (s *Store) ListCollections(dbName string) []string {
	s.mu.RLock()
	db, ok := s.databases[dbName]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	prefix := dbName + "/"
	out := make([]string, 0, len(db.collections))
	for ns := range db.collections {
		out = append(out, strings.TrimPrefix(ns, prefix))
	}
	return out
}

// Engine exposes the underlying Engine for stats and administration.
func (s *Store) Engine() *Engine { return s.eng }
