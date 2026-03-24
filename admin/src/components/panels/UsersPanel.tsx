/**
 * @fileoverview UsersPanel – manage VoidDB user accounts (admin only).
 */

"use client";

import React, { useEffect, useState, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { UserPlus, Trash2, RefreshCw, Shield, X, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { GlassCard } from "@/components/ui/glass-card";
import { useStore } from "@/store";
import * as api from "@/lib/api";
import type { User } from "@/lib/api";

const ROLE_COLORS: Record<string, string> = {
  admin: "text-red-300 bg-red-500/10 border-red-500/20",
  readwrite: "text-blue-300 bg-blue-500/10 border-blue-500/20",
  readonly: "text-muted-foreground bg-white/5 border-white/10",
};

/**
 * UsersPanel lists users and provides create/delete actions.
 */
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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-bold gradient-text">User Management</h2>
        <div className="flex items-center gap-2">
          <button onClick={load} className="text-muted-foreground hover:text-void-400 transition-colors">
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
          </button>
          {me?.role === "admin" && (
            <motion.button
              whileTap={{ scale: 0.96 }}
              onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 text-sm bg-void-600/40 hover:bg-void-600/60 text-void-200 px-3 py-2 rounded-lg border border-void-500/30 transition-colors"
            >
              <UserPlus className="w-4 h-4" />
              New User
            </motion.button>
          )}
        </div>
      </div>

      <div className="grid gap-3">
        {users.map((u, i) => (
          <GlassCard key={u.id} delay={i * 0.05} hoverable className="flex items-center gap-4 p-4">
            <div className="w-9 h-9 rounded-full bg-void-600/30 flex items-center justify-center text-void-300 font-bold uppercase">
              {u.id[0]}
            </div>
            <div className="flex-1">
              <p className="font-medium">{u.id}</p>
              <p className="text-xs text-muted-foreground">
                Created {new Date(u.created_at * 1000).toLocaleDateString()}
              </p>
            </div>
            <span
              className={`flex items-center gap-1 text-xs px-2 py-0.5 rounded-full border ${ROLE_COLORS[u.role] ?? ""}`}
            >
              <Shield className="w-3 h-3" />
              {u.role}
            </span>
            {me?.role === "admin" && u.id !== me.id && (
              <button
                onClick={() => handleDelete(u.id)}
                className="text-muted-foreground hover:text-red-400 transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            )}
          </GlassCard>
        ))}
      </div>

      {/* Create user modal */}
      <AnimatePresence>
        {showCreate && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
            onClick={() => setShowCreate(false)}
          >
            <motion.div
              initial={{ scale: 0.9, y: 16 }}
              animate={{ scale: 1, y: 0 }}
              exit={{ scale: 0.9, y: 16 }}
              onClick={(e) => e.stopPropagation()}
              className="glass rounded-2xl w-full max-w-sm border border-void-500/20 p-6 space-y-4"
            >
              <div className="flex items-center justify-between">
                <h3 className="font-semibold gradient-text">New User</h3>
                <button onClick={() => setShowCreate(false)}>
                  <X className="w-4 h-4 text-muted-foreground" />
                </button>
              </div>
              {["username", "password"].map((field) => (
                <div key={field} className="space-y-1">
                  <label className="text-xs text-muted-foreground capitalize">{field}</label>
                  <input
                    type={field === "password" ? "password" : "text"}
                    value={form[field as "username" | "password"]}
                    onChange={(e) => setForm((f) => ({ ...f, [field]: e.target.value }))}
                    className="w-full px-3 py-2 rounded-lg glass border border-void-500/20 text-sm focus:outline-none focus:border-void-400/50 transition-all bg-transparent"
                  />
                </div>
              ))}
              <div className="space-y-1">
                <label className="text-xs text-muted-foreground">Role</label>
                <select
                  value={form.role}
                  onChange={(e) => setForm((f) => ({ ...f, role: e.target.value as User["role"] }))}
                  className="w-full px-3 py-2 rounded-lg glass border border-void-500/20 text-sm focus:outline-none bg-transparent text-foreground"
                >
                  <option value="readonly">Read Only</option>
                  <option value="readwrite">Read/Write</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <button
                onClick={handleCreate}
                className="w-full py-2.5 bg-void-600/50 hover:bg-void-600/70 text-void-100 rounded-lg border border-void-500/30 transition-colors text-sm font-medium"
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
