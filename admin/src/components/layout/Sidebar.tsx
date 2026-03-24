/**
 * @fileoverview Sidebar navigation for VoidDB Admin.
 * Clean dark design with database tree navigation.
 */

"use client";

import React, { useEffect, useState, useCallback } from "react";
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
  Trash2,
  FolderPlus,
  Eye,
  TerminalSquare,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import { toast } from "sonner";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";

// ── Sub-components ────────────────────────────────────────────────────────────

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
      {badge !== undefined && (
        <span className="metric-badge">{badge}</span>
      )}
    </button>
  );
}

// ── Main Sidebar ──────────────────────────────────────────────────────────────

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

  const loadDatabases = useCallback(async () => {
    try {
      const dbs = await api.listDatabases();
      setDatabases(dbs);
    } catch {
      toast.error("Failed to load databases");
    }
  }, [setDatabases]);

  const toggleDb = useCallback(
    async (db: string) => {
      const isExpanded = expandedDbs[db];
      setExpandedDbs((prev) => ({ ...prev, [db]: !isExpanded }));
      setActiveDb(db);
      if (!isExpanded && !collections[db]) {
        setLoadingCols(db);
        try {
          const cols = await api.listCollections(db);
          setCollections(db, cols);
        } catch {
          toast.error(`Failed to load collections for ${db}`);
        } finally {
          setLoadingCols(null);
        }
      }
    },
    [expandedDbs, collections, setActiveDb, setCollections]
  );

  const handleLogout = () => {
    api.logout();
    setUser(null);
    window.location.href = "/login";
  };

  useEffect(() => { loadDatabases(); }, [loadDatabases]);

  return (
    <AnimatePresence>
      {sidebarOpen && (
        <motion.aside
          key="sidebar"
          initial={{ x: -260, opacity: 0 }}
          animate={{ x: 0, opacity: 1 }}
          exit={{ x: -260, opacity: 0 }}
          transition={{ type: "spring", stiffness: 300, damping: 30 }}
          className="flex flex-col w-60 shrink-0 h-screen bg-surface-1 border-r border-border overflow-hidden"
        >
          {/* Logo */}
          <div className="flex items-center gap-2.5 px-4 py-4 border-b border-border">
            <div className="p-1.5 rounded-md bg-neon-500/10">
              <Zap className="w-4 h-4 text-neon-500" />
            </div>
            <div>
              <h1 className="text-sm font-semibold text-foreground leading-tight">VoidDB</h1>
              <p className="text-[10px] text-muted-foreground">Admin Console</p>
            </div>
          </div>

          {/* Main nav */}
          <nav className="p-2 space-y-0.5 border-b border-border">
            <NavItem
              icon={<LayoutDashboard />}
              label="Dashboard"
              active={activeTab === "dashboard"}
              onClick={() => setActiveTab("dashboard")}
            />
            <NavItem
              icon={<HardDrive />}
              label="Blob Storage"
              active={activeTab === "blob"}
              onClick={() => setActiveTab("blob")}
            />
            <NavItem
              icon={<Users />}
              label="Users"
              active={activeTab === "users"}
              onClick={() => setActiveTab("users")}
            />
            <NavItem
              icon={<Settings />}
              label="Settings"
              active={activeTab === "settings"}
              onClick={() => setActiveTab("settings")}
            />
            <NavItem
              icon={<TerminalSquare />}
              label="Query"
              active={activeTab === "query"}
              onClick={() => setActiveTab("query")}
            />
            <NavItem
              icon={<BookOpen />}
              label="Docs"
              active={activeTab === "docs"}
              onClick={() => setActiveTab("docs")}
            />
            <NavItem
              icon={<TerminalSquare />}
              label="Logs"
              active={activeTab === "logs"}
              onClick={() => setActiveTab("logs")}
            />
          </nav>

          {/* Database tree */}
          <div className="flex-1 overflow-y-auto p-2">
            <div className="flex items-center justify-between mb-1.5 px-2">
              <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
                Databases
              </span>
              <button
                onClick={async () => {
                  const name = prompt("New database name:");
                  if (!name) return;
                  await api.createDatabase(name);
                  loadDatabases();
                  toast.success(`Database "${name}" created`);
                }}
                className="text-muted-foreground hover:text-neon-500 transition-colors"
              >
                <Plus className="w-3.5 h-3.5" />
              </button>
            </div>

            <div className="space-y-px">
              {databases.map((db) => (
                <div key={db}>
                  <button
                    onClick={() => toggleDb(db)}
                    className={cn(
                      "w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-all text-left",
                      activeDb === db
                        ? "text-foreground bg-surface-3"
                        : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
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
                    {loadingCols === db && (
                      <Loader2 className="w-3 h-3 animate-spin text-muted-foreground" />
                    )}
                  </button>

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
                          <button
                            key={col}
                            onClick={() => {
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
                        ))}
                        <button
                          onClick={async () => {
                            const name = prompt("New collection name:");
                            if (!name) return;
                            await api.createCollection(db, name);
                            const cols = await api.listCollections(db);
                            setCollections(db, cols);
                            toast.success(`Collection "${name}" created`);
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
              ))}
            </div>
          </div>

          {/* User info */}
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
      )}
    </AnimatePresence>
  );
}
