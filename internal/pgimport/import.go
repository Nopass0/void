package pgimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/engine/types"
)

// Options configures a PostgreSQL -> VoidDB import job.
type Options struct {
	SourceURL      string
	SourceSchema   string
	TargetDatabase string
	DropExisting   bool
}

// TableResult reports the imported row count for one table.
type TableResult struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

// Result reports the outcome of an import job.
type Result struct {
	Database   string        `json:"database"`
	Source     string        `json:"source"`
	Schema     string        `json:"schema"`
	Tables     []TableResult `json:"tables"`
	TotalRows  int           `json:"total_rows"`
	TotalTable int           `json:"total_tables"`
}

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
}

// ImportURL imports one PostgreSQL schema into a VoidDB database.
func ImportURL(ctx context.Context, store *engine.Store, opts Options) (*Result, error) {
	sourceSchema := strings.TrimSpace(opts.SourceSchema)
	if sourceSchema == "" {
		sourceSchema = "public"
	}
	targetDB := strings.TrimSpace(opts.TargetDatabase)
	if targetDB == "" {
		targetDB = defaultImportedDatabaseName(opts.SourceURL)
	}

	src, err := sql.Open("pgx", opts.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	if err := src.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping source: %w", err)
	}

	tables, err := introspectPostgresTables(ctx, src, sourceSchema, targetDB)
	if err != nil {
		return nil, fmt.Errorf("introspection failed: %w", err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no tables found in schema %q", sourceSchema)
	}

	if opts.DropExisting {
		_ = store.DropDatabase(targetDB)
	}
	if _, err := store.CreateDatabase(targetDB); err != nil {
		return nil, fmt.Errorf("create target database: %w", err)
	}

	result := &Result{
		Database: targetDB,
		Source:   redactSourceURL(opts.SourceURL),
		Schema:   sourceSchema,
	}

	for _, table := range tables {
		_ = store.DropCollection(targetDB, table.Name)
		col, err := store.CreateCollection(targetDB, table.Name)
		if err != nil {
			return nil, fmt.Errorf("create collection %s/%s: %w", targetDB, table.Name, err)
		}
		if err := col.SetSchema(table.Schema); err != nil {
			return nil, fmt.Errorf("set schema for %s/%s: %w", targetDB, table.Name, err)
		}

		rows, err := importRows(ctx, src, col, sourceSchema, table)
		if err != nil {
			return nil, err
		}
		result.Tables = append(result.Tables, TableResult{Name: table.Name, Rows: rows})
		result.TotalRows += rows
	}

	result.TotalTable = len(result.Tables)
	return result, nil
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
	return columns, rows.Err()
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
	case "date", "timestamp without time zone", "timestamp with time zone", "time without time zone", "time with time zone":
		return engine.TypeDateTime, "DateTime"
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
	case "date", "timestamp", "timestamptz", "time", "timetz":
		return engine.TypeDateTime, "DateTime"
	case "bytea":
		return engine.TypeString, "Bytes"
	}

	return engine.TypeString, "String"
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
	case strings.Contains(lower, "gen_random_uuid()"), strings.Contains(lower, "uuid_generate_v4()"):
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

func importRows(ctx context.Context, src *sql.DB, col *engine.Collection, sourceSchema string, table importedTable) (int, error) {
	query := fmt.Sprintf(
		`SELECT row_to_json(t)::text FROM (SELECT * FROM %s.%s) AS t`,
		quoteIdent(sourceSchema),
		quoteIdent(table.Name),
	)
	rows, err := src.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("query %s: %w", table.Name, err)
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
		doc := &types.Document{
			ID:     deriveImportedRowID(table, row, count+1),
			Fields: make(map[string]types.Value, len(row)),
		}
		for key, value := range row {
			doc.Fields[key] = jsonToValue(value)
		}
		if _, err := col.Insert(doc); err != nil {
			return count, fmt.Errorf("insert %s row %d: %w", table.Name, count+1, err)
		}
		count++
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

func redactSourceURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "postgresql://<redacted>"
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			username = "user"
		}
		parsed.User = url.UserPassword(username, "<redacted>")
	}
	return parsed.String()
}

func defaultModelName(database, collection string) string {
	base := toPascal(collection)
	if base == "" {
		base = "Model"
	}
	if database == "" || database == "default" {
		return base
	}
	return toPascal(database) + base
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

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func jsonToValue(v interface{}) types.Value {
	switch value := v.(type) {
	case nil:
		return types.Null()
	case string:
		return types.String(value)
	case json.Number:
		n, err := value.Float64()
		if err != nil {
			return types.String(value.String())
		}
		return types.Number(n)
	case float64:
		return types.Number(value)
	case bool:
		return types.Boolean(value)
	case []interface{}:
		items := make([]types.Value, len(value))
		for i, item := range value {
			items[i] = jsonToValue(item)
		}
		return types.Array(items)
	case map[string]interface{}:
		obj := make(map[string]types.Value, len(value))
		for k, item := range value {
			obj[k] = jsonToValue(item)
		}
		return types.Object(obj)
	default:
		return types.String(fmt.Sprint(value))
	}
}
