// Command voidcli is the VoidDB command-line management tool.
//
// It connects to a running VoidDB server via HTTP and lets you manage
// databases, collections, documents, backups, and the server itself.
//
// Usage:
//
//	voidcli [--url http://localhost:7700] [--token <jwt>] <command> [args...]
//
// Commands:
//
//	status                           Show server status
//	db list                          List all databases
//	db create <name>                 Create a database
//	db drop   <name>                 Drop a database
//	col list   <db>                  List collections in a database
//	col create <db> <name>           Create a collection
//	col drop   <db> <name>           Drop a collection
//	doc insert <db> <col> <json>     Insert a document (JSON string or @file)
//	doc find   <db> <col> [filter]   Find documents
//	doc get    <db> <col> <id>       Get one document by ID
//	doc delete <db> <col> <id>       Delete a document
//	backup  [db...]                  Export databases to .void archive
//	restore <file.void>              Restore from .void archive
//	login   [--user admin]           Obtain and cache a JWT token
//	logout                           Remove cached token
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// version is set by ldflags.
var version = "dev"

// ── Config ────────────────────────────────────────────────────────────────────

var (
	flagURL   = flag.String("url", envOr("VOID_URL", "http://localhost:7700"), "VoidDB server URL")
	flagToken = flag.String("token", "", "JWT access token (overrides cached token)")
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// tokenCachePath returns the path where the CLI caches the JWT token.
func tokenCachePath() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "voiddb", "token")
}

func loadToken() string {
	if *flagToken != "" {
		return *flagToken
	}
	data, _ := os.ReadFile(tokenCachePath())
	return strings.TrimSpace(string(data))
}

func saveToken(tok string) error {
	p := tokenCachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(tok), 0600)
}

func clearToken() {
	_ = os.Remove(tokenCachePath())
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

var client = &http.Client{Timeout: 30 * time.Second}

func request(method, path string, body interface{}) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, *flagURL+path, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := loadToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("connect to %s: %w", *flagURL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func mustOK(data []byte, status int, err error) []byte {
	if err != nil {
		fatalf("Error: %v", err)
	}
	if status >= 400 {
		fatalf("Server returned %d: %s", status, strings.TrimSpace(string(data)))
	}
	return data
}

func printJSON(data []byte) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Println(string(data))
		return
	}
	pretty, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(pretty))
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "voidcli: "+format+"\n", args...)
	os.Exit(1)
}

// ── Commands ──────────────────────────────────────────────────────────────────

func cmdStatus() {
	data, status, err := request("GET", "/health", nil)
	mustOK(data, status, err)
	printJSON(data)

	// Also print engine stats.
	data2, st2, _ := request("GET", "/v1/stats", nil)
	if st2 == 200 {
		fmt.Println()
		printJSON(data2)
	}
}

func cmdDBList() {
	data := mustOK(request("GET", "/v1/databases", nil))
	var resp struct {
		Databases []string `json:"databases"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		printJSON(data)
		return
	}
	if len(resp.Databases) == 0 {
		fmt.Println("(no databases)")
		return
	}
	fmt.Printf("%-30s\n", "DATABASE")
	fmt.Println(strings.Repeat("-", 30))
	for _, db := range resp.Databases {
		fmt.Println(db)
	}
}

func cmdDBCreate(name string) {
	data := mustOK(request("POST", "/v1/databases", map[string]string{"name": name}))
	printJSON(data)
}

func cmdDBDrop(name string) {
	data := mustOK(request("DELETE", "/v1/databases/"+name, nil))
	_ = data
	fmt.Printf("Database %q dropped.\n", name)
}

func cmdColList(db string) {
	data := mustOK(request("GET", "/v1/databases/"+db+"/collections", nil))
	var resp struct {
		Collections []string `json:"collections"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		printJSON(data)
		return
	}
	if len(resp.Collections) == 0 {
		fmt.Printf("(no collections in %s)\n", db)
		return
	}
	fmt.Printf("%-30s\n", "COLLECTION")
	fmt.Println(strings.Repeat("-", 30))
	for _, c := range resp.Collections {
		fmt.Println(c)
	}
}

func cmdColCreate(db, col string) {
	data := mustOK(request("POST", "/v1/databases/"+db+"/collections",
		map[string]string{"name": col}))
	printJSON(data)
}

func cmdColDrop(db, col string) {
	data := mustOK(request("DELETE", "/v1/databases/"+db+"/collections/"+col, nil))
	_ = data
	fmt.Printf("Collection %q dropped from %q.\n", col, db)
}

func cmdDocInsert(db, col, jsonArg string) {
	// Support @filename syntax.
	var raw []byte
	if strings.HasPrefix(jsonArg, "@") {
		var err error
		raw, err = os.ReadFile(jsonArg[1:])
		if err != nil {
			fatalf("read file: %v", err)
		}
	} else {
		raw = []byte(jsonArg)
	}
	var doc interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		fatalf("invalid JSON: %v", err)
	}
	data := mustOK(request("POST", "/v1/databases/"+db+"/"+col, doc))
	printJSON(data)
}

func cmdDocFind(db, col, filterArg string) {
	var spec interface{} = map[string]interface{}{"limit": 20}
	if filterArg != "" {
		if err := json.Unmarshal([]byte(filterArg), &spec); err != nil {
			fatalf("invalid filter JSON: %v", err)
		}
	}
	data := mustOK(request("POST", "/v1/databases/"+db+"/"+col+"/query", spec))
	var resp struct {
		Results []interface{} `json:"results"`
		Count   int64         `json:"count"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		printJSON(data)
		return
	}
	fmt.Printf("Found %d document(s):\n\n", resp.Count)
	for _, doc := range resp.Results {
		b, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(b))
		fmt.Println()
	}
}

func cmdDocGet(db, col, id string) {
	data := mustOK(request("GET", "/v1/databases/"+db+"/"+col+"/"+id, nil))
	printJSON(data)
}

func cmdDocDelete(db, col, id string) {
	data := mustOK(request("DELETE", "/v1/databases/"+db+"/"+col+"/"+id, nil))
	_ = data
	fmt.Printf("Document %q deleted from %s/%s.\n", id, db, col)
}

func cmdBackup(dbs []string) {
	body := map[string]interface{}{"databases": dbs}
	if len(dbs) == 0 {
		body = map[string]interface{}{}
	}

	var buf bytes.Buffer
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", *flagURL+"/v1/backup", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if tok := loadToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatalf("backup request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body2, _ := io.ReadAll(resp.Body)
		fatalf("backup failed (%d): %s", resp.StatusCode, body2)
	}

	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("voiddb_backup_%s.void", ts)
	f, err := os.Create(filename)
	if err != nil {
		fatalf("create file: %v", err)
	}
	defer f.Close()
	n, _ := io.Copy(f, io.TeeReader(resp.Body, &buf))
	fmt.Printf("Backup saved: %s (%d bytes)\n", filename, n)
}

func cmdRestore(file string) {
	f, err := os.Open(file)
	if err != nil {
		fatalf("open file: %v", err)
	}
	defer f.Close()

	req, _ := http.NewRequest("POST", *flagURL+"/v1/backup/restore", f)
	req.Header.Set("Content-Type", "application/octet-stream")
	if tok := loadToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatalf("restore request: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fatalf("restore failed (%d): %s", resp.StatusCode, data)
	}
	printJSON(data)
}

func cmdLogin(username, password string) {
	data := mustOK(request("POST", "/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	}))
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || resp.AccessToken == "" {
		fatalf("login failed: %s", data)
	}
	if err := saveToken(resp.AccessToken); err != nil {
		fatalf("save token: %v", err)
	}
	fmt.Println("Logged in. Token cached to", tokenCachePath())
}

func cmdLogout() {
	clearToken()
	fmt.Println("Token removed.")
}

// ── main ──────────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprintf(os.Stderr, `voidcli %s - VoidDB command-line tool

Usage:
  voidcli [--url URL] [--token JWT] <command> [args...]

Global flags:
  --url    Server URL  (default: $VOID_URL or http://localhost:7700)
  --token  JWT token   (overrides cached token)

Commands:
  status                           Show server health and engine stats
  login [--user <name>]            Log in and cache token
  logout                           Remove cached token

  db list                          List databases
  db create <name>                 Create a database
  db drop   <name>                 Drop a database

  col list   <db>                  List collections
  col create <db> <name>           Create a collection
  col drop   <db> <name>           Drop a collection

  doc insert <db> <col> <json>     Insert document (JSON or @file.json)
  doc find   <db> <col> [filter]   Find documents (optional JSON filter)
  doc get    <db> <col> <id>       Get document by ID
  doc delete <db> <col> <id>       Delete document by ID

  backup  [db...]                  Export to .void archive
  restore <file.void>              Restore from .void archive

Examples:
  voidcli login
  voidcli db create myapp
  voidcli col create myapp users
  voidcli doc insert myapp users '{"name":"Alice","age":30}'
  voidcli doc find myapp users '{"where":[{"field":"age","op":"gt","value":18}]}'
  voidcli backup
  voidcli restore voiddb_backup_20240101_020000.void

`, version)
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "status":
		cmdStatus()

	case "login":
		loginFlags := flag.NewFlagSet("login", flag.ExitOnError)
		user := loginFlags.String("user", "admin", "username")
		_ = loginFlags.Parse(rest)
		fmt.Printf("Password for %s: ", *user)
		var password string
		fmt.Scanln(&password)
		cmdLogin(*user, password)

	case "logout":
		cmdLogout()

	case "db":
		if len(rest) == 0 {
			fatalf("db requires a subcommand: list | create | drop")
		}
		switch rest[0] {
		case "list":
			cmdDBList()
		case "create":
			if len(rest) < 2 {
				fatalf("db create <name>")
			}
			cmdDBCreate(rest[1])
		case "drop":
			if len(rest) < 2 {
				fatalf("db drop <name>")
			}
			cmdDBDrop(rest[1])
		default:
			fatalf("unknown db subcommand: %s", rest[0])
		}

	case "col":
		if len(rest) == 0 {
			fatalf("col requires a subcommand: list | create | drop")
		}
		switch rest[0] {
		case "list":
			if len(rest) < 2 {
				fatalf("col list <db>")
			}
			cmdColList(rest[1])
		case "create":
			if len(rest) < 3 {
				fatalf("col create <db> <name>")
			}
			cmdColCreate(rest[1], rest[2])
		case "drop":
			if len(rest) < 3 {
				fatalf("col drop <db> <name>")
			}
			cmdColDrop(rest[1], rest[2])
		default:
			fatalf("unknown col subcommand: %s", rest[0])
		}

	case "doc":
		if len(rest) == 0 {
			fatalf("doc requires a subcommand: insert | find | get | delete")
		}
		switch rest[0] {
		case "insert":
			if len(rest) < 4 {
				fatalf("doc insert <db> <col> <json>")
			}
			cmdDocInsert(rest[1], rest[2], rest[3])
		case "find":
			if len(rest) < 3 {
				fatalf("doc find <db> <col> [filter]")
			}
			filter := ""
			if len(rest) >= 4 {
				filter = rest[3]
			}
			cmdDocFind(rest[1], rest[2], filter)
		case "get":
			if len(rest) < 4 {
				fatalf("doc get <db> <col> <id>")
			}
			cmdDocGet(rest[1], rest[2], rest[3])
		case "delete":
			if len(rest) < 4 {
				fatalf("doc delete <db> <col> <id>")
			}
			cmdDocDelete(rest[1], rest[2], rest[3])
		default:
			fatalf("unknown doc subcommand: %s", rest[0])
		}

	case "backup":
		cmdBackup(rest)

	case "restore":
		if len(rest) < 1 {
			fatalf("restore <file.void>")
		}
		cmdRestore(rest[0])

	default:
		fatalf("unknown command: %s\nRun 'voidcli --help' for usage.", cmd)
	}
}
