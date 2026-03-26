package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/voiddb/void/internal/engine/types"
)

// FieldType represents the data type of a schema field.
type FieldType string

const (
	TypeString   FieldType = "string"
	TypeNumber   FieldType = "number"
	TypeBoolean  FieldType = "boolean"
	TypeDateTime FieldType = "datetime"
	TypeArray    FieldType = "array"
	TypeObject   FieldType = "object"
	TypeBlob     FieldType = "blob"
	TypeRelation FieldType = "relation"
)

// SchemaRelation stores Prisma-like relation metadata for a field.
type SchemaRelation struct {
	Model      string   `json:"model,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	References []string `json:"references,omitempty"`
	OnDelete   string   `json:"on_delete,omitempty"`
	OnUpdate   string   `json:"on_update,omitempty"`
	Name       string   `json:"name,omitempty"`
}

// SchemaIndex stores index and unique metadata for a collection.
type SchemaIndex struct {
	Name    string   `json:"name,omitempty"`
	Fields  []string `json:"fields"`
	Unique  bool     `json:"unique,omitempty"`
	Primary bool     `json:"primary,omitempty"`
}

// SchemaField defines a single field in a collection schema.
type SchemaField struct {
	Name          string          `json:"name"`
	Type          FieldType       `json:"type"`
	Required      bool            `json:"required,omitempty"`
	Default       *string         `json:"default,omitempty"`
	DefaultExpr   *string         `json:"default_expr,omitempty"`
	PrismaType    string          `json:"prisma_type,omitempty"`
	Unique        bool            `json:"unique,omitempty"`
	IsID          bool            `json:"is_id,omitempty"`
	List          bool            `json:"list,omitempty"`
	Virtual       bool            `json:"virtual,omitempty"`
	AutoUpdatedAt bool            `json:"auto_updated_at,omitempty"`
	MappedName    string          `json:"mapped_name,omitempty"`
	Relation      *SchemaRelation `json:"relation,omitempty"`
}

// StorageName returns the persisted field name inside documents.
func (f SchemaField) StorageName() string {
	if f.MappedName != "" {
		return f.MappedName
	}
	if f.IsID {
		return "_id"
	}
	return f.Name
}

// Schema represents the schema definition for a collection.
type Schema struct {
	Database   string        `json:"database,omitempty"`
	Collection string        `json:"collection,omitempty"`
	Model      string        `json:"model,omitempty"`
	Fields     []SchemaField `json:"fields"`
	Indexes    []SchemaIndex `json:"indexes,omitempty"`
}

// NewDefaultSchema returns the default schema with _id, created_at, updated_at.
func NewDefaultSchema() *Schema {
	idDef := "uuid()"
	nowDef := "now()"
	return &Schema{
		Fields: []SchemaField{
			{
				Name:        "_id",
				Type:        TypeString,
				Required:    true,
				Default:     &idDef,
				DefaultExpr: &idDef,
				PrismaType:  "String",
				IsID:        true,
			},
			{
				Name:        "created_at",
				Type:        TypeDateTime,
				Default:     &nowDef,
				DefaultExpr: &nowDef,
				PrismaType:  "DateTime",
			},
			{
				Name:          "updated_at",
				Type:          TypeDateTime,
				Default:       &nowDef,
				DefaultExpr:   &nowDef,
				PrismaType:    "DateTime",
				AutoUpdatedAt: true,
			},
		},
	}
}

// Apply applies the schema to a document, generating defaults and validating types.
func (s *Schema) Apply(doc *types.Document, isUpdate bool) error {
	if s == nil {
		return nil
	}

	for _, f := range s.Fields {
		if f.Virtual || f.Type == TypeRelation {
			continue
		}

		if f.IsID || f.StorageName() == "_id" {
			if doc.ID == "" {
				def, ok, err := defaultValueForField(f, isUpdate)
				if err != nil {
					return fmt.Errorf("field %s default: %w", f.Name, err)
				}
				if ok {
					if def.Type() != types.TypeString {
						return fmt.Errorf("field %s: _id default must resolve to string", f.Name)
					}
					doc.ID = def.StringVal()
				} else if f.Required {
					return fmt.Errorf("field %s is required", f.Name)
				}
			}
			delete(doc.Fields, "_id")
			continue
		}

		name := f.StorageName()
		val := doc.Get(name)
		if val.IsNull() {
			if f.AutoUpdatedAt && isUpdate {
				doc.Set(name, types.String(time.Now().UTC().Format(time.RFC3339)))
				continue
			}
			def, ok, err := defaultValueForField(f, isUpdate)
			if err != nil {
				return fmt.Errorf("field %s default: %w", f.Name, err)
			}
			if ok {
				doc.Set(name, def)
				continue
			}
			if f.Required {
				return fmt.Errorf("field %s is required", f.Name)
			}
			continue
		}

		if err := validateFieldValue(f, val); err != nil {
			return err
		}
	}

	for _, f := range s.Fields {
		if f.Virtual || f.Type == TypeRelation {
			continue
		}
		if f.AutoUpdatedAt {
			doc.Set(f.StorageName(), types.String(time.Now().UTC().Format(time.RFC3339)))
		}
	}

	return nil
}

func defaultValueForField(f SchemaField, isUpdate bool) (types.Value, bool, error) {
	expr := ""
	switch {
	case f.DefaultExpr != nil:
		expr = strings.TrimSpace(*f.DefaultExpr)
	case f.Default != nil:
		expr = strings.TrimSpace(*f.Default)
	default:
		return types.Null(), false, nil
	}

	switch expr {
	case "uuid()", "cuid()", "ulid()":
		if isUpdate {
			return types.Null(), false, nil
		}
		return types.String(uuid.New().String()), true, nil
	case "now()":
		if f.AutoUpdatedAt || !isUpdate {
			return types.String(time.Now().UTC().Format(time.RFC3339)), true, nil
		}
		return types.Null(), false, nil
	}

	value, err := parseLiteralDefault(expr, f)
	if err != nil {
		return types.Null(), false, err
	}
	if isUpdate && !f.AutoUpdatedAt {
		return types.Null(), false, nil
	}
	return value, true, nil
}

func parseLiteralDefault(expr string, f SchemaField) (types.Value, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return types.Null(), nil
	}

	if strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"") && len(expr) >= 2 {
		unquoted, err := strconv.Unquote(expr)
		if err != nil {
			return types.Null(), err
		}
		return types.String(unquoted), nil
	}

	switch f.Type {
	case TypeString:
		return types.String(expr), nil
	case TypeNumber:
		n, err := strconv.ParseFloat(expr, 64)
		if err != nil {
			return types.Null(), err
		}
		return types.Number(n), nil
	case TypeBoolean:
		b, err := strconv.ParseBool(expr)
		if err != nil {
			return types.Null(), err
		}
		return types.Boolean(b), nil
	case TypeArray, TypeObject:
		var decoded interface{}
		if err := json.Unmarshal([]byte(expr), &decoded); err != nil {
			return types.Null(), err
		}
		return jsonInterfaceToValue(decoded), nil
	case TypeBlob:
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(expr), &decoded); err != nil {
			return types.Null(), err
		}
		bucket, _ := decoded["_blob_bucket"].(string)
		key, _ := decoded["_blob_key"].(string)
		if bucket == "" || key == "" {
			return types.Null(), fmt.Errorf("blob default must contain _blob_bucket and _blob_key")
		}
		return types.Blob(bucket, key), nil
	case TypeDateTime:
		if _, err := time.Parse(time.RFC3339, expr); err != nil {
			if _, errNano := time.Parse(time.RFC3339Nano, expr); errNano != nil {
				return types.Null(), err
			}
		}
		return types.String(expr), nil
	case TypeRelation:
		return types.Null(), nil
	default:
		return types.String(expr), nil
	}
}

func validateFieldValue(f SchemaField, val types.Value) error {
	if f.PrismaType == "Json" {
		switch val.Type() {
		case types.TypeNull, types.TypeString, types.TypeNumber, types.TypeBoolean, types.TypeArray, types.TypeObject:
			return nil
		}
	}

	expected := f.Type
	switch expected {
	case TypeString:
		if val.Type() != types.TypeString {
			return fmt.Errorf("field %s must be string", f.Name)
		}
	case TypeNumber:
		if val.Type() != types.TypeNumber {
			return fmt.Errorf("field %s must be number", f.Name)
		}
	case TypeBoolean:
		if val.Type() != types.TypeBoolean {
			return fmt.Errorf("field %s must be boolean", f.Name)
		}
	case TypeDateTime:
		if val.Type() != types.TypeString {
			return fmt.Errorf("field %s must be RFC3339 datetime string", f.Name)
		}
		if _, err := time.Parse(time.RFC3339, val.StringVal()); err != nil {
			if _, errNano := time.Parse(time.RFC3339Nano, val.StringVal()); errNano != nil {
				return fmt.Errorf("field %s must be RFC3339 datetime string", f.Name)
			}
		}
	case TypeArray:
		if val.Type() != types.TypeArray {
			return fmt.Errorf("field %s must be array", f.Name)
		}
	case TypeObject:
		if val.Type() != types.TypeObject {
			return fmt.Errorf("field %s must be object", f.Name)
		}
	case TypeBlob:
		if val.Type() != types.TypeBlob {
			return fmt.Errorf("field %s must be blob reference", f.Name)
		}
	case TypeRelation:
		return nil
	}
	return nil
}

func jsonInterfaceToValue(v interface{}) types.Value {
	switch val := v.(type) {
	case nil:
		return types.Null()
	case string:
		return types.String(val)
	case float64:
		return types.Number(val)
	case bool:
		return types.Boolean(val)
	case []interface{}:
		out := make([]types.Value, len(val))
		for i, item := range val {
			out[i] = jsonInterfaceToValue(item)
		}
		return types.Array(out)
	case map[string]interface{}:
		out := make(map[string]types.Value, len(val))
		for k, item := range val {
			out[k] = jsonInterfaceToValue(item)
		}
		return types.Object(out)
	default:
		return types.Null()
	}
}

// Normalize sorts indexes for stable comparisons and fills missing metadata.
func (s *Schema) Normalize() *Schema {
	if s == nil {
		return NewDefaultSchema()
	}
	out := *s
	out.Fields = append([]SchemaField(nil), s.Fields...)
	out.Indexes = append([]SchemaIndex(nil), s.Indexes...)

	for i := range out.Fields {
		if out.Fields[i].PrismaType == "" {
			out.Fields[i].PrismaType = prismaTypeForField(out.Fields[i])
		}
	}

	sortSchemaIndexes(out.Indexes)
	return &out
}

func sortSchemaIndexes(indexes []SchemaIndex) {
	for i := range indexes {
		fields := append([]string(nil), indexes[i].Fields...)
		indexes[i].Fields = fields
	}
	for i := 0; i < len(indexes)-1; i++ {
		for j := i + 1; j < len(indexes); j++ {
			left := strings.Join(indexes[i].Fields, ",") + "|" + indexes[i].Name
			right := strings.Join(indexes[j].Fields, ",") + "|" + indexes[j].Name
			if right < left {
				indexes[i], indexes[j] = indexes[j], indexes[i]
			}
		}
	}
}

func prismaTypeForField(f SchemaField) string {
	if f.PrismaType != "" {
		return f.PrismaType
	}
	switch f.Type {
	case TypeString:
		return "String"
	case TypeNumber:
		return "Float"
	case TypeBoolean:
		return "Boolean"
	case TypeDateTime:
		return "DateTime"
	case TypeArray, TypeObject, TypeRelation:
		return "Json"
	case TypeBlob:
		return "Blob"
	default:
		return "String"
	}
}

// marshalSchema serializes a schema to JSON bytes.
func marshalSchema(s *Schema) ([]byte, error) {
	return json.Marshal(s.Normalize())
}

// unmarshalSchema deserializes a schema from JSON bytes.
func unmarshalSchema(data []byte) (*Schema, error) {
	if len(data) == 0 {
		return NewDefaultSchema(), nil
	}
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Normalize(), nil
}
