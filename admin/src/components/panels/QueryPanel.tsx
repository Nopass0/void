/**
 * @fileoverview QueryPanel – dedicated raw query editor for any database/collection.
 * Lets users execute arbitrary queries with full dark-themed CodeMirror.
 */

"use client";

import React, { useState, useRef, useEffect } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Play, Loader2, Download, Copy, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import * as api from "@/lib/api";

// ── Dark CodeMirror Editor ────────────────────────────────────────────────────

function DarkEditor({
  value,
  onChange,
  height = "200px",
}: {
  value: string;
  onChange: (v: string) => void;
  height?: string;
}) {
  const [CodeMirror, setCodeMirror] = useState<React.ComponentType<any> | null>(null);
  const [theme, setTheme] = useState<any>(undefined);
  const [jsonExt, setJsonExt] = useState<any[]>([]);
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current) return;
    loaded.current = true;
    Promise.all([
      import("@uiw/react-codemirror").then((m) => m.default),
      import("@codemirror/theme-one-dark").then((m) => m.oneDark),
      import("@codemirror/lang-json").then((m) => [m.json()]),
      import("@codemirror/autocomplete").then((m) => m.autocompletion),
    ]).then(([cm, t, ext, ac]) => {
      setCodeMirror(() => cm);
      setTheme(t);

      const queryCompletions = (context: any) => {
        const word = context.matchBefore(/\\w*/);
        if (!word || (word.from === word.to && !context.explicit)) return null;
        return {
          from: word.from,
          options: [
            { label: "where", type: "keyword", info: "Array of predicates" },
            { label: "field", type: "property" },
            { label: "op", type: "property" },
            { label: "value", type: "property" },
            { label: "include", type: "keyword", info: "Eager load relations" },
            { label: "order_by", type: "keyword" },
            { label: "limit", type: "property" },
            { label: "skip", type: "property" },
            { label: '"eq"', type: "text" },
            { label: '"ne"', type: "text" },
            { label: '"gt"', type: "text" },
            { label: '"gte"', type: "text" },
            { label: '"lt"', type: "text" },
            { label: '"lte"', type: "text" },
            { label: '"contains"', type: "text" },
            { label: '"starts_with"', type: "text" },
          ]
        };
      };

      setJsonExt([...ext, ac({ override: [queryCompletions] })]);
    });
  }, []);

  if (!CodeMirror) {
    return (
      <div
        style={{ height }}
        className="flex items-center justify-center text-muted-foreground text-xs bg-surface-0 rounded-md border border-border"
      >
        Loading editor...
      </div>
    );
  }

  return (
    <div className="rounded-md overflow-hidden border border-border">
      <CodeMirror
        value={value}
        height={height}
        theme={theme}
        extensions={jsonExt}
        onChange={onChange}
        basicSetup={{ lineNumbers: true, foldGutter: true }}
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}

// ── Query Panel ──────────────────────────────────────────────────────────────

export function QueryPanel() {
  const { databases } = useStore();
  const [db, setDb] = useState("");
  const [col, setCol] = useState("");
  const [cols, setCols] = useState<string[]>([]);
  const [query, setQuery] = useState(JSON.stringify({
    where: [],
    order_by: [{ field: "_id", dir: "asc" }],
    limit: 50,
    skip: 0,
  }, null, 2));
  const [results, setResults] = useState<any[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  // Load collections when db changes
  useEffect(() => {
    if (!db) { setCols([]); setCol(""); return; }
    api.listCollections(db).then((cs) => setCols(cs)).catch(() => setCols([]));
  }, [db]);

  const execute = async () => {
    if (!db || !col) {
      toast.error("Select a database and collection");
      return;
    }
    let parsed;
    try {
      parsed = JSON.parse(query);
    } catch {
      toast.error("Invalid JSON query");
      return;
    }
    setLoading(true);
    const start = performance.now();
    try {
      const result = await api.queryDocuments(db, col, parsed);
      const docs = result.results || [];
      setResults(docs);
      setElapsed(Math.round(performance.now() - start));
      toast.success(`${docs.length} row${docs.length !== 1 ? "s" : ""} in ${Math.round(performance.now() - start)}ms`);
    } catch (err: any) {
      toast.error(err.message || "Query failed");
    } finally {
      setLoading(false);
    }
  };

  const copyAll = () => {
    if (results) {
      navigator.clipboard.writeText(JSON.stringify(results, null, 2));
      toast.success("Copied to clipboard");
    }
  };

  const resultCols = results && results.length > 0
    ? Array.from(new Set(results.flatMap((d) => Object.keys(d))))
    : [];

  return (
    <div className="flex flex-col h-full gap-4">
      <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
        <Play className="w-4 h-4 text-neon-500" />
        Query Editor
      </h2>

      {/* DB & Collection selector */}
      <div className="flex items-center gap-3">
        <select
          value={db}
          onChange={(e) => setDb(e.target.value)}
          className="select-field text-sm !py-1.5 w-48"
        >
          <option value="">Select database</option>
          {databases.map((d) => (
            <option key={d} value={d}>{d}</option>
          ))}
        </select>
        <span className="text-muted-foreground">/</span>
        <select
          value={col}
          onChange={(e) => setCol(e.target.value)}
          className="select-field text-sm !py-1.5 w-48"
        >
          <option value="">Select collection</option>
          {cols.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        <button
          onClick={execute}
          disabled={loading || !db || !col}
          className="btn-primary text-sm"
        >
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
          Execute
        </button>
      </div>

      {/* Editor */}
      <DarkEditor value={query} onChange={setQuery} height="180px" />

      {/* Results */}
      {results !== null && (
        <div className="flex-1 min-h-0 flex flex-col gap-2">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {results.length} row{results.length !== 1 ? "s" : ""} • {elapsed}ms
            </span>
            <div className="flex items-center gap-2">
              <button onClick={copyAll} className="btn-ghost text-xs !py-0.5">
                <Copy className="w-3 h-3" />
                Copy
              </button>
            </div>
          </div>

          <div className="flex-1 overflow-auto rounded-lg border border-border bg-surface-2">
            {resultCols.length > 0 ? (
              <table className="data-table text-xs">
                <thead>
                  <tr>
                    {resultCols.map((c) => (
                      <th key={c}>{c}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {results.map((row, i) => (
                    <tr key={i}>
                      {resultCols.map((c) => (
                        <td key={c} className="max-w-[200px] truncate">
                          {row[c] === undefined
                            ? <span className="text-muted-foreground/40">—</span>
                            : row[c] === null
                              ? <span className="text-amber-400">null</span>
                              : typeof row[c] === "object"
                                ? <span className="text-violet-400">{JSON.stringify(row[c])}</span>
                                : typeof row[c] === "boolean"
                                  ? <span className="text-blue-400">{String(row[c])}</span>
                                  : typeof row[c] === "number"
                                    ? <span className="text-cyan-400">{row[c]}</span>
                                    : String(row[c])}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className="flex items-center justify-center h-20 text-muted-foreground text-sm">
                No rows returned
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
