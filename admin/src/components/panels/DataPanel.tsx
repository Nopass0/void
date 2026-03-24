/**
 * @fileoverview DataPanel – table-focused CRUD interface for collections.
 * Features: visual filter bar, column management, dark CodeMirror modal.
 */

"use client";

import React, { useState, useEffect, useRef } from "react";
import { AnimatePresence, motion } from "framer-motion";
import {
  Plus, X, Save, Loader2, Filter, Search, SlidersHorizontal, Columns3,
} from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import { DocumentTable } from "@/components/features/DocumentTable";
import { Card } from "@/components/ui/glass-card";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";

// ── Visual Filter Builder ─────────────────────────────────────────────────────

interface FilterRule {
  field: string;
  op: string;
  value: string;
}

const OPS = [
  { value: "eq", label: "=" },
  { value: "ne", label: "≠" },
  { value: "gt", label: ">" },
  { value: "gte", label: "≥" },
  { value: "lt", label: "<" },
  { value: "lte", label: "≤" },
  { value: "contains", label: "contains" },
  { value: "starts_with", label: "starts with" },
];

function FilterBar() {
  const { queryText, setQueryText, setPage } = useStore();
  const [filters, setFilters] = useState<FilterRule[]>([]);
  const [quickSearch, setQuickSearch] = useState("");

  const applyFilters = () => {
    const where = filters
      .filter((f) => f.field && f.value)
      .map((f) => {
        let val: unknown = f.value;
        if (val === "true") val = true;
        else if (val === "false") val = false;
        else if (!isNaN(Number(val)) && val !== "") val = Number(val);
        return { field: f.field, op: f.op, value: val };
      });
    setQueryText(JSON.stringify({ where }));
    setPage(0);
  };

  const addFilter = () => {
    setFilters([...filters, { field: "", op: "eq", value: "" }]);
  };

  const removeFilter = (i: number) => {
    const next = [...filters];
    next.splice(i, 1);
    setFilters(next);
    if (next.length === 0) {
      setQueryText("{}");
      setPage(0);
    }
  };

  const updateFilter = (i: number, key: keyof FilterRule, val: string) => {
    const next = [...filters];
    next[i] = { ...next[i], [key]: val };
    setFilters(next);
  };

  useEffect(() => {
    if (filters.length > 0) {
      const timer = setTimeout(applyFilters, 400);
      return () => clearTimeout(timer);
    }
  }, [filters]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleQuickSearch = (val: string) => {
    setQuickSearch(val);
    if (!val) {
      setQueryText("{}");
      setPage(0);
      return;
    }
    // Quick search searches _id field
    setQueryText(JSON.stringify({ where: [{ field: "_id", op: "contains", value: val }] }));
    setPage(0);
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        {/* Quick search */}
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
          <input
            type="text"
            value={quickSearch}
            onChange={(e) => handleQuickSearch(e.target.value)}
            placeholder="Search by ID..."
            className="input-field !pl-8 text-xs !py-1.5"
          />
        </div>

        {/* Add filter */}
        <button onClick={addFilter} className="btn-ghost text-xs">
          <Filter className="w-3 h-3" />
          Add Filter
        </button>

        {filters.length > 0 && (
          <button
            onClick={() => { setFilters([]); setQueryText("{}"); setPage(0); }}
            className="btn-ghost text-xs text-red-400"
          >
            Clear All
          </button>
        )}
      </div>

      {/* Filter rows */}
      <AnimatePresence>
        {filters.map((f, i) => (
          <motion.div
            key={i}
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            className="flex items-center gap-2"
          >
            <span className="text-[10px] text-muted-foreground w-10">{i === 0 ? "WHERE" : "AND"}</span>
            <input
              type="text"
              value={f.field}
              onChange={(e) => updateFilter(i, "field", e.target.value)}
              placeholder="field name"
              className="input-field text-xs !py-1.5 w-36"
            />
            <select
              value={f.op}
              onChange={(e) => updateFilter(i, "op", e.target.value)}
              className="select-field text-xs !py-1.5 !w-auto"
            >
              {OPS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <input
              type="text"
              value={f.value}
              onChange={(e) => updateFilter(i, "value", e.target.value)}
              placeholder="value"
              className="input-field text-xs !py-1.5 flex-1"
            />
            <button onClick={() => removeFilter(i)} className="btn-ghost !p-1 text-muted-foreground hover:text-red-400">
              <X className="w-3 h-3" />
            </button>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}

// ── Dark CodeMirror Editor ────────────────────────────────────────────────────

function DarkEditor({
  value,
  onChange,
  height = "300px",
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
    ]).then(([cm, t, ext]) => {
      setCodeMirror(() => cm);
      setTheme(t);
      setJsonExt(ext);
    });
  }, []);

  if (!CodeMirror) {
    return (
      <div className="h-32 flex items-center justify-center text-muted-foreground text-xs bg-surface-0 rounded-md border border-border">
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
        basicSetup={{ lineNumbers: true, foldGutter: false }}
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}

// ── Document editor modal ────────────────────────────────────────────────────

interface DocEditorProps {
  doc: Document | null;
  onClose: () => void;
  onSaved: () => void;
}

function DocEditor({ doc, onClose, onSaved }: DocEditorProps) {
  const { activeDb, activeCol } = useStore();
  const isNew = doc === null;

  const initialJSON = isNew
    ? JSON.stringify({ name: "", value: null }, null, 2)
    : JSON.stringify(
        Object.fromEntries(Object.entries(doc).filter(([k]) => k !== "_id")),
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
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60"
      onClick={onClose}
    >
      <motion.div
        initial={{ scale: 0.95, y: 10 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.95, y: 10 }}
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg w-full max-w-2xl bg-surface-2 border border-border shadow-modal overflow-hidden"
      >
        <div className="flex items-center justify-between px-5 py-3 border-b border-border">
          <h2 className="font-semibold text-sm">
            {isNew ? "New Document" : `Edit: ${doc._id}`}
          </h2>
          <button onClick={onClose} className="btn-ghost !p-1">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-4">
          <DarkEditor value={text} onChange={setText} />
        </div>

        <div className="flex justify-end gap-3 px-5 py-3 border-t border-border">
          <button onClick={onClose} className="btn-ghost text-sm">
            Cancel
          </button>
          <button
            onClick={save}
            disabled={saving}
            className="btn-primary text-sm"
          >
            {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
            {isNew ? "Create" : "Save"}
          </button>
        </div>
      </motion.div>
    </motion.div>
  );
}

// ── Data Panel ────────────────────────────────────────────────────────────────

export function DataPanel() {
  const { activeDb, activeCol, setActiveSchema } = useStore();
  const [editDoc, setEditDoc] = useState<Document | null | "new">(undefined as unknown as "new");
  const [tableKey, setTableKey] = useState(0);

  const refresh = () => setTableKey((k) => k + 1);

  useEffect(() => {
    if (activeDb && activeCol) {
      api.getSchema(activeDb, activeCol)
         .then(setActiveSchema)
         .catch(() => setActiveSchema({ fields: [] }));
         
      // Real-time integration
      const token = localStorage.getItem("void_access_token") ?? "";
      const es = new EventSource(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700"}/v1/databases/${activeDb}/realtime?token=${encodeURIComponent(token)}`);
      es.onmessage = (e) => {
         try {
           const ev = JSON.parse(e.data);
           if (ev.collection === activeCol) {
             // For a robust system we'd merge the doc into Zustand directly,
             // but for simplicity we can trigger a refresh if the event matches the active collection.
             refresh();
           }
         } catch(err) {}
      };
      return () => es.close();
    }
  }, [activeDb, activeCol, setActiveSchema]);

  if (!activeDb || !activeCol) {
    return (
      <Card className="h-full flex items-center justify-center">
        <p className="text-muted-foreground text-sm">
          Select a database and collection from the sidebar
        </p>
      </Card>
    );
  }

  return (
    <div className="flex flex-col h-full gap-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-muted-foreground">
          <span className="text-foreground font-semibold">{activeDb}</span>
          <span className="mx-1.5 text-border">/</span>
          <span className="text-neon-500">{activeCol}</span>
        </h2>
        <button
          onClick={async () => {
             if (!activeDb || !activeCol) return;
             // Add empty document immediately to visually render an empty row inline
             try {
                const docFields: Record<string, unknown> = {};
                const schema = useStore.getState().activeSchema;
                if (schema && schema.fields) {
                  schema.fields.forEach(f => {
                    if (f.default === "now()") docFields[f.name] = new Date().toISOString();
                    else if (f.default === "uuid()") docFields[f.name] = crypto.randomUUID();
                  });
                }
                const res = await api.insertDocument(activeDb, activeCol, docFields);
                toast.success("Row inserted");
                refresh();
             } catch(e) {
                toast.error("Failed to insert row");
             }
          }}
          className="btn-primary text-sm"
        >
          <Plus className="w-4 h-4" />
          Add Row
        </button>
      </div>

      {/* Visual Filters */}
      <FilterBar />

      {/* Table */}
      <div className="flex-1 min-h-0">
        <DocumentTable
          key={tableKey}
          onEditDoc={(doc) => setEditDoc(doc)}
        />
      </div>

      {/* Modal */}
      <AnimatePresence>
        {editDoc !== (undefined as unknown as "new") && (
          <DocEditor
            doc={editDoc}
            onClose={() => setEditDoc(undefined as unknown as "new")}
            onSaved={() => { setEditDoc(undefined as unknown as "new"); refresh(); }}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
