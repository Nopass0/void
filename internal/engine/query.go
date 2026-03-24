// Package engine – query.go implements the VoidDB query DSL.
//
// A Query is constructed with a fluent builder:
//
//	q := engine.NewQuery().
//	    Where("age", engine.Gt, types.Number(18)).
//	    Where("active", engine.Eq, types.Boolean(true)).
//	    OrderBy("name", engine.Asc).
//	    Limit(20).
//	    Skip(0)
package engine

import (
	"sort"
	"strings"

	"github.com/voiddb/void/internal/engine/types"
)

// Op is a filter comparison operator.
type Op string

const (
	// Eq tests equality.
	Eq Op = "eq"
	// Ne tests inequality.
	Ne Op = "ne"
	// Gt tests greater-than.
	Gt Op = "gt"
	// Gte tests greater-than-or-equal.
	Gte Op = "gte"
	// Lt tests less-than.
	Lt Op = "lt"
	// Lte tests less-than-or-equal.
	Lte Op = "lte"
	// Contains tests substring (strings) or element membership (arrays).
	Contains Op = "contains"
	// StartsWith tests string prefix.
	StartsWith Op = "starts_with"
	// In tests if the field value is in a list.
	In Op = "in"
)

// SortDir is the sort direction.
type SortDir string

const (
	// Asc sorts ascending.
	Asc SortDir = "asc"
	// Desc sorts descending.
	Desc SortDir = "desc"
)

// filter represents a single WHERE clause predicate.
type filter struct {
	field string
	op    Op
	value types.Value
	// list is used by the In operator.
	list []types.Value
}

// sortSpec describes one level of sorting.
type sortSpec struct {
	field string
	dir   SortDir
}

// Query is an immutable query specification built with the fluent API.
type Query struct {
	filters []filter
	sorts   []sortSpec
	limit   int
	skip    int
}

// NewQuery creates an empty query that matches all documents.
func NewQuery() *Query {
	return &Query{limit: -1}
}

// Where adds a filter predicate.
func (q *Query) Where(field string, op Op, value types.Value) *Query {
	cp := q.clone()
	cp.filters = append(cp.filters, filter{field: field, op: op, value: value})
	return cp
}

// WhereIn adds a filter that tests if field is in a list of values.
func (q *Query) WhereIn(field string, values ...types.Value) *Query {
	cp := q.clone()
	cp.filters = append(cp.filters, filter{field: field, op: In, list: values})
	return cp
}

// OrderBy adds a sort key.
func (q *Query) OrderBy(field string, dir SortDir) *Query {
	cp := q.clone()
	cp.sorts = append(cp.sorts, sortSpec{field: field, dir: dir})
	return cp
}

// Limit caps the result set size.
func (q *Query) Limit(n int) *Query {
	cp := q.clone()
	cp.limit = n
	return cp
}

// Skip skips the first n results (for pagination).
func (q *Query) Skip(n int) *Query {
	cp := q.clone()
	cp.skip = n
	return cp
}

// clone creates a shallow copy of the Query.
func (q *Query) clone() *Query {
	cp := *q
	cp.filters = append([]filter(nil), q.filters...)
	cp.sorts = append([]sortSpec(nil), q.sorts...)
	return &cp
}

// matches returns true if doc satisfies all WHERE predicates.
func (q *Query) matches(doc *types.Document) bool {
	for _, f := range q.filters {
		if !evalFilter(doc, f) {
			return false
		}
	}
	return true
}

// applySort sorts results according to the OrderBy clauses.
func (q *Query) applySort(docs []*types.Document) []*types.Document {
	if len(q.sorts) == 0 {
		return docs
	}
	sorted := make([]*types.Document, len(docs))
	copy(sorted, docs)
	sort.SliceStable(sorted, func(i, j int) bool {
		for _, s := range q.sorts {
			vi := sorted[i].Get(s.field)
			vj := sorted[j].Get(s.field)
			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}
			if s.dir == Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	return sorted
}

// applyPagination slices results according to Skip/Limit.
func (q *Query) applyPagination(docs []*types.Document) []*types.Document {
	if q.skip > 0 {
		if q.skip >= len(docs) {
			return nil
		}
		docs = docs[q.skip:]
	}
	if q.limit >= 0 && q.limit < len(docs) {
		docs = docs[:q.limit]
	}
	return docs
}

// --- filter evaluation -------------------------------------------------------

// evalFilter applies a single filter to a document.
func evalFilter(doc *types.Document, f filter) bool {
	v := doc.Get(f.field)

	switch f.op {
	case Eq:
		return types.Equal(v, f.value)
	case Ne:
		return !types.Equal(v, f.value)
	case Gt:
		return compareValues(v, f.value) > 0
	case Gte:
		return compareValues(v, f.value) >= 0
	case Lt:
		return compareValues(v, f.value) < 0
	case Lte:
		return compareValues(v, f.value) <= 0
	case Contains:
		return evalContains(v, f.value)
	case StartsWith:
		if v.Type() == types.TypeString && f.value.Type() == types.TypeString {
			return strings.HasPrefix(v.StringVal(), f.value.StringVal())
		}
		return false
	case In:
		for _, item := range f.list {
			if types.Equal(v, item) {
				return true
			}
		}
		return false
	}
	return false
}

// evalContains handles the Contains operator.
func evalContains(v, needle types.Value) bool {
	switch v.Type() {
	case types.TypeString:
		if needle.Type() == types.TypeString {
			return strings.Contains(v.StringVal(), needle.StringVal())
		}
	case types.TypeArray:
		for _, item := range v.ArrayVal() {
			if types.Equal(item, needle) {
				return true
			}
		}
	}
	return false
}

// compareValues returns negative/zero/positive for <, =, > relationships.
// Only String and Number comparisons are ordered; others use equality.
func compareValues(a, b types.Value) int {
	if a.Type() != b.Type() {
		return int(a.Type()) - int(b.Type())
	}
	switch a.Type() {
	case types.TypeNumber:
		na, nb := a.NumberVal(), b.NumberVal()
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		return 0
	case types.TypeString:
		sa, sb := a.StringVal(), b.StringVal()
		if sa < sb {
			return -1
		}
		if sa > sb {
			return 1
		}
		return 0
	case types.TypeBoolean:
		ba, bb := a.BoolVal(), b.BoolVal()
		if ba == bb {
			return 0
		}
		if !ba {
			return -1
		}
		return 1
	}
	return 0
}
