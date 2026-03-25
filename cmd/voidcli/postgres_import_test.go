package main

import (
	"database/sql"
	"encoding/json"
	"testing"
)

func TestBuildImportedTableSinglePrimaryKey(t *testing.T) {
	table := buildImportedTable(
		"app",
		"users",
		[]postgresColumn{
			{Name: "user_id", DataType: "bigint", UDTName: "int8", Nullable: false, Default: sql.NullString{String: "nextval('users_user_id_seq'::regclass)", Valid: true}},
			{Name: "email", DataType: "text", UDTName: "text", Nullable: false},
		},
		[]postgresConstraint{
			{Name: "users_pkey", Type: "PRIMARY KEY", Columns: []string{"user_id"}},
			{Name: "users_email_key", Type: "UNIQUE", Columns: []string{"email"}},
		},
	)

	if table.SinglePK != "user_id" {
		t.Fatalf("expected single pk user_id, got %q", table.SinglePK)
	}
	if len(table.CompositePK) != 0 {
		t.Fatalf("expected no composite pk, got %#v", table.CompositePK)
	}

	foundID := false
	for _, field := range table.Schema.Fields {
		if field.Name != "user_id" {
			continue
		}
		foundID = true
		if !field.IsID {
			t.Fatalf("expected user_id field to become id field")
		}
		if field.MappedName != "_id" {
			t.Fatalf("expected user_id to map to _id, got %q", field.MappedName)
		}
		if field.PrismaType != "BigInt" {
			t.Fatalf("expected BigInt prisma type, got %q", field.PrismaType)
		}
	}
	if !foundID {
		t.Fatalf("expected mapped id field to exist")
	}

	if len(table.Schema.Indexes) != 1 || table.Schema.Indexes[0].Name != "users_email_key" {
		t.Fatalf("expected only unique email index, got %#v", table.Schema.Indexes)
	}
}

func TestBuildImportedTableCompositePrimaryKey(t *testing.T) {
	table := buildImportedTable(
		"app",
		"memberships",
		[]postgresColumn{
			{Name: "org_id", DataType: "uuid", UDTName: "uuid", Nullable: false},
			{Name: "user_id", DataType: "uuid", UDTName: "uuid", Nullable: false},
		},
		[]postgresConstraint{
			{Name: "memberships_pkey", Type: "PRIMARY KEY", Columns: []string{"org_id", "user_id"}},
		},
	)

	if table.SinglePK != "" {
		t.Fatalf("expected no single pk, got %q", table.SinglePK)
	}
	if len(table.CompositePK) != 2 {
		t.Fatalf("expected composite pk, got %#v", table.CompositePK)
	}
	if table.Schema.Fields[0].Name != "_id" || !table.Schema.Fields[0].IsID {
		t.Fatalf("expected synthetic _id field first, got %#v", table.Schema.Fields[0])
	}
	if len(table.Schema.Indexes) != 1 || !table.Schema.Indexes[0].Primary {
		t.Fatalf("expected primary composite index, got %#v", table.Schema.Indexes)
	}
}

func TestDeriveImportedRowID(t *testing.T) {
	table := importedTable{
		Name:        "memberships",
		CompositePK: []string{"org_id", "user_id"},
	}
	row := map[string]interface{}{
		"org_id":  json.Number("10"),
		"user_id": json.Number("42"),
	}
	got := deriveImportedRowID(table, row, 7)
	want := "org_id=10|user_id=42"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
