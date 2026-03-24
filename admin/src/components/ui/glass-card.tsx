/**
 * @fileoverview Card – a clean dark surface card component.
 * Replaces the old glassmorphism GlassCard.
 */

"use client";

import * as React from "react";
import { motion, HTMLMotionProps } from "framer-motion";
import { cn } from "@/lib/utils";

// ── Props ─────────────────────────────────────────────────────────────────────

interface CardProps extends Omit<HTMLMotionProps<"div">, "children"> {
  children: React.ReactNode;
  className?: string;
  /** Disables the entry animation. */
  noAnimation?: boolean;
  /** Delay for the entry animation in seconds. */
  delay?: number;
  /** Whether to enable the hover effect. */
  hoverable?: boolean;
}

// ── Component ─────────────────────────────────────────────────────────────────

/**
 * Card renders a clean dark surface panel with optional animations.
 */
export function Card({
  children,
  className,
  noAnimation = false,
  delay = 0,
  hoverable = false,
  ...props
}: CardProps) {
  const variants = {
    hidden: { opacity: 0, y: 8 },
    visible: { opacity: 1, y: 0 },
  };

  return (
    <motion.div
      variants={noAnimation ? undefined : variants}
      initial={noAnimation ? undefined : "hidden"}
      animate={noAnimation ? undefined : "visible"}
      transition={{ duration: 0.2, delay, ease: "easeOut" }}
      className={cn(
        "card-surface rounded-lg p-4",
        hoverable && "card-surface-hover cursor-pointer",
        className
      )}
      {...props}
    >
      {children}
    </motion.div>
  );
}

// ── Backward-compatible aliases ───────────────────────────────────────────────

export const GlassCard = Card;

// ── Stat Card ─────────────────────────────────────────────────────────────────

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  sub?: string;
  trend?: "up" | "down" | "neutral";
  delay?: number;
}

/**
 * StatCard displays a single KPI metric.
 */
export function StatCard({ icon, label, value, sub, trend, delay = 0 }: StatCardProps) {
  const trendColor =
    trend === "up"
      ? "text-neon-500"
      : trend === "down"
        ? "text-red-400"
        : "text-muted-foreground";

  return (
    <Card delay={delay} hoverable className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">{label}</span>
        <div className="p-2 rounded-md bg-surface-3 text-neon-500">{icon}</div>
      </div>
      <div className="space-y-1">
        <p className="text-2xl font-bold text-foreground">{value}</p>
        {sub && <p className={cn("text-xs", trendColor)}>{sub}</p>}
      </div>
    </Card>
  );
}
