// Package engine – collection.go exposes a document-oriented API built on top
// of the raw key/value Engine.  Each Collection corresponds to a namespace in
// the key space and stores VoidDB Documents as serialised binary values.
package engine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/voiddb/void/internal/engine/types"
)

const (
	metaDatabasePrefix   = "db:"
	metaCollectionPrefix = "col:"
	metaSchemaPrefix     = "schema:"
)

// Collection is a named set of Documents stored in the Engine.
// It is safe for concurrent use.
type Collection struct {
	mu     sync.RWMutex
	name   string
	eng    *Engine
	schema *Schema
	hub    *Hub
}

// newCollection returns a Collection backed by eng with the given name.
func newCollection(eng *Engine, name string, hub *Hub) *Collection {
	c := &Collection{name: name, eng: eng, hub: hub}
	c.loadSchema()
	return c
}

// loadSchema reads the schema from the _meta namespace.
func (c *Collection) loadSchema() {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, found, err := c.eng.Get("_meta", []byte(metaSchemaPrefix+c.name))
	if err == nil && found {
		s, _ := unmarshalSchema(data)
		c.schema = s
	} else {
		c.schema = NewDefaultSchema()
	}
}

// Schema returns the collection's current schema.
func (c *Collection) Schema() *Schema {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.schema
}

// SetSchema saves and applies a new schema to the collection.
func (c *Collection) SetSchema(s *Schema) error {
	if s == nil {
		s = NewDefaultSchema()
	}
	s = s.Normalize()
	s.Collection = collectionNameFromNamespace(c.name)
	s.Database = databaseNameFromNamespace(c.name)
	if s.Model == "" {
		s.Model = collectionNameFromNamespace(c.name)
	}
	data, err := marshalSchema(s)
	if err != nil {
		return err
	}
	if err := c.eng.Put("_meta", []byte(metaSchemaPrefix+c.name), data); err != nil {
		return err
	}
	c.mu.Lock()
	c.schema = s
	c.mu.Unlock()
	return nil
}

// Name returns the collection name.
func (c *Collection) Name() string { return c.name }

// Insert creates a new Document, generating a UUID if ID is empty.
// Returns the assigned document ID.
func (c *Collection) Insert(doc *types.Document) (string, error) {
	c.mu.RLock()
	s := c.schema
	c.mu.RUnlock()
	
	if err := s.Apply(doc, false); err != nil {
		return "", fmt.Errorf("collection %s: schema validate: %w", c.name, err)
	}
	
	// Ensure ID is set (Apply might have generated it)
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	if err := c.validateUniqueConstraints(doc, ""); err != nil {
		return "", err
	}
	
	data, err := marshalDoc(doc)
	if err != nil {
		return "", fmt.Errorf("collection %s: marshal: %w", c.name, err)
	}
	if err := c.eng.Put(c.name, []byte(doc.ID), data); err != nil {
		return "", fmt.Errorf("collection %s: put: %w", c.name, err)
	}
	
	if c.hub != nil {
		parts := strings.Split(c.name, "/")
		dbName, colName := parts[0], parts[1]
		c.hub.Broadcast(Event{
			Type:       EventInsert,
			Database:   dbName,
			Collection: colName,
			DocID:      doc.ID,
			Doc:        doc,
		})
	}
	
	return doc.ID, nil
}

// Update replaces the document with the given ID.
// Returns ErrNotFound if the ID does not exist.
func (c *Collection) Update(id string, doc *types.Document) error {
	c.mu.RLock()
	s := c.schema
	c.mu.RUnlock()

	if err := s.Apply(doc, true); err != nil {
		return fmt.Errorf("collection %s: schema validate: %w", c.name, err)
	}

	doc.ID = id
	if err := c.validateUniqueConstraints(doc, id); err != nil {
		return err
	}
	data, err := marshalDoc(doc)
	if err != nil {
		return fmt.Errorf("collection %s: marshal: %w", c.name, err)
	}
	err = c.eng.Put(c.name, []byte(id), data)
	if err == nil && c.hub != nil {
		parts := strings.Split(c.name, "/")
		dbName, colName := parts[0], parts[1]
		c.hub.Broadcast(Event{
			Type:       EventUpdate,
			Database:   dbName,
			Collection: colName,
			DocID:      id,
			Doc:        doc,
		})
	}
	return err
}

// Delete removes the document with the given ID.
func (c *Collection) Delete(id string) error {
	err := c.eng.Delete(c.name, []byte(id))
	if err == nil && c.hub != nil {
		parts := strings.Split(c.name, "/")
		dbName, colName := parts[0], parts[1]
		c.hub.Broadcast(Event{
			Type:       EventDelete,
			Database:   dbName,
			Collection: colName,
			DocID:      id,
		})
	}
	return err
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
		
		// Resolve Joins (Eager loading)
		if len(q.Joins()) > 0 {
			dbName := strings.Split(c.name, "/")[0]
			for _, join := range q.Joins() {
				targetNs := dbName + "/" + join.TargetCol
				for _, doc := range results {
					var localVal types.Value
					if join.LocalKey == "_id" {
						localVal = types.String(doc.ID)
					} else {
						localVal = doc.Get(join.LocalKey)
					}
					
					var joinedDocs []types.Value
					_ = c.eng.Scan(targetNs, func(k, v []byte) bool {
						tdoc, err := unmarshalDoc(v)
						if err != nil {
							return true
						}
						
						var foreignVal types.Value
						if join.ForeignKey == "_id" {
							foreignVal = types.String(tdoc.ID)
						} else {
							foreignVal = tdoc.Get(join.ForeignKey)
						}
						
						if types.Equal(localVal, foreignVal) {
							// Embed the whole document as an Object
							m := make(map[string]types.Value, len(tdoc.Fields)+1)
							m["_id"] = types.String(tdoc.ID)
							for tk, tv := range tdoc.Fields {
								m[tk] = tv
							}
							joinedDocs = append(joinedDocs, types.Object(m))
							
							if join.Relation == "one_to_one" || join.Relation == "many_to_one" {
								return false // stop scanning if we only need one
							}
						}
						return true
					})
					
					if join.Relation == "one_to_one" || join.Relation == "many_to_one" {
						if len(joinedDocs) > 0 {
							doc.Set(join.As, joinedDocs[0])
						} else {
							doc.Set(join.As, types.Null())
						}
					} else {
						doc.Set(join.As, types.Array(joinedDocs))
					}
				}
			}
		}
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

func (c *Collection) validateUniqueConstraints(doc *types.Document, currentID string) error {
	c.mu.RLock()
	schema := c.schema
	c.mu.RUnlock()
	if schema == nil {
		return nil
	}

	type uniqueCheck struct {
		Label  string
		Fields []string
	}

	var checks []uniqueCheck
	for _, field := range schema.Fields {
		if field.Unique {
			checks = append(checks, uniqueCheck{
				Label:  field.Name,
				Fields: []string{field.StorageName()},
			})
		}
	}
	for _, idx := range schema.Indexes {
		if idx.Unique || idx.Primary {
			checks = append(checks, uniqueCheck{
				Label:  idx.Name,
				Fields: append([]string(nil), idx.Fields...),
			})
		}
	}

	for _, check := range checks {
		if len(check.Fields) == 0 {
			continue
		}
		var target []types.Value
		skip := false
		for _, fieldName := range check.Fields {
			if fieldName == "_id" {
				target = append(target, types.String(doc.ID))
				continue
			}
			v := doc.Get(fieldName)
			if v.IsNull() {
				skip = true
				break
			}
			target = append(target, v)
		}
		if skip {
			continue
		}

		conflict := false
		err := c.eng.Scan(c.name, func(_, value []byte) bool {
			existing, err := unmarshalDoc(value)
			if err != nil {
				return true
			}
			if existing.ID == currentID {
				return true
			}
			for i, fieldName := range check.Fields {
				var existingValue types.Value
				if fieldName == "_id" {
					existingValue = types.String(existing.ID)
				} else {
					existingValue = existing.Get(fieldName)
				}
				if !types.Equal(existingValue, target[i]) {
					return true
				}
			}
			conflict = true
			return false
		})
		if err != nil {
			return err
		}
		if conflict {
			label := check.Label
			if label == "" {
				label = strings.Join(check.Fields, ",")
			}
			return fmt.Errorf("collection %s: unique constraint violation on %s", c.name, label)
		}
	}

	return nil
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
	hub         *Hub
	collections map[string]*Collection
}

// newDatabase creates a Database backed by eng.
func newDatabase(eng *Engine, name string, hub *Hub) *Database {
	return &Database{
		name:        name,
		eng:         eng,
		hub:         hub,
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
	c := newCollection(db.eng, ns, db.hub)
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
	hub       *Hub
	databases map[string]*Database
}

// NewStore wraps an Engine in a document-store interface.
func NewStore(eng *Engine) *Store {
	return &Store{eng: eng, hub: NewHub(), databases: make(map[string]*Database)}
}

// Hub returns the realtime pub/sub hub.
func (s *Store) Hub() *Hub {
	return s.hub
}

// DB returns (or lazily creates) the named database.
func (s *Store) DB(name string) *Database {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.databases[name]; ok {
		return db
	}
	db := newDatabase(s.eng, name, s.hub)
	s.databases[name] = db
	return db
}

// CreateDatabase registers a database in metadata and returns its handle.
func (s *Store) CreateDatabase(name string) (*Database, error) {
	db := s.DB(name)
	if err := s.eng.Put("_meta", []byte(metaDatabasePrefix+name), []byte{1}); err != nil {
		return nil, err
	}
	return db, nil
}

// CreateCollection registers a collection in metadata and returns its handle.
func (s *Store) CreateCollection(dbName, colName string) (*Collection, error) {
	db, err := s.CreateDatabase(dbName)
	if err != nil {
		return nil, err
	}
	col := db.Collection(colName)
	if err := s.eng.Put("_meta", []byte(metaCollectionPrefix+dbName+"/"+colName), []byte{1}); err != nil {
		return nil, err
	}
	return col, nil
}

// DropCollection removes all documents and metadata for a collection.
func (s *Store) DropCollection(dbName, colName string) error {
	ns := dbName + "/" + colName
	var keys [][]byte
	if err := s.eng.Scan(ns, func(key, _ []byte) bool {
		keys = append(keys, append([]byte(nil), key...))
		return true
	}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := s.eng.Delete(ns, key); err != nil {
			return err
		}
	}
	if err := s.eng.Delete("_meta", []byte(metaCollectionPrefix+ns)); err != nil {
		return err
	}
	if err := s.eng.Delete("_meta", []byte(metaSchemaPrefix+ns)); err != nil {
		return err
	}

	s.mu.Lock()
	if db, ok := s.databases[dbName]; ok {
		db.mu.Lock()
		delete(db.collections, ns)
		db.mu.Unlock()
	}
	s.mu.Unlock()
	return nil
}

// DropDatabase removes all collections and metadata for a database.
func (s *Store) DropDatabase(name string) error {
	for _, col := range s.ListCollections(name) {
		if err := s.DropCollection(name, col); err != nil {
			return err
		}
	}
	if err := s.eng.Delete("_meta", []byte(metaDatabasePrefix+name)); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.databases, name)
	s.mu.Unlock()
	return nil
}

// ListDatabases returns the names of all known databases
// (those that have at least one key in the engine).
func (s *Store) ListDatabases() []string {
	seen := make(map[string]struct{})

	s.mu.RLock()
	for name := range s.databases {
		seen[name] = struct{}{}
	}
	s.mu.RUnlock()

	_ = s.eng.Scan("_meta", func(key, _ []byte) bool {
		name := string(key)
		switch {
		case strings.HasPrefix(name, metaDatabasePrefix):
			seen[strings.TrimPrefix(name, metaDatabasePrefix)] = struct{}{}
		case strings.HasPrefix(name, metaCollectionPrefix):
			parts := strings.SplitN(strings.TrimPrefix(name, metaCollectionPrefix), "/", 2)
			if len(parts) == 2 && parts[0] != "" {
				seen[parts[0]] = struct{}{}
			}
		}
		return true
	})

	for _, ns := range s.eng.Namespaces() {
		if ns == "_meta" {
			continue
		}
		parts := strings.SplitN(ns, "/", 2)
		if len(parts) == 2 && parts[0] != "" {
			seen[parts[0]] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ListCollections returns collection names within a database.
func (s *Store) ListCollections(dbName string) []string {
	seen := make(map[string]struct{})

	s.mu.RLock()
	db, ok := s.databases[dbName]
	s.mu.RUnlock()
	if ok {
		db.mu.RLock()
		prefix := dbName + "/"
		for ns := range db.collections {
			seen[strings.TrimPrefix(ns, prefix)] = struct{}{}
		}
		db.mu.RUnlock()
	}

	metaPrefix := metaCollectionPrefix + dbName + "/"
	_ = s.eng.Scan("_meta", func(key, _ []byte) bool {
		name := string(key)
		if strings.HasPrefix(name, metaPrefix) {
			seen[strings.TrimPrefix(name, metaPrefix)] = struct{}{}
		}
		return true
	})

	namespacePrefix := dbName + "/"
	for _, ns := range s.eng.Namespaces() {
		if strings.HasPrefix(ns, namespacePrefix) {
			seen[strings.TrimPrefix(ns, namespacePrefix)] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Engine exposes the underlying Engine for stats and administration.
func (s *Store) Engine() *Engine { return s.eng }

func databaseNameFromNamespace(ns string) string {
	parts := strings.SplitN(ns, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func collectionNameFromNamespace(ns string) string {
	parts := strings.SplitN(ns, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ns
}
