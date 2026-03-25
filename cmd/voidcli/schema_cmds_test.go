package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationFilesStripsVoidPrismaSuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260325122255_add_posts.void.prisma")
	schema := `datasource db {
  provider = "voiddb"
  url      = env("VOID_URL")
}

generator client {
  provider = "voiddb-client-js"
  output   = "./generated"
}

model User {
  id String @id @map("_id")

  @@database("app")
  @@map("users")
}
`
	if err := os.WriteFile(path, []byte(schema), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	files, err := loadMigrationFiles(dir)
	if err != nil {
		t.Fatalf("loadMigrationFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 migration file, got %d", len(files))
	}
	if files[0].ID != "20260325122255_add_posts" {
		t.Fatalf("expected migration ID without .void suffix, got %q", files[0].ID)
	}
}
