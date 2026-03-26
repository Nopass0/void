/**
 * @fileoverview QueryEditor – JSON query editor for VoidDB.
 */

"use client";

import React, { useState, useEffect, useRef } from "react";
import { Play, Eraser } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import dynamic from "next/dynamic";

const CodeMirror = dynamic(() => import("@uiw/react-codemirror"), {
  ssr: false,
  loading: () => (
    <div className="h-32 flex items-center justify-center text-muted-foreground text-xs">
      Loading editor...
    </div>
  ),
});

const DEFAULT_QUERY = JSON.stringify(
  { where: [], order_by: [{ field: "_id", dir: "asc" }], limit: 25 },
  null,
  2
);

export function QueryEditor() {
  const {
    queryText, setQueryText,
    activeDb, activeCol,
    setDocuments, setDataLoading,
    page, pageSize, setPage,
  } = useStore();

  const [localText, setLocalText] = useState(queryText || DEFAULT_QUERY);
  const [theme, setTheme] = useState<any>(undefined);
  const [jsonExt, setJsonExt] = useState<any[]>([]);
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current) return;
    loaded.current = true;
    Promise.all([
      import("@codemirror/theme-one-dark").then((m) => m.oneDark),
      import("@codemirror/lang-json").then((m) => [m.json()]),
    ]).then(([t, ext]) => {
      setTheme(t);
      setJsonExt(ext);
    });
  }, []);

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
    <div className="rounded-lg overflow-hidden border border-border bg-surface-2">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          Query
        </span>
        <div className="flex items-center gap-2">
          <button onClick={clear} className="btn-ghost text-xs">
            <Eraser className="w-3 h-3" />
            Clear
          </button>
          <button onClick={runQuery} className="btn-primary text-xs !py-1">
            <Play className="w-3 h-3" />
            Run
          </button>
        </div>
      </div>

      {/* CodeMirror */}
      <CodeMirror
        value={localText}
        height="140px"
        theme={theme}
        extensions={jsonExt}
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
    </div>
  );
}
