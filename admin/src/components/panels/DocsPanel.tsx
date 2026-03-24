/**
 * @fileoverview DocsPanel – full built-in documentation for VoidDB.
 * Comprehensive, beautiful docs with syntax highlighted code blocks,
 * section navigation, and copy-to-clipboard.
 */

"use client";

import React, { useState, useRef, useEffect } from "react";
import { motion } from "framer-motion";
import {
  BookOpen,
  Rocket,
  Code2,
  Database,
  Search,
  HardDrive,
  Terminal,
  Server,
  Copy,
  CheckCircle,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";

// ── Code block with copy ─────────────────────────────────────────────────────

function CodeBlock({ code, lang = "bash" }: { code: string; lang?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <div className="relative group my-3">
      <div className="flex items-center justify-between px-3 py-1.5 bg-surface-0 border border-border border-b-0 rounded-t-md">
        <span className="text-[10px] font-mono text-muted-foreground uppercase">{lang}</span>
        <button
          onClick={copy}
          className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
        >
          {copied ? <CheckCircle className="w-3.5 h-3.5 text-neon-500" /> : <Copy className="w-3.5 h-3.5" />}
        </button>
      </div>
      <pre className="text-xs font-mono text-muted-foreground bg-surface-0 rounded-b-md p-3 overflow-x-auto border border-border border-t-0 leading-relaxed">
        {code}
      </pre>
    </div>
  );
}

function Heading({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h3 id={id} className="text-base font-semibold text-foreground mt-8 mb-3 flex items-center gap-2 scroll-mt-4">
      {children}
    </h3>
  );
}

function SubHeading({ children }: { children: React.ReactNode }) {
  return <h4 className="text-sm font-medium text-foreground mt-5 mb-2">{children}</h4>;
}

function P({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-muted-foreground leading-relaxed mb-3">{children}</p>;
}

function EndpointRow({ method, path, desc }: { method: string; path: string; desc: string }) {
  const color =
    method === "GET"
      ? "text-neon-500 bg-neon-500/10"
      : method === "POST"
        ? "text-blue-400 bg-blue-500/10"
        : method === "PUT"
          ? "text-amber-400 bg-amber-500/10"
          : method === "PATCH"
            ? "text-violet-400 bg-violet-500/10"
            : "text-red-400 bg-red-500/10";

  return (
    <div className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-surface-3 transition-colors">
      <span className={cn("text-[10px] font-bold px-1.5 py-0.5 rounded font-mono w-14 text-center", color)}>
        {method}
      </span>
      <code className="text-xs font-mono text-foreground flex-1">{path}</code>
      <span className="text-xs text-muted-foreground">{desc}</span>
    </div>
  );
}

// ── Section definitions ──────────────────────────────────────────────────────

const BASE = "process.env.NEXT_PUBLIC_API_URL || 'http://localhost:7700'";

interface SectionDef {
  id: string;
  icon: React.ReactNode;
  label: string;
  content: React.ReactNode;
}

function useSections(): SectionDef[] {
  const apiUrl = typeof window !== "undefined"
    ? (process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700")
    : "http://localhost:7700";

  return [
    {
      id: "quickstart",
      icon: <Rocket className="w-3.5 h-3.5" />,
      label: "Quick Start",
      content: (
        <>
          <Heading id="quickstart">Quick Start</Heading>
          <P>Get VoidDB running in seconds. Choose one of the methods below.</P>

          <SubHeading>One-Command Deploy (Linux server)</SubHeading>
          <P>
            Deploy VoidDB to a fresh Ubuntu/Debian server with auto-generated admin credentials,
            systemd service, and optional HTTPS via Caddy reverse proxy — all in one command.
          </P>
          <CodeBlock lang="bash" code={`# Basic deploy (HTTP only, port 7700)
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash

# Deploy with domain + auto-HTTPS
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash -s -- \\
  --domain db.example.com

# The script will:
#  1. Install Go (if missing)
#  2. Clone & build VoidDB
#  3. Generate random admin credentials (printed at the end)
#  4. Create systemd service (auto-start on boot)
#  5. Set up Caddy reverse proxy with Let's Encrypt SSL (if --domain)`} />

          <SubHeading>Docker Compose</SubHeading>
          <CodeBlock lang="bash" code={`cp .env.example .env
docker-compose up -d
# Admin panel: http://localhost:3000
# API: http://localhost:7700`} />

          <SubHeading>Build from Source</SubHeading>
          <CodeBlock lang="bash" code={`git clone https://github.com/voiddb/void.git
cd void
go build -o voiddb ./cmd/voiddb
./voiddb -config config.yaml`} />

          <SubHeading>Interactive Setup Wizard</SubHeading>
          <CodeBlock lang="bash" code={`chmod +x scripts/setup.sh
./scripts/setup.sh
# Follow the interactive prompts to configure ports, security, TLS, etc.`} />
        </>
      ),
    },
    {
      id: "api",
      icon: <Code2 className="w-3.5 h-3.5" />,
      label: "API Reference",
      content: (
        <>
          <Heading id="api">REST API Reference</Heading>
          <P>
            All endpoints require a valid JWT Bearer token (except <code className="text-neon-400">/v1/auth/login</code>).
            Send JSON request bodies with <code className="text-neon-400">Content-Type: application/json</code>.
          </P>

          <SubHeading>Authentication</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="POST" path="/v1/auth/login" desc="Login → JWT token pair" />
            <EndpointRow method="POST" path="/v1/auth/refresh" desc="Refresh access token" />
            <EndpointRow method="GET" path="/v1/auth/me" desc="Current user info" />
          </div>
          <CodeBlock lang="bash" code={`curl -X POST ${apiUrl}/v1/auth/login \\
  -H 'Content-Type: application/json' \\
  -d '{"username":"admin","password":"your-password"}'

# Response:
# { "access_token": "eyJ...", "refresh_token": "eyJ...", "expires_in": 86400 }`} />

          <SubHeading>Databases</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="GET" path="/v1/databases" desc="List all databases" />
            <EndpointRow method="POST" path="/v1/databases" desc="Create database" />
            <EndpointRow method="DELETE" path="/v1/databases/{db}" desc="Drop database" />
          </div>

          <SubHeading>Collections</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="GET" path="/v1/databases/{db}/collections" desc="List collections" />
            <EndpointRow method="POST" path="/v1/databases/{db}/collections" desc="Create collection" />
            <EndpointRow method="DELETE" path="/v1/databases/{db}/collections/{col}" desc="Drop collection" />
          </div>

          <SubHeading>Documents</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="POST" path="/v1/databases/{db}/{col}" desc="Insert document" />
            <EndpointRow method="GET" path="/v1/databases/{db}/{col}/{id}" desc="Get by ID" />
            <EndpointRow method="PUT" path="/v1/databases/{db}/{col}/{id}" desc="Replace document" />
            <EndpointRow method="PATCH" path="/v1/databases/{db}/{col}/{id}" desc="Partial update" />
            <EndpointRow method="DELETE" path="/v1/databases/{db}/{col}/{id}" desc="Delete document" />
            <EndpointRow method="POST" path="/v1/databases/{db}/{col}/query" desc="Query documents" />
          </div>

          <SubHeading>Engine</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="GET" path="/v1/stats" desc="Engine metrics" />
            <EndpointRow method="GET" path="/health" desc="Health check" />
          </div>

          <SubHeading>S3 Blob Storage</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="PUT" path="/s3/{bucket}" desc="Create bucket" />
            <EndpointRow method="GET" path="/s3/{bucket}" desc="List objects" />
            <EndpointRow method="PUT" path="/s3/{bucket}/{key}" desc="Upload object" />
            <EndpointRow method="GET" path="/s3/{bucket}/{key}" desc="Download object" />
            <EndpointRow method="DELETE" path="/s3/{bucket}/{key}" desc="Delete object" />
          </div>
        </>
      ),
    },
    {
      id: "agents",
      icon: <BookOpen className="w-3.5 h-3.5" />,
      label: "AI Agents",
      content: (
        <>
          <Heading id="agents">AI Agent Guide</Heading>
          <P>
            VoidDB exposes a machine-readable markdown guide for automation and AI tooling.
            Agents can fetch it directly from the running server and use it as the source of truth
            for auth, query syntax, cache, blobs, and safe write behavior.
          </P>

          <SubHeading>Server URL</SubHeading>
          <CodeBlock lang="bash" code={`curl ${apiUrl}/skill.md`} />

          <SubHeading>Alternate Well-Known URL</SubHeading>
          <CodeBlock lang="bash" code={`curl ${apiUrl}/.well-known/voiddb-skill.md`} />

          <SubHeading>Repository Copy</SubHeading>
          <CodeBlock lang="text" code={`https://github.com/Nopass0/void/blob/main/SKILL.md`} />
        </>
      ),
    },
    {
      id: "query",
      icon: <Search className="w-3.5 h-3.5" />,
      label: "Query DSL",
      content: (
        <>
          <Heading id="query">Query DSL</Heading>
          <P>
            The query endpoint accepts a JSON body with <code className="text-neon-400">where</code>,{" "}
            <code className="text-neon-400">order_by</code>,{" "}
            <code className="text-neon-400">limit</code>, and <code className="text-neon-400">skip</code> fields.
          </P>

          <SubHeading>Full Query Spec</SubHeading>
          <CodeBlock lang="json" code={`{
  "where": {
    "AND": [
      { "field": "age", "op": "gte", "value": 18 },
      { "field": "status", "op": "eq", "value": "active" }
    ]
  },
  "order_by": [
    { "field": "created_at", "dir": "desc" }
  ],
  "limit": 25,
  "skip": 0
}`} />

          <SubHeading>Supported Operators</SubHeading>
          <div className="overflow-x-auto mb-4">
            <table className="data-table text-xs">
              <thead>
                <tr>
                  <th>Operator</th>
                  <th>Description</th>
                  <th>Example</th>
                </tr>
              </thead>
              <tbody>
                {[
                  ["eq", "Equal", '{"field":"name","op":"eq","value":"Alice"}'],
                  ["ne", "Not equal", '{"field":"status","op":"ne","value":"deleted"}'],
                  ["gt", "Greater than", '{"field":"age","op":"gt","value":21}'],
                  ["gte", "Greater than or equal", '{"field":"score","op":"gte","value":90}'],
                  ["lt", "Less than", '{"field":"price","op":"lt","value":100}'],
                  ["lte", "Less than or equal", '{"field":"quantity","op":"lte","value":0}'],
                  ["contains", "String contains", '{"field":"name","op":"contains","value":"ali"}'],
                  ["starts_with", "String starts with", '{"field":"email","op":"starts_with","value":"admin"}'],
                  ["in", "Value in array", '{"field":"role","op":"in","value":["admin","mod"]}'],
                ].map(([op, desc, ex]) => (
                  <tr key={op}>
                    <td><code className="text-neon-400">{op}</code></td>
                    <td>{desc}</td>
                    <td><code className="text-xs text-muted-foreground">{ex}</code></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <SubHeading>curl Example</SubHeading>
          <CodeBlock lang="bash" code={`curl -X POST ${apiUrl}/v1/databases/myapp/users/query \\
  -H 'Authorization: Bearer <token>' \\
  -H 'Content-Type: application/json' \\
  -d '{
    "where": {"AND":[{"field":"age","op":"gte","value":18}]},
    "order_by": [{"field":"name","dir":"asc"}],
    "limit": 10
  }'`} />
        </>
      ),
    },
    {
      id: "typescript",
      icon: <Code2 className="w-3.5 h-3.5" />,
      label: "TypeScript ORM",
      content: (
        <>
          <Heading id="typescript">TypeScript ORM</Heading>
          <P>The official TypeScript ORM provides a type-safe, fluent API for interacting with VoidDB.</P>

          <SubHeading>Installation</SubHeading>
          <CodeBlock lang="bash" code={`npm install @voiddb/orm
# or
yarn add @voiddb/orm
# or
bun add @voiddb/orm`} />

          <SubHeading>Connect</SubHeading>
          <CodeBlock lang="typescript" code={`import { VoidClient, query } from '@voiddb/orm'

const client = new VoidClient({
  url: '${apiUrl}',
  token: process.env.VOID_TOKEN!,
})`} />

          <SubHeading>CRUD Operations</SubHeading>
          <CodeBlock lang="typescript" code={`// Get a typed collection
interface User { name: string; age: number; email: string }
const users = client.database('myapp').collection<User>('users')

// Insert
const id = await users.insert({ name: 'Alice', age: 30, email: 'alice@example.com' })

// Find by ID
const user = await users.get(id)

// Query with fluent builder
const adults = await users.find(
  query()
    .where('age', 'gte', 18)
    .where('email', 'contains', '@example.com')
    .orderBy('name', 'asc')
    .limit(25)
    .skip(0)
)

// Update (partial)
await users.patch(id, { age: 31 })

// Replace (full document)
await users.put(id, { name: 'Alice', age: 31, email: 'alice@new.com' })

// Delete
await users.delete(id)

// Count
const count = await users.count(query().where('age', 'gte', 18))

// Cache
await client.cache.set('session:alice', { loggedIn: true }, 3600)
const session = await client.cache.get<{ loggedIn: boolean }>('session:alice')`} />
        </>
      ),
    },
    {
      id: "go",
      icon: <Code2 className="w-3.5 h-3.5" />,
      label: "Go ORM",
      content: (
        <>
          <Heading id="go">Go ORM</Heading>
          <P>The Go ORM provides an idiomatic Go client for VoidDB.</P>

          <SubHeading>Installation</SubHeading>
          <CodeBlock lang="bash" code={`go get github.com/voiddb/void/orm/go`} />

          <SubHeading>Usage</SubHeading>
          <CodeBlock lang="go" code={`package main

import (
    "context"
    "fmt"
    "os"

    voidorm "github.com/voiddb/void/orm/go"
)

func main() {
    client, err := voidorm.New(voidorm.Config{
        URL:   "${apiUrl}",
        Token: os.Getenv("VOID_TOKEN"),
    })
    if err != nil { panic(err) }

    ctx := context.Background()
    col := client.DB("myapp").Collection("users")

    // Insert
    id, _ := col.Insert(ctx, voidorm.Doc{
        "name": "Alice",
        "age":  30,
    })
    fmt.Println("Inserted:", id)

    // Query
    docs, _ := col.Find(ctx,
        voidorm.NewQuery().
            Where("age", voidorm.Gte, 18).
            OrderBy("name", voidorm.Asc).
            Limit(25),
    )
    for _, doc := range docs {
        fmt.Println(doc["name"])
    }

    // Update
    _ = col.Patch(ctx, id, voidorm.Doc{"age": 31})

    // Delete
    _ = col.Delete(ctx, id)
}`} />
        </>
      ),
    },
    {
      id: "s3",
      icon: <HardDrive className="w-3.5 h-3.5" />,
      label: "S3 Blob Storage",
      content: (
        <>
          <Heading id="s3">S3-Compatible Blob Storage</Heading>
          <P>VoidDB includes a built-in S3-compatible object storage API.</P>

          <SubHeading>AWS CLI</SubHeading>
          <CodeBlock lang="bash" code={`# Create bucket
aws s3 --endpoint-url ${apiUrl}/s3 mb s3://my-bucket

# Upload file
aws s3 --endpoint-url ${apiUrl}/s3 cp ./photo.jpg s3://my-bucket/photo.jpg

# List objects
aws s3 --endpoint-url ${apiUrl}/s3 ls s3://my-bucket/

# Download
aws s3 --endpoint-url ${apiUrl}/s3 cp s3://my-bucket/photo.jpg ./downloaded.jpg`} />

          <SubHeading>AWS SDK (Node.js)</SubHeading>
          <CodeBlock lang="typescript" code={`import { S3Client, PutObjectCommand, GetObjectCommand } from '@aws-sdk/client-s3'

const s3 = new S3Client({
  endpoint: '${apiUrl}/s3',
  region: 'void-1',
  credentials: { accessKeyId: 'void', secretAccessKey: '<token>' },
  forcePathStyle: true,
})

// Upload
await s3.send(new PutObjectCommand({
  Bucket: 'my-bucket',
  Key: 'hello.txt',
  Body: Buffer.from('Hello VoidDB!'),
  ContentType: 'text/plain',
}))

// Download
const { Body } = await s3.send(new GetObjectCommand({
  Bucket: 'my-bucket',
  Key: 'hello.txt',
}))
const content = await Body.transformToString()`} />
        </>
      ),
    },
    {
      id: "cache",
      icon: <Database className="w-3.5 h-3.5" />,
      label: "In-Memory Cache",
      content: (
        <>
          <Heading id="cache">Key-Value Cache API</Heading>
          <P>VoidDB includes a built-in blazing fast distributed-safe Key-Value store with TTL expirations (acting as a pure Redis alternative).</P>

          <SubHeading>REST API</SubHeading>
          <div className="space-y-0.5 mb-4">
            <EndpointRow method="GET" path="/v1/cache/{key}" desc="Retrieve value" />
            <EndpointRow method="POST" path="/v1/cache/{key}" desc="Set value (requires JSON body)" />
            <EndpointRow method="DELETE" path="/v1/cache/{key}" desc="Delete value" />
          </div>

          <SubHeading>cURL Examples</SubHeading>
          <CodeBlock lang="bash" code={`# Set a key with 3600 second TTL
curl -X POST \${apiUrl}/v1/cache/my-key \\
  -H 'Authorization: Bearer <token>' \\
  -H 'Content-Type: application/json' \\
  -d '{"value": "hello world", "ttl": 3600}'

# Get a key
curl \${apiUrl}/v1/cache/my-key \\
  -H 'Authorization: Bearer <token>'`} />

          <SubHeading>TypeScript ORM</SubHeading>
          <CodeBlock lang="typescript" code={`import { VoidClient } from '@voiddb/orm'

const db = new VoidClient({ url: '\${apiUrl}', token: '<token>' })

// Set an object (auto-serialized) for 1 hour
await db.cache.set('session:123', { user: 'Alice', active: true }, 3600)

// Get an object
const session = await db.cache.get<{ user: string }>('session:123')

// Delete
await db.cache.delete('session:123')`} />
        </>
      ),
    },
    {
      id: "deploy",
      icon: <Server className="w-3.5 h-3.5" />,
      label: "Deploy Guide",
      content: (
        <>
          <Heading id="deploy">Production Deploy Guide</Heading>
          <P>
            VoidDB ships with scripts for one-command deployment on Linux servers. The deploy script
            handles building, configuring, and daemonizing — and prints random-word credentials at the end.
          </P>

          <SubHeading>One-Line Deploy</SubHeading>
          <CodeBlock lang="bash" code={`# HTTP only (internal use)
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash

# With domain + SSL (production)
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash -s -- \\
  --domain db.example.com

# Custom port
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash -s -- \\
  --domain db.example.com --port 8080`} />

          <SubHeading>What the Script Does</SubHeading>
          <div className="space-y-2 mb-4">
            {[
              "Checks and installs dependencies (Go, Node.js)",
              "Clones the VoidDB repository to /opt/voiddb",
              "Builds the voiddb and voidcli binaries",
              "Generates random admin login/password (random word phrases)",
              "Creates config.yaml and .env with generated secrets",
              "Installs and starts a systemd service (auto-start on boot)",
              "If --domain: installs Caddy, creates Caddyfile, obtains Let's Encrypt SSL cert",
              "Builds the admin panel for production",
              "Prints access URL and credentials",
            ].map((step, i) => (
              <div key={i} className="flex items-start gap-2 text-sm">
                <span className="text-neon-500 font-mono text-xs mt-0.5">{i + 1}.</span>
                <span className="text-muted-foreground">{step}</span>
              </div>
            ))}
          </div>

          <SubHeading>Output Example</SubHeading>
          <CodeBlock lang="text" code={`╔════════════════════════════════════════╗
║    VoidDB Deployed Successfully!      ║
╠════════════════════════════════════════╣
║  URL:    https://db.example.com       ║
║  Login:  clever-frog-rapid-moon       ║
║  Pass:   bright-star-wild-ocean       ║
╚════════════════════════════════════════╝`} />

          <SubHeading>Manual Setup (Interactive)</SubHeading>
          <CodeBlock lang="bash" code={`# Clone the repo
git clone https://github.com/voiddb/void.git /opt/voiddb
cd /opt/voiddb

# Run the interactive setup wizard
chmod +x scripts/setup.sh
./scripts/setup.sh`} />

          <SubHeading>systemd Management</SubHeading>
          <CodeBlock lang="bash" code={`sudo systemctl status voiddb    # check status
sudo systemctl restart voiddb   # restart
sudo systemctl stop voiddb      # stop
sudo journalctl -u voiddb -f    # view live logs`} />
        </>
      ),
    },
    {
      id: "cli",
      icon: <Terminal className="w-3.5 h-3.5" />,
      label: "CLI Reference",
      content: (
        <>
          <Heading id="cli">VoidDB CLI</Heading>
          <P>
            <code className="text-neon-400">voidcli</code> is a command-line tool for managing VoidDB from the terminal.
          </P>

          <SubHeading>Connection</SubHeading>
          <CodeBlock lang="bash" code={`# Login
voidcli login --url ${apiUrl} --user admin --pass <password>

# Check server status
voidcli status`} />

          <SubHeading>Database Management</SubHeading>
          <CodeBlock lang="bash" code={`# List databases
voidcli db list

# Create a database
voidcli db create myapp

# Drop a database
voidcli db drop myapp`} />

          <SubHeading>Collection Management</SubHeading>
          <CodeBlock lang="bash" code={`# List collections in a database
voidcli col list myapp

# Create a collection
voidcli col create myapp users

# Drop a collection
voidcli col drop myapp users`} />

          <SubHeading>Document Operations</SubHeading>
          <CodeBlock lang="bash" code={`# Insert a document
voidcli doc insert myapp users '{"name":"Alice","age":30}'

# Get a document by ID
voidcli doc get myapp users <document-id>

# Update a document
voidcli doc patch myapp users <document-id> '{"age":31}'

# Delete a document
voidcli doc delete myapp users <document-id>

# Query documents
voidcli doc query myapp users '{"where":[{"field":"age","op":"gte","value":18}]}'`} />

          <SubHeading>Backup & Restore</SubHeading>
          <CodeBlock lang="bash" code={`# Create a backup
./scripts/backup.sh backup

# Restore from backup
./scripts/backup.sh restore <backup-file>`} />
        </>
      ),
    },
  ];
}

// ── Main component ────────────────────────────────────────────────────────────

export function DocsPanel() {
  const sections = useSections();
  const [activeSection, setActiveSection] = useState(sections[0].id);
  const contentRef = useRef<HTMLDivElement>(null);

  const current = sections.find((s) => s.id === activeSection);

  useEffect(() => {
    contentRef.current?.scrollTo(0, 0);
  }, [activeSection]);

  return (
    <div className="flex h-[calc(100vh-7rem)] gap-0 rounded-lg overflow-hidden border border-border">
      {/* Left nav */}
      <nav className="w-52 shrink-0 bg-surface-1 border-r border-border p-2 space-y-0.5 overflow-y-auto">
        <div className="flex items-center gap-2 px-3 py-2 mb-2">
          <BookOpen className="w-4 h-4 text-neon-500" />
          <span className="text-sm font-semibold text-foreground">Documentation</span>
        </div>
        {sections.map((s) => (
          <button
            key={s.id}
            onClick={() => setActiveSection(s.id)}
            className={cn(
              "w-full flex items-center gap-2.5 px-3 py-1.5 rounded-md text-sm transition-all text-left",
              activeSection === s.id
                ? "bg-surface-3 text-foreground"
                : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
            )}
          >
            <span className={cn("shrink-0", activeSection === s.id && "text-neon-500")}>{s.icon}</span>
            <span className="flex-1 truncate">{s.label}</span>
            {activeSection === s.id && <ChevronRight className="w-3 h-3 text-neon-500" />}
          </button>
        ))}
      </nav>

      {/* Content */}
      <div ref={contentRef} className="flex-1 overflow-y-auto bg-surface-2 p-6">
        <motion.div
          key={activeSection}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.15 }}
          className="max-w-3xl"
        >
          {current?.content}
        </motion.div>
      </div>
    </div>
  );
}
