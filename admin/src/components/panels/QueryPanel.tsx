"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { Play, Loader2, Copy, Sparkles, Database, Table2 } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import { Card } from "@/components/ui/glass-card";

function defaultQuery(): string {
  return JSON.stringify(
    {
      where: {
        AND: [],
      },
      order_by: [{ field: "_id", dir: "asc" }],
      limit: 50,
      skip: 0,
    },
    null,
    2
  );
}

function QueryEditor({
  value,
  onChange,
  fields,
}: {
  value: string;
  onChange: (value: string) => void;
  fields: string[];
}) {
  const [CodeMirror, setCodeMirror] = useState<React.ComponentType<any> | null>(null);
  const [theme, setTheme] = useState<any>(undefined);
  const [jsonExtension, setJsonExtension] = useState<any>(null);
  const [autocompletionFactory, setAutocompletionFactory] = useState<any>(null);
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current) return;
    loaded.current = true;

    Promise.all([
      import("@uiw/react-codemirror").then((module) => module.default),
      import("@codemirror/theme-one-dark").then((module) => module.oneDark),
      import("@codemirror/lang-json").then((module) => module.json()),
      import("@codemirror/autocomplete"),
    ]).then(([cm, oneDark, json, autocomplete]) => {
      setCodeMirror(() => cm);
      setTheme(oneDark);
      setJsonExtension(json);
      setAutocompletionFactory(() => autocomplete.autocompletion);
    });
  }, [fields]);

  const extensions = useMemo(() => {
    if (!jsonExtension || !autocompletionFactory) return [];
    const completionSource = (context: any) => {
      const word = context.matchBefore(/[\w"_-]*/);
      if (!word || (word.from === word.to && !context.explicit)) {
        return null;
      }

      const options = [
        { label: "where", type: "keyword", info: "Filter tree root" },
        { label: "AND", type: "keyword", info: "Combine filters" },
        { label: "OR", type: "keyword", info: "Alternative filters" },
        { label: "field", type: "property" },
        { label: "op", type: "property" },
        { label: "value", type: "property" },
        { label: "order_by", type: "property" },
        { label: "limit", type: "property" },
        { label: "skip", type: "property" },
        { label: "include", type: "property" },
        { label: "target_col", type: "property" },
        { label: "local_key", type: "property" },
        { label: "foreign_key", type: "property" },
        { label: '"eq"', type: "constant" },
        { label: '"ne"', type: "constant" },
        { label: '"gt"', type: "constant" },
        { label: '"gte"', type: "constant" },
        { label: '"lt"', type: "constant" },
        { label: '"lte"', type: "constant" },
        { label: '"contains"', type: "constant" },
        { label: '"starts_with"', type: "constant" },
        { label: '"in"', type: "constant" },
        ...fields.map((field) => ({
          label: `"${field}"`,
          type: "variable",
          info: "Collection field",
        })),
      ];

      return {
        from: word.from,
        options,
      };
    };

    return [
      jsonExtension,
      autocompletionFactory({
        override: [completionSource],
        activateOnTyping: true,
      }),
    ];
  }, [autocompletionFactory, fields, jsonExtension]);

  if (!CodeMirror) {
    return (
      <div className="h-full min-h-[24rem] flex items-center justify-center rounded-lg border border-border bg-surface-1 text-xs text-muted-foreground">
        Loading editor...
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <CodeMirror
        value={value}
        height="560px"
        theme={theme}
        extensions={extensions}
        onChange={onChange}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: true,
          bracketMatching: true,
          autocompletion: true,
        }}
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}

export function QueryPanel() {
  const { databases, activeDb, activeCol } = useStore();
  const [db, setDb] = useState(activeDb ?? "");
  const [col, setCol] = useState(activeCol ?? "");
  const [cols, setCols] = useState<string[]>([]);
  const [schema, setSchema] = useState<api.Schema | null>(null);
  const [query, setQuery] = useState(defaultQuery());
  const [results, setResults] = useState<any[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    if (!db) {
      setCols([]);
      setCol("");
      return;
    }
    api.listCollections(db).then(setCols).catch(() => setCols([]));
  }, [db]);

  useEffect(() => {
    if (!db || !col) {
      setSchema(null);
      return;
    }
    api.getSchema(db, col).then(setSchema).catch(() => setSchema(null));
  }, [col, db]);

  useEffect(() => {
    if (activeDb && !db) setDb(activeDb);
    if (activeCol && !col) setCol(activeCol);
  }, [activeCol, activeDb, col, db]);

  const execute = async () => {
    if (!db || !col) {
      toast.error("Select a database and collection");
      return;
    }
    let parsed: api.QuerySpec;
    try {
      parsed = JSON.parse(query);
    } catch {
      toast.error("Invalid JSON query");
      return;
    }
    setLoading(true);
    const started = performance.now();
    try {
      const result = await api.queryDocuments(db, col, parsed);
      setResults(result.results || []);
      setElapsed(Math.round(performance.now() - started));
      toast.success(`${result.results.length} row${result.results.length !== 1 ? "s" : ""} in ${Math.round(performance.now() - started)}ms`);
    } catch (err: any) {
      toast.error(err?.message || "Query failed");
    } finally {
      setLoading(false);
    }
  };

  const resultColumns = useMemo(() => {
    if (!results || results.length === 0) return [];
    return Array.from(new Set(results.flatMap((row) => Object.keys(row))));
  }, [results]);

  const autocompleteFields = useMemo(() => {
    const fields = ["_id"];
    for (const field of schema?.fields ?? []) {
      if (field.mapped_name === "_id" || field.is_id) continue;
      fields.push(field.name);
      if (field.mapped_name && field.mapped_name !== field.name) {
        fields.push(field.mapped_name);
      }
    }
    return Array.from(new Set(fields));
  }, [schema]);

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-foreground flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-neon-500" />
            Query Editor
          </h2>
          <p className="text-sm text-muted-foreground">
            Schema-aware JSON DSL with autocomplete, results and timings side by side.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setQuery(defaultQuery())}
            className="btn-ghost text-sm"
          >
            Reset
          </button>
          <button
            onClick={execute}
            disabled={loading || !db || !col}
            className="btn-primary text-sm"
          >
            {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
            Run Query
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
        <Card className="min-h-0 p-4 flex flex-col gap-4 overflow-hidden">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
            <label className="space-y-1.5">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground flex items-center gap-2">
                <Database className="w-3.5 h-3.5" />
                Database
              </span>
              <select value={db} onChange={(event) => setDb(event.target.value)} className="select-field text-sm">
                <option value="">Select database</option>
                {databases.map((database) => (
                  <option key={database} value={database}>
                    {database}
                  </option>
                ))}
              </select>
            </label>

            <div className="flex items-end justify-center pb-2 text-muted-foreground">/</div>

            <label className="space-y-1.5">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground flex items-center gap-2">
                <Table2 className="w-3.5 h-3.5" />
                Collection
              </span>
              <select value={col} onChange={(event) => setCol(event.target.value)} className="select-field text-sm">
                <option value="">Select collection</option>
                {cols.map((collection) => (
                  <option key={collection} value={collection}>
                    {collection}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-1/80 px-3 py-2 text-xs text-muted-foreground">
            <span className="metric-badge">{autocompleteFields.length} fields</span>
            <span>Autocomplete includes query keywords, operators and collection fields.</span>
          </div>

          <div className="min-h-0 flex-1">
            <QueryEditor value={query} onChange={setQuery} fields={autocompleteFields} />
          </div>
        </Card>

        <Card className="min-h-0 p-4 flex flex-col gap-4 overflow-hidden">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-semibold text-foreground">Results</h3>
              <p className="text-xs text-muted-foreground">
                {results ? `${results.length} row${results.length !== 1 ? "s" : ""} • ${elapsed}ms` : "Run a query to inspect results"}
              </p>
            </div>
            {results && (
              <button
                onClick={() => {
                  navigator.clipboard.writeText(JSON.stringify(results, null, 2));
                  toast.success("Results copied");
                }}
                className="btn-ghost text-sm"
              >
                <Copy className="w-4 h-4" />
                Copy
              </button>
            )}
          </div>

          <div className="min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-surface-1">
            {results === null ? (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                Query output will appear here.
              </div>
            ) : resultColumns.length === 0 ? (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                No rows returned
              </div>
            ) : (
              <table className="data-table w-full min-w-full">
                <thead>
                  <tr>
                    {resultColumns.map((column) => (
                      <th key={column}>{column}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {results.map((row, rowIndex) => (
                    <tr key={row._id ?? rowIndex}>
                      {resultColumns.map((column) => (
                        <td key={column} className="max-w-[18rem] align-top">
                          {row[column] === undefined ? (
                            <span className="text-muted-foreground/40">—</span>
                          ) : row[column] === null ? (
                            <span className="text-amber-400">null</span>
                          ) : typeof row[column] === "object" ? (
                            <span className="text-violet-400 whitespace-pre-wrap break-words">
                              {JSON.stringify(row[column], null, 2)}
                            </span>
                          ) : typeof row[column] === "boolean" ? (
                            <span className="text-blue-400">{String(row[column])}</span>
                          ) : typeof row[column] === "number" ? (
                            <span className="text-cyan-400">{row[column]}</span>
                          ) : (
                            <span className="whitespace-pre-wrap break-words">{String(row[column])}</span>
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
