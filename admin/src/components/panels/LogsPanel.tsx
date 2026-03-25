"use client";

import React, { useEffect, useState, useCallback, useMemo } from "react";
import { RefreshCw, Search, ChevronLeft, ChevronRight, Braces } from "lucide-react";
import { toast } from "sonner";
import { formatNumber, cn } from "@/lib/utils";
import axios from "axios";
import { Card } from "@/components/ui/glass-card";
import { AnimatePresence, motion } from "framer-motion";

interface LogEntry {
  level: string;
  time: string;
  message: string;
  fields?: Record<string, unknown>;
}

function prettyContext(fields?: Record<string, unknown>): string {
  if (!fields || Object.keys(fields).length === 0) return "{}";
  return JSON.stringify(fields, null, 2);
}

export function LogsPanel() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [search, setSearch] = useState("");
  const [contextDialog, setContextDialog] = useState<LogEntry | null>(null);

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
    es.onmessage = (event) => {
      try {
        const entry: LogEntry = JSON.parse(event.data);
        setLogs((current) => [entry, ...current]);
        setTotalCount((count) => count + 1);
      } catch (err) {
        console.error("Failed to parse live log", err);
      }
    };
    return () => es.close();
  }, []);

  const filteredLogs = useMemo(() => {
    if (!search) return logs;
    const needle = search.toLowerCase();
    return logs.filter(
      (log) =>
        log.message.toLowerCase().includes(needle) ||
        log.level.toLowerCase().includes(needle) ||
        prettyContext(log.fields).toLowerCase().includes(needle)
    );
  }, [logs, search]);

  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize));

  const getLevelColor = (level: string) => {
    const normalized = level.toLowerCase();
    if (normalized === "error" || normalized === "fatal" || normalized === "panic") return "text-red-400";
    if (normalized === "warn" || normalized === "warning") return "text-amber-400";
    if (normalized === "info") return "text-blue-400";
    if (normalized === "debug") return "text-purple-400";
    return "text-muted-foreground";
  };

  return (
    <div className="flex h-full flex-col gap-4 w-full">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold gradient-text tracking-tight">System Logs</h2>
          <p className="text-sm text-muted-foreground">Real-time engine, API and admin diagnostics.</p>
        </div>
        <button onClick={fetchLogs} disabled={loading} className="btn-secondary">
          <RefreshCw className={cn("w-4 h-4", loading && "animate-spin")} />
          Refresh
        </button>
      </div>

      <Card className="flex-1 min-h-0 flex flex-col p-4 overflow-hidden gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative min-w-[16rem] flex-1 max-w-sm">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              placeholder="Search logs..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              className="input-field !pl-9"
            />
          </div>
          <div className="flex-1" />
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span>Rows:</span>
            <select
              value={pageSize}
              onChange={(event) => {
                setPageSize(Number(event.target.value));
                setPage(0);
              }}
              className="select-field !py-1.5 !px-2 !w-auto"
            >
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
              <option value={500}>500</option>
            </select>
          </div>
        </div>

        <div className="flex-1 min-h-0 overflow-auto rounded border border-border bg-surface-1">
          <table className="data-table w-full min-w-full">
            <colgroup>
              <col style={{ width: "14rem" }} />
              <col style={{ width: "7rem" }} />
              <col />
              <col style={{ width: "20rem" }} />
              <col style={{ width: "5.5rem" }} />
            </colgroup>
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Level</th>
                <th>Message</th>
                <th>Context</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {filteredLogs.map((log, index) => (
                <tr key={`${log.time}-${index}`} className="font-mono text-[13px] border-b border-border/50">
                  <td className="px-3 py-2 text-muted-foreground whitespace-nowrap align-top">
                    {new Date(log.time).toLocaleString(undefined, {
                      month: "short",
                      day: "2-digit",
                      hour: "2-digit",
                      minute: "2-digit",
                      second: "2-digit",
                    })}
                  </td>
                  <td className={`px-3 py-2 font-medium uppercase align-top ${getLevelColor(log.level)}`}>
                    {log.level}
                  </td>
                  <td className="px-3 py-2 text-foreground whitespace-pre-wrap break-words align-top">
                    {log.message}
                  </td>
                  <td className="px-3 py-2 text-amber-500/80 whitespace-pre-wrap break-words align-top">
                    <div className="max-h-24 overflow-hidden">
                      {prettyContext(log.fields)}
                    </div>
                  </td>
                  <td className="px-3 py-2 align-top">
                    <button
                      onClick={() => setContextDialog(log)}
                      className="btn-ghost text-xs !py-1"
                      title="Open full context"
                    >
                      <Braces className="w-3.5 h-3.5" />
                      Open
                    </button>
                  </td>
                </tr>
              ))}
              {filteredLogs.length === 0 && (
                <tr>
                  <td colSpan={5} className="h-32 text-center text-muted-foreground">
                    {loading ? "Loading logs..." : "No logs found"}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">
            Showing {formatNumber(filteredLogs.length)} log entries
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

      <AnimatePresence>
        {contextDialog && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
            onClick={() => setContextDialog(null)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.96, y: 8 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.96, y: 8 }}
              className="w-full max-w-3xl rounded-xl border border-border bg-surface-2 shadow-modal overflow-hidden"
              onClick={(event) => event.stopPropagation()}
            >
              <div className="flex items-center justify-between border-b border-border px-5 py-3">
                <div>
                  <h3 className="text-sm font-semibold text-foreground">Log Context</h3>
                  <p className="text-xs text-muted-foreground">
                    {contextDialog.level.toUpperCase()} • {contextDialog.message}
                  </p>
                </div>
                <button onClick={() => setContextDialog(null)} className="btn-ghost !p-1.5">
                  Close
                </button>
              </div>
              <div className="p-5">
                <textarea
                  readOnly
                  value={prettyContext(contextDialog.fields)}
                  className="input-field min-h-[20rem] resize-y font-mono text-xs leading-6"
                />
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
