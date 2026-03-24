// Package types defines the core value types used throughout VoidDB.
// Every document field is represented as a Value which carries both
// type information and the actual payload in a compact binary form.
package types

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// ValueType is a single-byte tag that identifies the kind of value.
type ValueType byte

const (
	// TypeNull represents a missing or explicit null value.
	TypeNull ValueType = 0
	// TypeString is a UTF-8 encoded string.
	TypeString ValueType = 1
	// TypeNumber is an IEEE-754 double-precision float (covers all integers too).
	TypeNumber ValueType = 2
	// TypeBoolean is a single true/false bit.
	TypeBoolean ValueType = 3
	// TypeArray is an ordered sequence of Values.
	TypeArray ValueType = 4
	// TypeObject is a map of string keys to Values (nested document).
	TypeObject ValueType = 5
	// TypeBlob is a reference (bucket + key) into the blob store.
	TypeBlob ValueType = 6
)

// Value is a tagged union holding any VoidDB-supported data type.
// The zero value is TypeNull.
type Value struct {
	t   ValueType
	num float64
	b   bool
	s   string
	arr []Value
	obj map[string]Value
	// blob fields
	blobBucket string
	blobKey    string
}

// --- Constructors ------------------------------------------------------------

// Null returns a null Value.
func Null() Value { return Value{t: TypeNull} }

// String returns a string Value.
func String(s string) Value { return Value{t: TypeString, s: s} }

// Number returns a numeric Value from any numeric Go type.
func Number(n float64) Value { return Value{t: TypeNumber, num: n} }

// Boolean returns a boolean Value.
func Boolean(b bool) Value { return Value{t: TypeBoolean, b: b} }

// Array returns an array Value from a slice of Values.
func Array(items []Value) Value { return Value{t: TypeArray, arr: items} }

// Object returns an object Value from a map.
func Object(m map[string]Value) Value { return Value{t: TypeObject, obj: m} }

// Blob returns a blob reference Value.
func Blob(bucket, key string) Value {
	return Value{t: TypeBlob, blobBucket: bucket, blobKey: key}
}

// --- Accessors ---------------------------------------------------------------

// Type returns the ValueType tag.
func (v Value) Type() ValueType { return v.t }

// IsNull reports whether v is null.
func (v Value) IsNull() bool { return v.t == TypeNull }

// StringVal returns the string payload. Panics if type is not TypeString.
func (v Value) StringVal() string {
	if v.t != TypeString {
		panic(fmt.Sprintf("voiddb/types: StringVal called on %s", v.t))
	}
	return v.s
}

// NumberVal returns the numeric payload. Panics if type is not TypeNumber.
func (v Value) NumberVal() float64 {
	if v.t != TypeNumber {
		panic(fmt.Sprintf("voiddb/types: NumberVal called on %s", v.t))
	}
	return v.num
}

// BoolVal returns the boolean payload. Panics if type is not TypeBoolean.
func (v Value) BoolVal() bool {
	if v.t != TypeBoolean {
		panic(fmt.Sprintf("voiddb/types: BoolVal called on %s", v.t))
	}
	return v.b
}

// ArrayVal returns the array items. Panics if type is not TypeArray.
func (v Value) ArrayVal() []Value {
	if v.t != TypeArray {
		panic(fmt.Sprintf("voiddb/types: ArrayVal called on %s", v.t))
	}
	return v.arr
}

// ObjectVal returns the object map. Panics if type is not TypeObject.
func (v Value) ObjectVal() map[string]Value {
	if v.t != TypeObject {
		panic(fmt.Sprintf("voiddb/types: ObjectVal called on %s", v.t))
	}
	return v.obj
}

// BlobRef returns (bucket, key) for a blob Value.
func (v Value) BlobRef() (bucket, key string) {
	if v.t != TypeBlob {
		panic(fmt.Sprintf("voiddb/types: BlobRef called on %s", v.t))
	}
	return v.blobBucket, v.blobKey
}

// String implements fmt.Stringer for human-readable output.
func (v Value) String() string {
	switch v.t {
	case TypeNull:
		return "null"
	case TypeString:
		return fmt.Sprintf("%q", v.s)
	case TypeNumber:
		if v.num == math.Trunc(v.num) {
			return fmt.Sprintf("%d", int64(v.num))
		}
		return fmt.Sprintf("%g", v.num)
	case TypeBoolean:
		if v.b {
			return "true"
		}
		return "false"
	case TypeArray:
		parts := make([]string, len(v.arr))
		for i, item := range v.arr {
			parts[i] = item.String()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case TypeObject:
		parts := make([]string, 0, len(v.obj))
		for k, val := range v.obj {
			parts = append(parts, fmt.Sprintf("%q: %s", k, val.String()))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case TypeBlob:
		return fmt.Sprintf("blob(%s/%s)", v.blobBucket, v.blobKey)
	default:
		return "<unknown>"
	}
}

// --- Comparison --------------------------------------------------------------

// Equal returns true if two Values are deeply equal.
func Equal(a, b Value) bool {
	if a.t != b.t {
		return false
	}
	switch a.t {
	case TypeNull:
		return true
	case TypeString:
		return a.s == b.s
	case TypeNumber:
		return a.num == b.num
	case TypeBoolean:
		return a.b == b.b
	case TypeArray:
		if len(a.arr) != len(b.arr) {
			return false
		}
		for i := range a.arr {
			if !Equal(a.arr[i], b.arr[i]) {
				return false
			}
		}
		return true
	case TypeObject:
		if len(a.obj) != len(b.obj) {
			return false
		}
		for k, av := range a.obj {
			bv, ok := b.obj[k]
			if !ok || !Equal(av, bv) {
				return false
			}
		}
		return true
	case TypeBlob:
		return a.blobBucket == b.blobBucket && a.blobKey == b.blobKey
	}
	return false
}

// --- Binary Serialization ----------------------------------------------------

// EncodedSize returns the number of bytes needed to encode v.
func (v Value) EncodedSize() int {
	switch v.t {
	case TypeNull:
		return 1
	case TypeBoolean:
		return 2
	case TypeNumber:
		return 9 // 1 tag + 8 float64
	case TypeString:
		return 1 + 4 + len(v.s)
	case TypeArray:
		size := 1 + 4 // tag + count
		for _, item := range v.arr {
			size += item.EncodedSize()
		}
		return size
	case TypeObject:
		size := 1 + 4 // tag + count
		for k, val := range v.obj {
			size += 4 + len(k) + val.EncodedSize()
		}
		return size
	case TypeBlob:
		return 1 + 4 + len(v.blobBucket) + 4 + len(v.blobKey)
	}
	return 1
}

// Encode serializes v into buf (which must be at least v.EncodedSize() bytes).
// Returns the number of bytes written.
func Encode(v Value, buf []byte) int {
	buf[0] = byte(v.t)
	off := 1
	switch v.t {
	case TypeNull:
		// nothing extra
	case TypeBoolean:
		if v.b {
			buf[1] = 1
		} else {
			buf[1] = 0
		}
		off = 2
	case TypeNumber:
		binary.LittleEndian.PutUint64(buf[1:], math.Float64bits(v.num))
		off = 9
	case TypeString:
		binary.LittleEndian.PutUint32(buf[1:], uint32(len(v.s)))
		off = 5
		off += copy(buf[off:], v.s)
	case TypeArray:
		binary.LittleEndian.PutUint32(buf[1:], uint32(len(v.arr)))
		off = 5
		for _, item := range v.arr {
			n := Encode(item, buf[off:])
			off += n
		}
	case TypeObject:
		binary.LittleEndian.PutUint32(buf[1:], uint32(len(v.obj)))
		off = 5
		for k, val := range v.obj {
			binary.LittleEndian.PutUint32(buf[off:], uint32(len(k)))
			off += 4
			off += copy(buf[off:], k)
			n := Encode(val, buf[off:])
			off += n
		}
	case TypeBlob:
		binary.LittleEndian.PutUint32(buf[1:], uint32(len(v.blobBucket)))
		off = 5
		off += copy(buf[off:], v.blobBucket)
		binary.LittleEndian.PutUint32(buf[off:], uint32(len(v.blobKey)))
		off += 4
		off += copy(buf[off:], v.blobKey)
	}
	return off
}

// Decode deserializes a Value from buf.
// Returns the Value and the number of bytes consumed.
func Decode(buf []byte) (Value, int, error) {
	if len(buf) < 1 {
		return Null(), 0, fmt.Errorf("voiddb/types: empty buffer")
	}
	t := ValueType(buf[0])
	off := 1
	switch t {
	case TypeNull:
		return Null(), 1, nil
	case TypeBoolean:
		if len(buf) < 2 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated boolean")
		}
		return Boolean(buf[1] != 0), 2, nil
	case TypeNumber:
		if len(buf) < 9 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated number")
		}
		bits := binary.LittleEndian.Uint64(buf[1:])
		return Number(math.Float64frombits(bits)), 9, nil
	case TypeString:
		if len(buf) < 5 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated string header")
		}
		slen := int(binary.LittleEndian.Uint32(buf[1:]))
		off = 5
		if len(buf) < off+slen {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated string body")
		}
		return String(string(buf[off : off+slen])), off + slen, nil
	case TypeArray:
		if len(buf) < 5 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated array header")
		}
		count := int(binary.LittleEndian.Uint32(buf[1:]))
		off = 5
		arr := make([]Value, count)
		for i := 0; i < count; i++ {
			v, n, err := Decode(buf[off:])
			if err != nil {
				return Null(), 0, err
			}
			arr[i] = v
			off += n
		}
		return Array(arr), off, nil
	case TypeObject:
		if len(buf) < 5 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated object header")
		}
		count := int(binary.LittleEndian.Uint32(buf[1:]))
		off = 5
		obj := make(map[string]Value, count)
		for i := 0; i < count; i++ {
			if len(buf) < off+4 {
				return Null(), 0, fmt.Errorf("voiddb/types: truncated object key length")
			}
			klen := int(binary.LittleEndian.Uint32(buf[off:]))
			off += 4
			if len(buf) < off+klen {
				return Null(), 0, fmt.Errorf("voiddb/types: truncated object key")
			}
			k := string(buf[off : off+klen])
			off += klen
			v, n, err := Decode(buf[off:])
			if err != nil {
				return Null(), 0, err
			}
			obj[k] = v
			off += n
		}
		return Object(obj), off, nil
	case TypeBlob:
		if len(buf) < 5 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated blob bucket length")
		}
		blen := int(binary.LittleEndian.Uint32(buf[1:]))
		off = 5
		if len(buf) < off+blen {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated blob bucket")
		}
		bucket := string(buf[off : off+blen])
		off += blen
		if len(buf) < off+4 {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated blob key length")
		}
		klen := int(binary.LittleEndian.Uint32(buf[off:]))
		off += 4
		if len(buf) < off+klen {
			return Null(), 0, fmt.Errorf("voiddb/types: truncated blob key")
		}
		key := string(buf[off : off+klen])
		off += klen
		return Blob(bucket, key), off, nil
	}
	return Null(), 0, fmt.Errorf("voiddb/types: unknown type tag %d", t)
}

// String returns the human-readable name of the type.
func (vt ValueType) String() string {
	switch vt {
	case TypeNull:
		return "Null"
	case TypeString:
		return "String"
	case TypeNumber:
		return "Number"
	case TypeBoolean:
		return "Boolean"
	case TypeArray:
		return "Array"
	case TypeObject:
		return "Object"
	case TypeBlob:
		return "Blob"
	default:
		return fmt.Sprintf("Unknown(%d)", byte(vt))
	}
}

// Document is a top-level record stored in a Collection.
// It always has an immutable string ID and a map of field values.
type Document struct {
	// ID is the unique primary key of the document.
	ID string
	// Fields maps field names to their Values.
	Fields map[string]Value
}

// Get retrieves a field value by name, returning Null if not found.
func (d *Document) Get(field string) Value {
	if v, ok := d.Fields[field]; ok {
		return v
	}
	return Null()
}

// Set updates or inserts a field.
func (d *Document) Set(field string, v Value) {
	if d.Fields == nil {
		d.Fields = make(map[string]Value)
	}
	d.Fields[field] = v
}
