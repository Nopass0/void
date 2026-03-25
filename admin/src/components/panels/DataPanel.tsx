"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import {
  Plus,
  X,
  Save,
  Loader2,
  Search,
  Sparkles,
  SlidersHorizontal,
} from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import { DocumentTable } from "@/components/features/DocumentTable";
import { Card } from "@/components/ui/glass-card";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";

interface FilterRule {
  id: string;
  field: string;
  op: api.QueryFilter["op"];
  value: string;
}

const FILTER_OPERATORS: Array<{ value: api.QueryFilter["op"]; label: string }> = [
  { value: "eq", label: "is" },
  { value: "ne", label: "is not" },
  { value: "gt", label: ">" },
  { value: "gte", label: ">=" },
  { value: "lt", label: "<" },
  { value: "lte", label: "<=" },
  { value: "contains", label: "contains" },
  { value: "starts_with", label: "starts with" },
  { value: "in", label: "in" },
];

function nextRule(field = "", op: api.QueryFilter["op"] = "eq", value = ""): FilterRule {
  return {
    id: typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random()}`,
    field,
    op,
    value,
  };
}

function storageFieldName(field: api.SchemaField): string {
  if (field.mapped_name) return field.mapped_name;
  if (field.is_id) return "_id";
  return field.name;
}

function queryFieldName(field: string, schema: api.Schema | null): string {
  if (field === "_id") return "_id";
  const schemaField = schema?.fields?.find((item) => item.name === field || storageFieldName(item) === field);
  return schemaField ? storageFieldName(schemaField) : field;
}

function displayFieldName(field: string, schema: api.Schema | null): string {
  if (field === "_id") return "_id";
  const schemaField = schema?.fields?.find((item) => item.name === field || storageFieldName(item) === field);
  return schemaField?.name ?? field;
}

function fieldTypeFor(field: string, schema: api.Schema | null): api.SchemaField["type"] | "string" {
  if (field === "_id") return "string";
  const schemaField = schema?.fields?.find((item) => item.name === field || storageFieldName(item) === field);
  if (!schemaField) return "string";
  if (schemaField.is_id || storageFieldName(schemaField) === "_id") return "string";
  return schemaField.type;
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

function stringifyFilterValue(value: unknown, type: api.SchemaField["type"] | "string"): string {
  if (value === null || value === undefined) return "";
  if (type === "datetime" && typeof value === "string") {
    return datetimeToInput(value);
  }
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function coerceFilterValue(raw: string, type: api.SchemaField["type"] | "string", op: api.QueryFilter["op"]): unknown {
  if (type === "number") {
    const num = Number(raw);
    return Number.isNaN(num) ? raw : num;
  }
  if (type === "boolean") {
    return raw === "true";
  }
  if (type === "datetime") {
    return inputToDatetime(raw);
  }
  if (op === "in") {
    return raw
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return raw;
}

function leafFilter(node: api.QueryNode | undefined): api.QueryFilter | null {
  if (!node) return null;
  if ("field" in node && "op" in node) return node;
  return null;
}

function simpleFiltersFromQuery(queryText: string, schema: api.Schema | null): {
  quickSearch: string;
  filters: FilterRule[];
} {
  try {
    const parsed = JSON.parse(queryText) as api.QuerySpec;
    const nodes: api.QueryFilter[] = [];
    let quickSearch = "";
    const where = parsed.where;
    if (!where) {
      return { quickSearch, filters: [] };
    }

    if ("field" in where && "op" in where) {
      nodes.push(where);
    } else if (Array.isArray(where.AND)) {
      for (const child of where.AND) {
        const leaf = leafFilter(child);
        if (leaf) nodes.push(leaf);
      }
    } else {
      return { quickSearch, filters: [] };
    }

    const filters = nodes.flatMap((node) => {
      if (node.field === "_id" && node.op === "contains" && typeof node.value === "string") {
        quickSearch = node.value;
        return [];
      }
      const type = fieldTypeFor(node.field, schema);
      return [nextRule(displayFieldName(node.field, schema), node.op, stringifyFilterValue(node.value, type))];
    });

    return { quickSearch, filters };
  } catch {
    return { quickSearch: "", filters: [] };
  }
}

function buildQuerySpec(
  quickSearch: string,
  rules: FilterRule[],
  schema: api.Schema | null
): api.QuerySpec {
  const predicates: api.QueryFilter[] = [];

  if (quickSearch.trim()) {
    predicates.push({
      field: "_id",
      op: "contains",
      value: quickSearch.trim(),
    });
  }

  for (const rule of rules) {
    if (!rule.field.trim()) continue;
    if (rule.value === "" && rule.op !== "eq" && rule.op !== "ne") continue;
    const type = fieldTypeFor(rule.field, schema);
    predicates.push({
      field: queryFieldName(rule.field, schema),
      op: rule.op,
      value: coerceFilterValue(rule.value, type, rule.op),
    });
  }

  if (predicates.length === 0) {
    return {};
  }
  if (predicates.length === 1) {
    return { where: predicates[0] };
  }
  return { where: { AND: predicates } };
}

function FilterValueInput({
  rule,
  schema,
  onChange,
}: {
  rule: FilterRule;
  schema: api.Schema | null;
  onChange: (value: string) => void;
}) {
  const type = fieldTypeFor(rule.field, schema);

  if (type === "boolean") {
    return (
      <select
        value={rule.value}
        onChange={(event) => onChange(event.target.value)}
        className="select-field text-xs !py-2 w-[8.5rem]"
      >
        <option value="">Select</option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    );
  }

  if (type === "datetime") {
    return (
      <input
        type="datetime-local"
        value={rule.value}
        onChange={(event) => onChange(event.target.value)}
        className="input-field text-xs !py-2"
      />
    );
  }

  return (
    <input
      type={type === "number" ? "number" : "text"}
      value={rule.value}
      onChange={(event) => onChange(event.target.value)}
      placeholder={rule.op === "in" ? "comma,separated,values" : "value"}
      className="input-field text-xs !py-2"
    />
  );
}

function FilterBar() {
  const { activeSchema, queryText, setPage, setQueryText } = useStore();
  const queryRef = useRef(queryText);
  const [quickSearch, setQuickSearch] = useState("");
  const [filters, setFilters] = useState<FilterRule[]>([]);

  const fieldOptions = useMemo(() => {
    const options = ["_id"];
    for (const field of activeSchema?.fields ?? []) {
      if (storageFieldName(field) === "_id") continue;
      options.push(field.name);
    }
    return options;
  }, [activeSchema]);

  useEffect(() => {
    if (queryText === queryRef.current) return;
    const parsed = simpleFiltersFromQuery(queryText, activeSchema);
    setQuickSearch(parsed.quickSearch);
    setFilters(parsed.filters);
    queryRef.current = queryText;
  }, [activeSchema, queryText]);

  useEffect(() => {
    const spec = buildQuerySpec(quickSearch, filters, activeSchema);
    const nextQuery = JSON.stringify(spec, null, 2);
    queryRef.current = nextQuery;
    setQueryText(nextQuery);
    setPage(0);
  }, [activeSchema, filters, quickSearch, setPage, setQueryText]);

  const updateRule = (id: string, patch: Partial<FilterRule>) => {
    setFilters((current) =>
      current.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    );
  };

  const clearAll = () => {
    setQuickSearch("");
    setFilters([]);
  };

  return (
    <div className="rounded-xl border border-border bg-surface-2/80 p-3 space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[16rem] flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
          <input
            type="text"
            value={quickSearch}
            onChange={(event) => setQuickSearch(event.target.value)}
            placeholder="Search by _id..."
            className="input-field !pl-9 text-xs !py-2"
          />
        </div>

        <button
          onClick={() => setFilters((current) => [...current, nextRule()])}
          className="btn-ghost text-xs"
        >
          <SlidersHorizontal className="w-3.5 h-3.5" />
          Add Filter
        </button>

        {(quickSearch || filters.length > 0) && (
          <button onClick={clearAll} className="btn-ghost text-xs text-red-400">
            <X className="w-3.5 h-3.5" />
            Clear
          </button>
        )}
      </div>

      {filters.length > 0 && (
        <div className="space-y-2">
          {filters.map((rule, index) => {
            const type = fieldTypeFor(rule.field, activeSchema);
            return (
              <motion.div
                key={rule.id}
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -4 }}
                className="grid gap-2 rounded-lg border border-border/80 bg-surface-1/70 px-3 py-2 md:grid-cols-[4rem_minmax(10rem,0.8fr)_10rem_minmax(12rem,1fr)_auto]"
              >
                <div className="flex items-center text-[10px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">
                  {index === 0 ? "Where" : "And"}
                </div>

                <select
                  value={rule.field}
                  onChange={(event) =>
                    updateRule(rule.id, {
                      field: event.target.value,
                      value: "",
                    })
                  }
                  className="select-field text-xs !py-2"
                >
                  <option value="">Field</option>
                  {fieldOptions.map((field) => (
                    <option key={field} value={field}>
                      {field}
                    </option>
                  ))}
                </select>

                <select
                  value={rule.op}
                  onChange={(event) => updateRule(rule.id, { op: event.target.value as api.QueryFilter["op"] })}
                  className="select-field text-xs !py-2"
                >
                  {FILTER_OPERATORS.map((operator) => (
                    <option key={operator.value} value={operator.value}>
                      {operator.label}
                    </option>
                  ))}
                </select>

                <div className="flex items-center gap-2">
                  <FilterValueInput
                    rule={rule}
                    schema={activeSchema}
                    onChange={(value) => updateRule(rule.id, { value })}
                  />
                  <span className="metric-badge hidden lg:inline-flex">
                    {type}
                  </span>
                </div>

                <button
                  onClick={() => setFilters((current) => current.filter((item) => item.id !== rule.id))}
                  className="btn-ghost !p-2 text-muted-foreground hover:text-red-400"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </motion.div>
            );
          })}
        </div>
      )}

      {!quickSearch && filters.length === 0 && (
        <div className="flex items-center gap-2 rounded-lg border border-dashed border-border/70 px-3 py-2 text-xs text-muted-foreground">
          <Sparkles className="w-3.5 h-3.5 text-neon-500" />
          Build filters visually. They are translated into the real VoidDB query DSL under the hood.
        </div>
      )}
    </div>
  );
}

interface DocEditorProps {
  doc: Document | null;
  onClose: () => void;
  onSaved: () => void;
}

function DocEditor({ doc, onClose, onSaved }: DocEditorProps) {
  const { activeDb, activeCol } = useStore();
  const isNew = doc === null;

  const initialJSON = isNew
    ? JSON.stringify({}, null, 2)
    : JSON.stringify(
        Object.fromEntries(Object.entries(doc).filter(([key]) => key !== "_id")),
        null,
        2
      );

  const [text, setText] = useState(initialJSON);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    if (!activeDb || !activeCol) return;
    let fields: Record<string, unknown>;
    try {
      fields = JSON.parse(text);
    } catch {
      toast.error("Invalid JSON");
      return;
    }
    setSaving(true);
    try {
      if (isNew) {
        await api.insertDocument(activeDb, activeCol, fields);
        toast.success("Document created");
      } else {
        await api.updateDocument(activeDb, activeCol, doc._id, fields);
        toast.success("Document updated");
      }
      onSaved();
    } catch {
      toast.error("Failed to save document");
    } finally {
      setSaving(false);
    }
  };

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
        className="w-full max-w-3xl rounded-xl border border-border bg-surface-2 shadow-modal overflow-hidden"
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-3">
          <div>
            <h2 className="text-sm font-semibold text-foreground">
              {isNew ? "New Document" : `Edit JSON • ${doc._id}`}
            </h2>
            <p className="text-xs text-muted-foreground">
              Use full-document JSON editing when you need to reshape nested data.
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
            className="input-field min-h-[24rem] resize-y font-mono text-xs leading-6"
          />
        </div>

        <div className="flex justify-end gap-3 border-t border-border px-5 py-3">
          <button onClick={onClose} className="btn-ghost text-sm">
            Cancel
          </button>
          <button onClick={save} disabled={saving} className="btn-primary text-sm">
            {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
            {isNew ? "Create" : "Save"}
          </button>
        </div>
      </motion.div>
    </motion.div>
  );
}

export function DataPanel() {
  const { activeDb, activeCol, setActiveSchema } = useStore();
  const [editorDoc, setEditorDoc] = useState<Document | null | undefined>(undefined);
  const [refreshToken, setRefreshToken] = useState(0);

  const refresh = () => setRefreshToken((value) => value + 1);

  useEffect(() => {
    if (!activeDb || !activeCol) return;

    api.getSchema(activeDb, activeCol)
      .then(setActiveSchema)
      .catch(() => setActiveSchema({ fields: [] }));

    const token = localStorage.getItem("void_access_token") ?? "";
    const baseUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700";
    const es = new EventSource(
      `${baseUrl}/v1/databases/${encodeURIComponent(activeDb)}/realtime?token=${encodeURIComponent(token)}`
    );

    es.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.collection === activeCol) {
          refresh();
        }
      } catch {
        // noop
      }
    };

    return () => es.close();
  }, [activeCol, activeDb, setActiveSchema]);

  if (!activeDb || !activeCol) {
    return (
      <Card className="h-full flex items-center justify-center">
        <p className="text-sm text-muted-foreground">
          Select a database and collection from the sidebar
        </p>
      </Card>
    );
  }

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium text-muted-foreground">
            <span className="font-semibold text-foreground">{activeDb}</span>
            <span className="mx-1.5 text-border">/</span>
            <span className="text-neon-500">{activeCol}</span>
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Inline editing is type-aware. Nested JSON stays in a dedicated editor.
          </p>
        </div>

        <button onClick={() => setEditorDoc(null)} className="btn-primary text-sm">
          <Plus className="w-4 h-4" />
          Add Row
        </button>
      </div>

      <FilterBar />

      <div className="min-h-0 flex-1">
        <DocumentTable
          refreshToken={refreshToken}
          onEditDoc={(doc) => setEditorDoc(doc)}
        />
      </div>

      <AnimatePresence>
        {editorDoc !== undefined && (
          <DocEditor
            doc={editorDoc}
            onClose={() => setEditorDoc(undefined)}
            onSaved={() => {
              setEditorDoc(undefined);
              refresh();
            }}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
