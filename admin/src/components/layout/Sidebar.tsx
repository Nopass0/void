/**
 * @fileoverview Sidebar navigation for VoidDB Admin.
 */

"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  Database,
  Table2,
  HardDrive,
  Users,
  Settings,
  LayoutDashboard,
  ChevronRight,
  Plus,
  Loader2,
  LogOut,
  Zap,
  BookOpen,
  TerminalSquare,
  Copy,
  Trash2,
  FileJson,
  FolderInput,
  Boxes,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import { toast } from "sonner";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";

interface NavItemProps {
  icon: React.ReactNode;
  label: string;
  active?: boolean;
  onClick: () => void;
  badge?: string | number;
}

function NavItem({ icon, label, active, onClick, badge }: NavItemProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "w-full flex items-center gap-3.5 px-3 py-2.5 mb-0.5 rounded-md text-[13px] transition-all duration-150 text-left font-medium",
        active
          ? "bg-surface-3 text-foreground"
          : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
      )}
    >
      <span className={cn("flex items-center justify-center shrink-0 [&>svg]:w-[18px] [&>svg]:h-[18px]", active ? "text-neon-500" : "opacity-80")}>{icon}</span>
      <span className="truncate flex-1">{label}</span>
      {active && <span className="w-1 h-4 rounded-full bg-neon-500" />}
      {badge !== undefined && <span className="metric-badge">{badge}</span>}
    </button>
  );
}

type DbModalMode = "create" | "import";

interface DbModalProps {
  mode: DbModalMode;
  targetDatabase?: string | null;
  onClose: () => void;
  onSuccess: (database: string) => void;
}

function DatabaseModal({ mode, targetDatabase, onClose, onSuccess }: DbModalProps) {
  const [tab, setTab] = useState<DbModalMode>(mode);
  const [databaseName, setDatabaseName] = useState(targetDatabase || "");
  const [sourceUrl, setSourceUrl] = useState("");
  const [sourceSchema, setSourceSchema] = useState("public");
  const [dropExisting, setDropExisting] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    setTab(mode);
    setDatabaseName(targetDatabase || "");
  }, [mode, targetDatabase]);

  const submitCreate = async () => {
    if (!databaseName.trim()) {
      toast.error("Database name is required");
      return;
    }
    setSubmitting(true);
    try {
      await api.createDatabase(databaseName.trim());
      toast.success(`Database "${databaseName.trim()}" created`);
      onSuccess(databaseName.trim());
    } catch (err: any) {
      toast.error(err?.response?.data?.error || "Failed to create database");
    } finally {
      setSubmitting(false);
    }
  };

  const submitImport = async () => {
    if (!sourceUrl.trim()) {
      toast.error("PostgreSQL URL is required");
      return;
    }
    setSubmitting(true);
    try {
      const result = await api.importPostgresDatabase({
        source_url: sourceUrl.trim(),
        target_database: databaseName.trim() || undefined,
        source_schema: sourceSchema.trim() || "public",
        drop_existing: dropExisting,
      });
      toast.success(`Imported ${result.total_rows} row(s) from PostgreSQL`);
      onSuccess(result.database);
    } catch (err: any) {
      toast.error(err?.response?.data?.error || "PostgreSQL import failed");
    } finally {
      setSubmitting(false);
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
        transition={{ duration: 0.16 }}
        className="w-full max-w-xl rounded-xl border border-border bg-surface-2 shadow-modal"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-border">
          <div>
            <h3 className="text-sm font-semibold text-foreground">Database Setup</h3>
            <p className="text-xs text-muted-foreground">Create an empty database or import one from PostgreSQL.</p>
          </div>
          <button onClick={onClose} className="btn-ghost !p-1.5">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="px-5 pt-4">
          <div className="inline-flex rounded-md border border-border bg-surface-1 p-1">
            <button
              onClick={() => setTab("create")}
              className={cn("px-3 py-1.5 rounded text-xs font-medium", tab === "create" ? "bg-surface-3 text-foreground" : "text-muted-foreground")}
            >
              Create
            </button>
            <button
              onClick={() => setTab("import")}
              className={cn("px-3 py-1.5 rounded text-xs font-medium", tab === "import" ? "bg-surface-3 text-foreground" : "text-muted-foreground")}
            >
              Import PostgreSQL
            </button>
          </div>
        </div>

        <div className="p-5 space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              Target Database
            </label>
            <input
              value={databaseName}
              onChange={(e) => setDatabaseName(e.target.value)}
              placeholder="app"
              className="input-field text-sm"
            />
          </div>

          {tab === "create" ? (
            <p className="text-sm text-muted-foreground">
              Creates an empty VoidDB database that you can fill from the admin UI or API.
            </p>
          ) : (
            <>
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                  PostgreSQL URL
                </label>
                <textarea
                  value={sourceUrl}
                  onChange={(e) => setSourceUrl(e.target.value)}
                  placeholder="postgresql://user:pass@host:5432/db?sslmode=require"
                  className="input-field min-h-[92px] resize-y text-sm font-mono"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    Source Schema
                  </label>
                  <input
                    value={sourceSchema}
                    onChange={(e) => setSourceSchema(e.target.value)}
                    placeholder="public"
                    className="input-field text-sm"
                  />
                </div>

                <label className="flex items-center gap-3 rounded-lg border border-border bg-surface-1 px-3 py-2.5 self-end">
                  <input
                    type="checkbox"
                    checked={dropExisting}
                    onChange={(e) => setDropExisting(e.target.checked)}
                    className="accent-emerald-400"
                  />
                  <div>
                    <p className="text-sm font-medium text-foreground">Recreate target database</p>
                    <p className="text-xs text-muted-foreground">Drops the target VoidDB database before import.</p>
                  </div>
                </label>
              </div>
            </>
          )}
        </div>

        <div className="flex justify-end gap-3 px-5 py-4 border-t border-border">
          <button onClick={onClose} className="btn-ghost text-sm">
            Cancel
          </button>
          <button
            onClick={tab === "create" ? submitCreate : submitImport}
            disabled={submitting}
            className="btn-primary text-sm"
          >
            {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : tab === "create" ? <Plus className="w-4 h-4" /> : <FolderInput className="w-4 h-4" />}
            {tab === "create" ? "Create Database" : "Import Database"}
          </button>
        </div>
      </motion.div>
    </motion.div>
  );
}

export function Sidebar() {
  const {
    databases, setDatabases,
    collections, setCollections,
    activeDb, setActiveDb,
    activeCol, setActiveCol,
    activeTab, setActiveTab,
    user, setUser,
    sidebarOpen,
  } = useStore();

  const [expandedDbs, setExpandedDbs] = useState<Record<string, boolean>>({});
  const [loadingCols, setLoadingCols] = useState<string | null>(null);
  const [modalState, setModalState] = useState<{ mode: DbModalMode; targetDatabase?: string | null } | null>(null);

  const loadDatabases = useCallback(async () => {
    try {
      const dbs = await api.listDatabases();
      setDatabases(dbs);
    } catch {
      toast.error("Failed to load databases");
    }
  }, [setDatabases]);

  const loadCollections = useCallback(async (db: string) => {
    setLoadingCols(db);
    try {
      const cols = await api.listCollections(db);
      setCollections(db, cols);
      return cols;
    } catch {
      toast.error(`Failed to load collections for ${db}`);
      return [] as string[];
    } finally {
      setLoadingCols(null);
    }
  }, [setCollections]);

  const toggleDb = useCallback(async (db: string) => {
    const isExpanded = !!expandedDbs[db];
    setExpandedDbs((prev) => ({ ...prev, [db]: !isExpanded }));
    setActiveDb(db);
    if (!isExpanded && !collections[db]) {
      await loadCollections(db);
    }
  }, [collections, expandedDbs, loadCollections, setActiveDb]);

  const handleLogout = () => {
    api.logout();
    setUser(null);
    window.location.href = "/login";
  };

  const copyText = useCallback(async (text: string, label: string) => {
    await navigator.clipboard.writeText(text);
    toast.success(`${label} copied`);
  }, []);

  const handleDeleteDatabase = useCallback(async (db: string) => {
    if (!window.confirm(`Delete database "${db}"?`)) {
      return;
    }
    try {
      await api.deleteDatabase(db);
      if (activeDb === db) {
        setActiveDb(null);
        setActiveCol(null);
      }
      await loadDatabases();
      toast.success(`Database "${db}" deleted`);
    } catch (err: any) {
      toast.error(err?.response?.data?.error || `Failed to delete ${db}`);
    }
  }, [activeDb, loadDatabases, setActiveCol, setActiveDb]);

  const handleDeleteCollection = useCallback(async (db: string, col: string) => {
    if (!window.confirm(`Delete collection "${db}/${col}"?`)) {
      return;
    }
    try {
      await api.deleteCollection(db, col);
      if (activeDb === db && activeCol === col) {
        setActiveCol(null);
      }
      await loadCollections(db);
      toast.success(`Collection "${col}" deleted`);
    } catch (err: any) {
      toast.error(err?.response?.data?.error || `Failed to delete ${col}`);
    }
  }, [activeCol, activeDb, loadCollections, setActiveCol]);

  const handleCopyDatabaseSchema = useCallback(async (db: string) => {
    try {
      const schema = await api.exportDatabaseSchema(db);
      await copyText(schema, `Schema for ${db}`);
    } catch {
      toast.error("Failed to export database schema");
    }
  }, [copyText]);

  const handleCopyCollectionSchema = useCallback(async (db: string, col: string) => {
    try {
      const schema = await api.exportCollectionSchema(db, col);
      await copyText(schema, `Schema for ${db}/${col}`);
    } catch {
      toast.error("Failed to export collection schema");
    }
  }, [copyText]);

  const openImportModal = useCallback((targetDatabase?: string | null) => {
    setModalState({ mode: "import", targetDatabase });
  }, []);

  const handleModalSuccess = async (database: string) => {
    setModalState(null);
    await loadDatabases();
    setExpandedDbs((prev) => ({ ...prev, [database]: true }));
    setActiveDb(database);
    setActiveCol(null);
    setActiveTab("data");
    await loadCollections(database);
  };

  const dbMenu = useCallback((db: string): ContextMenuEntry[] => [
    {
      label: "Copy database name",
      icon: <Copy className="w-4 h-4" />,
      onClick: () => { void copyText(db, "Database name"); },
    },
    {
      label: "Copy database schema",
      icon: <FileJson className="w-4 h-4" />,
      onClick: () => { void handleCopyDatabaseSchema(db); },
    },
    { separator: true },
    {
      label: "Import into this database",
      icon: <FolderInput className="w-4 h-4" />,
      onClick: () => openImportModal(db),
    },
    {
      label: "Delete database",
      icon: <Trash2 className="w-4 h-4" />,
      danger: true,
      onClick: () => { void handleDeleteDatabase(db); },
    },
  ], [copyText, handleCopyDatabaseSchema, handleDeleteDatabase, openImportModal]);

  const collectionMenu = useCallback((db: string, col: string): ContextMenuEntry[] => [
    {
      label: "Copy collection name",
      icon: <Copy className="w-4 h-4" />,
      onClick: () => { void copyText(col, "Collection name"); },
    },
    {
      label: "Copy collection schema",
      icon: <FileJson className="w-4 h-4" />,
      onClick: () => { void handleCopyCollectionSchema(db, col); },
    },
    { separator: true },
    {
      label: "Delete collection",
      icon: <Trash2 className="w-4 h-4" />,
      danger: true,
      onClick: () => { void handleDeleteCollection(db, col); },
    },
  ], [copyText, handleCopyCollectionSchema, handleDeleteCollection]);

  useEffect(() => { void loadDatabases(); }, [loadDatabases]);

  const databaseRows = useMemo(() => databases.map((db) => (
    <div key={db}>
      <ContextMenu items={dbMenu(db)}>
        <button
          onClick={() => { void toggleDb(db); }}
          className={cn(
            "w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-all text-left",
            activeDb === db ? "text-foreground bg-surface-3" : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
          )}
        >
          <ChevronRight
            className={cn(
              "w-3 h-3 shrink-0 transition-transform duration-150",
              expandedDbs[db] && "rotate-90"
            )}
          />
          <Database className="w-3.5 h-3.5 shrink-0 text-neon-600" />
          <span className="truncate flex-1 font-medium">{db}</span>
          {loadingCols === db && <Loader2 className="w-3 h-3 animate-spin text-muted-foreground" />}
        </button>
      </ContextMenu>

      <AnimatePresence>
        {expandedDbs[db] && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="overflow-hidden ml-5 mt-0.5 space-y-px"
          >
            {(collections[db] || []).map((col) => (
              <ContextMenu key={col} items={collectionMenu(db, col)}>
                <button
                  onClick={() => {
                    setActiveDb(db);
                    setActiveCol(col);
                    setActiveTab("data");
                  }}
                  className={cn(
                    "w-full flex items-center gap-2 px-2 py-1 rounded-md text-xs transition-all text-left",
                    activeCol === col && activeDb === db
                      ? "text-neon-500 bg-neon-500/5"
                      : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
                  )}
                >
                  <Table2 className="w-3 h-3 shrink-0" />
                  <span className="truncate">{col}</span>
                </button>
              </ContextMenu>
            ))}
            <button
              onClick={async () => {
                const name = window.prompt("New collection name:");
                if (!name) return;
                try {
                  await api.createCollection(db, name);
                  await loadCollections(db);
                  toast.success(`Collection "${name}" created`);
                } catch (err: any) {
                  toast.error(err?.response?.data?.error || "Failed to create collection");
                }
              }}
              className="w-full flex items-center gap-2 px-2 py-1 text-xs text-muted-foreground hover:text-neon-500 transition-colors"
            >
              <Plus className="w-3 h-3" />
              <span>New collection</span>
            </button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )), [activeCol, activeDb, collectionMenu, collections, databases, dbMenu, expandedDbs, loadCollections, loadingCols, setActiveCol, setActiveDb, setActiveTab, toggleDb]);

  return (
    <AnimatePresence>
      {sidebarOpen && (
        <>
          <motion.aside
            key="sidebar"
            initial={{ x: -260, opacity: 0 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: -260, opacity: 0 }}
            transition={{ type: "spring", stiffness: 300, damping: 30 }}
            className="flex flex-col w-60 shrink-0 h-screen bg-surface-1 border-r border-border overflow-hidden"
          >
            <div className="flex items-center gap-2.5 px-4 py-4 border-b border-border">
              <div className="p-1.5 rounded-md bg-neon-500/10">
                <Zap className="w-4 h-4 text-neon-500" />
              </div>
              <div>
                <h1 className="text-sm font-semibold text-foreground leading-tight">VoidDB</h1>
                <p className="text-[10px] text-muted-foreground">Admin Console</p>
              </div>
            </div>

            <nav className="p-2 space-y-0.5 border-b border-border">
              <NavItem icon={<LayoutDashboard />} label="Dashboard" active={activeTab === "dashboard"} onClick={() => setActiveTab("dashboard")} />
              <NavItem icon={<HardDrive />} label="Blob Storage" active={activeTab === "blob"} onClick={() => setActiveTab("blob")} />
              <NavItem icon={<Users />} label="Users" active={activeTab === "users"} onClick={() => setActiveTab("users")} />
              <NavItem icon={<Settings />} label="Settings" active={activeTab === "settings"} onClick={() => setActiveTab("settings")} />
              <NavItem icon={<TerminalSquare />} label="Query" active={activeTab === "query"} onClick={() => setActiveTab("query")} />
              <NavItem icon={<BookOpen />} label="Docs" active={activeTab === "docs"} onClick={() => setActiveTab("docs")} />
              <NavItem icon={<TerminalSquare />} label="Logs" active={activeTab === "logs"} onClick={() => setActiveTab("logs")} />
            </nav>

            <div className="flex-1 overflow-y-auto p-2">
              <div className="flex items-center justify-between mb-1.5 px-2">
                <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
                  Databases
                </span>
                <button
                  onClick={() => setModalState({ mode: "create" })}
                  className="text-muted-foreground hover:text-neon-500 transition-colors"
                  title="Create or import database"
                >
                  <Plus className="w-3.5 h-3.5" />
                </button>
              </div>

              <div className="space-y-px">
                {databaseRows}
                {databases.length === 0 && (
                  <button
                    onClick={() => setModalState({ mode: "import" })}
                    className="w-full rounded-md border border-dashed border-border px-3 py-4 text-left text-xs text-muted-foreground hover:text-foreground hover:border-neon-500/40"
                  >
                    <div className="flex items-center gap-2 mb-1">
                      <Boxes className="w-3.5 h-3.5" />
                      <span className="font-medium">No databases yet</span>
                    </div>
                    <p>Create an empty DB or import one from PostgreSQL.</p>
                  </button>
                )}
              </div>
            </div>

            <div className="p-2 border-t border-border">
              <div className="flex items-center gap-2 px-2 py-2">
                <div className="w-7 h-7 rounded-md bg-surface-4 flex items-center justify-center text-neon-500 text-xs font-bold uppercase">
                  {user?.id?.[0] ?? "?"}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium truncate">{user?.id ?? "…"}</p>
                  <p className="text-xs text-muted-foreground capitalize">{user?.role ?? ""}</p>
                </div>
                <button
                  onClick={handleLogout}
                  className="text-muted-foreground hover:text-red-400 transition-colors"
                  title="Logout"
                >
                  <LogOut className="w-4 h-4" />
                </button>
              </div>
            </div>
          </motion.aside>

          <AnimatePresence>
            {modalState && (
              <DatabaseModal
                mode={modalState.mode}
                targetDatabase={modalState.targetDatabase}
                onClose={() => setModalState(null)}
                onSuccess={(database) => { void handleModalSuccess(database); }}
              />
            )}
          </AnimatePresence>
        </>
      )}
    </AnimatePresence>
  );
}
