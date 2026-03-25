package schemafile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/voiddb/void/internal/engine"
)

// Datasource models a Prisma-like datasource block.
type Datasource struct {
	Name     string
	Provider string
	URL      string
}

// Generator models a Prisma-like generator block.
type Generator struct {
	Name     string
	Provider string
	Output   string
}

// Model binds a Prisma-like model block to a VoidDB collection schema.
type Model struct {
	Name   string
	Schema *engine.Schema
}

// Project is the top-level schema file.
type Project struct {
	Datasource *Datasource
	Generator  *Generator
	Models     []Model
}

// OperationType is a planned schema change.
type OperationType string

const (
	OpCreateDatabase   OperationType = "create_database"
	OpDeleteDatabase   OperationType = "delete_database"
	OpCreateCollection OperationType = "create_collection"
	OpDeleteCollection OperationType = "delete_collection"
	OpSetSchema        OperationType = "set_schema"
)

// Operation describes one schema change step.
type Operation struct {
	Type       OperationType
	Database   string
	Collection string
	Schema     *engine.Schema
	Summary    string
}

// Plan is the diff between current and desired state.
type Plan struct {
	Operations []Operation
}

// HasChanges reports whether the plan contains any changes.
func (p Plan) HasChanges() bool { return len(p.Operations) > 0 }

var scalarTypes = map[string]engine.FieldType{
	"String":   engine.TypeString,
	"Int":      engine.TypeNumber,
	"BigInt":   engine.TypeNumber,
	"Float":    engine.TypeNumber,
	"Decimal":  engine.TypeNumber,
	"Boolean":  engine.TypeBoolean,
	"DateTime": engine.TypeDateTime,
	"Json":     engine.TypeObject,
	"Bytes":    engine.TypeString,
}

// Parse parses a minimal Prisma-like schema file.
func Parse(src string) (*Project, error) {
	project := &Project{}

	type state struct {
		kind string
		name string
	}
	var current *state
	var currentModel *Model

	lines := strings.Split(src, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}

		if current == nil {
			switch {
			case strings.HasPrefix(line, "datasource "):
				name, err := parseBlockStart(line, "datasource")
				if err != nil {
					return nil, lineError(i, err)
				}
				current = &state{kind: "datasource", name: name}
				project.Datasource = &Datasource{Name: name}
			case strings.HasPrefix(line, "generator "):
				name, err := parseBlockStart(line, "generator")
				if err != nil {
					return nil, lineError(i, err)
				}
				current = &state{kind: "generator", name: name}
				project.Generator = &Generator{Name: name}
			case strings.HasPrefix(line, "model "):
				name, err := parseBlockStart(line, "model")
				if err != nil {
					return nil, lineError(i, err)
				}
				current = &state{kind: "model", name: name}
				currentModel = &Model{
					Name: name,
					Schema: &engine.Schema{
						Model:  name,
						Fields: []engine.SchemaField{},
					},
				}
			default:
				return nil, lineError(i, fmt.Errorf("unexpected token %q", line))
			}
			continue
		}

		if line == "}" {
			if current.kind == "model" && currentModel != nil {
				if currentModel.Schema.Database == "" {
					currentModel.Schema.Database = "default"
				}
				if currentModel.Schema.Collection == "" {
					currentModel.Schema.Collection = currentModel.Name
				}
				project.Models = append(project.Models, *currentModel)
				currentModel = nil
			}
			current = nil
			continue
		}

		switch current.kind {
		case "datasource":
			key, value, err := parseAssignment(line)
			if err != nil {
				return nil, lineError(i, err)
			}
			switch key {
			case "provider":
				project.Datasource.Provider = strings.Trim(value, "\"")
			case "url":
				project.Datasource.URL = value
			}
		case "generator":
			key, value, err := parseAssignment(line)
			if err != nil {
				return nil, lineError(i, err)
			}
			switch key {
			case "provider":
				project.Generator.Provider = strings.Trim(value, "\"")
			case "output":
				project.Generator.Output = strings.Trim(value, "\"")
			}
		case "model":
			if strings.HasPrefix(line, "@@") {
				if err := parseModelAttribute(currentModel, line); err != nil {
					return nil, lineError(i, err)
				}
				continue
			}
			field, err := parseFieldLine(line)
			if err != nil {
				return nil, lineError(i, err)
			}
			currentModel.Schema.Fields = append(currentModel.Schema.Fields, field)
		}
	}

	if current != nil {
		return nil, fmt.Errorf("unterminated %s block %q", current.kind, current.name)
	}

	if project.Datasource == nil {
		project.Datasource = &Datasource{
			Name:     "db",
			Provider: "voiddb",
			URL:      `env("VOID_URL")`,
		}
	}
	if project.Generator == nil {
		project.Generator = &Generator{
			Name:     "client",
			Provider: "voiddb-client-js",
			Output:   "./generated",
		}
	}

	return project, nil
}

// Render serializes a Project to a Prisma-like schema file.
func Render(project *Project) string {
	if project == nil {
		project = &Project{}
	}
	ds := project.Datasource
	if ds == nil {
		ds = &Datasource{Name: "db", Provider: "voiddb", URL: `env("VOID_URL")`}
	}
	gen := project.Generator
	if gen == nil {
		gen = &Generator{Name: "client", Provider: "voiddb-client-js", Output: "./generated"}
	}

	models := append([]Model(nil), project.Models...)
	sort.Slice(models, func(i, j int) bool {
		leftSchema := models[i].Schema
		rightSchema := models[j].Schema
		leftKey := leftSchema.Database + "/" + leftSchema.Collection
		rightKey := rightSchema.Database + "/" + rightSchema.Collection
		return leftKey < rightKey
	})

	var buf bytes.Buffer
	buf.WriteString("datasource ")
	buf.WriteString(ds.Name)
	buf.WriteString(" {\n")
	buf.WriteString(`  provider = "`)
	buf.WriteString(ds.Provider)
	buf.WriteString("\"\n")
	buf.WriteString("  url      = ")
	buf.WriteString(ds.URL)
	buf.WriteString("\n}\n\n")

	buf.WriteString("generator ")
	buf.WriteString(gen.Name)
	buf.WriteString(" {\n")
	buf.WriteString(`  provider = "`)
	buf.WriteString(gen.Provider)
	buf.WriteString("\"\n")
	if gen.Output != "" {
		buf.WriteString(`  output   = "`)
		buf.WriteString(gen.Output)
		buf.WriteString("\"\n")
	}
	buf.WriteString("}\n")

	for _, model := range models {
		schema := model.Schema.Normalize()
		modelName := model.Name
		if modelName == "" {
			modelName = schema.Model
		}
		if modelName == "" {
			modelName = schema.Collection
		}
		if modelName == "" {
			modelName = "Model"
		}

		buf.WriteString("\nmodel ")
		buf.WriteString(modelName)
		buf.WriteString(" {\n")

		for _, field := range schema.Fields {
			renderFieldLine(&buf, field)
		}
		for _, index := range schema.Indexes {
			renderIndexLine(&buf, index)
		}

		if schema.Database != "" {
			buf.WriteString(`  @@database("`)
			buf.WriteString(schema.Database)
			buf.WriteString("\")\n")
		}
		if schema.Collection != "" {
			buf.WriteString(`  @@map("`)
			buf.WriteString(schema.Collection)
			buf.WriteString("\")\n")
		}
		buf.WriteString("}\n")
	}

	return buf.String()
}

// Diff computes a plan between current and desired schema state.
func Diff(current, desired *Project, forceDrop bool) Plan {
	if current == nil {
		current = &Project{}
	}
	if desired == nil {
		desired = &Project{}
	}

	currentModels := current.modelMap()
	desiredModels := desired.modelMap()
	currentDBs := current.databaseSet()
	desiredDBs := desired.databaseSet()

	dbCreateSet := make(map[string]struct{})
	var ops []Operation

	desiredKeys := sortedKeys(desiredModels)
	for _, key := range desiredKeys {
		model := desiredModels[key]
		schema := model.Schema.Normalize()
		if _, ok := currentDBs[schema.Database]; !ok {
			if _, seen := dbCreateSet[schema.Database]; !seen {
				dbCreateSet[schema.Database] = struct{}{}
				ops = append(ops, Operation{
					Type:     OpCreateDatabase,
					Database: schema.Database,
					Summary:  fmt.Sprintf("create database %s", schema.Database),
				})
			}
		}

		currentModel, ok := currentModels[key]
		if !ok {
			ops = append(ops, Operation{
				Type:       OpCreateCollection,
				Database:   schema.Database,
				Collection: schema.Collection,
				Summary:    fmt.Sprintf("create collection %s/%s", schema.Database, schema.Collection),
			})
			ops = append(ops, Operation{
				Type:       OpSetSchema,
				Database:   schema.Database,
				Collection: schema.Collection,
				Schema:     schema,
				Summary:    fmt.Sprintf("set schema %s/%s", schema.Database, schema.Collection),
			})
			continue
		}

		if !schemasEqual(currentModel.Schema, schema) {
			ops = append(ops, Operation{
				Type:       OpSetSchema,
				Database:   schema.Database,
				Collection: schema.Collection,
				Schema:     schema,
				Summary:    fmt.Sprintf("update schema %s/%s", schema.Database, schema.Collection),
			})
		}
	}

	if forceDrop {
		for _, key := range sortedKeys(currentModels) {
			if _, ok := desiredModels[key]; ok {
				continue
			}
			schema := currentModels[key].Schema.Normalize()
			ops = append(ops, Operation{
				Type:       OpDeleteCollection,
				Database:   schema.Database,
				Collection: schema.Collection,
				Summary:    fmt.Sprintf("drop collection %s/%s", schema.Database, schema.Collection),
			})
		}
		for dbName := range currentDBs {
			if _, ok := desiredDBs[dbName]; ok {
				continue
			}
			ops = append(ops, Operation{
				Type:     OpDeleteDatabase,
				Database: dbName,
				Summary:  fmt.Sprintf("drop database %s", dbName),
			})
		}
	}

	return Plan{Operations: ops}
}

func (p *Project) modelMap() map[string]Model {
	out := make(map[string]Model, len(p.Models))
	for _, model := range p.Models {
		schema := model.Schema.Normalize()
		model.Schema = schema
		out[schema.Database+"/"+schema.Collection] = model
	}
	return out
}

func (p *Project) databaseSet() map[string]struct{} {
	out := make(map[string]struct{})
	for _, model := range p.Models {
		if model.Schema == nil {
			continue
		}
		schema := model.Schema.Normalize()
		if schema.Database != "" {
			out[schema.Database] = struct{}{}
		}
	}
	return out
}

func schemasEqual(left, right *engine.Schema) bool {
	leftJSON := canonicalSchemaJSON(left)
	rightJSON := canonicalSchemaJSON(right)
	return bytes.Equal(leftJSON, rightJSON)
}

func canonicalSchemaJSON(schema *engine.Schema) []byte {
	if schema == nil {
		return nil
	}
	clone := *schema.Normalize()
	clone.Fields = append([]engine.SchemaField(nil), clone.Fields...)
	sort.Slice(clone.Fields, func(i, j int) bool {
		return clone.Fields[i].StorageName() < clone.Fields[j].StorageName()
	})
	clone.Indexes = append([]engine.SchemaIndex(nil), clone.Indexes...)
	sort.Slice(clone.Indexes, func(i, j int) bool {
		left := strings.Join(clone.Indexes[i].Fields, ",") + "|" + clone.Indexes[i].Name
		right := strings.Join(clone.Indexes[j].Fields, ",") + "|" + clone.Indexes[j].Name
		return left < right
	})
	data, _ := json.Marshal(clone)
	return data
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func parseBlockStart(line, kind string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, kind))
	if !strings.HasSuffix(trimmed, "{") {
		return "", fmt.Errorf("%s block must end with {", kind)
	}
	name := strings.TrimSpace(strings.TrimSuffix(trimmed, "{"))
	if name == "" {
		return "", fmt.Errorf("%s name is required", kind)
	}
	return name, nil
}

func parseAssignment(line string) (string, string, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid assignment %q", line)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func parseModelAttribute(model *Model, line string) error {
	switch {
	case strings.HasPrefix(line, "@@database("):
		model.Schema.Database = parseCallStringArg(line, "@@database")
	case strings.HasPrefix(line, "@@map("):
		model.Schema.Collection = parseCallStringArg(line, "@@map")
	case strings.HasPrefix(line, "@@index("):
		index, err := parseIndexAttribute(line, false, false)
		if err != nil {
			return err
		}
		model.Schema.Indexes = append(model.Schema.Indexes, index)
	case strings.HasPrefix(line, "@@unique("):
		index, err := parseIndexAttribute(line, true, false)
		if err != nil {
			return err
		}
		model.Schema.Indexes = append(model.Schema.Indexes, index)
	case strings.HasPrefix(line, "@@id("):
		index, err := parseIndexAttribute(line, true, true)
		if err != nil {
			return err
		}
		model.Schema.Indexes = append(model.Schema.Indexes, index)
	default:
		return fmt.Errorf("unsupported model attribute %q", line)
	}
	return nil
}

func parseFieldLine(line string) (engine.SchemaField, error) {
	name, rest, ok := cutToken(line)
	if !ok {
		return engine.SchemaField{}, fmt.Errorf("invalid field line %q", line)
	}
	typeToken, attrText, ok := cutToken(rest)
	if !ok {
		return engine.SchemaField{}, fmt.Errorf("invalid field line %q", line)
	}

	baseType, optional, list := parseTypeToken(typeToken)
	field := engine.SchemaField{
		Name:       name,
		Required:   !optional,
		List:       list,
		PrismaType: baseType,
		Type:       mapPrismaType(baseType, list),
		Virtual:    !isScalarType(baseType),
	}

	for _, attr := range splitAttributes(attrText) {
		switch {
		case attr == "@id":
			field.IsID = true
			field.Required = true
			field.Type = engine.TypeString
			if field.PrismaType == "" {
				field.PrismaType = "String"
			}
		case attr == "@unique":
			field.Unique = true
		case attr == "@updatedAt":
			field.AutoUpdatedAt = true
			field.Type = engine.TypeDateTime
			if field.PrismaType == "" {
				field.PrismaType = "DateTime"
			}
		case strings.HasPrefix(attr, "@default("):
			expr := parseCallArg(attr, "@default")
			field.DefaultExpr = &expr
			runtime := runtimeDefaultExpr(expr)
			field.Default = &runtime
		case strings.HasPrefix(attr, "@map("):
			field.MappedName = parseCallStringArg(attr, "@map")
		case strings.HasPrefix(attr, "@relation("):
			relation, err := parseRelation(attr, baseType)
			if err != nil {
				return engine.SchemaField{}, err
			}
			field.Relation = relation
			field.Type = engine.TypeRelation
			field.Virtual = true
		}
	}

	if field.IsID && field.MappedName == "" && field.Name != "_id" {
		field.MappedName = "_id"
	}
	if field.IsID && field.PrismaType == "" {
		field.PrismaType = "String"
	}
	if field.AutoUpdatedAt && field.DefaultExpr == nil {
		nowExpr := "now()"
		field.DefaultExpr = &nowExpr
		field.Default = &nowExpr
	}

	return field, nil
}

func parseIndexAttribute(line string, unique, primary bool) (engine.SchemaIndex, error) {
	inside := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, line[:strings.Index(line, "(")]+"("), ")"))
	if inside == "" {
		return engine.SchemaIndex{}, fmt.Errorf("index fields are required")
	}
	parts := splitCSVRespectingBrackets(inside)
	if len(parts) == 0 {
		return engine.SchemaIndex{}, fmt.Errorf("index fields are required")
	}

	fieldsRaw := strings.TrimSpace(parts[0])
	fieldsRaw = strings.TrimPrefix(fieldsRaw, "[")
	fieldsRaw = strings.TrimSuffix(fieldsRaw, "]")
	fields := splitCSVRespectingBrackets(fieldsRaw)
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	index := engine.SchemaIndex{
		Fields:  fields,
		Unique:  unique,
		Primary: primary,
	}
	for _, part := range parts[1:] {
		k, v, err := parseAssignment(strings.ReplaceAll(part, ":", "="))
		if err != nil {
			continue
		}
		if strings.TrimSpace(k) == "name" {
			index.Name = strings.Trim(v, "\"")
		}
	}
	return index, nil
}

func parseRelation(attr, modelName string) (*engine.SchemaRelation, error) {
	inside := parseCallArg(attr, "@relation")
	relation := &engine.SchemaRelation{Model: modelName}
	if inside == "" {
		return relation, nil
	}
	for _, part := range splitCSVRespectingBrackets(inside) {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "\"") {
			relation.Name = strings.Trim(part, "\"")
			continue
		}
		key, value, err := parseAssignment(strings.ReplaceAll(part, ":", "="))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			relation.Name = strings.Trim(value, "\"")
		case "fields":
			relation.Fields = parseListLiteral(value)
		case "references":
			relation.References = parseListLiteral(value)
		case "onDelete":
			relation.OnDelete = strings.Trim(value, "\"")
		case "onUpdate":
			relation.OnUpdate = strings.Trim(value, "\"")
		}
	}
	return relation, nil
}

func parseListLiteral(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := splitCSVRespectingBrackets(value)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "\"")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitAttributes(text string) []string {
	var out []string
	var current strings.Builder
	depth := 0
	inString := false
	for i, ch := range text {
		switch ch {
		case '"':
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString && depth > 0 {
				depth--
			}
		case '@':
			if !inString && depth == 0 && i > 0 {
				part := strings.TrimSpace(current.String())
				if part != "" {
					out = append(out, part)
				}
				current.Reset()
			}
		}
		current.WriteRune(ch)
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		out = append(out, part)
	}
	return out
}

func splitCSVRespectingBrackets(text string) []string {
	var out []string
	var current strings.Builder
	depthParen := 0
	depthBracket := 0
	inString := false
	for _, ch := range text {
		switch ch {
		case '"':
			inString = !inString
		case '(':
			if !inString {
				depthParen++
			}
		case ')':
			if !inString && depthParen > 0 {
				depthParen--
			}
		case '[':
			if !inString {
				depthBracket++
			}
		case ']':
			if !inString && depthBracket > 0 {
				depthBracket--
			}
		case ',':
			if !inString && depthParen == 0 && depthBracket == 0 {
				out = append(out, strings.TrimSpace(current.String()))
				current.Reset()
				continue
			}
		}
		current.WriteRune(ch)
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		out = append(out, part)
	}
	return out
}

func parseCallArg(attr, prefix string) string {
	start := strings.Index(attr, "(")
	end := strings.LastIndex(attr, ")")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(attr[start+1 : end])
}

func parseCallStringArg(attr, prefix string) string {
	return strings.Trim(parseCallArg(attr, prefix), "\"")
}

func runtimeDefaultExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"") && len(expr) >= 2 {
		return strings.Trim(expr, "\"")
	}
	return expr
}

func parseTypeToken(token string) (string, bool, bool) {
	token = strings.TrimSpace(token)
	list := strings.HasSuffix(token, "[]")
	if list {
		token = strings.TrimSuffix(token, "[]")
	}
	optional := strings.HasSuffix(token, "?")
	if optional {
		token = strings.TrimSuffix(token, "?")
	}
	return token, optional, list
}

func mapPrismaType(name string, list bool) engine.FieldType {
	if list {
		return engine.TypeArray
	}
	if scalar, ok := scalarTypes[name]; ok {
		return scalar
	}
	return engine.TypeRelation
}

func isScalarType(name string) bool {
	_, ok := scalarTypes[name]
	return ok
}

func renderFieldLine(buf *bytes.Buffer, field engine.SchemaField) {
	localName := field.Name
	mappedName := field.MappedName
	if field.IsID && field.StorageName() == "_id" && localName == "_id" {
		localName = "id"
		mappedName = "_id"
	}

	typeName := field.PrismaType
	if typeName == "" {
		switch field.Type {
		case engine.TypeString:
			typeName = "String"
		case engine.TypeNumber:
			typeName = "Float"
		case engine.TypeBoolean:
			typeName = "Boolean"
		case engine.TypeDateTime:
			typeName = "DateTime"
		case engine.TypeArray, engine.TypeObject:
			typeName = "Json"
		case engine.TypeRelation:
			typeName = field.Relation.Model
			if typeName == "" {
				typeName = "Json"
			}
		default:
			typeName = "String"
		}
	}
	if field.List && !strings.HasSuffix(typeName, "[]") {
		typeName += "[]"
	}
	if !field.Required && !field.List {
		typeName += "?"
	}

	buf.WriteString("  ")
	buf.WriteString(localName)
	buf.WriteString(" ")
	buf.WriteString(typeName)

	if field.IsID {
		buf.WriteString(" @id")
	}
	if field.Unique {
		buf.WriteString(" @unique")
	}
	if field.DefaultExpr != nil && *field.DefaultExpr != "" {
		buf.WriteString(" @default(")
		buf.WriteString(*field.DefaultExpr)
		buf.WriteString(")")
	}
	if field.AutoUpdatedAt {
		buf.WriteString(" @updatedAt")
	}
	if relation := field.Relation; relation != nil {
		buf.WriteString(" @relation(")
		parts := make([]string, 0, 5)
		if relation.Name != "" {
			parts = append(parts, fmt.Sprintf(`name: "%s"`, relation.Name))
		}
		if len(relation.Fields) > 0 {
			parts = append(parts, fmt.Sprintf("fields: [%s]", strings.Join(relation.Fields, ", ")))
		}
		if len(relation.References) > 0 {
			parts = append(parts, fmt.Sprintf("references: [%s]", strings.Join(relation.References, ", ")))
		}
		if relation.OnDelete != "" {
			parts = append(parts, "onDelete: "+relation.OnDelete)
		}
		if relation.OnUpdate != "" {
			parts = append(parts, "onUpdate: "+relation.OnUpdate)
		}
		buf.WriteString(strings.Join(parts, ", "))
		buf.WriteString(")")
	}
	if mappedName != "" && mappedName != localName {
		buf.WriteString(` @map("`)
		buf.WriteString(mappedName)
		buf.WriteString("\")")
	}
	buf.WriteString("\n")
}

func renderIndexLine(buf *bytes.Buffer, index engine.SchemaIndex) {
	kind := "@@index"
	switch {
	case index.Primary:
		kind = "@@id"
	case index.Unique:
		kind = "@@unique"
	}
	buf.WriteString("  ")
	buf.WriteString(kind)
	buf.WriteString("([")
	buf.WriteString(strings.Join(index.Fields, ", "))
	buf.WriteString("]")
	if index.Name != "" {
		buf.WriteString(`, name: "`)
		buf.WriteString(index.Name)
		buf.WriteString(`"`)
	}
	buf.WriteString(")\n")
}

func cutToken(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}
	for i, r := range input {
		if r == ' ' || r == '\t' {
			return input[:i], strings.TrimSpace(input[i+1:]), true
		}
	}
	return input, "", true
}

func stripComment(line string) string {
	inString := false
	for i := 0; i < len(line)-1; i++ {
		switch line[i] {
		case '"':
			inString = !inString
		case '/':
			if !inString && line[i+1] == '/' {
				return line[:i]
			}
		}
	}
	return line
}

func lineError(line int, err error) error {
	return fmt.Errorf("line %d: %w", line+1, err)
}
