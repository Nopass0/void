package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/voiddb/void/internal/engine"
	pgshared "github.com/voiddb/void/internal/pgimport"
)

type postgresColumn struct {
	Name     string
	DataType string
	UDTName  string
	Nullable bool
	Default  sql.NullString
}

type postgresConstraint struct {
	Name    string
	Type    string
	Columns []string
}

type importedTable struct {
	Name        string
	Schema      *engine.Schema
	SinglePK    string
	CompositePK []string
	Columns     []postgresColumn
}

func cmdImportPostgres(sourceURL, targetDB, sourceSchema string, dropExisting bool, progressEvery int) {
	if progressEvery <= 0 {
		progressEvery = 250
	}

	if strings.TrimSpace(sourceSchema) == "" {
		sourceSchema = "public"
	}
	if strings.TrimSpace(targetDB) == "" {
		targetDB = defaultImportedDatabaseName(sourceURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	src, err := sql.Open("pgx", sourceURL)
	if err != nil {
		fatalf("import postgres: open source: %v", err)
	}
	defer src.Close()

	if err := src.PingContext(ctx); err != nil {
		fatalf("import postgres: ping source: %v", err)
	}

	tables, err := introspectPostgresTables(ctx, src, sourceSchema, targetDB)
	if err != nil {
		fatalf("import postgres: introspection failed: %v", err)
	}
	if len(tables) == 0 {
		fatalf("import postgres: no tables found in schema %q", sourceSchema)
	}

	fmt.Printf("Discovered %d PostgreSQL table(s) in schema %s\n", len(tables), sourceSchema)

	if dropExisting {
		_, _, _ = request("DELETE", "/v1/databases/"+pathEscape(targetDB), nil)
	}
	if _, err := mustRequest("POST", "/v1/databases", map[string]string{"name": targetDB}); err != nil {
		fatalf("import postgres: create target database %s: %v", targetDB, err)
	}

	totalRows := 0
	for _, table := range tables {
		fmt.Printf("Preparing %s/%s...\n", targetDB, table.Name)
		_, _, _ = request("DELETE", "/v1/databases/"+pathEscape(targetDB)+"/collections/"+pathEscape(table.Name), nil)
		if _, err := mustRequest(
			"POST",
			"/v1/databases/"+pathEscape(targetDB)+"/collections",
			map[string]string{"name": table.Name},
		); err != nil {
			fatalf("import postgres: create collection %s/%s: %v", targetDB, table.Name, err)
		}
		if _, err := mustRequest(
			"PUT",
			"/v1/databases/"+pathEscape(targetDB)+"/"+pathEscape(table.Name)+"/schema",
			table.Schema,
		); err != nil {
			fatalf("import postgres: set schema for %s/%s: %v", targetDB, table.Name, err)
		}

		imported, err := importPostgresRows(ctx, src, sourceSchema, targetDB, table, progressEvery)
		if err != nil {
			fatalf("import postgres: %v", err)
		}
		fmt.Printf("Imported %d row(s) into %s/%s\n", imported, targetDB, table.Name)
		totalRows += imported
	}

	fmt.Printf("PostgreSQL import complete: %d table(s), %d row(s), target database %s\n", len(tables), totalRows, targetDB)
}

func introspectPostgresTables(ctx context.Context, db *sql.DB, sourceSchema, targetDB string) ([]importedTable, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`, sourceSchema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []importedTable
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		columns, err := loadPostgresColumns(ctx, db, sourceSchema, tableName)
		if err != nil {
			return nil, err
		}
		constraints, err := loadPostgresConstraints(ctx, db, sourceSchema, tableName)
		if err != nil {
			return nil, err
		}
		tables = append(tables, buildImportedTable(targetDB, tableName, columns, constraints))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func loadPostgresColumns(ctx context.Context, db *sql.DB, sourceSchema, tableName string) ([]postgresColumn, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, udt_name, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = $2
		ORDER BY ordinal_position
	`, sourceSchema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []postgresColumn
	for rows.Next() {
		var col postgresColumn
		var nullable string
		if err := rows.Scan(&col.Name, &col.DataType, &col.UDTName, &nullable, &col.Default); err != nil {
			return nil, err
		}
		col.Nullable = strings.EqualFold(nullable, "YES")
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func loadPostgresConstraints(ctx context.Context, db *sql.DB, sourceSchema, tableName string) ([]postgresConstraint, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tc.constraint_name, tc.constraint_type, kcu.column_name, kcu.ordinal_position
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		 AND tc.table_name = kcu.table_name
		WHERE tc.table_schema = $1
		  AND tc.table_name = $2
		  AND tc.constraint_type IN ('PRIMARY KEY', 'UNIQUE')
		ORDER BY tc.constraint_type, tc.constraint_name, kcu.ordinal_position
	`, sourceSchema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexByName := make(map[string]*postgresConstraint)
	var ordered []*postgresConstraint
	for rows.Next() {
		var name, kind, column string
		var ordinal int
		if err := rows.Scan(&name, &kind, &column, &ordinal); err != nil {
			return nil, err
		}
		entry := indexByName[name]
		if entry == nil {
			entry = &postgresConstraint{Name: name, Type: kind}
			indexByName[name] = entry
			ordered = append(ordered, entry)
		}
		entry.Columns = append(entry.Columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]postgresConstraint, 0, len(ordered))
	for _, entry := range ordered {
		out = append(out, *entry)
	}
	return out, nil
}

func buildImportedTable(targetDB, tableName string, columns []postgresColumn, constraints []postgresConstraint) importedTable {
	var singlePK string
	var compositePK []string
	for _, constraint := range constraints {
		if constraint.Type != "PRIMARY KEY" {
			continue
		}
		if len(constraint.Columns) == 1 {
			singlePK = constraint.Columns[0]
		} else if len(constraint.Columns) > 1 {
			compositePK = append([]string(nil), constraint.Columns...)
		}
		break
	}

	fields := make([]engine.SchemaField, 0, len(columns)+1)
	if singlePK == "" {
		fields = append(fields, engine.SchemaField{
			Name:       "_id",
			Type:       engine.TypeString,
			Required:   true,
			IsID:       true,
			PrismaType: "String",
		})
	}

	for _, column := range columns {
		field := mapPostgresColumnToField(column)
		if column.Name == singlePK {
			field.IsID = true
			field.Required = true
			field.MappedName = "_id"
			field.Type = engine.TypeString
		}
		fields = append(fields, field)
	}

	indexes := make([]engine.SchemaIndex, 0, len(constraints))
	for _, constraint := range constraints {
		if len(constraint.Columns) == 0 {
			continue
		}
		if constraint.Type == "PRIMARY KEY" {
			if len(constraint.Columns) <= 1 {
				continue
			}
			indexes = append(indexes, engine.SchemaIndex{
				Name:    constraint.Name,
				Fields:  append([]string(nil), constraint.Columns...),
				Unique:  true,
				Primary: true,
			})
			continue
		}
		indexes = append(indexes, engine.SchemaIndex{
			Name:   constraint.Name,
			Fields: append([]string(nil), constraint.Columns...),
			Unique: true,
		})
	}

	return importedTable{
		Name: tableName,
		Schema: (&engine.Schema{
			Database:   targetDB,
			Collection: tableName,
			Model:      defaultModelName(targetDB, tableName),
			Fields:     fields,
			Indexes:    indexes,
		}).Normalize(),
		SinglePK:    singlePK,
		CompositePK: compositePK,
		Columns:     append([]postgresColumn(nil), columns...),
	}
}

func mapPostgresColumnToField(column postgresColumn) engine.SchemaField {
	fieldType, prismaType, list := postgresTypeMapping(column)
	field := engine.SchemaField{
		Name:       column.Name,
		Type:       fieldType,
		Required:   !column.Nullable,
		PrismaType: prismaType,
		List:       list,
	}
	if expr := postgresDefaultExpr(column); expr != nil {
		field.DefaultExpr = expr
		field.Default = expr
	}
	return field
}

func postgresTypeMapping(column postgresColumn) (engine.FieldType, string, bool) {
	if strings.EqualFold(column.DataType, "ARRAY") || strings.HasPrefix(column.UDTName, "_") {
		elementType := strings.TrimPrefix(column.UDTName, "_")
		if elementType == "" || strings.EqualFold(column.DataType, "ARRAY") {
			return engine.TypeArray, "Json", true
		}
		_, prismaType := postgresScalarType(elementType, elementType)
		return engine.TypeArray, prismaType, true
	}

	fieldType, prismaType := postgresScalarType(column.DataType, column.UDTName)
	return fieldType, prismaType, false
}

func postgresScalarType(dataType, udtName string) (engine.FieldType, string) {
	return pgshared.MapScalarType(dataType, udtName)
}

func postgresDefaultExpr(column postgresColumn) *string {
	if !column.Default.Valid {
		return nil
	}
	raw := strings.TrimSpace(column.Default.String)
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "nextval("):
		return nil
	case strings.Contains(lower, "gen_random_uuid()"):
		expr := "uuid()"
		return &expr
	case strings.Contains(lower, "uuid_generate_v4()"):
		expr := "uuid()"
		return &expr
	case strings.Contains(lower, "current_timestamp"), strings.Contains(lower, "transaction_timestamp()"), strings.Contains(lower, "now()"):
		expr := "now()"
		return &expr
	}

	trimmed := trimPostgresDefault(raw)
	if trimmed == "" {
		return nil
	}
	if strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "false") {
		expr := strings.ToLower(trimmed)
		return &expr
	}
	if strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'") && len(trimmed) >= 2 {
		value := strings.ReplaceAll(trimmed[1:len(trimmed)-1], "''", "'")
		expr := fmt.Sprintf("%q", value)
		return &expr
	}
	if _, err := json.Number(trimmed).Float64(); err == nil {
		expr := trimmed
		return &expr
	}

	return nil
}

func trimPostgresDefault(value string) string {
	value = strings.TrimSpace(value)
	for strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") && len(value) > 2 {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	if idx := strings.Index(value, "::"); idx != -1 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func importPostgresRows(ctx context.Context, src *sql.DB, sourceSchema, targetDB string, table importedTable, progressEvery int) (int, error) {
	query := fmt.Sprintf(
		`SELECT row_to_json(t)::text FROM (SELECT * FROM %s.%s) AS t`,
		quoteIdent(sourceSchema),
		quoteIdent(table.Name),
	)
	rows, err := src.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return count, err
		}
		row, err := decodeImportedRow(raw)
		if err != nil {
			return count, fmt.Errorf("decode %s row %d: %w", table.Name, count+1, err)
		}
		if err := normalizeImportedRow(table, row); err != nil {
			return count, fmt.Errorf("normalize %s row %d: %w", table.Name, count+1, err)
		}

		docID := deriveImportedRowID(table, row, count+1)
		payload := make(map[string]interface{}, len(row)+1)
		for key, value := range row {
			payload[key] = value
		}
		payload["_id"] = docID
		if table.SinglePK != "" {
			delete(payload, table.SinglePK)
		}

		if _, err := mustRequest(
			"POST",
			"/v1/databases/"+pathEscape(targetDB)+"/"+pathEscape(table.Name),
			payload,
		); err != nil {
			return count, fmt.Errorf("insert %s row %d: %w", table.Name, count+1, err)
		}

		count++
		if progressEvery > 0 && count%progressEvery == 0 {
			fmt.Printf("  %s: imported %d row(s)\n", table.Name, count)
		}
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func decodeImportedRow(raw string) (map[string]interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var row map[string]interface{}
	if err := decoder.Decode(&row); err != nil {
		return nil, err
	}
	return row, nil
}

func normalizeImportedRow(table importedTable, row map[string]interface{}) error {
	for _, column := range table.Columns {
		value, ok := row[column.Name]
		if !ok || value == nil {
			continue
		}
		normalized, err := pgshared.NormalizeColumnValue(column.DataType, column.UDTName, value)
		if err != nil {
			return fmt.Errorf("field %s: %w", column.Name, err)
		}
		row[column.Name] = normalized
	}
	return nil
}

func deriveImportedRowID(table importedTable, row map[string]interface{}, rowNumber int) string {
	if table.SinglePK != "" {
		if value, ok := row[table.SinglePK]; ok && value != nil {
			return importIDString(value)
		}
	}
	if len(table.CompositePK) > 0 {
		parts := make([]string, 0, len(table.CompositePK))
		for _, field := range table.CompositePK {
			parts = append(parts, field+"="+importIDString(row[field]))
		}
		return strings.Join(parts, "|")
	}
	return fmt.Sprintf("%s-%d", table.Name, rowNumber)
}

func importIDString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case json.Number:
		return typed.String()
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(typed)
	}
}

func defaultImportedDatabaseName(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return "postgres_import"
	}
	name := strings.Trim(parsed.Path, "/")
	if name == "" {
		return "postgres_import"
	}
	return strings.ReplaceAll(name, "-", "_")
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
