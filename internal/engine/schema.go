package engine

import (
	"encoding/json"
	"fmt"
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
)

// SchemaField defines a single field in a collection schema.
type SchemaField struct {
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	// Default can be a static value or a special string like "uuid()", "now()".
	Default *string `json:"default,omitempty"`
}

// Schema represents the schema definition for a collection.
type Schema struct {
	Fields []SchemaField `json:"fields"`
}

// NewDefaultSchema returns the default schema with _id, created_at, updated_at.
func NewDefaultSchema() *Schema {
	idDef := "uuid()"
	nowDef := "now()"
	return &Schema{
		Fields: []SchemaField{
			{Name: "_id", Type: TypeString, Default: &idDef},
			{Name: "created_at", Type: TypeDateTime, Default: &nowDef},
			{Name: "updated_at", Type: TypeDateTime, Default: &nowDef},
		},
	}
}

// Apply applies the schema to a document, generating default values.
func (s *Schema) Apply(doc *types.Document, isUpdate bool) error {
	for _, f := range s.Fields {
		val := doc.Get(f.Name)
		
		if val.IsNull() {
			if f.Default != nil {
				// Generate defaults
				switch *f.Default {
				case "uuid()":
					if !isUpdate {
						doc.Set(f.Name, types.String(uuid.New().String()))
					}
				case "now()":
					if f.Name == "updated_at" || !isUpdate {
						doc.Set(f.Name, types.String(time.Now().UTC().Format(time.RFC3339)))
					}
				default:
					if !isUpdate {
						doc.Set(f.Name, types.String(*f.Default)) // Simplify for string defaults
					}
				}
			} else if f.Required {
				return fmt.Errorf("field %s is required", f.Name)
			}
		}
	}
	return nil
}

// marshalSchema serializes a schema to JSON bytes.
func marshalSchema(s *Schema) ([]byte, error) {
	return json.Marshal(s)
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
	return &s, nil
}
