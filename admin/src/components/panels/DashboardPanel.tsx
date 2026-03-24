/**
 * @fileoverview DashboardPanel – overview of engine health and key metrics.
 */

"use client";

import React from "react";
import { Database, Layers, HardDrive, Activity, Cpu, MemoryStick } from "lucide-react";
import { useStore } from "@/store";
import { Card, StatCard } from "@/components/ui/glass-card";
import { MetricsChart } from "@/components/features/MetricsChart";
import { formatBytes, formatNumber } from "@/lib/utils";
import { toast } from "sonner";
import { Copy } from "lucide-react";

export function DashboardPanel() {
  const { stats, databases, statsHistory } = useStore();

  const writeOps =
    statsHistory.length >= 2
      ? Math.max(
          0,
          (statsHistory[statsHistory.length - 1]?.wal_seq ?? 0) -
            (statsHistory[statsHistory.length - 2]?.wal_seq ?? 0)
        )
      : 0;

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Hero banner */}
      <Card className="p-6">
        <div className="flex items-center gap-4">
          <div className="space-y-1">
            <h2 className="text-xl font-bold text-foreground">VoidDB Engine</h2>
            <p className="text-sm text-muted-foreground">
              LSM-tree · Bloom filters · Concurrent skip-list memtable
            </p>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-500 opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-neon-500" />
            </span>
            <span className="text-xs text-neon-500 font-medium">Running</span>
          </div>
        </div>
      </Card>

      {/* KPI grid */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        <StatCard
          icon={<Database className="w-4 h-4" />}
          label="Databases"
          value={databases.length}
          delay={0.05}
        />
        <StatCard
          icon={<Layers className="w-4 h-4" />}
          label="Segments"
          value={stats ? formatNumber(stats.segments) : "—"}
          sub="SSTable files on disk"
          delay={0.1}
        />
        <StatCard
          icon={<MemoryStick className="w-4 h-4" />}
          label="Memtable"
          value={stats ? formatBytes(stats.memtable_size) : "—"}
          sub={stats ? `${formatNumber(stats.memtable_count)} entries` : undefined}
          delay={0.15}
        />
        <StatCard
          icon={<HardDrive className="w-4 h-4" />}
          label="Block Cache"
          value={stats ? formatBytes(stats.cache_used) : "—"}
          sub={stats ? `${formatNumber(stats.cache_len)} entries` : undefined}
          delay={0.2}
        />
        <StatCard
          icon={<Activity className="w-4 h-4" />}
          label="WAL Seq"
          value={stats ? formatNumber(stats.wal_seq) : "—"}
          sub="Write sequence number"
          delay={0.25}
        />
        <StatCard
          icon={<Cpu className="w-4 h-4" />}
          label="Write Ops/s"
          value={writeOps}
          sub="Last 5-second window"
          trend={writeOps > 0 ? "up" : "neutral"}
          delay={0.3}
        />
      </div>

      {/* Charts */}
      <Card delay={0.35} className="p-5">
        <h3 className="text-sm font-medium text-muted-foreground mb-4">
          Memory Usage (live)
        </h3>
        <MetricsChart />
      </Card>

      {/* Quick-start */}
      <Card delay={0.4} className="p-5">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-foreground">Quick Start & Connection</h3>
          <div className="flex gap-2">
            <button 
              onClick={() => {
                const token = localStorage.getItem("void_access_token");
                navigator.clipboard.writeText(token || "");
                toast.success("Auth Token copied");
              }}
              className="btn-secondary text-xs !py-1"
            >
              <Copy className="w-3 h-3" />
              Copy Token
            </button>
            <button 
              onClick={() => {
                const token = localStorage.getItem("void_access_token");
                const code = `import { VoidClient } from 'voiddb-orm';\n\nconst db = new VoidClient({\n  url: '${process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700"}',\n  token: '${token}'\n});`;
                navigator.clipboard.writeText(code);
                toast.success("ORM Config copied");
              }}
              className="btn-primary text-xs !py-1"
            >
              <Copy className="w-3 h-3" />
              Copy ORM Config
            </button>
          </div>
        </div>
        <pre className="text-xs font-mono text-muted-foreground bg-surface-0 rounded-md p-4 overflow-x-auto border border-border">
{`# Connect with the TypeScript ORM
import { VoidClient } from 'voiddb-orm'
const db = new VoidClient({ url: 'http://localhost:7700', token: '<token>' })
const col = db.database('myapp').collection('users')
await col.insert({ name: 'Alice', age: 30 })
const users = await col.find({ where: [{ field: 'age', op: 'gte', value: 18 }] })`}
        </pre>
      </Card>
    </div>
  );
}
