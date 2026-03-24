/**
 * @fileoverview Header – top bar with breadcrumbs and sidebar toggle.
 */

"use client";

import React from "react";
import { motion } from "framer-motion";
import { Menu, RefreshCw } from "lucide-react";
import { useStore } from "@/store";
import { formatBytes, formatNumber } from "@/lib/utils";
import * as api from "@/lib/api";

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

  React.useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const breadcrumb = [activeDb, activeCol].filter(Boolean).join(" / ");

  return (
    <header className="h-12 flex items-center px-4 gap-4 border-b border-border bg-surface-1 shrink-0 z-10">
      {/* Sidebar toggle */}
      <button
        onClick={() => setSidebarOpen(!sidebarOpen)}
        className="btn-ghost p-1.5"
      >
        <Menu className="w-4 h-4" />
      </button>

      {/* Breadcrumb */}
      <div className="flex-1 min-w-0">
        {breadcrumb ? (
          <motion.p
            key={breadcrumb}
            initial={{ opacity: 0, x: -4 }}
            animate={{ opacity: 1, x: 0 }}
            className="text-sm font-mono text-muted-foreground truncate"
          >
            {breadcrumb}
          </motion.p>
        ) : (
          <p className="text-sm text-muted-foreground">VoidDB Console</p>
        )}
      </div>

      {/* Stats pills */}
      {stats && (
        <div className="hidden md:flex items-center gap-4 text-xs font-mono text-muted-foreground">
          <span>
            mem <span className="text-foreground">{formatBytes(stats.memtable_size)}</span>
          </span>
          <span className="text-border">|</span>
          <span>
            segs <span className="text-foreground">{formatNumber(stats.segments)}</span>
          </span>
          <span className="text-border">|</span>
          <span>
            cache <span className="text-foreground">{formatBytes(stats.cache_used)}</span>
          </span>
        </div>
      )}

      {/* Refresh */}
      <button
        onClick={refresh}
        className="btn-ghost p-1.5"
        title="Refresh stats"
      >
        <RefreshCw className={`w-3.5 h-3.5 ${refreshing ? "animate-spin" : ""}`} />
      </button>
    </header>
  );
}
