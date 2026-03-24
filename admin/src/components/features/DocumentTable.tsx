/**
 * @fileoverview DocumentTable – a professional data table for documents.
 * Features: column sorting, row selection, inline editing, improved pagination,
 * column type indicators, and expandable objects.
 */

"use client";

import React, { useEffect, useCallback, useMemo, useState } from "react";
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
} from "lucide-react";
import { toast } from "sonner";
import { cn, truncate, formatNumber } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";

// ── Cell renderer ─────────────────────────────────────────────────────────────

function CellValue({ value }: { value: unknown }) {
  const [expanded, setExpanded] = useState(false);

  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic text-xs">null</span>;
  }
  if (typeof value === "boolean") {
    return (
      <span className={cn("text-xs font-mono", value ? "text-neon-500" : "text-red-400")}>
        {String(value)}
      </span>
    );
  }
  if (typeof value === "number") {
    return <span className="text-xs font-mono text-blue-400">{value}</span>;
  }
  if (typeof value === "object") {
    const json = JSON.stringify(value, null, 2);
    const short = JSON.stringify(value);
    if (short.length <= 50) {
      return <span className="block truncate text-xs font-mono text-amber-400">{short}</span>;
    }
    return (
      <span className="text-xs font-mono text-amber-400">
        {expanded ? (
          <span className="block max-w-[36rem] whitespace-pre-wrap break-words cursor-pointer" onClick={() => setExpanded(false)}>
            {json}
          </span>
        ) : (
          <span className="block truncate cursor-pointer hover:text-amber-300" onClick={() => setExpanded(true)}>
            {truncate(short, 50)}
          </span>
        )}
      </span>
    );
  }
  return (
    <span className="block truncate text-xs text-foreground" title={String(value)}>{String(value)}</span>
  );
}

// ── Inline edit cell ──────────────────────────────────────────────────────────

interface InlineCellProps {
  value: unknown;
  field: string;
  docId: string;
  db: string;
  col: string;
  onSaved: () => void;
}

function InlineCell({ value, field, docId, db, col, onSaved }: InlineCellProps) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");

  const startEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (field === "_id") return;
    const val = typeof value === "object" ? JSON.stringify(value) : String(value ?? "");
    setText(val);
    setEditing(true);
  };

  const cancel = () => setEditing(false);

  const save = async () => {
    try {
      let parsed: unknown;
      try { parsed = JSON.parse(text); } catch { parsed = text; }
      await api.patchDocument(db, col, docId, { [field]: parsed });
      toast.success("Cell updated");
      setEditing(false);
      onSaved();
    } catch {
      toast.error("Failed to update");
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); save(); }
    if (e.key === "Escape") cancel();
  };

  if (editing) {
    // Determine input type from schema
    const schemaField = useStore.getState().activeSchema?.fields?.find(f => f.name === field);
    const inputType = schemaField?.type === "datetime" ? "datetime-local" : 
                      schemaField?.type === "boolean" ? "checkbox" : "text";

    return (
      <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
        {inputType === "checkbox" ? (
          <input
            autoFocus
            type="checkbox"
            checked={text === "true"}
            onChange={(e) => setText(String(e.target.checked))}
            onKeyDown={handleKeyDown}
            className="w-4 h-4 rounded border-border accent-neon-500"
          />
        ) : (
          <input
            autoFocus
            type={inputType}
            value={inputType === "datetime-local" && text.length > 16 ? text.slice(0, 16) : text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={cancel}
            className="cell-edit-input"
          />
        )}
        <button onMouseDown={save} className="text-neon-500 hover:text-neon-400 shrink-0">
          <Check className="w-3 h-3" />
        </button>
        <button onMouseDown={cancel} className="text-muted-foreground hover:text-foreground shrink-0">
          <X className="w-3 h-3" />
        </button>
      </div>
    );
  }

  return (
    <div
      className={cn(field !== "_id" && "cell-editable")}
      onDoubleClick={startEdit}
    >
      <CellValue value={value} />
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

interface DocumentTableProps {
  onEditDoc?: (doc: Document) => void;
}

const PAGE_SIZES = [10, 25, 50, 100];

function getColumnStyle(col: string): React.CSSProperties {
  const normalized = col.toLowerCase();
  if (col === "_id") return { width: "22rem" };
  if (normalized.includes("created") || normalized.includes("updated") || normalized.includes("date")) {
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

export function DocumentTable({ onEditDoc }: DocumentTableProps) {
  const {
    activeDb, activeCol, activeSchema,
    documents, docCount, dataLoading,
    setDocuments, setDataLoading,
    queryText,
    page, setPage, pageSize, setPageSize,
    sortBy, setSortBy,
    selectedIds, toggleSelectedId, clearSelectedIds, setSelectedIds,
  } = useStore();

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
      setDocuments(result.results, result.count);
      clearSelectedIds();
    } catch {
      toast.error("Failed to fetch documents");
    } finally {
      setDataLoading(false);
    }
  }, [activeDb, activeCol, queryText, page, pageSize, sortBy, setDocuments, setDataLoading, clearSelectedIds]);

  useEffect(() => { fetchDocs(); }, [activeDb, activeCol, page, pageSize, sortBy]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleDelete = async (doc: Document, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!activeDb || !activeCol) return;
    if (!confirm(`Delete document ${doc._id}?`)) return;
    try {
      await api.deleteDocument(activeDb, activeCol, doc._id);
      toast.success("Document deleted");
      fetchDocs();
    } catch {
      toast.error("Failed to delete document");
    }
  };

  const handleBulkDelete = async () => {
    if (!activeDb || !activeCol || selectedIds.size === 0) return;
    if (!confirm(`Delete ${selectedIds.size} document(s)?`)) return;
    try {
      const ids = Array.from(selectedIds);
      for (let i = 0; i < ids.length; i++) {
        await api.deleteDocument(activeDb, activeCol, ids[i]);
      }
      toast.success(`${selectedIds.size} document(s) deleted`);
      clearSelectedIds();
      fetchDocs();
    } catch {
      toast.error("Failed to delete some documents");
    }
  };

  const columns = useMemo<string[]>(() => {
    if (activeSchema && activeSchema.fields && activeSchema.fields.length > 0) {
      const cols = ["_id"];
      activeSchema.fields.forEach(f => {
        if (f.name !== "_id") cols.push(f.name);
      });
      // Include fields present in documents but not in schema
      documents.forEach((d) => Object.keys(d).forEach((k) => {
        if (!cols.includes(k)) cols.push(k);
      }));
      return cols.slice(0, 15);
    }
    const cols = new Set<string>(["_id"]);
    documents.forEach((d) => Object.keys(d).forEach((k) => cols.add(k)));
    const arr = Array.from(cols);
    return arr.slice(0, 15);
  }, [documents, activeSchema]);

  const totalPages = Math.ceil(docCount / pageSize);
  const startRow = page * pageSize + 1;
  const endRow = Math.min((page + 1) * pageSize, docCount);
  const gridTemplateColumns = useMemo(
    () => ["2.5rem", ...columns.map(getColumnWidth), "2.5rem"].join(" "),
    [columns]
  );

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

  const allSelected = documents.length > 0 && documents.every((d) => selectedIds.has(d._id));

  const toggleSelectAll = () => {
    if (allSelected) {
      clearSelectedIds();
    } else {
      setSelectedIds(new Set(documents.map((d) => d._id)));
    }
  };

  const handleAddColumn = () => {
    const name = prompt("Column Name");
    if (!name) return;
    const type = prompt("Type (string, number, boolean, datetime)", "string") as any;
    if (!type) return;
    if (activeDb && activeCol && activeSchema) {
      const next = { ...activeSchema };
      next.fields = [...(next.fields || []), { name, type }];
      useStore.getState().setActiveSchema(next);
      api.setSchema(activeDb, activeCol, next).then(() => {
        toast.success(`Column ${name} added`);
      }).catch(() => toast.error("Failed to add column"));
    }
  };

  if (!activeDb || !activeCol) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-3 py-20">
        <FileText className="w-12 h-12 opacity-20" />
        <p className="text-sm">Select a collection from the sidebar</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full gap-3">
      {/* Toolbar */}
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
          <button
            onClick={handleAddColumn}
            className="btn-ghost"
            title="Add Column"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={fetchDocs}
            disabled={dataLoading}
            className="btn-ghost"
          >
            <RefreshCw className={cn("w-3.5 h-3.5", dataLoading && "animate-spin")} />
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded-lg border border-border bg-surface-2">
        {dataLoading ? (
          <div className="flex items-center justify-center h-40">
            <Loader2 className="w-5 h-5 animate-spin text-neon-500" />
          </div>
        ) : documents.length === 0 && columns.length <= 1 ? (
          <div className="flex flex-col items-center justify-center h-40 text-muted-foreground gap-2">
            <FileText className="w-8 h-8 opacity-20" />
            <p className="text-sm">No documents found</p>
            <p className="text-xs">Try changing your query or inserting documents</p>
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
              {columns.map((col) => (
                <button
                  key={col}
                  type="button"
                  className="group flex items-center gap-1.5 px-3 py-2.5 text-left text-xs font-medium uppercase tracking-wider text-muted-foreground hover:text-foreground"
                  onClick={() => toggleSort(col)}
                >
                  <span>{col}</span>
                  {sortBy?.field === col ? (
                    sortBy.dir === "asc" ? (
                      <ArrowUp className="w-3 h-3 text-neon-500" />
                    ) : (
                      <ArrowDown className="w-3 h-3 text-neon-500" />
                    )
                  ) : (
                    <ArrowUpDown className="w-3 h-3 opacity-0 group-hover:opacity-40 transition-opacity" />
                  )}
                </button>
              ))}
              <div className="px-2 py-2.5" />
            </div>

            <AnimatePresence>
              {documents.map((doc, i) => {
                const rowMenuItems: ContextMenuEntry[] = [
                  { label: "Edit document", icon: <Pencil className="w-3.5 h-3.5" />, onClick: () => onEditDoc?.(doc), shortcut: "Enter" },
                  { label: "Copy JSON", icon: <Copy className="w-3.5 h-3.5" />, onClick: () => { navigator.clipboard.writeText(JSON.stringify(doc, null, 2)); toast.success("JSON copied"); } },
                  { label: "Copy ID", icon: <ClipboardCopy className="w-3.5 h-3.5" />, onClick: () => { navigator.clipboard.writeText(doc._id); toast.success("ID copied"); } },
                  { separator: true },
                  { label: "Delete", icon: <Trash2 className="w-3.5 h-3.5" />, danger: true, onClick: () => { if (activeDb && activeCol && confirm(`Delete ${doc._id}?`)) { api.deleteDocument(activeDb, activeCol, doc._id).then(() => { toast.success("Deleted"); fetchDocs(); }); } }, shortcut: "Del" },
                ];

                return (
                  <ContextMenu key={doc._id} items={rowMenuItems}>
                    <motion.div
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={{ opacity: 0 }}
                      transition={{ delay: i * 0.01 }}
                      onClick={() => onEditDoc?.(doc)}
                      className={cn(
                        "grid cursor-pointer border-b border-border/50 hover:bg-surface-3 group",
                        selectedIds.has(doc._id) && "bg-neon-500/5"
                      )}
                      style={{ gridTemplateColumns }}
                    >
                      <div className="flex items-center justify-center px-2 py-2" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={selectedIds.has(doc._id)}
                          onChange={() => toggleSelectedId(doc._id)}
                          className="w-3.5 h-3.5 rounded border-border accent-neon-500 cursor-pointer"
                        />
                      </div>
                      {columns.map((col) => (
                        <div key={col} className="px-3 py-2 text-sm">
                          <InlineCell
                            value={doc[col]}
                            field={col}
                            docId={doc._id}
                            db={activeDb}
                            col={activeCol}
                            onSaved={fetchDocs}
                          />
                        </div>
                      ))}
                      <div className="flex items-center justify-center px-2 py-2">
                        <button
                          onClick={(e) => handleDelete(doc, e)}
                          className="opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-400 transition-all"
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

      {/* Pagination */}
      <div className="pagination-bar">
        <div className="flex items-center gap-3">
          <span className="text-xs">
            Rows {startRow}–{endRow} of {formatNumber(docCount)}
          </span>
          <select
            value={pageSize}
            onChange={(e) => { setPageSize(Number(e.target.value)); setPage(0); }}
            className="select-field text-xs !py-1 !px-2 !w-auto"
          >
            {PAGE_SIZES.map((s) => (
              <option key={s} value={s}>{s} / page</option>
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
            {page + 1} / {totalPages || 1}
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
    </div>
  );
}
