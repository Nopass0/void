/**
 * @fileoverview GlassCard – a glassmorphism container component.
 * Wraps content with a frosted glass effect, an optional glow border,
 * and entry animations via Framer Motion.
 */

"use client";

import * as React from "react";
import { motion, HTMLMotionProps } from "framer-motion";
import { cn } from "@/lib/utils";

// ── Props ─────────────────────────────────────────────────────────────────────

interface GlassCardProps extends Omit<HTMLMotionProps<"div">, "children"> {
  /** Card content. */
  children: React.ReactNode;
  /** Extra Tailwind classes. */
  className?: string;
  /** Whether to render an animated neon glow border. */
  glowBorder?: boolean;
  /** Disables the entry animation (useful for lists). */
  noAnimation?: boolean;
  /** Delay for the entry animation in seconds. */
  delay?: number;
  /** Whether to enable the hover lift effect. */
  hoverable?: boolean;
}

// ── Component ─────────────────────────────────────────────────────────────────

/**
 * GlassCard renders a frosted-glass panel with optional animations.
 *
 * @example
 * <GlassCard glowBorder hoverable>
 *   <h2>Hello</h2>
 * </GlassCard>
 */
export function GlassCard({
  children,
  className,
  glowBorder = false,
  noAnimation = false,
  delay = 0,
  hoverable = false,
  ...props
}: GlassCardProps) {
  const variants = {
    hidden: { opacity: 0, y: 16, scale: 0.98 },
    visible: { opacity: 1, y: 0, scale: 1 },
  };

  return (
    <motion.div
      variants={noAnimation ? undefined : variants}
      initial={noAnimation ? undefined : "hidden"}
      animate={noAnimation ? undefined : "visible"}
      transition={{ duration: 0.35, delay, ease: "easeOut" }}
      whileHover={
        hoverable
          ? { y: -2, boxShadow: "0 0 30px rgba(96,96,255,0.3)" }
          : undefined
      }
      className={cn(
        "glass rounded-xl p-4",
        glowBorder && "animated-border",
        className
      )}
      {...props}
    >
      {children}
    </motion.div>
  );
}

// ── Stat Card ─────────────────────────────────────────────────────────────────

interface StatCardProps {
  /** Icon element to display. */
  icon: React.ReactNode;
  /** Card title / label. */
  label: string;
  /** Primary metric value. */
  value: string | number;
  /** Optional secondary text shown below the value. */
  sub?: string;
  /** Optional trend indicator: "up" | "down" | "neutral". */
  trend?: "up" | "down" | "neutral";
  /** Animation delay in seconds. */
  delay?: number;
}

/**
 * StatCard displays a single KPI metric in a glassmorphism card.
 */
export function StatCard({ icon, label, value, sub, trend, delay = 0 }: StatCardProps) {
  const trendColor =
    trend === "up"
      ? "text-green-400"
      : trend === "down"
        ? "text-red-400"
        : "text-muted-foreground";

  return (
    <GlassCard delay={delay} hoverable className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">{label}</span>
        <div className="p-2 rounded-lg bg-void-600/20 text-void-400">{icon}</div>
      </div>
      <div className="space-y-1">
        <p className="text-2xl font-bold gradient-text">{value}</p>
        {sub && <p className={cn("text-xs", trendColor)}>{sub}</p>}
      </div>
    </GlassCard>
  );
}
