package pgimport

import (
	"fmt"
	"strings"
	"time"

	"github.com/voiddb/void/internal/engine"
)

type temporalKind int

const (
	temporalNone temporalKind = iota
	temporalDate
	temporalTimestamp
	temporalTime
)

// MapScalarType maps a PostgreSQL scalar column type to a VoidDB schema field type.
func MapScalarType(dataType, udtName string) (engine.FieldType, string) {
	key := strings.ToLower(strings.TrimSpace(dataType))
	udt := strings.ToLower(strings.TrimSpace(udtName))
	switch key {
	case "smallint", "integer":
		return engine.TypeNumber, "Int"
	case "bigint":
		return engine.TypeNumber, "BigInt"
	case "real", "double precision":
		return engine.TypeNumber, "Float"
	case "numeric", "decimal":
		return engine.TypeNumber, "Decimal"
	case "boolean":
		return engine.TypeBoolean, "Boolean"
	case "json", "jsonb":
		return engine.TypeObject, "Json"
	case "date", "timestamp without time zone", "timestamp with time zone":
		return engine.TypeDateTime, "DateTime"
	case "time without time zone", "time with time zone":
		// VoidDB does not have a time-only scalar, so preserve these as strings.
		return engine.TypeString, "String"
	case "bytea":
		return engine.TypeString, "Bytes"
	case "uuid":
		return engine.TypeString, "String"
	}

	switch udt {
	case "int2", "int4":
		return engine.TypeNumber, "Int"
	case "int8":
		return engine.TypeNumber, "BigInt"
	case "float4", "float8":
		return engine.TypeNumber, "Float"
	case "numeric":
		return engine.TypeNumber, "Decimal"
	case "bool":
		return engine.TypeBoolean, "Boolean"
	case "json", "jsonb":
		return engine.TypeObject, "Json"
	case "date", "timestamp", "timestamptz":
		return engine.TypeDateTime, "DateTime"
	case "time", "timetz":
		return engine.TypeString, "String"
	case "bytea":
		return engine.TypeString, "Bytes"
	}

	return engine.TypeString, "String"
}

// NormalizeColumnValue coerces PostgreSQL temporal JSON values into formats
// accepted by VoidDB schema validation.
func NormalizeColumnValue(dataType, udtName string, value interface{}) (interface{}, error) {
	kind := temporalKindForType(dataType, udtName)
	if kind == temporalNone || value == nil {
		return value, nil
	}

	raw, ok := value.(string)
	if !ok {
		return value, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, nil
	}

	switch kind {
	case temporalDate:
		if t, ok := parseWithLayouts(raw, []string{"2006-01-02"}); ok {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339), nil
		}
		if t, ok := parseRFC3339Like(raw); ok {
			return t.UTC().Format(time.RFC3339Nano), nil
		}
		return nil, fmt.Errorf("invalid date value %q", raw)
	case temporalTimestamp:
		if t, ok := parseRFC3339Like(raw); ok {
			return t.UTC().Format(time.RFC3339Nano), nil
		}
		if t, ok := parseWithLayouts(raw, []string{
			"2006-01-02T15:04:05.999999999",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
		}); ok {
			return t.UTC().Format(time.RFC3339Nano), nil
		}
		return nil, fmt.Errorf("invalid timestamp value %q", raw)
	case temporalTime:
		return raw, nil
	default:
		return value, nil
	}
}

func temporalKindForType(dataType, udtName string) temporalKind {
	key := strings.ToLower(strings.TrimSpace(dataType))
	udt := strings.ToLower(strings.TrimSpace(udtName))

	switch key {
	case "date":
		return temporalDate
	case "timestamp without time zone", "timestamp with time zone":
		return temporalTimestamp
	case "time without time zone", "time with time zone":
		return temporalTime
	}

	switch udt {
	case "date":
		return temporalDate
	case "timestamp", "timestamptz":
		return temporalTimestamp
	case "time", "timetz":
		return temporalTime
	default:
		return temporalNone
	}
}

func parseRFC3339Like(value string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999Z07",
		"2006-01-02 15:04:05Z07",
	} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseWithLayouts(value string, layouts []string) (time.Time, bool) {
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
