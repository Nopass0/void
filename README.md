<p align="center">
  <img src="./docs/assets/voiddb-logo.svg" alt="VoidDB" width="720">
</p>

<p align="center">
  <strong>Self-hosted document database for fast product backends.</strong><br>
  Documents, blobs, cache, realtime admin, Prisma-like schema sync, migrations, and PostgreSQL import in one stack.
</p>

<p align="center">
  <a href="https://nopass0.github.io/void/">Docs</a> |
  <a href="https://github.com/Nopass0/void_ts">TypeScript ORM</a> |
  <a href="https://github.com/Nopass0/void/tree/main/orm/go">Go SDK</a> |
  <a href="./SKILL.md">AI Agent Guide</a>
</p>

## Why VoidDB

VoidDB is a self-hosted database built for teams that want one deployable service instead of a pile of moving parts.
You get a document database, S3-style blob storage, an in-memory cache, backups, schema sync, a live admin console, and direct PostgreSQL import without leaving the repo.

Highlights:

- Fast custom LSM storage engine written in Go
- Document API with schemas, indexes, relations, realtime updates, and imports
- S3-compatible blob storage with buckets and objects
- Built-in cache API for sessions, hot keys, and ephemeral state
- Admin console for data, users, logs, backups, query editing, and imports
- Prisma-like schema pull/push and migration workflow via `voidcli`
- Companion ORMs for TypeScript and Go

## Install Locally

### Windows

```powershell
git clone https://github.com/Nopass0/void.git
cd void
.\scripts\setup.ps1
.\scripts\run.ps1 -WithAdmin
```

### Linux / macOS

```bash
git clone https://github.com/Nopass0/void.git
cd void
chmod +x scripts/*.sh
./scripts/setup.sh
./scripts/run.sh --with-admin
```

### Docker Compose

```bash
git clone https://github.com/Nopass0/void.git
cd void
cp .env.example .env
docker compose up -d
```

Default local endpoints:

- API: `http://localhost:7700`
- Admin: `http://localhost:3000`
- AI guide: `http://localhost:7700/skill.md`

## Deploy On A Server

### One-command Linux deploy

```bash
curl -sSL https://raw.githubusercontent.com/Nopass0/void/main/scripts/deploy.sh | sudo bash
```

With domain and automatic HTTPS:

```bash
curl -sSL https://raw.githubusercontent.com/Nopass0/void/main/scripts/deploy.sh | sudo bash -s -- \
  --domain db.example.com
```

The deploy script:

- installs Go if missing
- clones or updates the repo
- builds `voiddb` and `voidcli`
- generates admin credentials and JWT secret
- writes `config.yaml`
- installs a `systemd` service
- optionally configures Caddy for TLS

### Manual build

```bash
go mod download
go build -o voiddb ./cmd/voiddb
go build -o voidcli ./cmd/voidcli
./voiddb -config config.yaml
```

## First Request In Under A Minute

```bash
curl -X POST http://localhost:7700/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
```

```bash
curl -X POST http://localhost:7700/v1/databases/app/collections \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"users"}'
```

```bash
curl -X POST http://localhost:7700/v1/databases/app/users \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","age":30,"active":true}'
```

## Schema Sync And Migrations

VoidDB supports Prisma-like schema files and CLI workflows for pull, push, diff, and migrations.

```prisma
datasource db {
  provider = "voiddb"
  url      = env("VOID_URL")
}

generator client {
  provider = "voiddb-client-js"
  output   = "./generated"
}

model User {
  id        String   @id @default(uuid()) @map("_id")
  email     String   @unique
  name      String?
  createdAt DateTime @map("created_at")

  @@database("app")
  @@map("users")
}
```

```bash
voidcli schema pull --out void.prisma
voidcli schema push --schema void.prisma --dry-run
voidcli schema push --schema void.prisma
voidcli migrate dev --schema void.prisma --name init
voidcli migrate deploy --dir void/migrations
voidcli migrate status --dir void/migrations
```

## Import PostgreSQL

Clone schema and rows directly from PostgreSQL:

```bash
voidcli import postgres "postgresql://user:pass@host:5432/app?sslmode=require" \
  --database app \
  --schema public \
  --drop-existing
```

The admin console also supports importing a new database directly from a PostgreSQL connection string.

## Official Client Ecosystem

### TypeScript ORM

Repository: [Nopass0/void_ts](https://github.com/Nopass0/void_ts)  
Docs: [nopass0.github.io/void_ts](https://nopass0.github.io/void_ts/)

```ts
import { VoidClient, query } from "@voiddb/orm";

const client = new VoidClient({ url: "http://localhost:7700" });
await client.login("admin", "admin");

const users = client.db("app").collection<{ name: string; age: number }>("users");

await users.insert({ name: "Alice", age: 30 });

const adults = await users.find(
  query().where("age", "gte", 18).orderBy("name", "asc").limit(25)
);
```

### Go SDK

Package path: [github.com/voiddb/void/orm/go](https://github.com/Nopass0/void/tree/main/orm/go)

```go
client, _ := voidorm.New(voidorm.Config{
    URL:      "http://localhost:7700",
    Username: "admin",
    Password: "admin",
})

col := client.DB("app").Collection("users")
id, _ := col.Insert(voidorm.Doc{"name": "Alice", "age": 30})
doc, _ := col.GetByID(id)
_ = doc
```

## Admin Console

The Next.js admin console ships in this repository and covers:

- databases and collections
- typed cell editing
- blob buckets and objects
- query editor
- users and roles
- logs and realtime activity
- backups, retention, and schedules
- PostgreSQL import

Run it locally:

```bash
cd admin
npm install
npm run dev
```

## AI Agent Guide

Running VoidDB servers expose a machine-readable usage guide for agents:

```bash
curl http://localhost:7700/skill.md
curl http://localhost:7700/.well-known/voiddb-skill.md
```

Repository copy: [SKILL.md](./SKILL.md)

## Documentation

- Main docs site: [nopass0.github.io/void](https://nopass0.github.io/void/)
- TypeScript ORM docs: [nopass0.github.io/void_ts](https://nopass0.github.io/void_ts/)
- TypeScript ORM repo: [Nopass0/void_ts](https://github.com/Nopass0/void_ts)
- Go SDK source: [orm/go](https://github.com/Nopass0/void/tree/main/orm/go)

## Repository Layout

```text
void/
|-- admin/              Next.js admin console
|-- benchmark/          Benchmarks and performance checks
|-- cmd/                voiddb and voidcli binaries
|-- docs/               GitHub Pages documentation
|-- internal/           Engine, API, auth, blob, cache, import, backup
|-- orm/go/             Go SDK
|-- orm/typescript/     TypeScript ORM
|-- scripts/            Local run, setup, backup, deploy helpers
|-- config.yaml         Default server configuration
`-- docker-compose.yml  Local stack
```

## License

MIT
