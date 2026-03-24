/**
 * @fileoverview Top header bar with breadcrumbs, sidebar toggle and stats pill.
 */

"use client";

import React from "react";
import { motion } from "framer-motion";
import { Menu, Activity, RefreshCw } from "lucide-react";
import { useStore } from "@/store";
import { formatBytes, formatNumber } from "@/lib/utils";
import * as api from "@/lib/api";

/**
 * Header renders the top navigation bar.
 * It shows the current db/collection breadcrumb, a sidebar toggle button,
 * and a real-time engine stats pill.
 */
export function Header() {
  const { sidebarOpen, setSidebarOpen, activeDb, activeCol, stats, setStats } =
    useStore();
  const [refreshing, setRefreshing] = React.useState(false);

  const refresh = async () => {
    setRefreshing(true);
    try {
      const s = await api.getStats();
      setStats(s);
    } finally {
      setRefreshing(false);
    }
  };

  // Poll stats every 5 s.
  React.useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const breadcrumb = [activeDb, activeCol].filter(Boolean).join(" / ");

  return (
    <header className="glass-dark h-14 flex items-center px-4 gap-4 border-b border-void-500/20 shrink-0 z-10">
      {/* Sidebar toggle */}
      <button
        onClick={() => setSidebarOpen(!sidebarOpen)}
        className="text-muted-foreground hover:text-foreground transition-colors"
      >
        <Menu className="w-5 h-5" />
      </button>

      {/* Breadcrumb */}
      <div className="flex-1 min-w-0">
        {breadcrumb ? (
          <motion.p
            key={breadcrumb}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            className="text-sm font-mono text-void-300 truncate"
          >
            {breadcrumb}
          </motion.p>
        ) : (
          <p className="text-sm text-muted-foreground">VoidDB Admin</p>
        )}
      </div>

      {/* Stats pill */}
      {stats && (
        <div className="hidden md:flex items-center gap-3 text-xs font-mono">
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-full glass border border-void-500/20">
            <Activity className="w-3 h-3 text-void-400 animate-pulse-slow" />
            <span className="text-muted-foreground">mem</span>
            <span className="text-void-300">{formatBytes(stats.memtable_size)}</span>
            <span className="text-muted-foreground mx-1">•</span>
            <span className="text-muted-foreground">segs</span>
            <span className="text-void-300">{formatNumber(stats.segments)}</span>
            <span className="text-muted-foreground mx-1">•</span>
            <span className="text-muted-foreground">cache</span>
            <span className="text-void-300">{formatBytes(stats.cache_used)}</span>
          </div>
        </div>
      )}

      {/* Refresh */}
      <button
        onClick={refresh}
        className="text-muted-foreground hover:text-void-400 transition-colors"
        title="Refresh stats"
      >
        <RefreshCw className={`w-4 h-4 ${refreshing ? "animate-spin" : ""}`} />
      </button>
    </header>
  );
}
