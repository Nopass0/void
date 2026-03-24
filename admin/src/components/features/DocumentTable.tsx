/**
 * @fileoverview DocumentTable – displays a paginated table of documents
 * fetched from the active database / collection.
 * Supports inline delete, row click-to-edit, and column auto-detection.
 */

"use client";

import React, { useEffect, useCallback, useMemo } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  Trash2,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Loader2,
  FileText,
} from "lucide-react";
import { toast } from "sonner";
import { cn, truncate, formatNumber } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";

// ── Cell renderer ─────────────────────────────────────────────────────────────

function CellValue({ value }: { value: unknown }) {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic text-xs">null</span>;
  }
  if (typeof value === "boolean") {
    return (
      <span className={cn("text-xs font-mono", value ? "text-green-400" : "text-red-400")}>
        {String(value)}
      </span>
    );
  }
  if (typeof value === "number") {
    return <span className="text-xs font-mono text-blue-300">{value}</span>;
  }
  if (typeof value === "object") {
    return (
      <span className="text-xs font-mono text-amber-300">
        {truncate(JSON.stringify(value), 40)}
      </span>
    );
  }
  return (
    <span className="text-xs text-foreground">{truncate(String(value), 60)}</span>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

interface DocumentTableProps {
  /** Called when a row is clicked to open the document editor. */
  onEditDoc?: (doc: Document) => void;
}

/**
 * DocumentTable fetches and renders documents for the active collection.
 */
export function DocumentTable({ onEditDoc }: DocumentTableProps) {
  const {
    activeDb, activeCol,
    documents, docCount, dataLoading,
    setDocuments, setDataLoading,
    queryText,
    page, setPage, pageSize,
  } = useStore();

  /** Fetches the current page of documents using the stored query. */
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
      const result = await api.queryDocuments(activeDb, activeCol, spec);
      setDocuments(result.results, result.count);
    } catch (err) {
      toast.error("Failed to fetch documents");
    } finally {
      setDataLoading(false);
    }
  }, [activeDb, activeCol, queryText, page, pageSize, setDocuments, setDataLoading]);

  useEffect(() => { fetchDocs(); }, [activeDb, activeCol, page]);

  /** Deletes a document and refreshes the list. */
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

  /** Derive column headers from all documents (union of field names). */
  const columns = useMemo<string[]>(() => {
    const cols = new Set<string>(["_id"]);
    documents.forEach((d) => Object.keys(d).forEach((k) => cols.add(k)));
    const arr = Array.from(cols);
    return arr.slice(0, 10); // cap at 10 visible columns
  }, [documents]);

  const totalPages = Math.ceil(docCount / pageSize);

  if (!activeDb || !activeCol) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-3">
        <FileText className="w-12 h-12 opacity-20" />
        <p className="text-sm">Select a collection from the sidebar</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full gap-3">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            {formatNumber(docCount)} document{docCount !== 1 ? "s" : ""}
          </span>
        </div>
        <button
          onClick={fetchDocs}
          disabled={dataLoading}
          className="text-muted-foreground hover:text-void-400 transition-colors"
        >
          <RefreshCw className={cn("w-4 h-4", dataLoading && "animate-spin")} />
        </button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded-xl border border-void-500/20 glass">
        {dataLoading ? (
          <div className="flex items-center justify-center h-40">
            <Loader2 className="w-6 h-6 animate-spin text-void-400" />
          </div>
        ) : documents.length === 0 ? (
          <div className="flex items-center justify-center h-40 text-muted-foreground text-sm">
            No documents found
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-void-500/20">
                {columns.map((col) => (
                  <th
                    key={col}
                    className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground uppercase tracking-wider whitespace-nowrap"
                  >
                    {col}
                  </th>
                ))}
                <th className="px-3 py-2.5 w-10" />
              </tr>
            </thead>
            <tbody>
              <AnimatePresence>
                {documents.map((doc, i) => (
                  <motion.tr
                    key={doc._id}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0 }}
                    transition={{ delay: i * 0.02 }}
                    onClick={() => onEditDoc?.(doc)}
                    className="border-b border-void-500/10 hover:bg-void-600/10 cursor-pointer transition-colors group"
                  >
                    {columns.map((col) => (
                      <td key={col} className="px-3 py-2 max-w-[200px] truncate">
                        <CellValue value={doc[col]} />
                      </td>
                    ))}
                    <td className="px-3 py-2">
                      <button
                        onClick={(e) => handleDelete(doc, e)}
                        className="opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-400 transition-all"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </motion.tr>
                ))}
              </AnimatePresence>
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-3 text-sm">
          <button
            onClick={() => setPage(Math.max(0, page - 1))}
            disabled={page === 0}
            className="text-muted-foreground hover:text-foreground disabled:opacity-30 transition-colors"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
          <span className="text-muted-foreground">
            Page {page + 1} of {totalPages}
          </span>
          <button
            onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
            disabled={page >= totalPages - 1}
            className="text-muted-foreground hover:text-foreground disabled:opacity-30 transition-colors"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      )}
    </div>
  );
}
