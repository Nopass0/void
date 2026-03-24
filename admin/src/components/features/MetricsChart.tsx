/**
 * @fileoverview MetricsChart – renders a live sparkline for engine metrics
 * using Recharts, showing memtable size and cache usage over time.
 */

"use client";

import React from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { useStore } from "@/store";
import { formatBytes } from "@/lib/utils";

/** Custom tooltip styled to match the glassmorphism theme. */
const CustomTooltip = ({
  active,
  payload,
}: {
  active?: boolean;
  payload?: { name: string; value: number; color: string }[];
}) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="glass rounded-lg p-2 border border-void-500/20 text-xs space-y-1">
      {payload.map((p) => (
        <div key={p.name} className="flex items-center gap-2">
          <span style={{ color: p.color }}>■</span>
          <span className="text-muted-foreground">{p.name}:</span>
          <span className="font-mono">{formatBytes(p.value)}</span>
        </div>
      ))}
    </div>
  );
};

/**
 * MetricsChart renders live AreaCharts for memtable size and cache usage.
 * Data comes from the statsHistory slice in the Zustand store.
 */
export function MetricsChart() {
  const { statsHistory } = useStore();

  const data = statsHistory.map((s, i) => ({
    t: i,
    memtable: s.memtable_size,
    cache: s.cache_used,
  }));

  if (data.length === 0) {
    return (
      <div className="h-40 flex items-center justify-center text-muted-foreground text-sm">
        Collecting metrics…
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id="memGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#6060ff" stopOpacity={0.4} />
            <stop offset="95%" stopColor="#6060ff" stopOpacity={0} />
          </linearGradient>
          <linearGradient id="cacheGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#c060ff" stopOpacity={0.4} />
            <stop offset="95%" stopColor="#c060ff" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
        <XAxis dataKey="t" hide />
        <YAxis
          tickFormatter={(v: number) => formatBytes(v, 0)}
          tick={{ fontSize: 10, fill: "rgba(255,255,255,0.4)" }}
          width={56}
        />
        <Tooltip content={<CustomTooltip />} />
        <Legend
          iconSize={8}
          wrapperStyle={{ fontSize: 11, color: "rgba(255,255,255,0.5)" }}
        />
        <Area
          type="monotone"
          dataKey="memtable"
          name="Memtable"
          stroke="#6060ff"
          strokeWidth={1.5}
          fill="url(#memGrad)"
          dot={false}
          isAnimationActive={false}
        />
        <Area
          type="monotone"
          dataKey="cache"
          name="Cache"
          stroke="#c060ff"
          strokeWidth={1.5}
          fill="url(#cacheGrad)"
          dot={false}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
