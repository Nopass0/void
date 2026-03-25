/**
 * @fileoverview ContextMenu – custom right-click context menu component.
 */

"use client";

import React, { useEffect, useRef, useState, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { cn } from "@/lib/utils";

// ── Types ─────────────────────────────────────────────────────────────────────

export interface ContextMenuItem {
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
  /** keyboard shortcut hint */
  shortcut?: string;
}

export interface ContextMenuSeparator {
  separator: true;
}

export type ContextMenuEntry = ContextMenuItem | ContextMenuSeparator;

function isSeparator(entry: ContextMenuEntry): entry is ContextMenuSeparator {
  return "separator" in entry;
}

// ── Menu Panel ────────────────────────────────────────────────────────────────

interface MenuPanelProps {
  x: number;
  y: number;
  items: ContextMenuEntry[];
  onClose: () => void;
}

function MenuPanel({ x, y, items, onClose }: MenuPanelProps) {
  const ref = useRef<HTMLDivElement>(null);

  // Adjust position so menu stays on-screen
  const [pos, setPos] = useState({ x, y });

  useEffect(() => {
    if (!ref.current) return;
    const rect = ref.current.getBoundingClientRect();
    const nx = x + rect.width > window.innerWidth ? window.innerWidth - rect.width - 8 : x;
    const ny = y + rect.height > window.innerHeight ? window.innerHeight - rect.height - 8 : y;
    setPos({ x: nx, y: ny });
  }, [x, y]);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("mousedown", handler);
    document.addEventListener("keydown", keyHandler);
    return () => {
      document.removeEventListener("mousedown", handler);
      document.removeEventListener("keydown", keyHandler);
    };
  }, [onClose]);

  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, scale: 0.95 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={{ opacity: 0, scale: 0.95 }}
      transition={{ duration: 0.1 }}
      style={{ left: pos.x, top: pos.y }}
      className="fixed z-[100] py-1 min-w-[180px] rounded-md bg-surface-2 border border-border shadow-modal"
    >
      {items.map((item, i) => {
        if (isSeparator(item)) {
          return <div key={`sep-${i}`} className="h-px my-1 bg-border" />;
        }
        return (
          <button
            key={i}
            disabled={item.disabled}
            onClick={() => {
              item.onClick();
              onClose();
            }}
            className={cn(
              "w-full flex items-center gap-2.5 px-3 py-1.5 text-sm text-left transition-colors",
              item.disabled
                ? "text-muted-foreground/50 cursor-not-allowed"
                : item.danger
                  ? "text-red-400 hover:bg-red-500/10"
                  : "text-foreground hover:bg-surface-4"
            )}
          >
            {item.icon && <span className="w-4 h-4 shrink-0 flex items-center justify-center">{item.icon}</span>}
            <span className="flex-1">{item.label}</span>
            {item.shortcut && (
              <span className="text-[10px] text-muted-foreground font-mono ml-4">{item.shortcut}</span>
            )}
          </button>
        );
      })}
    </motion.div>
  );
}

// ── Context state hook ────────────────────────────────────────────────────────

interface ContextState {
  visible: boolean;
  x: number;
  y: number;
}

export function useContextMenu() {
  const [state, setState] = useState<ContextState>({ visible: false, x: 0, y: 0 });

  const show = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setState({ visible: true, x: e.clientX, y: e.clientY });
  }, []);

  const hide = useCallback(() => {
    setState((s) => ({ ...s, visible: false }));
  }, []);

  return { ...state, show, hide };
}

// ── Wrapper component ─────────────────────────────────────────────────────────

interface ContextMenuProps {
  children: React.ReactNode;
  items: ContextMenuEntry[] | ((e: React.MouseEvent) => ContextMenuEntry[]);
  className?: string;
  as?: keyof JSX.IntrinsicElements;
  asChild?: boolean;
}

/**
 * ContextMenu wraps children with a right-click handler.
 * Prevents native context menu and shows a custom one.
 */
export function ContextMenu({ children, items, className, as: Tag = "div", asChild = false }: ContextMenuProps) {
  const ctx = useContextMenu();
  const [menuItems, setMenuItems] = useState<ContextMenuEntry[]>([]);

  const handleContext = (e: React.MouseEvent) => {
    ctx.show(e);
    if (typeof items === "function") {
      setMenuItems(items(e));
    } else {
      setMenuItems(items);
    }
  };

  const Component = Tag as React.ElementType;

  if (asChild && React.isValidElement(children)) {
    const child = children as React.ReactElement<{
      className?: string;
      onContextMenu?: (e: React.MouseEvent) => void;
    }>;

    return (
      <>
        {React.cloneElement(child, {
          className: cn(child.props.className, className),
          onContextMenu: (e: React.MouseEvent) => {
            child.props.onContextMenu?.(e);
            if (!e.defaultPrevented) {
              handleContext(e);
            }
          },
        })}
        <AnimatePresence>
          {ctx.visible && (
            <MenuPanel x={ctx.x} y={ctx.y} items={menuItems} onClose={ctx.hide} />
          )}
        </AnimatePresence>
      </>
    );
  }

  return (
    <>
      <Component onContextMenu={handleContext} className={className}>
        {children}
      </Component>
      <AnimatePresence>
        {ctx.visible && (
          <MenuPanel x={ctx.x} y={ctx.y} items={menuItems} onClose={ctx.hide} />
        )}
      </AnimatePresence>
    </>
  );
}

// ── Global context menu suppression ───────────────────────────────────────────

/**
 * GlobalContextSuppressor prevents the native browser context menu
 * on the entire page, showing nothing if no custom menu is attached.
 * Place this in root layout.
 */
export function GlobalContextSuppressor() {
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      // Allow context menu on input/textarea for copy/paste
      const target = e.target as HTMLElement;
      if (target.tagName === "INPUT" || target.tagName === "TEXTAREA") return;
      e.preventDefault();
    };
    document.addEventListener("contextmenu", handler);
    return () => document.removeEventListener("contextmenu", handler);
  }, []);
  return null;
}
