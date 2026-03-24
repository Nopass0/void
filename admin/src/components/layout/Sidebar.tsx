/**
 * @fileoverview Sidebar navigation for VoidDB Admin.
 * Shows databases, collections and top-level navigation links.
 * Uses Framer Motion for smooth open/close animations.
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
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import { toast } from "sonner";

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
    <motion.button
      whileTap={{ scale: 0.97 }}
      onClick={onClick}
      className={cn(
        "w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-all duration-150 text-left",
        active
          ? "bg-void-600/30 text-void-300 border border-void-500/30 shadow-neon-sm"
          : "text-muted-foreground hover:text-foreground hover:bg-white/5"
      )}
    >
      <span className={cn("w-4 h-4 shrink-0", active && "text-void-400")}>{icon}</span>
      <span className="truncate flex-1">{label}</span>
      {badge !== undefined && (
        <span className="metric-badge">{badge}</span>
      )}
    </motion.button>
  );
}

// ── Main Sidebar ──────────────────────────────────────────────────────────────

/**
 * Sidebar renders the left navigation panel.
 * On mobile it can be hidden/shown via the sidebarOpen store flag.
 */
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

  /** Loads the list of databases from the API. */
  const loadDatabases = useCallback(async () => {
    try {
      const dbs = await api.listDatabases();
      setDatabases(dbs);
    } catch {
      toast.error("Failed to load databases");
    }
  }, [setDatabases]);

  /** Toggles a database in the sidebar tree, loading its collections on demand. */
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
          initial={{ x: -280, opacity: 0 }}
          animate={{ x: 0, opacity: 1 }}
          exit={{ x: -280, opacity: 0 }}
          transition={{ type: "spring", stiffness: 300, damping: 30 }}
          className="glass-dark flex flex-col w-64 shrink-0 h-screen border-r border-void-500/20 overflow-hidden"
        >
          {/* Logo */}
          <div className="flex items-center gap-2 px-4 py-5 border-b border-void-500/20">
            <div className="p-1.5 rounded-lg bg-void-600/30 shadow-neon-sm">
              <Zap className="w-5 h-5 text-void-400" />
            </div>
            <div>
              <h1 className="text-base font-bold gradient-text leading-tight">VoidDB</h1>
              <p className="text-[10px] text-muted-foreground">Admin Panel</p>
            </div>
          </div>

          {/* Main nav */}
          <nav className="p-3 space-y-1 border-b border-void-500/20">
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
          </nav>

          {/* Database tree */}
          <div className="flex-1 overflow-y-auto p-3">
            <div className="flex items-center justify-between mb-2 px-1">
              <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
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
                className="text-muted-foreground hover:text-void-400 transition-colors"
              >
                <Plus className="w-3.5 h-3.5" />
              </button>
            </div>

            <div className="space-y-0.5">
              {databases.map((db) => (
                <div key={db}>
                  {/* Database row */}
                  <motion.button
                    whileTap={{ scale: 0.98 }}
                    onClick={() => toggleDb(db)}
                    className={cn(
                      "w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-all text-left",
                      activeDb === db
                        ? "text-void-300 bg-void-600/20"
                        : "text-muted-foreground hover:text-foreground hover:bg-white/5"
                    )}
                  >
                    <ChevronRight
                      className={cn(
                        "w-3 h-3 shrink-0 transition-transform",
                        expandedDbs[db] && "rotate-90"
                      )}
                    />
                    <Database className="w-3.5 h-3.5 shrink-0" />
                    <span className="truncate flex-1 font-medium">{db}</span>
                    {loadingCols === db && (
                      <Loader2 className="w-3 h-3 animate-spin" />
                    )}
                  </motion.button>

                  {/* Collections */}
                  <AnimatePresence>
                    {expandedDbs[db] && (
                      <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.2 }}
                        className="overflow-hidden ml-4 mt-0.5 space-y-0.5"
                      >
                        {(collections[db] || []).map((col) => (
                          <motion.button
                            key={col}
                            whileTap={{ scale: 0.98 }}
                            onClick={() => {
                              setActiveCol(col);
                              setActiveTab("data");
                            }}
                            className={cn(
                              "w-full flex items-center gap-2 px-2 py-1 rounded-lg text-xs transition-all text-left",
                              activeCol === col && activeDb === db
                                ? "text-void-300 bg-void-600/20 border-l border-void-500"
                                : "text-muted-foreground hover:text-foreground hover:bg-white/5"
                            )}
                          >
                            <Table2 className="w-3 h-3 shrink-0" />
                            <span className="truncate">{col}</span>
                          </motion.button>
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
                          className="w-full flex items-center gap-2 px-2 py-1 text-xs text-muted-foreground hover:text-void-400 transition-colors"
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
          <div className="p-3 border-t border-void-500/20">
            <div className="flex items-center gap-2 px-2 py-2">
              <div className="w-7 h-7 rounded-full bg-void-600/30 flex items-center justify-center text-void-400 text-xs font-bold uppercase">
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
