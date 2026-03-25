/**
 * @fileoverview VoidDB HTTP API client.
 * All requests are sent to NEXT_PUBLIC_API_URL (default http://localhost:7700).
 * The JWT access token is automatically attached from localStorage.
 */

import axios, { AxiosInstance, AxiosResponse } from "axios";

/** Base URL for the VoidDB API server. */
const BASE_URL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700";

// ── Types ─────────────────────────────────────────────────────────────────────

/** JWT token pair returned by /auth/login and /auth/refresh. */
export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: number;
}

/** A VoidDB user account (credentials are never returned). */
export interface User {
  id: string;
  role: "admin" | "readwrite" | "readonly";
  created_at: number;
  databases?: string[];
}

/** A single document stored in a collection. */
export interface Document {
  _id: string;
  [field: string]: unknown;
}

/** A query filter clause. */
export interface QueryFilter {
  field: string;
  op: "eq" | "ne" | "gt" | "gte" | "lt" | "lte" | "contains" | "starts_with" | "in";
  value: unknown;
}

/** A sort specification. */
export interface QuerySort {
  field: string;
  dir: "asc" | "desc";
}

/** Full query DSL sent to POST /databases/{db}/{col}/query. */
export interface QuerySpec {
  where?: QueryFilter[];
  order_by?: QuerySort[];
  limit?: number;
  skip?: number;
}

/** Query result envelope. */
export interface QueryResult {
  results: Document[];
  count: number;
}

/** Engine statistics. */
export interface EngineStats {
  memtable_size: number;
  memtable_count: number;
  segments: number;
  cache_len: number;
  cache_used: number;
  wal_seq: number;
}

/** Collection Schema Definitions */
export interface SchemaField {
  name: string;
  type: "string" | "number" | "boolean" | "datetime" | "array" | "object" | "relation";
  required?: boolean;
  default?: string;
  default_expr?: string;
  prisma_type?: string;
  unique?: boolean;
  is_id?: boolean;
  list?: boolean;
  virtual?: boolean;
  auto_updated_at?: boolean;
  mapped_name?: string;
}

export interface Schema {
  database?: string;
  collection?: string;
  model?: string;
  fields: SchemaField[];
  indexes?: Array<{
    name?: string;
    fields: string[];
    unique?: boolean;
    primary?: boolean;
  }>;
}

export interface BackupSettings {
  dir: string;
  retain: number;
  schedule_cron: string;
  next_run?: string;
}

export interface BackupFileInfo {
  name: string;
  size: number;
  modified_at: string;
}

export interface PostgresImportResult {
  database: string;
  source: string;
  schema: string;
  total_rows: number;
  total_tables: number;
  tables: Array<{ name: string; rows: number }>;
}

/** Blob object metadata. */
export interface ObjectMeta {
  bucket: string;
  key: string;
  size: number;
  content_type: string;
  etag: string;
  last_modified: string;
  metadata?: Record<string, string>;
}

function getAuthHeaders(): Record<string, string> {
  if (typeof window === "undefined") return {};
  const token = localStorage.getItem("void_access_token");
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function parseS3Xml(xml: string): globalThis.Document {
  return new DOMParser().parseFromString(xml, "application/xml");
}

function getText(node: Element, selector: string): string {
  return node.querySelector(selector)?.textContent?.trim() || "";
}

// ── Client ────────────────────────────────────────────────────────────────────

/**
 * Creates and configures an axios instance that automatically attaches
 * the stored JWT Bearer token to every request.
 */
function createAxiosInstance(): AxiosInstance {
  const instance = axios.create({ baseURL: BASE_URL, timeout: 30_000 });

  instance.interceptors.request.use((config) => {
    if (typeof window !== "undefined") {
      const token = localStorage.getItem("void_access_token");
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  });

  // Auto-refresh on 401.
  instance.interceptors.response.use(
    (res) => res,
    async (error) => {
      if (error.response?.status === 401 && typeof window !== "undefined") {
        const refresh = localStorage.getItem("void_refresh_token");
        if (refresh) {
          try {
            const res = await axios.post<TokenPair>(`${BASE_URL}/v1/auth/refresh`, {
              refresh_token: refresh,
            });
            localStorage.setItem("void_access_token", res.data.access_token);
            localStorage.setItem("void_refresh_token", res.data.refresh_token);
            error.config.headers.Authorization = `Bearer ${res.data.access_token}`;
            return instance(error.config);
          } catch {
            localStorage.removeItem("void_access_token");
            localStorage.removeItem("void_refresh_token");
            window.location.href = "/login";
          }
        }
      }
      return Promise.reject(error);
    }
  );

  return instance;
}

const http = createAxiosInstance();

// ── Auth API ──────────────────────────────────────────────────────────────────

/** Authenticates and stores tokens in localStorage. */
export async function login(username: string, password: string): Promise<TokenPair> {
  const res = await http.post<TokenPair>("/v1/auth/login", { username, password });
  localStorage.setItem("void_access_token", res.data.access_token);
  localStorage.setItem("void_refresh_token", res.data.refresh_token);
  return res.data;
}

/** Clears stored tokens. */
export function logout(): void {
  localStorage.removeItem("void_access_token");
  localStorage.removeItem("void_refresh_token");
}

/** Returns the currently authenticated user. */
export async function getMe(): Promise<User> {
  const res = await http.get<User>("/v1/auth/me");
  return res.data;
}

/** Returns true if an access token is present in localStorage. */
export function isLoggedIn(): boolean {
  if (typeof window === "undefined") return false;
  return !!localStorage.getItem("void_access_token");
}

// ── User Management ───────────────────────────────────────────────────────────

export async function listUsers(): Promise<User[]> {
  const res = await http.get<User[]>("/v1/users");
  return res.data;
}

export async function createUser(
  username: string,
  password: string,
  role: User["role"]
): Promise<{ id: string }> {
  const res = await http.post<{ id: string }>("/v1/users", { username, password, role });
  return res.data;
}

export async function deleteUser(id: string): Promise<void> {
  await http.delete(`/v1/users/${id}`);
}

// ── Database / Collection ─────────────────────────────────────────────────────

export async function listDatabases(): Promise<string[]> {
  const res = await http.get<{ databases: string[] }>("/v1/databases");
  return res.data.databases || [];
}

export async function createDatabase(name: string): Promise<void> {
  await http.post("/v1/databases", { name });
}

export async function deleteDatabase(name: string): Promise<void> {
  await http.delete(`/v1/databases/${encodeURIComponent(name)}`);
}

export async function listCollections(db: string): Promise<string[]> {
  const res = await http.get<{ collections: string[] }>(`/v1/databases/${db}/collections`);
  return res.data.collections || [];
}

export async function createCollection(db: string, name: string): Promise<void> {
  await http.post(`/v1/databases/${db}/collections`, { name });
}

export async function deleteCollection(db: string, name: string): Promise<void> {
  await http.delete(`/v1/databases/${encodeURIComponent(db)}/collections/${encodeURIComponent(name)}`);
}

export async function getSchema(db: string, col: string): Promise<Schema> {
  const res = await http.get<Schema>(`/v1/databases/${db}/${col}/schema`);
  return res.data;
}

export async function setSchema(db: string, col: string, schema: Schema): Promise<Schema> {
  const res = await http.put<Schema>(`/v1/databases/${db}/${col}/schema`, schema);
  return res.data;
}

export async function exportDatabaseSchema(db: string): Promise<string> {
  const res = await http.get<string>(`/v1/databases/${encodeURIComponent(db)}/schema/export`, {
    responseType: "text" as any,
  });
  return res.data;
}

export async function exportCollectionSchema(db: string, col: string): Promise<string> {
  const res = await http.get<string>(`/v1/databases/${encodeURIComponent(db)}/${encodeURIComponent(col)}/schema/export`, {
    responseType: "text" as any,
  });
  return res.data;
}

export async function getBackupSettings(): Promise<BackupSettings> {
  const res = await http.get<BackupSettings>("/v1/settings/backup");
  return res.data;
}

export async function updateBackupSettings(settings: BackupSettings): Promise<BackupSettings> {
  const res = await http.put<BackupSettings>("/v1/settings/backup", settings);
  return res.data;
}

export async function listBackupFiles(): Promise<BackupFileInfo[]> {
  const res = await http.get<{ files: BackupFileInfo[] }>("/v1/backups");
  return res.data.files || [];
}

export async function createBackupFile(databases?: string[]): Promise<BackupFileInfo> {
  const res = await http.post<BackupFileInfo>("/v1/backups", { databases: databases || [] });
  return res.data;
}

export async function deleteBackupFile(name: string): Promise<void> {
  await http.delete(`/v1/backups/${encodeURIComponent(name)}`);
}

export async function downloadBackupFile(name: string): Promise<void> {
  const res = await http.get<Blob>(`/v1/backups/${encodeURIComponent(name)}`, {
    responseType: "blob",
  });
  const blob = new Blob([res.data], { type: "application/octet-stream" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export async function importPostgresDatabase(payload: {
  source_url: string;
  target_database?: string;
  source_schema?: string;
  drop_existing?: boolean;
}): Promise<PostgresImportResult> {
  const res = await http.post<PostgresImportResult>("/v1/import/postgres", payload);
  return res.data;
}

// ── Documents ─────────────────────────────────────────────────────────────────

export async function insertDocument(
  db: string,
  col: string,
  doc: Record<string, unknown>
): Promise<{ _id: string }> {
  const res = await http.post<{ _id: string }>(`/v1/databases/${db}/${col}`, doc);
  return res.data;
}

export async function getDocument(db: string, col: string, id: string): Promise<Document> {
  const res = await http.get<Document>(`/v1/databases/${db}/${col}/${id}`);
  return res.data;
}

export async function updateDocument(
  db: string,
  col: string,
  id: string,
  doc: Record<string, unknown>
): Promise<void> {
  await http.put(`/v1/databases/${db}/${col}/${id}`, doc);
}

export async function patchDocument(
  db: string,
  col: string,
  id: string,
  patch: Record<string, unknown>
): Promise<Document> {
  const res = await http.patch<Document>(`/v1/databases/${db}/${col}/${id}`, patch);
  return res.data;
}

export async function deleteDocument(db: string, col: string, id: string): Promise<void> {
  await http.delete(`/v1/databases/${db}/${col}/${id}`);
}

export async function queryDocuments(
  db: string,
  col: string,
  query: QuerySpec
): Promise<QueryResult> {
  const res = await http.post<QueryResult>(`/v1/databases/${db}/${col}/query`, query);
  return res.data;
}

export async function countDocuments(db: string, col: string): Promise<number> {
  const res = await http.get<{ count: number }>(`/v1/databases/${db}/${col}/count`);
  return res.data.count;
}

// ── Engine Stats ──────────────────────────────────────────────────────────────

export async function getStats(): Promise<EngineStats> {
  const res = await http.get<EngineStats>("/v1/stats");
  return res.data;
}

// ── Blob / S3 API ─────────────────────────────────────────────────────────────

export async function listBuckets(): Promise<string[]> {
  const res = await axios.get<string>(`${BASE_URL}/s3/`, {
    headers: getAuthHeaders(),
    responseType: "text",
  });
  const xml = parseS3Xml(res.data);
  return Array.from(xml.querySelectorAll("Bucket > Name"))
    .map((node) => node.textContent?.trim() || "")
    .filter(Boolean);
}

export async function listObjects(bucket: string, prefix = ""): Promise<ObjectMeta[]> {
  const res = await axios.get<string>(`${BASE_URL}/s3/${encodeURIComponent(bucket)}`, {
    headers: getAuthHeaders(),
    params: prefix ? { prefix } : undefined,
    responseType: "text",
  });
  const xml = parseS3Xml(res.data);
  return Array.from(xml.querySelectorAll("Contents")).map((entry) => ({
    bucket,
    key: getText(entry, "Key"),
    size: Number(getText(entry, "Size") || "0"),
    content_type: "application/octet-stream",
    etag: getText(entry, "ETag").replaceAll('"', ""),
    last_modified: getText(entry, "LastModified"),
    metadata: {},
  }));
}

export async function createBucket(bucket: string): Promise<void> {
  await http.put(`/s3/${bucket}`);
}

export async function uploadObject(
  bucket: string,
  key: string,
  file: File
): Promise<ObjectMeta> {
  const res = await http.put<ObjectMeta>(
    `/s3/${bucket}/${key}`,
    file,
    {
      headers: {
        "Content-Type": file.type || "application/octet-stream",
        "Content-Length": String(file.size),
      },
    }
  );
  return res.data;
}

export async function deleteObject(bucket: string, key: string): Promise<void> {
  await http.delete(`/s3/${bucket}/${key}`);
}

export function getObjectUrl(bucket: string, key: string): string {
  const token = typeof window !== "undefined"
    ? localStorage.getItem("void_access_token") ?? ""
    : "";
  return `${BASE_URL}/s3/${bucket}/${key}?token=${encodeURIComponent(token)}`;
}
