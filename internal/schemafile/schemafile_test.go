package schemafile

import (
	"strings"
	"testing"

	"github.com/voiddb/void/internal/engine"
)

func TestParseRenderRoundTrip(t *testing.T) {
	src := `
datasource db {
  provider = "voiddb"
  url      = env("VOID_URL")
}

generator client {
  provider = "voiddb-client-js"
  output   = "./generated"
}

model User {
  id         String   @id @default(uuid()) @map("_id")
  email      String   @unique
  profile    Json?
  tags       String[]
  updatedAt  DateTime @updatedAt @map("updated_at")
  @@index([email], name: "user_email_idx")
  @@database("app")
  @@map("users")
}
`

	project, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(project.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(project.Models))
	}

	schema := project.Models[0].Schema
	if schema.Database != "app" {
		t.Fatalf("expected database app, got %q", schema.Database)
	}
	if schema.Collection != "users" {
		t.Fatalf("expected collection users, got %q", schema.Collection)
	}
	if len(schema.Indexes) != 1 || schema.Indexes[0].Name != "user_email_idx" {
		t.Fatalf("expected named index user_email_idx, got %#v", schema.Indexes)
	}

	var idField engine.SchemaField
	for _, field := range schema.Fields {
		if field.Name == "id" {
			idField = field
			break
		}
	}
	if !idField.IsID {
		t.Fatalf("expected id field to be primary key")
	}
	if idField.MappedName != "_id" {
		t.Fatalf("expected id field to map to _id, got %q", idField.MappedName)
	}
	if idField.DefaultExpr == nil || *idField.DefaultExpr != "uuid()" {
		t.Fatalf("expected uuid default, got %#v", idField.DefaultExpr)
	}

	rendered := Render(project)
	for _, needle := range []string{
		`model User {`,
		`id String @id @default(uuid()) @map("_id")`,
		`email String @unique`,
		`@@index([email], name: "user_email_idx")`,
		`@@database("app")`,
		`@@map("users")`,
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("rendered schema missing %q:\n%s", needle, rendered)
		}
	}

	project2, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(rendered) error = %v", err)
	}
	plan := Diff(project, project2, true)
	if plan.HasChanges() {
		t.Fatalf("expected round-trip diff to be empty, got %#v", plan.Operations)
	}
}

func TestDiffCreatesDatabaseCollectionAndSchema(t *testing.T) {
	desired := &Project{
		Models: []Model{
			{
				Name: "User",
				Schema: &engine.Schema{
					Database:   "app",
					Collection: "users",
					Model:      "User",
					Fields: []engine.SchemaField{
						{Name: "id", Type: engine.TypeString, Required: true, IsID: true, MappedName: "_id", PrismaType: "String"},
						{Name: "email", Type: engine.TypeString, Required: true, PrismaType: "String", Unique: true},
					},
				},
			},
		},
	}

	plan := Diff(&Project{}, desired, false)
	if len(plan.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(plan.Operations))
	}
	if plan.Operations[0].Type != OpCreateDatabase {
		t.Fatalf("expected first op create database, got %s", plan.Operations[0].Type)
	}
	if plan.Operations[1].Type != OpCreateCollection {
		t.Fatalf("expected second op create collection, got %s", plan.Operations[1].Type)
	}
	if plan.Operations[2].Type != OpSetSchema {
		t.Fatalf("expected third op set schema, got %s", plan.Operations[2].Type)
	}
}

func TestDiffForceDropScopesToExplicitDatabases(t *testing.T) {
	current := &Project{
		Models: []Model{
			{
				Name: "Users",
				Schema: &engine.Schema{
					Database:   "lowkey",
					Collection: "users",
					Model:      "Users",
					Fields: []engine.SchemaField{
						{Name: "id", Type: engine.TypeString, Required: true, IsID: true, MappedName: "_id", PrismaType: "String"},
					},
				},
			},
			{
				Name: "Posts",
				Schema: &engine.Schema{
					Database:   "other",
					Collection: "posts",
					Model:      "Posts",
					Fields: []engine.SchemaField{
						{Name: "id", Type: engine.TypeString, Required: true, IsID: true, MappedName: "_id", PrismaType: "String"},
					},
				},
			},
		},
	}
	desired := &Project{
		Models: []Model{
			{
				Name: "Users",
				Schema: &engine.Schema{
					Database:   "lowkey",
					Collection: "users",
					Model:      "Users",
					Fields: []engine.SchemaField{
						{Name: "id", Type: engine.TypeString, Required: true, IsID: true, MappedName: "_id", PrismaType: "String"},
					},
				},
			},
		},
	}

	plan := Diff(current, desired, true)
	for _, op := range plan.Operations {
		if op.Database != "" && op.Database != "lowkey" {
			t.Fatalf("expected force drop to stay within explicit databases, got %#v", op)
		}
		if op.Type == OpDeleteDatabase {
			t.Fatalf("expected no delete_database ops, got %#v", op)
		}
	}
}

func TestParseAndRenderBlobField(t *testing.T) {
	src := `
model Asset {
  id   String @id @default(uuid()) @map("_id")
  file Blob
  @@database("app")
  @@map("assets")
}
`

	project, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(project.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(project.Models))
	}

	field := project.Models[0].Schema.Fields[1]
	if field.Type != engine.TypeBlob {
		t.Fatalf("expected blob field type, got %q", field.Type)
	}

	rendered := Render(project)
	if !strings.Contains(rendered, "file Blob") {
		t.Fatalf("expected rendered blob field, got:\n%s", rendered)
	}
}
