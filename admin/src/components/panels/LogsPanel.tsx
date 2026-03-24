"use client";

import React, { useEffect, useState, useCallback, useMemo } from "react";
import { RefreshCw, Search, ChevronLeft, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { formatNumber, cn } from "@/lib/utils";
import axios from "axios";
import { Card } from "@/components/ui/glass-card";

interface LogEntry {
  level: string;
  time: string;
  message: string;
  fields?: Record<string, unknown>;
}

export function LogsPanel() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [search, setSearch] = useState("");

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const token = localStorage.getItem("void_access_token");
      const res = await axios.get<{ logs: LogEntry[]; count: number }>(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700"}/v1/logs`,
        {
          headers: { Authorization: `Bearer ${token}` },
          params: { limit: pageSize, skip: page * pageSize },
        }
      );
      setLogs(res.data.logs || []);
      setTotalCount(res.data.count || 0);
    } catch {
      toast.error("Failed to load logs");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    const token = localStorage.getItem("void_access_token") ?? "";
    const es = new EventSource(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700"}/v1/logs/realtime?token=${encodeURIComponent(token)}`);
    es.onmessage = (e) => {
      try {
        const entry: LogEntry = JSON.parse(e.data);
        setLogs(prev => [entry, ...prev]);
        setTotalCount(c => c + 1);
      } catch (err) {
        console.error("Failed to parse live log", err);
      }
    };
    return () => es.close();
  }, []);

  const filteredLogs = useMemo(() => {
    if (!search) return logs;
    const s = search.toLowerCase();
    return logs.filter(
      l => l.message.toLowerCase().includes(s) || 
           l.level.toLowerCase().includes(s) ||
           (l.fields && JSON.stringify(l.fields).toLowerCase().includes(s))
    );
  }, [logs, search]);

  const totalPages = Math.ceil(totalCount / pageSize) || 1;

  const getLevelColor = (level: string) => {
    level = level.toLowerCase();
    if (level === "error" || level === "fatal" || level === "panic") return "text-red-400";
    if (level === "warn" || level === "warning") return "text-amber-400";
    if (level === "info") return "text-blue-400";
    if (level === "debug") return "text-purple-400";
    return "text-muted-foreground";
  };

  return (
    <div className="flex flex-col h-full gap-4 max-w-6xl mx-auto w-full">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold gradient-text tracking-tight">System Logs</h2>
          <p className="text-sm text-muted-foreground">Real-time internal engine diagnostics.</p>
        </div>
        <button onClick={fetchLogs} disabled={loading} className="btn-secondary">
          <RefreshCw className={cn("w-4 h-4", loading && "animate-spin")} />
          Refresh
        </button>
      </div>

      <Card className="flex-1 flex flex-col p-4 overflow-hidden gap-3">
        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <div className="relative w-64">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <input 
              type="text" 
              placeholder="Search logs..." 
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="input-field !pl-9"
            />
          </div>
          <div className="flex-1" />
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span>Rows:</span>
            <select
              value={pageSize}
              onChange={(e) => { setPageSize(Number(e.target.value)); setPage(0); }}
              className="select-field !py-1.5 !px-2 !w-auto"
            >
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
              <option value={500}>500</option>
            </select>
          </div>
        </div>

        {/* Logs Table */}
        <div className="flex-1 overflow-x-auto rounded border border-border bg-surface-1">
          <table className="data-table">
            <thead>
              <tr>
                <th className="w-40">Timestamp</th>
                <th className="w-24">Level</th>
                <th>Message</th>
                <th>Context</th>
              </tr>
            </thead>
            <tbody>
              {filteredLogs.map((log, i) => (
                <tr key={i} className="font-mono text-[13px] hover:bg-surface-3 transition-colors border-b border-border/50">
                  <td className="px-3 py-2 text-muted-foreground whitespace-nowrap">
                    {new Date(log.time).toLocaleString(undefined, {
                       month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'
                    })}
                  </td>
                  <td className={`px-3 py-2 font-medium ${getLevelColor(log.level)} uppercase`}>
                    {log.level}
                  </td>
                  <td className="px-3 py-2 text-foreground whitespace-pre-wrap break-words">
                    {log.message}
                  </td>
                  <td className="px-3 py-2 text-amber-500/80 truncate max-w-xs" title={JSON.stringify(log.fields || {})}>
                    {log.fields && Object.keys(log.fields).length > 0 ? JSON.stringify(log.fields) : "-"}
                  </td>
                </tr>
              ))}
              {filteredLogs.length === 0 && (
                <tr>
                  <td colSpan={4} className="h-32 text-center text-muted-foreground">
                    {loading ? "Loading logs..." : "No logs found"}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination control */}
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">
            Showing {formatNumber(filteredLogs.length)} logs
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage(Math.max(0, page - 1))}
              disabled={page === 0}
              className="btn-ghost"
            >
              <ChevronLeft className="w-4 h-4" />
              Prev
            </button>
            <span className="font-medium px-2">{page + 1} / {totalPages}</span>
            <button
              onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
              disabled={page >= totalPages - 1}
              className="btn-ghost"
            >
              Next
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      </Card>
    </div>
  );
}
