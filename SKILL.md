# VoidDB Server Skill

Use this guide when an AI agent needs to inspect, query, or mutate a live VoidDB server.

## What VoidDB Includes

- Document database: databases -> collections -> documents
- Prisma-like schema sync and migrations via `voidcli`
- Built-in S3-compatible blob storage
- Typed `Blob` document fields that point at stored objects
- In-memory cache API
- JWT auth and role-based access control
- Backups, restore, logs, realtime streams, and PostgreSQL import

## Base URLs

- API root: `http://host:7700`
- Health: `GET /health`
- Skill: `GET /skill.md`
- Alternate skill path: `GET /.well-known/voiddb-skill.md`
- Blob API root: `http://host:7700/s3`

## Authentication

1. Login with `POST /v1/auth/login`
2. Send `Authorization: Bearer <access_token>` on protected routes
3. Refresh with `POST /v1/auth/refresh` when needed

Example:

```bash
curl -X POST http://host:7700/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"your-password"}'
```

## Roles

- `admin`: full access, including users, imports, backups, schema, and destructive actions
- `readwrite`: document, cache, query, and blob operations
- `readonly`: read-only access

## Documents

- List databases: `GET /v1/databases`
- Create database: `POST /v1/databases` with `{ "name": "app" }`
- Delete database: `DELETE /v1/databases/{db}`
- List collections: `GET /v1/databases/{db}/collections`
- Create collection: `POST /v1/databases/{db}/collections` with `{ "name": "users" }`
- Delete collection: `DELETE /v1/databases/{db}/collections/{col}`
- Insert document: `POST /v1/databases/{db}/{col}`
- Get document: `GET /v1/databases/{db}/{col}/{id}`
- Replace document: `PUT /v1/databases/{db}/{col}/{id}`
- Patch document: `PATCH /v1/databases/{db}/{col}/{id}`
- Delete document: `DELETE /v1/databases/{db}/{col}/{id}`
- Count documents: `GET /v1/databases/{db}/{col}/count`
- Query documents: `POST /v1/databases/{db}/{col}/query`

Insert returns:

```json
{ "_id": "uuid" }
```

## Query DSL

The server expects a tree in `where`.

```json
{
  "where": {
    "AND": [
      { "field": "age", "op": "gte", "value": 18 },
      { "field": "active", "op": "eq", "value": true }
    ]
  },
  "order_by": [
    { "field": "created_at", "dir": "desc" }
  ],
  "limit": 25,
  "skip": 0
}
```

Supported operators:

- `eq`
- `ne`
- `gt`
- `gte`
- `lt`
- `lte`
- `contains`
- `starts_with`
- `in`

## Schema Sync And Migrations

- Pull schema: `voidcli schema pull --out void.prisma`
- Preview diff: `voidcli schema push --schema void.prisma --dry-run`
- Apply schema: `voidcli schema push --schema void.prisma`
- Create migration: `voidcli migrate dev --schema void.prisma --name add_users`
- Apply migrations: `voidcli migrate deploy --dir void/migrations`
- Check status: `voidcli migrate status --dir void/migrations`

Important behavior:

- Schema sync only mutates databases explicitly declared in the schema file
- Databases not declared in the schema must not be deleted, rewritten, or modified
- `--force-drop` only drops collections inside explicitly declared databases

Supported schema field types:

- `String`
- `Float` / `Int` / `BigInt` / `Decimal`
- `Boolean`
- `DateTime`
- `Json`
- `Blob`

## Blob Storage

S3-compatible routes:

- List buckets: `GET /s3/`
- Create bucket: `PUT /s3/{bucket}`
- Delete bucket: `DELETE /s3/{bucket}`
- List objects: `GET /s3/{bucket}`
- Upload object: `PUT /s3/{bucket}/{key}`
- Download object: `GET /s3/{bucket}/{key}`
- Head object: `HEAD /s3/{bucket}/{key}`
- Delete object: `DELETE /s3/{bucket}/{key}`

Blob requests also require Bearer auth.

## Blob Fields In Documents

VoidDB supports `Blob` as a document/schema field type.

Manual blob reference shape:

```json
{
  "_blob_bucket": "media",
  "_blob_key": "assets/123/original/photo.jpg"
}
```

When documents are returned by the API, blob fields also include `_blob_url`:

```json
{
  "_blob_bucket": "media",
  "_blob_key": "assets/123/original/photo.jpg",
  "_blob_url": "https://db.example.com/s3/media/assets/123/original/photo.jpg"
}
```

### Direct file upload into a document field

Upload a file and atomically store its blob reference into the target document field:

- `POST /v1/databases/{db}/{col}/{id}/files/{field}`
- `DELETE /v1/databases/{db}/{col}/{id}/files/{field}`

Multipart example:

```bash
curl -X POST http://host:7700/v1/databases/media/assets/123/files/original \
  -H 'Authorization: Bearer <token>' \
  -F 'file=@./photo.jpg'
```

Optional query params:

- `bucket`: override the default bucket name
- `key`: override the generated object key

Defaults:

- bucket defaults to the database name
- key defaults to `{collection}/{document_id}/{field}/{filename}`

## Cache

- Get: `GET /v1/cache/{key}`
- Set: `POST /v1/cache/{key}` with `{ "value": "...", "ttl": 60 }`
- Delete: `DELETE /v1/cache/{key}`

## Realtime

- Database events: `GET /v1/databases/{db}/realtime`
- Logs stream: `GET /v1/logs/realtime`

Both endpoints use Server-Sent Events.

## Logs And Operations

- Read recent logs: `GET /v1/logs`
- Logs are persisted with retention configured in server settings/config
- CRUD, imports, backups, and stream lifecycle events are logged

## Backup And Restore

- Export backup stream: `POST /v1/backup`
- Restore from backup body: `POST /v1/backup/restore`
- Admin file backups:
  - `GET /v1/backups`
  - `POST /v1/backups`
  - `GET /v1/backups/{name}`
  - `DELETE /v1/backups/{name}`
- Backup settings:
  - `GET /v1/settings/backup`
  - `PUT /v1/settings/backup`

## PostgreSQL Import

CLI:

```bash
voidcli import postgres "postgresql://user:pass@host:5432/app?sslmode=require" \
  --database app \
  --schema public \
  --drop-existing
```

Admin/API:

- `POST /v1/import/postgres`

## Admin And Docs

- Admin panel is a separate Next.js app inside the main repository
- Static docs live in `docs/`
- Built-in docs exist in the admin console
- TypeScript ORM repo: `https://github.com/Nopass0/void_ts`
- Go SDK repo: `https://github.com/Nopass0/void_go`

## Safe Agent Defaults

- Prefer `PATCH` over `PUT` unless replacing the whole document is intentional
- Use low `limit` values for exploration
- Treat `_id` as server-controlled unless explicitly mapped
- Before destructive actions, confirm the target database and collection
- When working with schemas, do not assume missing databases should be deleted
- Prefer direct file upload endpoints for `Blob` fields instead of hand-crafting object keys unless the caller asked for a custom key layout
- When returning blob references to users, prefer the `_blob_url` supplied by the server
