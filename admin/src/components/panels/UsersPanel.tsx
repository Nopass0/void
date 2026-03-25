/**
 * @fileoverview UsersPanel – manage VoidDB user accounts (admin only).
 */

"use client";

import React, { useEffect, useState, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { UserPlus, Trash2, RefreshCw, Shield, X, Copy } from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import type { User } from "@/lib/api";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";

const ROLE_COLORS: Record<string, string> = {
  admin: "text-red-300 bg-red-500/10 border-red-500/20",
  readwrite: "text-blue-300 bg-blue-500/10 border-blue-500/20",
  readonly: "text-muted-foreground bg-surface-4 border-border",
};

export function UsersPanel() {
  const { user: me } = useStore();
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [showCreate, setShowCreate] = useState(false);

  const [form, setForm] = useState({ username: "", password: "", role: "readwrite" as User["role"] });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setUsers(await api.listUsers());
    } catch {
      toast.error("Failed to load users");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    try {
      await api.createUser(form.username, form.password, form.role);
      toast.success(`User "${form.username}" created`);
      setShowCreate(false);
      setForm({ username: "", password: "", role: "readwrite" });
      load();
    } catch {
      toast.error("Failed to create user");
    }
  };

  const handleDelete = async (id: string) => {
    if (id === me?.id) { toast.error("Cannot delete your own account"); return; }
    if (!confirm(`Delete user "${id}"?`)) return;
    try {
      await api.deleteUser(id);
      toast.success("User deleted");
      load();
    } catch {
      toast.error("Failed to delete user");
    }
  };

  const formatCreatedAt = (createdAt: number) =>
    new Date(createdAt * 1000).toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "2-digit",
    });

  return (
    <div className="space-y-4 max-w-6xl">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-foreground">User Management</h2>
        <div className="flex items-center gap-2">
          <button onClick={load} className="btn-ghost">
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
          </button>
          {me?.role === "admin" && (
            <button
              onClick={() => setShowCreate(true)}
              className="btn-primary text-sm"
            >
              <UserPlus className="w-4 h-4" />
              New User
            </button>
          )}
        </div>
      </div>

      {/* Users table */}
      <div className="rounded-lg border border-border bg-surface-2 overflow-x-auto">
        <table className="data-table w-full min-w-full">
          <colgroup>
            <col />
            <col style={{ width: "11rem" }} />
            <col style={{ width: "10rem" }} />
            <col style={{ width: "3.5rem" }} />
          </colgroup>
          <thead>
            <tr>
              <th>User</th>
              <th>Role</th>
              <th>Created</th>
              <th className="w-10" />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => {
              const userMenu: ContextMenuEntry[] = [
                { label: "Copy username", icon: <Copy className="w-3.5 h-3.5" />, onClick: () => { navigator.clipboard.writeText(u.id); toast.success("Username copied"); } },
                ...(me?.role === "admin" && u.id !== me.id ? [
                  { separator: true } as ContextMenuEntry,
                  { label: "Delete user", icon: <Trash2 className="w-3.5 h-3.5" />, danger: true, onClick: () => handleDelete(u.id) } as ContextMenuEntry,
                ] : []),
              ];
              return (
              <ContextMenu key={u.id} items={userMenu} asChild>
                <tr className="group">
                  <td>
                    <div className="flex items-center gap-3">
                      <div className="w-7 h-7 rounded-md bg-surface-4 flex items-center justify-center text-neon-500 text-xs font-bold uppercase">
                        {u.id[0]}
                      </div>
                      <span className="font-medium text-sm break-all">{u.id}</span>
                    </div>
                  </td>
                  <td>
                    <span
                      className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-md border ${ROLE_COLORS[u.role] ?? ""}`}
                    >
                      <Shield className="w-3 h-3" />
                      {u.role}
                    </span>
                  </td>
                  <td className="text-muted-foreground text-xs whitespace-nowrap">
                    {formatCreatedAt(u.created_at)}
                  </td>
                  <td className="text-right">
                    {me?.role === "admin" && u.id !== me.id && (
                      <button
                        onClick={() => handleDelete(u.id)}
                        className="opacity-0 group-hover:opacity-100 inline-flex text-muted-foreground hover:text-red-400 transition-all"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    )}
                  </td>
                </tr>
              </ContextMenu>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Create user modal */}
      <AnimatePresence>
        {showCreate && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60"
            onClick={() => setShowCreate(false)}
          >
            <motion.div
              initial={{ scale: 0.95, y: 10 }}
              animate={{ scale: 1, y: 0 }}
              exit={{ scale: 0.95, y: 10 }}
              onClick={(e) => e.stopPropagation()}
              className="rounded-lg w-full max-w-sm bg-surface-2 border border-border shadow-modal p-6 space-y-4"
            >
              <div className="flex items-center justify-between">
                <h3 className="font-semibold text-sm">New User</h3>
                <button onClick={() => setShowCreate(false)} className="btn-ghost !p-1">
                  <X className="w-4 h-4" />
                </button>
              </div>
              {["username", "password"].map((field) => (
                <div key={field} className="space-y-1.5">
                  <label className="text-xs text-muted-foreground capitalize">{field}</label>
                  <input
                    type={field === "password" ? "password" : "text"}
                    value={form[field as "username" | "password"]}
                    onChange={(e) => setForm((f) => ({ ...f, [field]: e.target.value }))}
                    className="input-field"
                  />
                </div>
              ))}
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">Role</label>
                <select
                  value={form.role}
                  onChange={(e) => setForm((f) => ({ ...f, role: e.target.value as User["role"] }))}
                  className="select-field"
                >
                  <option value="readonly">Read Only</option>
                  <option value="readwrite">Read/Write</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <button
                onClick={handleCreate}
                className="btn-primary w-full justify-center text-sm py-2"
              >
                Create User
              </button>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
