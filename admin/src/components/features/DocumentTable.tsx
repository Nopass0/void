"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import {
  Trash2,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Loader2,
  FileText,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
  Check,
  X,
  Pencil,
  Copy,
  ClipboardCopy,
  Plus,
  CalendarDays,
  ExternalLink,
  Save,
  Undo2,
  Binary,
  Braces,
  Type,
  ToggleLeft,
} from "lucide-react";
import { toast } from "sonner";
import { cn, truncate, formatNumber } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";

type EditableKind = "readonly" | "text" | "number" | "boolean" | "datetime" | "json" | "blob";
type DraftMap = Record<string, Record<string, unknown>>;

interface DocumentTableProps {
  onEditDoc?: (doc: Document) => void;
  refreshToken?: number;
}

interface JsonEditorState {
  docId: string;
  field: string;
  initialValue: unknown;
}

interface InlineCellProps {
  value: unknown;
  field: string;
  schemaField?: api.SchemaField;
  dirty: boolean;
  relationTarget: string | null;
  onCommit: (value: unknown) => void;
  onNavigateRelation: () => void;
  onOpenJsonEditor: () => void;
}

const PAGE_SIZES = [10, 25, 50, 100];

function storageFieldName(field: api.SchemaField): string {
  if (field.mapped_name) return field.mapped_name;
  if (field.is_id) return "_id";
  return field.name;
}

function fieldIcon(kind: EditableKind) {
  switch (kind) {
    case "number":
      return <Binary className="w-3 h-3" />;
    case "boolean":
      return <ToggleLeft className="w-3 h-3" />;
    case "datetime":
      return <CalendarDays className="w-3 h-3" />;
    case "json":
      return <Braces className="w-3 h-3" />;
    case "blob":
      return <ExternalLink className="w-3 h-3" />;
    default:
      return <Type className="w-3 h-3" />;
  }
}

function getColumnStyle(col: string): React.CSSProperties {
  const normalized = col.toLowerCase();
  if (col === "_id") return { width: "18rem" };
  if (normalized.includes("created") || normalized.includes("updated") || normalized.includes("date") || normalized.includes("time")) {
    return { width: "12rem" };
  }
  if (normalized.includes("name") || normalized.includes("title") || normalized.includes("email")) {
    return { width: "14rem" };
  }
  if (normalized.includes("status") || normalized.includes("type") || normalized.includes("role")) {
    return { width: "10rem" };
  }
  return { width: "12rem" };
}

function getColumnWidth(col: string): string {
  return String(getColumnStyle(col).width ?? "12rem");
}

function fieldForColumn(field: string, schema: api.Schema | null): api.SchemaField | undefined {
  if (!schema) return undefined;
  if (field === "_id") {
    return schema.fields.find((item) => item.is_id || storageFieldName(item) === "_id");
  }
  return schema.fields.find((item) => item.name === field || storageFieldName(item) === field);
}

function editorKind(field: string, schemaField: api.SchemaField | undefined, value: unknown): EditableKind {
  if (field === "_id") return "readonly";
  const type = schemaField?.type;
  if (type === "blob") return "blob";
  if (type === "number" || typeof value === "number") return "number";
  if (type === "boolean" || typeof value === "boolean") return "boolean";
  if (type === "datetime") return "datetime";
  if (type === "object" || type === "array" || (typeof value === "object" && value !== null)) return "json";
  return "text";
}

function datetimeToInput(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function inputToDatetime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toISOString();
}

function valueToText(value: unknown, kind: EditableKind): string {
  if (value === null || value === undefined) return "";
  if (kind === "json") {
    return JSON.stringify(value, null, 2);
  }
  if (kind === "datetime" && typeof value === "string") {
    return datetimeToInput(value);
  }
  return String(value);
}

function coerceEditedValue(text: string, kind: EditableKind): unknown {
  switch (kind) {
    case "number": {
      if (text.trim() === "") return null;
      const num = Number(text);
      return Number.isNaN(num) ? null : num;
    }
    case "boolean":
      if (text === "") return null;
      return text === "true";
    case "datetime":
      return inputToDatetime(text);
    case "json":
      return JSON.parse(text);
    default:
      return text;
  }
}

function serializeValue(value: unknown): string {
  if (value === undefined) return "undefined";
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function isBlobRef(value: unknown): value is api.BlobRef {
  return !!value && typeof value === "object" && "_blob_bucket" in value && "_blob_key" in value;
}

function CellValue({ value }: { value: unknown }) {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic text-xs">null</span>;
  }
  if (isBlobRef(value)) {
    const label = value._blob_key || `${value._blob_bucket}`;
    return (
      <div className="flex items-center gap-2 min-w-0">
        <span className="block truncate text-xs font-mono text-cyan-400" title={label}>
          {truncate(label, 48)}
        </span>
        {value._blob_url && (
          <a
            href={value._blob_url}
            target="_blank"
            rel="noreferrer"
            className="shrink-0 text-neon-500 hover:text-neon-400"
            onClick={(event) => event.stopPropagation()}
            title="Open file"
          >
            <ExternalLink className="w-3.5 h-3.5" />
          </a>
        )}
      </div>
    );
  }
  if (typeof value === "boolean") {
    return (
      <span className={cn("text-xs font-mono", value ? "text-neon-500" : "text-amber-400")}>
        {String(value)}
      </span>
    );
  }
  if (typeof value === "number") {
    return <span className="text-xs font-mono text-blue-400">{value}</span>;
  }
  if (typeof value === "object") {
    const short = JSON.stringify(value);
    return (
      <span className="block truncate text-xs font-mono text-amber-400" title={short}>
        {truncate(short, 72)}
      </span>
    );
  }
  return (
    <span className="block truncate text-xs text-foreground" title={String(value)}>
      {String(value)}
    </span>
  );
}

function guessRelationTarget(
  field: string,
  schemaField: api.SchemaField | undefined,
  collections: string[]
): string | null {
  const candidates: string[] = [];

  if (schemaField?.relation?.model) {
    candidates.push(schemaField.relation.model);
    candidates.push(schemaField.relation.model.toLowerCase());
  }

  const stripped = field.replace(/_id$/i, "").replace(/Id$/i, "");
  if (stripped && stripped !== field) {
    candidates.push(stripped);
    candidates.push(stripped.toLowerCase());
    candidates.push(`${stripped}s`);
    candidates.push(`${stripped.toLowerCase()}s`);
    if (stripped.endsWith("y")) {
      candidates.push(`${stripped.slice(0, -1)}ies`);
      candidates.push(`${stripped.slice(0, -1).toLowerCase()}ies`);
    }
  }

  const existing = new Set(collections);
  for (const candidate of candidates) {
    if (existing.has(candidate)) return candidate;
  }
  return null;
}

function JsonCellEditor({
  state,
  onClose,
  onSave,
}: {
  state: JsonEditorState;
  onClose: () => void;
  onSave: (value: unknown) => void;
}) {
  const [text, setText] = useState(JSON.stringify(state.initialValue ?? {}, null, 2));

  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
    >
      <motion.div
        initial={{ opacity: 0, scale: 0.96, y: 8 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        exit={{ opacity: 0, scale: 0.96, y: 8 }}
        onClick={(event) => event.stopPropagation()}
        className="w-full max-w-2xl rounded-xl border border-border bg-surface-2 shadow-modal overflow-hidden"
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground">JSON Cell Editor</h3>
            <p className="text-xs text-muted-foreground">
              {state.docId} • {state.field}
            </p>
          </div>
          <button onClick={onClose} className="btn-ghost !p-1.5">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5">
          <textarea
            value={text}
            onChange={(event) => setText(event.target.value)}
            className="input-field min-h-[22rem] resize-y font-mono text-xs leading-6"
          />
        </div>

        <div className="flex justify-end gap-3 border-t border-border px-5 py-3">
          <button onClick={onClose} className="btn-ghost text-sm">
            Cancel
          </button>
          <button
            onClick={() => {
              try {
                onSave(JSON.parse(text));
              } catch {
                toast.error("Invalid JSON");
              }
            }}
            className="btn-primary text-sm"
          >
            <Save className="w-4 h-4" />
            Apply
          </button>
        </div>
      </motion.div>
    </motion.div>
  );
}

function InlineCell({
  value,
  field,
  schemaField,
  dirty,
  relationTarget,
  onCommit,
  onNavigateRelation,
  onOpenJsonEditor,
}: InlineCellProps) {
  const kind = editorKind(field, schemaField, value);
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(valueToText(value, kind));

  useEffect(() => {
    setText(valueToText(value, kind));
  }, [kind, value]);

  const beginEdit = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (kind === "readonly") return;
    if (kind === "blob") {
      if (isBlobRef(value) && value._blob_url) {
        window.open(value._blob_url, "_blank", "noopener,noreferrer");
      }
      return;
    }
    if (kind === "json") {
      onOpenJsonEditor();
      return;
    }
    setText(valueToText(value, kind));
    setEditing(true);
  };

  const commit = () => {
    try {
      onCommit(coerceEditedValue(text, kind));
      setEditing(false);
    } catch {
      toast.error(kind === "json" ? "Invalid JSON" : "Invalid value");
    }
  };

  const cancel = () => {
    setText(valueToText(value, kind));
    setEditing(false);
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement | HTMLSelectElement>) => {
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
    }
    if (event.key === "Escape") {
      event.preventDefault();
      cancel();
    }
  };

  const relationLink =
    relationTarget && typeof value === "string" && value ? (
      <button
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          onNavigateRelation();
        }}
        className="opacity-0 transition-opacity group-hover:opacity-100 text-neon-500 hover:text-neon-400"
        title={`Open ${relationTarget}`}
      >
        <ExternalLink className="w-3.5 h-3.5" />
      </button>
    ) : null;

  if (editing) {
    return (
      <div className="flex min-w-0 items-center gap-2 rounded-md border border-neon-500/25 bg-surface-1/90 px-2 py-1.5 shadow-modal">
        <span className="shrink-0 text-neon-500">{fieldIcon(kind)}</span>

        {kind === "boolean" ? (
          <select
            autoFocus
            value={text}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={handleKeyDown}
            className="select-field text-xs !py-1.5"
          >
            <option value="">unset</option>
            <option value="true">true</option>
            <option value="false">false</option>
          </select>
        ) : (
          <input
            autoFocus
            type={kind === "number" ? "number" : kind === "datetime" ? "datetime-local" : "text"}
            value={text}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={handleKeyDown}
            className="cell-edit-input"
          />
        )}

        <button type="button" onMouseDown={commit} className="text-neon-500 hover:text-neon-400 shrink-0">
          <Check className="w-3.5 h-3.5" />
        </button>
        <button type="button" onMouseDown={cancel} className="text-muted-foreground hover:text-foreground shrink-0">
          <X className="w-3.5 h-3.5" />
        </button>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={beginEdit}
      className={cn(
        "group flex w-full min-w-0 items-center justify-between gap-2 rounded-md px-1 py-0.5 text-left transition-colors",
        kind !== "readonly" && kind !== "blob" && "hover:bg-surface-3/80",
        dirty && "bg-neon-500/10"
      )}
    >
      <div className="min-w-0 flex-1">
        <CellValue value={value} />
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {dirty && <span className="h-2 w-2 rounded-full bg-neon-500/80" />}
        {relationLink}
        {kind === "json" && (
          <span className="opacity-0 transition-opacity group-hover:opacity-100 text-muted-foreground">
            <Pencil className="w-3.5 h-3.5" />
          </span>
        )}
      </div>
    </button>
  );
}

export function DocumentTable({ onEditDoc, refreshToken = 0 }: DocumentTableProps) {
  const {
    activeDb,
    activeCol,
    activeSchema,
    collections,
    setActiveDb,
    setActiveCol,
    setActiveTab,
    setQueryText,
    documents,
    docCount,
    dataLoading,
    setDocuments,
    setDataLoading,
    queryText,
    page,
    setPage,
    pageSize,
    setPageSize,
    sortBy,
    setSortBy,
    selectedIds,
    toggleSelectedId,
    clearSelectedIds,
    setSelectedIds,
  } = useStore();

  const [drafts, setDrafts] = useState<DraftMap>({});
  const [jsonEditor, setJsonEditor] = useState<JsonEditorState | null>(null);

  useEffect(() => {
    setDrafts({});
    setJsonEditor(null);
    clearSelectedIds();
  }, [activeDb, activeCol, clearSelectedIds]);

  const fetchDocs = useCallback(async () => {
    if (!activeDb || !activeCol) return;
    setDataLoading(true);
    try {
      let spec: api.QuerySpec = {};
      try {
        spec = JSON.parse(queryText);
      } catch {
        toast.error("Invalid query JSON");
        return;
      }
      spec.limit = pageSize;
      spec.skip = page * pageSize;
      if (sortBy) {
        spec.order_by = [{ field: sortBy.field, dir: sortBy.dir }];
      }
      const result = await api.queryDocuments(activeDb, activeCol, spec);
      setDocuments(result.results, Number(result.count ?? result.results.length));
      clearSelectedIds();
    } catch {
      toast.error("Failed to fetch documents");
    } finally {
      setDataLoading(false);
    }
  }, [activeCol, activeDb, clearSelectedIds, page, pageSize, queryText, setDataLoading, setDocuments, sortBy]);

  useEffect(() => {
    fetchDocs();
  }, [fetchDocs, refreshToken]);

  const mergedDocuments = useMemo(
    () =>
      documents.map((doc) => ({
        ...doc,
        ...(drafts[doc._id] ?? {}),
        _id: doc._id,
      })),
    [documents, drafts]
  );

  const columns = useMemo<string[]>(() => {
    const cols = new Set<string>(["_id"]);
    for (const field of activeSchema?.fields ?? []) {
      if (storageFieldName(field) === "_id") continue;
      cols.add(field.name);
    }
    for (const doc of documents) {
      for (const key of Object.keys(doc)) {
        cols.add(key);
      }
    }
    return Array.from(cols).slice(0, 20);
  }, [activeSchema, documents]);

  const gridTemplateColumns = useMemo(
    () => ["2.5rem", ...columns.map(getColumnWidth), "2.5rem"].join(" "),
    [columns]
  );

  const totalPages = Math.max(1, Math.ceil(docCount / pageSize));
  const startRow = docCount === 0 ? 0 : page * pageSize + 1;
  const endRow = Math.min((page + 1) * pageSize, docCount);

  const dirtyRows = useMemo(
    () => Object.entries(drafts).filter(([, patch]) => Object.keys(patch).length > 0).map(([id]) => id),
    [drafts]
  );

  const setDraftValue = (docId: string, field: string, value: unknown, originalValue: unknown) => {
    setDrafts((current) => {
      const next = { ...current };
      const currentPatch = { ...(next[docId] ?? {}) };
      if (serializeValue(value) === serializeValue(originalValue)) {
        delete currentPatch[field];
      } else {
        currentPatch[field] = value;
      }
      if (Object.keys(currentPatch).length === 0) {
        delete next[docId];
      } else {
        next[docId] = currentPatch;
      }
      return next;
    });
  };

  const discardDrafts = () => {
    setDrafts({});
    toast.success("Pending edits discarded");
  };

  const saveDrafts = async () => {
    if (!activeDb || !activeCol || dirtyRows.length === 0) return;
    try {
      for (const docId of dirtyRows) {
        const patch = drafts[docId];
        if (!patch || Object.keys(patch).length === 0) continue;
        await api.patchDocument(activeDb, activeCol, docId, patch);
      }
      toast.success(`Saved ${dirtyRows.length} row${dirtyRows.length !== 1 ? "s" : ""}`);
      setDrafts({});
      await fetchDocs();
    } catch {
      toast.error("Failed to save pending edits");
    }
  };

  const handleDelete = async (doc: Document, event: React.MouseEvent) => {
    event.stopPropagation();
    if (!activeDb || !activeCol) return;
    if (!window.confirm(`Delete document ${doc._id}?`)) return;
    try {
      await api.deleteDocument(activeDb, activeCol, doc._id);
      toast.success("Document deleted");
      setDrafts((current) => {
        const next = { ...current };
        delete next[doc._id];
        return next;
      });
      fetchDocs();
    } catch {
      toast.error("Failed to delete document");
    }
  };

  const handleBulkDelete = async () => {
    if (!activeDb || !activeCol || selectedIds.size === 0) return;
    if (!window.confirm(`Delete ${selectedIds.size} document(s)?`)) return;
    try {
      for (const id of Array.from(selectedIds)) {
        await api.deleteDocument(activeDb, activeCol, id);
      }
      toast.success(`${selectedIds.size} document(s) deleted`);
      clearSelectedIds();
      fetchDocs();
    } catch {
      toast.error("Failed to delete some documents");
    }
  };

  const toggleSort = (field: string) => {
    if (!sortBy || sortBy.field !== field) {
      setSortBy({ field, dir: "asc" });
    } else if (sortBy.dir === "asc") {
      setSortBy({ field, dir: "desc" });
    } else {
      setSortBy(null);
    }
    setPage(0);
  };

  const allSelected = mergedDocuments.length > 0 && mergedDocuments.every((doc) => selectedIds.has(doc._id));

  const toggleSelectAll = () => {
    if (allSelected) {
      clearSelectedIds();
    } else {
      setSelectedIds(new Set(mergedDocuments.map((doc) => doc._id)));
    }
  };

  const handleAddColumn = () => {
    const name = window.prompt("Column name");
    if (!name || !activeDb || !activeCol || !activeSchema) return;
    const type = window.prompt("Type (string, number, boolean, datetime, object, array, blob)", "string") as api.SchemaField["type"] | null;
    if (!type) return;
    const nextSchema: api.Schema = {
      ...activeSchema,
      fields: [...(activeSchema.fields ?? []), { name, type }],
    };
    useStore.getState().setActiveSchema(nextSchema);
    api
      .setSchema(activeDb, activeCol, nextSchema)
      .then(() => toast.success(`Column ${name} added`))
      .catch(() => toast.error("Failed to add column"));
  };

  const navigateRelation = (targetCol: string, id: string) => {
    if (!activeDb) return;
    setActiveDb(activeDb);
    setActiveCol(targetCol);
    setActiveTab("data");
    setPage(0);
    setQueryText(
      JSON.stringify(
        {
          where: {
            field: "_id",
            op: "eq",
            value: id,
          },
        },
        null,
        2
      )
    );
    toast.success(`Opened ${targetCol} filtered by ${id}`);
  };

  if (!activeDb || !activeCol) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3 py-20 text-muted-foreground">
        <FileText className="w-12 h-12 opacity-20" />
        <p className="text-sm">Select a collection from the sidebar</p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col gap-3">
      <AnimatePresence>
        {dirtyRows.length > 0 && (
          <motion.div
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            className="sticky top-0 z-20 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-neon-500/25 bg-surface-2/95 px-4 py-3 shadow-modal backdrop-blur"
          >
            <div className="flex items-center gap-3">
              <span className="metric-badge">{dirtyRows.length} changed row{dirtyRows.length !== 1 ? "s" : ""}</span>
              <p className="text-sm text-muted-foreground">
                Edited rows are highlighted until you save or discard them.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button onClick={discardDrafts} className="btn-ghost text-sm">
                <Undo2 className="w-4 h-4" />
                Discard
              </button>
              <button onClick={saveDrafts} className="btn-primary text-sm">
                <Save className="w-4 h-4" />
                Save Changes
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">
            {formatNumber(docCount)} document{docCount !== 1 ? "s" : ""}
          </span>
          {selectedIds.size > 0 && (
            <button onClick={handleBulkDelete} className="btn-danger text-xs py-1 px-2">
              <Trash2 className="w-3 h-3" />
              Delete {selectedIds.size} selected
            </button>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button onClick={handleAddColumn} className="btn-ghost" title="Add Column">
            <Plus className="w-3.5 h-3.5" />
          </button>
          <button onClick={fetchDocs} disabled={dataLoading} className="btn-ghost">
            <RefreshCw className={cn("w-3.5 h-3.5", dataLoading && "animate-spin")} />
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-auto rounded-lg border border-border bg-surface-2">
        {dataLoading ? (
          <div className="flex h-40 items-center justify-center">
            <Loader2 className="w-5 h-5 animate-spin text-neon-500" />
          </div>
        ) : mergedDocuments.length === 0 && columns.length <= 1 ? (
          <div className="flex h-40 flex-col items-center justify-center gap-2 text-muted-foreground">
            <FileText className="w-8 h-8 opacity-20" />
            <p className="text-sm">No documents found</p>
            <p className="text-xs">Try changing filters or inserting documents</p>
          </div>
        ) : (
          <div className="min-w-max">
            <div
              className="sticky top-0 z-10 grid border-b border-border bg-surface-1"
              style={{ gridTemplateColumns }}
            >
              <div className="flex items-center justify-center px-2 py-2.5">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={toggleSelectAll}
                  className="w-3.5 h-3.5 rounded border-border accent-neon-500 cursor-pointer"
                />
              </div>

              {columns.map((column) => (
                <button
                  key={column}
                  type="button"
                  className="group flex items-center gap-1.5 px-3 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-muted-foreground hover:text-foreground"
                  onClick={() => toggleSort(column)}
                >
                  <span>{column}</span>
                  {sortBy?.field === column ? (
                    sortBy.dir === "asc" ? (
                      <ArrowUp className="w-3 h-3 text-neon-500" />
                    ) : (
                      <ArrowDown className="w-3 h-3 text-neon-500" />
                    )
                  ) : (
                    <ArrowUpDown className="w-3 h-3 opacity-0 transition-opacity group-hover:opacity-40" />
                  )}
                </button>
              ))}

              <div className="px-2 py-2.5" />
            </div>

            <AnimatePresence initial={false}>
              {mergedDocuments.map((doc, index) => {
                const originalDoc = documents.find((item) => item._id === doc._id) ?? doc;
                const rowMenuItems: ContextMenuEntry[] = [
                  {
                    label: "Edit JSON",
                    icon: <Pencil className="w-3.5 h-3.5" />,
                    onClick: () => onEditDoc?.(originalDoc),
                    shortcut: "Enter",
                  },
                  {
                    label: "Copy JSON",
                    icon: <Copy className="w-3.5 h-3.5" />,
                    onClick: () => {
                      navigator.clipboard.writeText(JSON.stringify(doc, null, 2));
                      toast.success("JSON copied");
                    },
                  },
                  {
                    label: "Copy ID",
                    icon: <ClipboardCopy className="w-3.5 h-3.5" />,
                    onClick: () => {
                      navigator.clipboard.writeText(doc._id);
                      toast.success("ID copied");
                    },
                  },
                  { separator: true },
                  {
                    label: "Delete",
                    icon: <Trash2 className="w-3.5 h-3.5" />,
                    danger: true,
                    onClick: () => {
                      if (activeDb && activeCol && window.confirm(`Delete ${doc._id}?`)) {
                        api.deleteDocument(activeDb, activeCol, doc._id).then(() => {
                          toast.success("Deleted");
                          fetchDocs();
                        });
                      }
                    },
                    shortcut: "Del",
                  },
                ];

                return (
                  <ContextMenu key={doc._id} items={rowMenuItems}>
                    <motion.div
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={{ opacity: 0 }}
                      transition={{ delay: index * 0.01 }}
                      className={cn(
                        "group grid border-b border-border/50 transition-colors hover:bg-surface-3/70",
                        selectedIds.has(doc._id) && "bg-neon-500/5",
                        drafts[doc._id] && "bg-neon-500/[0.07]"
                      )}
                      style={{ gridTemplateColumns }}
                    >
                      <div className="flex items-center justify-center px-2 py-2" onClick={(event) => event.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={selectedIds.has(doc._id)}
                          onChange={() => toggleSelectedId(doc._id)}
                          className="w-3.5 h-3.5 rounded border-border accent-neon-500 cursor-pointer"
                        />
                      </div>

                      {columns.map((column) => {
                        const schemaField = fieldForColumn(column, activeSchema);
                        const actualFieldKey = schemaField ? storageFieldName(schemaField) : column;
                        const relationTarget = guessRelationTarget(column, schemaField, collections[activeDb] ?? []);
                        const currentValue = doc[actualFieldKey] ?? doc[column];
                        const originalValue = originalDoc[actualFieldKey] ?? originalDoc[column];
                        const fieldDirty = !!drafts[doc._id]?.hasOwnProperty(actualFieldKey);

                        return (
                          <div key={column} className="px-3 py-2 text-sm">
                            <InlineCell
                              value={currentValue}
                              field={column}
                              schemaField={schemaField}
                              dirty={fieldDirty}
                              relationTarget={relationTarget}
                              onCommit={(nextValue) => setDraftValue(doc._id, actualFieldKey, nextValue, originalValue)}
                              onNavigateRelation={() => {
                                if (relationTarget && typeof currentValue === "string") {
                                  navigateRelation(relationTarget, currentValue);
                                }
                              }}
                              onOpenJsonEditor={() =>
                                setJsonEditor({
                                  docId: doc._id,
                                  field: actualFieldKey,
                                  initialValue: currentValue,
                                })
                              }
                            />
                          </div>
                        );
                      })}

                      <div className="flex items-center justify-center px-2 py-2">
                        <button
                          onClick={(event) => handleDelete(originalDoc, event)}
                          className="opacity-0 transition-opacity text-muted-foreground hover:text-red-400 group-hover:opacity-100"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </motion.div>
                  </ContextMenu>
                );
              })}
            </AnimatePresence>
          </div>
        )}
      </div>

      <div className="pagination-bar">
        <div className="flex items-center gap-3">
          <span className="text-xs">
            Rows {startRow}–{endRow} of {formatNumber(docCount)}
          </span>
          <select
            value={pageSize}
            onChange={(event) => {
              setPageSize(Number(event.target.value));
              setPage(0);
            }}
            className="select-field text-xs !py-1 !px-2 !w-auto"
          >
            {PAGE_SIZES.map((size) => (
              <option key={size} value={size}>
                {size} / page
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage(0)}
            disabled={page === 0}
            className="btn-ghost !p-1 disabled:opacity-30"
          >
            <ChevronsLeft className="w-4 h-4" />
          </button>
          <button
            onClick={() => setPage(Math.max(0, page - 1))}
            disabled={page === 0}
            className="btn-ghost !p-1 disabled:opacity-30"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
          <span className="text-xs px-2 font-medium">
            {page + 1} / {totalPages}
          </span>
          <button
            onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
            disabled={page >= totalPages - 1}
            className="btn-ghost !p-1 disabled:opacity-30"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
          <button
            onClick={() => setPage(totalPages - 1)}
            disabled={page >= totalPages - 1}
            className="btn-ghost !p-1 disabled:opacity-30"
          >
            <ChevronsRight className="w-4 h-4" />
          </button>
        </div>
      </div>

      <AnimatePresence>
        {jsonEditor && (
          <JsonCellEditor
            state={jsonEditor}
            onClose={() => setJsonEditor(null)}
            onSave={(value) => {
              const originalDoc = documents.find((item) => item._id === jsonEditor.docId);
              setDraftValue(jsonEditor.docId, jsonEditor.field, value, originalDoc?.[jsonEditor.field]);
              setJsonEditor(null);
            }}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
