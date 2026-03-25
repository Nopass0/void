# VoidDB Server Skill

Use this guide when an AI agent needs to talk to a live VoidDB server safely.

## Base URL

- API root: `http://host:7700`
- Health: `GET /health`
- Agent guide: `GET /skill.md`

## Authentication

1. Login with `POST /v1/auth/login`
2. Send `Authorization: Bearer <access_token>` on every `/v1/*` request
3. Refresh with `POST /v1/auth/refresh` when needed

Example:

```bash
curl -X POST http://host:7700/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"your-password"}'
```

## Documents

- List databases: `GET /v1/databases`
- Create database: `POST /v1/databases` with `{ "name": "mydb" }`
- Delete database: `DELETE /v1/databases/{db}`
- List collections: `GET /v1/databases/{db}/collections`
- Create collection: `POST /v1/databases/{db}/collections` with `{ "name": "users" }`
- Delete collection: `DELETE /v1/databases/{db}/collections/{col}`
- Get schema: `GET /v1/databases/{db}/{col}/schema`
- Set schema: `PUT /v1/databases/{db}/{col}/schema`
- Insert document: `POST /v1/databases/{db}/{col}`
- Get document: `GET /v1/databases/{db}/{col}/{id}`
- Replace document: `PUT /v1/databases/{db}/{col}/{id}`
- Patch document: `PATCH /v1/databases/{db}/{col}/{id}`
- Delete document: `DELETE /v1/databases/{db}/{col}/{id}`

Insert returns:

```json
{ "_id": "uuid" }
```

## Query DSL

The server expects a tree in `where`, not a flat array.

```json
{
  "where": {
    "AND": [
      { "field": "age", "op": "gte", "value": 18 },
      { "field": "active", "op": "eq", "value": true }
    ]
  },
  "order_by": [
    { "field": "name", "dir": "asc" }
  ],
  "limit": 25,
  "skip": 0
}
```

## Cache

- Get: `GET /v1/cache/{key}`
- Set: `POST /v1/cache/{key}` with `{ "value": "...", "ttl": 60 }`
- Delete: `DELETE /v1/cache/{key}`

## Blob Storage

- List buckets: `GET /s3/`
- Create bucket: `PUT /s3/{bucket}`
- List objects: `GET /s3/{bucket}`
- Upload object: `PUT /s3/{bucket}/{key}`
- Download object: `GET /s3/{bucket}/{key}`
- Delete object: `DELETE /s3/{bucket}/{key}`

## Realtime

- Database events: `GET /v1/databases/{db}/realtime`
- Logs stream: `GET /v1/logs/realtime`

## Schema Sync And Migrations

- Pull schema from a live server: `voidcli schema pull --out void.prisma`
- Preview schema changes: `voidcli schema push --schema void.prisma --dry-run`
- Apply schema changes: `voidcli schema push --schema void.prisma`
- Create and apply a migration: `voidcli migrate dev --schema void.prisma --name add_users`
- Apply pending migrations from disk: `voidcli migrate deploy --dir void/migrations`
- Check migration status: `voidcli migrate status --dir void/migrations`

VoidDB uses a Prisma-like schema file with `datasource`, `generator` and `model` blocks.

## PostgreSQL Import

- Import a PostgreSQL database by URL:
  `voidcli import postgres "postgres://user:pass@host:5432/app?sslmode=disable" --database app`
- Use `--drop-existing` to recreate the target VoidDB database before import.

## Safe Agent Defaults

- Prefer `PATCH` over `PUT` unless a full replacement is intended.
- Do not assume a collection exists; create it explicitly or handle `404`.
- Use small `limit` values for exploratory queries.
- Treat `_id` as server-controlled unless the caller explicitly sets it.
