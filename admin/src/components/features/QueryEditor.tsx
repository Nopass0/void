/**
 * @fileoverview QueryEditor – a CodeMirror-powered JSON query editor
 * integrated with the Zustand store and the document table.
 */

"use client";

import React, { useState } from "react";
import { motion } from "framer-motion";
import { Play, Eraser } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import dynamic from "next/dynamic";

// Dynamic import to avoid SSR issues with CodeMirror.
const CodeMirror = dynamic(() => import("@uiw/react-codemirror"), { ssr: false });
const { json } = await import("@codemirror/lang-json").catch(() => ({ json: () => [] }));
const oneDark = await import("@uiw/codemirror-theme-one-dark")
  .then((m) => m.oneDark)
  .catch(() => undefined);

/** Default query template shown to users. */
const DEFAULT_QUERY = JSON.stringify(
  { where: [], order_by: [{ field: "_id", dir: "asc" }], limit: 25 },
  null,
  2
);

/**
 * QueryEditor renders a JSON editor for building VoidDB queries.
 * Results are loaded into the document table on "Run".
 */
export function QueryEditor() {
  const {
    queryText, setQueryText,
    activeDb, activeCol,
    setDocuments, setDataLoading,
    page, pageSize, setPage,
  } = useStore();

  const [localText, setLocalText] = useState(queryText || DEFAULT_QUERY);

  const runQuery = async () => {
    if (!activeDb || !activeCol) {
      toast.error("Select a database and collection first");
      return;
    }
    let spec: api.QuerySpec;
    try {
      spec = JSON.parse(localText);
    } catch {
      toast.error("Invalid JSON query");
      return;
    }
    setQueryText(localText);
    setPage(0);
    setDataLoading(true);
    try {
      spec.limit = spec.limit ?? pageSize;
      spec.skip = page * pageSize;
      const result = await api.queryDocuments(activeDb, activeCol, spec);
      setDocuments(result.results, result.count);
      toast.success(`${result.count} document${result.count !== 1 ? "s" : ""} found`);
    } catch {
      toast.error("Query failed");
    } finally {
      setDataLoading(false);
    }
  };

  const clear = () => {
    setLocalText(DEFAULT_QUERY);
    setQueryText(DEFAULT_QUERY);
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      className="glass rounded-xl overflow-hidden border border-void-500/20"
    >
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-void-500/20">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Query Editor
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={clear}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors px-2 py-1"
          >
            <Eraser className="w-3 h-3" />
            Clear
          </button>
          <button
            onClick={runQuery}
            className="flex items-center gap-1.5 text-xs bg-void-600/40 hover:bg-void-600/60 text-void-200 px-3 py-1.5 rounded-lg transition-colors border border-void-500/30"
          >
            <Play className="w-3 h-3" />
            Run
          </button>
        </div>
      </div>

      {/* CodeMirror */}
      <CodeMirror
        value={localText}
        height="160px"
        theme={oneDark}
        extensions={[json()]}
        onChange={setLocalText}
        basicSetup={{
          lineNumbers: false,
          foldGutter: false,
          highlightActiveLine: false,
        }}
        style={{
          fontSize: "12px",
          background: "transparent",
        }}
      />
    </motion.div>
  );
}
