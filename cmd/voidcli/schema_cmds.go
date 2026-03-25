package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/schemafile"
)

const (
	defaultSchemaPath       = "void.prisma"
	defaultMigrationDir     = "void/migrations"
	internalMetaDatabase    = "__void"
	internalMetaCollection  = "schema_migrations"
)

type migrationRecord struct {
	ID         string   `json:"migration_id"`
	Name       string   `json:"name"`
	Checksum   string   `json:"checksum"`
	AppliedAt  string   `json:"applied_at"`
	SourcePath string   `json:"source_path,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Schema     string   `json:"schema"`
}

type migrationFile struct {
	ID       string
	Path     string
	Schema   string
	Checksum string
	Project  *schemafile.Project
}

func cmdSchemaPull(outPath string) {
	project, err := pullCurrentProject()
	if err != nil {
		fatalf("schema pull: %v", err)
	}
	rendered := schemafile.Render(project)
	if outPath == "" {
		fmt.Println(rendered)
		return
	}
	if err := os.WriteFile(outPath, []byte(rendered), 0644); err != nil {
		fatalf("schema pull: write %s: %v", outPath, err)
	}
	fmt.Printf("Schema written to %s\n", outPath)
}

func cmdSchemaPush(path string, dryRun, forceDrop bool) {
	project, _, err := readSchemaProject(path)
	if err != nil {
		fatalf("schema push: %v", err)
	}
	current, err := pullCurrentProject()
	if err != nil {
		fatalf("schema push: %v", err)
	}
	plan := schemafile.Diff(current, project, forceDrop)
	printPlan(plan)
	if dryRun || !plan.HasChanges() {
		return
	}
	if err := applyPlan(plan); err != nil {
		fatalf("schema push: %v", err)
	}
	fmt.Println("Schema applied.")
}

func cmdMigrateDev(path, dir, name string, forceDrop bool) {
	if strings.TrimSpace(name) == "" {
		fatalf("migrate dev: --name is required")
	}
	project, schemaText, err := readSchemaProject(path)
	if err != nil {
		fatalf("migrate dev: %v", err)
	}
	current, err := pullCurrentProject()
	if err != nil {
		fatalf("migrate dev: %v", err)
	}
	plan := schemafile.Diff(current, project, forceDrop)
	printPlan(plan)
	if !plan.HasChanges() {
		fmt.Println("No schema changes detected.")
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		fatalf("migrate dev: mkdir %s: %v", dir, err)
	}
	migrationID := time.Now().UTC().Format("20060102150405") + "_" + sanitizeMigrationName(name)
	migrationPath := filepath.Join(dir, migrationID+".void.prisma")
	if err := os.WriteFile(migrationPath, []byte(schemaText), 0644); err != nil {
		fatalf("migrate dev: write migration: %v", err)
	}
	if err := applyPlan(plan); err != nil {
		fatalf("migrate dev: %v", err)
	}
	record := migrationRecord{
		ID:         migrationID,
		Name:       name,
		Checksum:   checksumString(schemaText),
		AppliedAt:  time.Now().UTC().Format(time.RFC3339),
		SourcePath: migrationPath,
		Operations: planSummaries(plan),
		Schema:     schemaText,
	}
	if err := recordMigration(record); err != nil {
		fatalf("migrate dev: record migration: %v", err)
	}
	fmt.Printf("Migration created and applied: %s\n", migrationPath)
}

func cmdMigrateDeploy(dir string, forceDrop bool) {
	files, err := loadMigrationFiles(dir)
	if err != nil {
		fatalf("migrate deploy: %v", err)
	}
	if len(files) == 0 {
		fmt.Println("No migration files found.")
		return
	}

	applied, err := listAppliedMigrations()
	if err != nil {
		fatalf("migrate deploy: %v", err)
	}
	appliedSet := make(map[string]migrationRecord, len(applied))
	for _, record := range applied {
		appliedSet[record.ID] = record
	}

	appliedAny := false
	for _, file := range files {
		if record, ok := appliedSet[file.ID]; ok {
			if record.Checksum != file.Checksum {
				fatalf("migrate deploy: checksum mismatch for %s", file.ID)
			}
			continue
		}

		current, err := pullCurrentProject()
		if err != nil {
			fatalf("migrate deploy: %v", err)
		}
		plan := schemafile.Diff(current, file.Project, forceDrop)
		printPlan(plan)
		if plan.HasChanges() {
			if err := applyPlan(plan); err != nil {
				fatalf("migrate deploy: apply %s: %v", file.ID, err)
			}
		}
		if err := recordMigration(migrationRecord{
			ID:         file.ID,
			Name:       file.ID,
			Checksum:   file.Checksum,
			AppliedAt:  time.Now().UTC().Format(time.RFC3339),
			SourcePath: file.Path,
			Operations: planSummaries(plan),
			Schema:     file.Schema,
		}); err != nil {
			fatalf("migrate deploy: record %s: %v", file.ID, err)
		}
		fmt.Printf("Applied migration %s\n", file.ID)
		appliedAny = true
	}
	if !appliedAny {
		fmt.Println("Database is already up to date.")
	}
}

func cmdMigrateStatus(dir string) {
	files, err := loadMigrationFiles(dir)
	if err != nil {
		fatalf("migrate status: %v", err)
	}
	applied, err := listAppliedMigrations()
	if err != nil {
		fatalf("migrate status: %v", err)
	}
	appliedSet := make(map[string]migrationRecord, len(applied))
	for _, record := range applied {
		appliedSet[record.ID] = record
	}

	if len(files) == 0 && len(applied) == 0 {
		fmt.Println("No migrations found.")
		return
	}

	for _, file := range files {
		record, ok := appliedSet[file.ID]
		switch {
		case !ok:
			fmt.Printf("PENDING  %s\n", file.ID)
		case record.Checksum != file.Checksum:
			fmt.Printf("DRIFT    %s\n", file.ID)
		default:
			fmt.Printf("APPLIED  %s  (%s)\n", file.ID, record.AppliedAt)
		}
	}

	localSet := make(map[string]struct{}, len(files))
	for _, file := range files {
		localSet[file.ID] = struct{}{}
	}
	for _, record := range applied {
		if _, ok := localSet[record.ID]; !ok {
			fmt.Printf("REMOTE   %s  (%s)\n", record.ID, record.AppliedAt)
		}
	}
}

func pullCurrentProject() (*schemafile.Project, error) {
	dbs, err := listDatabasesRaw()
	if err != nil {
		return nil, err
	}

	project := &schemafile.Project{}
	usedNames := make(map[string]int)
	for _, dbName := range dbs {
		if dbName == internalMetaDatabase {
			continue
		}
		cols, err := listCollectionsRaw(dbName)
		if err != nil {
			return nil, err
		}
		for _, colName := range cols {
			var schema engine.Schema
			if err := requestJSON("GET", "/v1/databases/"+pathEscape(dbName)+"/"+pathEscape(colName)+"/schema", nil, &schema); err != nil {
				return nil, err
			}
			schema.Database = dbName
			schema.Collection = colName
			if schema.Model == "" {
				schema.Model = uniqueModelName(defaultModelName(dbName, colName), usedNames)
			} else {
				schema.Model = uniqueModelName(schema.Model, usedNames)
			}
			project.Models = append(project.Models, schemafile.Model{
				Name:   schema.Model,
				Schema: &schema,
			})
		}
	}
	return project, nil
}

func applyPlan(plan schemafile.Plan) error {
	for _, op := range plan.Operations {
		switch op.Type {
		case schemafile.OpCreateDatabase:
			_, _, err := request("POST", "/v1/databases", map[string]string{"name": op.Database})
			if err != nil {
				return err
			}
		case schemafile.OpDeleteDatabase:
			_, _, err := request("DELETE", "/v1/databases/"+pathEscape(op.Database), nil)
			if err != nil {
				return err
			}
		case schemafile.OpCreateCollection:
			_, _, err := request("POST", "/v1/databases/"+pathEscape(op.Database)+"/collections", map[string]string{"name": op.Collection})
			if err != nil {
				return err
			}
		case schemafile.OpDeleteCollection:
			_, _, err := request("DELETE", "/v1/databases/"+pathEscape(op.Database)+"/collections/"+pathEscape(op.Collection), nil)
			if err != nil {
				return err
			}
		case schemafile.OpSetSchema:
			if _, err := mustRequest("PUT", "/v1/databases/"+pathEscape(op.Database)+"/"+pathEscape(op.Collection)+"/schema", op.Schema); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureMigrationCollection() error {
	_, _, _ = request("POST", "/v1/databases", map[string]string{"name": internalMetaDatabase})
	_, _, _ = request("POST", "/v1/databases/"+pathEscape(internalMetaDatabase)+"/collections", map[string]string{"name": internalMetaCollection})
	schema := &engine.Schema{
		Database:   internalMetaDatabase,
		Collection: internalMetaCollection,
		Model:      "SchemaMigration",
		Fields: []engine.SchemaField{
			{
				Name:        "_id",
				Type:        engine.TypeString,
				Required:    true,
				IsID:        true,
				PrismaType:  "String",
			},
			{Name: "migration_id", Type: engine.TypeString, Required: true, PrismaType: "String", Unique: true},
			{Name: "name", Type: engine.TypeString, PrismaType: "String"},
			{Name: "checksum", Type: engine.TypeString, Required: true, PrismaType: "String"},
			{Name: "applied_at", Type: engine.TypeDateTime, Required: true, PrismaType: "DateTime"},
			{Name: "source_path", Type: engine.TypeString, PrismaType: "String"},
			{Name: "operations", Type: engine.TypeArray, PrismaType: "String", List: true},
			{Name: "schema", Type: engine.TypeString, Required: true, PrismaType: "String"},
		},
		Indexes: []engine.SchemaIndex{
			{Name: "migration_id_unique", Fields: []string{"migration_id"}, Unique: true},
		},
	}
	_, err := mustRequest("PUT", "/v1/databases/"+pathEscape(internalMetaDatabase)+"/"+pathEscape(internalMetaCollection)+"/schema", schema)
	return err
}

func listAppliedMigrations() ([]migrationRecord, error) {
	dbs, err := listDatabasesRaw()
	if err != nil {
		return nil, err
	}
	foundDB := false
	for _, db := range dbs {
		if db == internalMetaDatabase {
			foundDB = true
			break
		}
	}
	if !foundDB {
		return nil, nil
	}
	cols, err := listCollectionsRaw(internalMetaDatabase)
	if err != nil {
		return nil, err
	}
	foundCollection := false
	for _, col := range cols {
		if col == internalMetaCollection {
			foundCollection = true
			break
		}
	}
	if !foundCollection {
		return nil, nil
	}

	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := requestJSON(
		"POST",
		"/v1/databases/"+pathEscape(internalMetaDatabase)+"/"+pathEscape(internalMetaCollection)+"/query",
		map[string]interface{}{
			"order_by": []map[string]string{{"field": "applied_at", "dir": "asc"}},
			"limit":    10000,
		},
		&resp,
	); err != nil {
		return nil, err
	}

	records := make([]migrationRecord, 0, len(resp.Results))
	for _, doc := range resp.Results {
		record := migrationRecord{
			ID:         stringValue(doc["_id"]),
			Name:       stringValue(doc["name"]),
			Checksum:   stringValue(doc["checksum"]),
			AppliedAt:  stringValue(doc["applied_at"]),
			SourcePath: stringValue(doc["source_path"]),
			Schema:     stringValue(doc["schema"]),
		}
		if record.ID == "" {
			record.ID = stringValue(doc["migration_id"])
		}
		switch value := doc["operations"].(type) {
		case []interface{}:
			for _, item := range value {
				record.Operations = append(record.Operations, stringValue(item))
			}
		}
		records = append(records, record)
	}
	return records, nil
}

func recordMigration(record migrationRecord) error {
	if err := ensureMigrationCollection(); err != nil {
		return err
	}
	body := map[string]interface{}{
		"_id":          record.ID,
		"migration_id": record.ID,
		"name":         record.Name,
		"checksum":     record.Checksum,
		"applied_at":   record.AppliedAt,
		"source_path":  record.SourcePath,
		"operations":   record.Operations,
		"schema":       record.Schema,
	}
	_, err := mustRequest("POST", "/v1/databases/"+pathEscape(internalMetaDatabase)+"/"+pathEscape(internalMetaCollection), body)
	return err
}

func loadMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".prisma") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), ".prisma")
		id = strings.TrimSuffix(id, ".void")
		project, schemaText, err := readSchemaProject(path)
		if err != nil {
			return nil, err
		}
		files = append(files, migrationFile{
			ID:       id,
			Path:     path,
			Schema:   schemaText,
			Checksum: checksumString(schemaText),
			Project:  project,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ID < files[j].ID
	})
	return files, nil
}

func readSchemaProject(path string) (*schemafile.Project, string, error) {
	if path == "" {
		path = defaultSchemaPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	text := string(data)
	project, err := schemafile.Parse(text)
	if err != nil {
		return nil, "", err
	}
	return project, text, nil
}

func printPlan(plan schemafile.Plan) {
	if !plan.HasChanges() {
		fmt.Println("No schema changes.")
		return
	}
	for _, op := range plan.Operations {
		fmt.Printf("- %s\n", op.Summary)
	}
}

func planSummaries(plan schemafile.Plan) []string {
	out := make([]string, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		out = append(out, op.Summary)
	}
	return out
}

func listDatabasesRaw() ([]string, error) {
	var resp struct {
		Databases []string `json:"databases"`
	}
	if err := requestJSON("GET", "/v1/databases", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Databases, nil
}

func listCollectionsRaw(dbName string) ([]string, error) {
	var resp struct {
		Collections []string `json:"collections"`
	}
	if err := requestJSON("GET", "/v1/databases/"+pathEscape(dbName)+"/collections", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Collections, nil
}

func requestJSON(method, path string, body interface{}, out interface{}) error {
	data, status, err := request(method, path, body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("server returned %d: %s", status, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func mustRequest(method, path string, body interface{}) ([]byte, error) {
	data, status, err := request(method, path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("server returned %d: %s", status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func checksumString(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func sanitizeMigrationName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "migration"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "migration"
	}
	return out
}

func defaultModelName(dbName, colName string) string {
	base := toPascal(colName)
	if base == "" {
		base = "Model"
	}
	if dbName == "" || dbName == "default" {
		return base
	}
	return toPascal(dbName) + base
}

func uniqueModelName(name string, used map[string]int) string {
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	used[name]++
	return fmt.Sprintf("%s%d", name, used[name])
}

func toPascal(value string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			if upperNext {
				b.WriteString(strings.ToUpper(string(r)))
				upperNext = false
			} else {
				b.WriteRune(r)
			}
		default:
			upperNext = true
		}
	}
	return b.String()
}

func stringValue(v interface{}) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func pathEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", "/", "%2F", " ", "%20", "#", "%23", "?", "%3F")
	return replacer.Replace(value)
}
