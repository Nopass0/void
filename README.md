# VoidDB ⚡

**A blazing-fast, self-hosted document database with S3-compatible blob storage.**

VoidDB is built from scratch in Go using a custom LSM-tree storage engine:
concurrent skip-list memtable, Bloom filter segments, memory-mapped SSTables,
and a write-ahead log — all designed for maximum throughput on modest hardware.

---

## Architecture

```
Client
  │
  ▼
REST API (Gorilla Mux + JWT Auth)
  │
  ├── Document Store (Collection / Database)
  │     │
  │     ▼
  │   LSM Engine
  │     ├── Memtable  (lock-free concurrent skip list)
  │     ├── WAL       (sequential append-only log)
  │     ├── SSTables  (immutable sorted segments + Bloom filters)
  │     └── LRU Cache (hot block cache)
  │
  └── Blob Store (S3-compatible HTTP API)
        └── Files on disk with JSON metadata sidecar
```

## Quick Deploy (Linux Server)

Deploy VoidDB to a production server with **one command**. Generates random admin credentials, sets up a systemd service, and optionally configures HTTPS via Caddy.

```bash
# Basic deploy (HTTP, port 7700)
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | sudo bash

# With domain + auto-HTTPS
curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | sudo bash -s -- \
  --domain db.example.com
```

After the script finishes it prints:

```
╔════════════════════════════════════════╗
║    VoidDB Deployed Successfully!       ║
╠════════════════════════════════════════╣
║  URL:    https://db.example.com        ║
║  Login:  clever-frog-rapid-moon        ║
║  Pass:   bright-star-wild-ocean        ║
╚════════════════════════════════════════╝
```

---

## Quick Start

### Docker Compose (recommended)

```bash
cp .env.example .env          # edit secrets
docker-compose up -d          # start VoidDB + Admin + PostgreSQL
open http://localhost:3000    # Admin panel (login: admin / admin)
```

### Build from source

```bash
go mod download
go build -o voiddb ./cmd/voiddb
./voiddb -config config.yaml
```

### Admin panel

```bash
cd admin
npm install
npm run dev   # http://localhost:3000
```

---

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/auth/login` | Login → JWT |
| POST | `/v1/auth/refresh` | Refresh token |
| GET | `/v1/databases` | List databases |
| POST | `/v1/databases` | Create database |
| GET | `/v1/databases/{db}/collections` | List collections |
| POST | `/v1/databases/{db}/collections` | Create collection |
| POST | `/v1/databases/{db}/{col}` | Insert document |
| GET | `/v1/databases/{db}/{col}/{id}` | Get document |
| PUT | `/v1/databases/{db}/{col}/{id}` | Replace document |
| PATCH | `/v1/databases/{db}/{col}/{id}` | Partial update |
| DELETE | `/v1/databases/{db}/{col}/{id}` | Delete document |
| POST | `/v1/databases/{db}/{col}/query` | Query documents |
| GET | `/v1/stats` | Engine stats |
| PUT | `/s3/{bucket}/{key}` | Upload blob (S3) |
| GET | `/s3/{bucket}/{key}` | Download blob (S3) |

## AI Agent Guide

Running VoidDB servers expose a machine-readable markdown guide for agents:

```bash
curl http://localhost:7700/skill.md
```

The same guide also lives in this repository at [`SKILL.md`](./SKILL.md).

## TypeScript ORM

```typescript
import { VoidClient, query } from '@voiddb/orm'

const client = new VoidClient({ url: 'http://localhost:7700', token: '...' })
const users = client.database('myapp').collection<User>('users')

// Insert
const id = await users.insert({ name: 'Alice', age: 30 })

// Query
const adults = await users.find(
  query().where('age', 'gte', 18).orderBy('name').limit(25)
)

// Update & delete
await users.patch(id, { age: 31 })
await users.delete(id)
```

## Go ORM

```go
client, _ := voidorm.New(voidorm.Config{
    URL:   "http://localhost:7700",
    Token: os.Getenv("VOID_TOKEN"),
})
col := client.DB("myapp").Collection("users")
id, _ := col.Insert(ctx, voidorm.Doc{"name": "Alice", "age": 30})
docs, _ := col.Find(ctx, voidorm.NewQuery().Where("age", voidorm.Gte, 18))
```

## Benchmark (VoidDB vs PostgreSQL)

```bash
docker-compose up -d postgres voiddb
cd benchmark
go run main.go -records 100000 -workers 8
```

Expected results on a typical developer machine:
- **Point GET**: VoidDB ~50–200x faster (sub-μs in-memory vs disk I/O)
- **Range scan**: VoidDB ~10–50x faster (Bloom filters + mmap vs PostgreSQL planner)
- **Concurrent INSERT**: VoidDB ~5–20x faster (batched WAL vs row-level locks)

## Project Structure

```
void/
├── cmd/voiddb/          # Server entry point
├── internal/
│   ├── config/          # YAML + env configuration
│   ├── engine/          # LSM-tree storage engine
│   │   ├── storage/     # Pages, segments, skip list, Bloom filter
│   │   ├── cache/       # LRU block cache
│   │   └── wal/         # Write-ahead log
│   ├── auth/            # JWT authentication
│   ├── blob/            # S3-compatible object storage
│   └── api/             # HTTP handlers + middleware
├── orm/
│   ├── typescript/      # TypeScript ORM (@voiddb/orm)
│   └── go/              # Go ORM (github.com/voiddb/void/orm/go)
├── admin/               # Next.js admin panel
├── benchmark/           # VoidDB vs PostgreSQL benchmark
├── config.yaml          # Default configuration
└── docker-compose.yml   # Docker deployment
```

## License

MIT
