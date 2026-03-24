/**
 * @fileoverview DataPanel – full CRUD interface for documents in a collection.
 */

"use client";

import React, { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, X, Save, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import { DocumentTable } from "@/components/features/DocumentTable";
import { QueryEditor } from "@/components/features/QueryEditor";
import { GlassCard } from "@/components/ui/glass-card";
import * as api from "@/lib/api";
import type { Document } from "@/lib/api";
import dynamic from "next/dynamic";

const CodeMirror = dynamic(() => import("@uiw/react-codemirror"), { ssr: false });

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
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      onClick={onClose}
    >
      <motion.div
        initial={{ scale: 0.9, y: 20 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.9, y: 20 }}
        onClick={(e) => e.stopPropagation()}
        className="glass rounded-2xl w-full max-w-2xl border border-void-500/20 overflow-hidden"
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-void-500/20">
          <h2 className="font-semibold gradient-text">
            {isNew ? "New Document" : `Edit: ${doc._id}`}
          </h2>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-4">
          <CodeMirror
            value={text}
            height="300px"
            onChange={setText}
            basicSetup={{ lineNumbers: true }}
            style={{ fontSize: "13px" }}
          />
        </div>

        <div className="flex justify-end gap-3 px-5 py-4 border-t border-void-500/20">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={save}
            disabled={saving}
            className="flex items-center gap-2 px-4 py-2 bg-void-600/50 hover:bg-void-600/70 text-void-100 text-sm rounded-lg border border-void-500/30 transition-colors disabled:opacity-50"
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

/**
 * DataPanel shows the QueryEditor and DocumentTable with Insert/Edit support.
 */
export function DataPanel() {
  const { activeDb, activeCol } = useStore();
  const [editDoc, setEditDoc] = useState<Document | null | "new">(undefined as unknown as "new");
  const [tableKey, setTableKey] = useState(0);

  const refresh = () => setTableKey((k) => k + 1);

  if (!activeDb || !activeCol) {
    return (
      <GlassCard className="h-full flex items-center justify-center">
        <p className="text-muted-foreground text-sm">
          Select a database and collection from the sidebar
        </p>
      </GlassCard>
    );
  }

  return (
    <div className="flex flex-col h-full gap-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-muted-foreground">
          <span className="text-foreground">{activeDb}</span>
          <span className="mx-1 opacity-40">/</span>
          <span className="text-void-300">{activeCol}</span>
        </h2>
        <motion.button
          whileTap={{ scale: 0.96 }}
          onClick={() => setEditDoc(null)}
          className="flex items-center gap-1.5 text-sm bg-void-600/40 hover:bg-void-600/60 text-void-200 px-3 py-2 rounded-lg border border-void-500/30 transition-colors"
        >
          <Plus className="w-4 h-4" />
          Insert
        </motion.button>
      </div>

      {/* Query editor */}
      <QueryEditor />

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
