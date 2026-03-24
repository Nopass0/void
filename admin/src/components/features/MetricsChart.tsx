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

const CustomTooltip = ({
  active,
  payload,
}: {
  active?: boolean;
  payload?: { name: string; value: number; color: string }[];
}) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="bg-surface-3 rounded-md p-2 border border-border text-xs space-y-1">
      {payload.map((p) => (
        <div key={p.name} className="flex items-center gap-2">
          <span style={{ color: p.color }}>■</span>
          <span className="text-muted-foreground">{p.name}:</span>
          <span className="font-mono text-foreground">{formatBytes(p.value)}</span>
        </div>
      ))}
    </div>
  );
};

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
            <stop offset="5%" stopColor="#00E599" stopOpacity={0.3} />
            <stop offset="95%" stopColor="#00E599" stopOpacity={0} />
          </linearGradient>
          <linearGradient id="cacheGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#00C4FF" stopOpacity={0.3} />
            <stop offset="95%" stopColor="#00C4FF" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(240 5% 14%)" />
        <XAxis dataKey="t" hide />
        <YAxis
          tickFormatter={(v: number) => formatBytes(v, 0)}
          tick={{ fontSize: 10, fill: "hsl(240 4% 46%)" }}
          width={56}
        />
        <Tooltip content={<CustomTooltip />} />
        <Legend
          iconSize={8}
          wrapperStyle={{ fontSize: 11, color: "hsl(240 4% 46%)" }}
        />
        <Area
          type="monotone"
          dataKey="memtable"
          name="Memtable"
          stroke="#00E599"
          strokeWidth={1.5}
          fill="url(#memGrad)"
          dot={false}
          isAnimationActive={false}
        />
        <Area
          type="monotone"
          dataKey="cache"
          name="Cache"
          stroke="#00C4FF"
          strokeWidth={1.5}
          fill="url(#cacheGrad)"
          dot={false}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
