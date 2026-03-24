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

// Predicate represents a node in the WHERE clause tree.
type Predicate struct {
	// If IsLogic is true, this is an AND/OR node containing Children.
	IsLogic  bool
	LogicOp  string // "AND" or "OR"
	Children []Predicate

	// Otherwise, it's a field comparison node.
	Field string
	Op    Op
	Value types.Value
	List  []types.Value
}

// sortSpec describes one level of sorting.
type sortSpec struct {
	field string
	dir   SortDir
}

// JoinSpec defines how to eager-load related documents.
type JoinSpec struct {
	As         string // Output field name
	Relation   string // "one_to_one", "one_to_many"
	TargetCol  string 
	LocalKey   string // Field in this collection
	ForeignKey string // Field in target collection
}

// Query is an immutable query specification built with the fluent API.
type Query struct {
	root  Predicate
	sorts []sortSpec
	joins []JoinSpec
	limit int
	skip  int
}

// NewQuery creates an empty query that matches all documents.
// The root predicate defaults to an AND node.
func NewQuery() *Query {
	return &Query{
		root:  Predicate{IsLogic: true, LogicOp: "AND"},
		limit: -1,
	}
}

// Where adds a field predicate to the root AND node.
func (q *Query) Where(field string, op Op, value types.Value) *Query {
	cp := q.clone()
	cp.root.Children = append(cp.root.Children, Predicate{
		Field: field,
		Op:    op,
		Value: value,
	})
	return cp
}

// WhereNode replaces the root with a specific Predicate tree.
func (q *Query) WhereNode(root Predicate) *Query {
	cp := q.clone()
	cp.root = root
	return cp
}

// OrderBy adds a sort key.
func (q *Query) OrderBy(field string, dir SortDir) *Query {
	cp := q.clone()
	cp.sorts = append(cp.sorts, sortSpec{field: field, dir: dir})
	return cp
}

// Include adds an eager-loading join specification.
func (q *Query) Include(join JoinSpec) *Query {
	cp := q.clone()
	cp.joins = append(cp.joins, join)
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
	// deeply copy the predicate tree
	cp.root = clonePredicate(q.root)
	cp.sorts = append([]sortSpec(nil), q.sorts...)
	cp.joins = append([]JoinSpec(nil), q.joins...)
	return &cp
}

func clonePredicate(p Predicate) Predicate {
	cp := p
	if len(p.Children) > 0 {
		cp.Children = make([]Predicate, len(p.Children))
		for i, c := range p.Children {
			cp.Children[i] = clonePredicate(c)
		}
	}
	if len(p.List) > 0 {
		cp.List = append([]types.Value(nil), p.List...)
	}
	return cp
}

// Joins returns the active Join specifications.
func (q *Query) Joins() []JoinSpec {
	return q.joins
}

// matches returns true if doc satisfies the root predicate tree.
func (q *Query) matches(doc *types.Document) bool {
	if !q.root.IsLogic || len(q.root.Children) > 0 {
		return evalPredicate(doc, q.root)
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

// --- predicate evaluation ----------------------------------------------------

// evalPredicate applies a predicate tree to a document.
func evalPredicate(doc *types.Document, p Predicate) bool {
	if p.IsLogic {
		if p.LogicOp == "OR" {
			if len(p.Children) == 0 {
				return true
			}
			for _, c := range p.Children {
				if evalPredicate(doc, c) {
					return true
				}
			}
			return false
		}
		// Default to AND
		for _, c := range p.Children {
			if !evalPredicate(doc, c) {
				return false
			}
		}
		return true
	}

	var v types.Value
	if p.Field == "_id" {
		v = types.String(doc.ID)
	} else {
		v = doc.Get(p.Field)
	}

	switch p.Op {
	case Eq:
		return types.Equal(v, p.Value)
	case Ne:
		return !types.Equal(v, p.Value)
	case Gt:
		return compareValues(v, p.Value) > 0
	case Gte:
		return compareValues(v, p.Value) >= 0
	case Lt:
		return compareValues(v, p.Value) < 0
	case Lte:
		return compareValues(v, p.Value) <= 0
	case Contains:
		return evalContains(v, p.Value)
	case StartsWith:
		if v.Type() == types.TypeString && p.Value.Type() == types.TypeString {
			return strings.HasPrefix(v.StringVal(), p.Value.StringVal())
		}
		return false
	case In:
		for _, item := range p.List {
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
