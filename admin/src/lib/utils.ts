/**
 * @fileoverview Utility helpers used throughout VoidDB Admin.
 */

import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Merges Tailwind CSS class names, resolving conflicts intelligently.
 * This is the standard shadcn/ui utility.
 *
 * @param inputs - One or more class values (strings, arrays, conditionals).
 * @returns Merged class name string.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}

/**
 * Formats a byte count into a human-readable string (e.g. 1.2 MB).
 */
export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

/**
 * Formats a Unix timestamp (seconds) as a locale date-time string.
 */
export function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleString();
}

/**
 * Truncates a string to maxLen characters, appending "…" if needed.
 */
export function truncate(s: string, maxLen: number): string {
  if (s.length <= maxLen) return s;
  return s.slice(0, maxLen - 1) + "…";
}

/**
 * Returns a debounced version of fn that fires only after delay ms of silence.
 */
export function debounce<T extends (...args: unknown[]) => void>(
  fn: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timer: ReturnType<typeof setTimeout>;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), delay);
  };
}

/**
 * Formats a number with thousands separators.
 */
export function formatNumber(n: number): string {
  return new Intl.NumberFormat().format(n);
}
