/**
 * Login page for VoidDB Admin.
 * Unauthenticated users are redirected here automatically.
 */

"use client";

import React, { useState } from "react";
import { motion } from "framer-motion";
import { Zap, Loader2, Eye, EyeOff } from "lucide-react";
import { toast } from "sonner";
import { useRouter } from "next/navigation";
import * as api from "@/lib/api";
import { useStore } from "@/store";

export default function LoginPage() {
  const router = useRouter();
  const setUser = useStore((s) => s.setUser);

  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [showPw, setShowPw] = useState(false);
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await api.login(username, password);
      const me = await api.getMe();
      setUser(me);
      router.replace("/");
    } catch {
      toast.error("Invalid username or password");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      {/* Ambient background glow */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-void-600/20 rounded-full blur-3xl" />
        <div className="absolute bottom-1/4 right-1/4 w-96 h-96 bg-violet-600/10 rounded-full blur-3xl" />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.96 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className="relative w-full max-w-sm"
      >
        <div className="glass rounded-2xl p-8 border border-void-500/20 shadow-glass">
          {/* Logo */}
          <div className="flex flex-col items-center mb-8">
            <div className="p-3 rounded-2xl bg-void-600/20 shadow-neon mb-4">
              <Zap className="w-8 h-8 text-void-400" />
            </div>
            <h1 className="text-2xl font-bold gradient-text">VoidDB</h1>
            <p className="text-sm text-muted-foreground mt-1">Admin Panel</p>
          </div>

          {/* Form */}
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Username
              </label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full px-3 py-2.5 rounded-lg glass border border-void-500/20 bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-void-400/50 focus:shadow-neon-sm transition-all"
                placeholder="admin"
                autoComplete="username"
                required
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Password
              </label>
              <div className="relative">
                <input
                  type={showPw ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full px-3 py-2.5 pr-10 rounded-lg glass border border-void-500/20 bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-void-400/50 focus:shadow-neon-sm transition-all"
                  placeholder="••••••••"
                  autoComplete="current-password"
                  required
                />
                <button
                  type="button"
                  onClick={() => setShowPw(!showPw)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  {showPw ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <motion.button
              whileTap={{ scale: 0.98 }}
              type="submit"
              disabled={loading}
              className="w-full flex items-center justify-center gap-2 bg-void-600/50 hover:bg-void-600/70 text-void-100 font-medium py-2.5 rounded-lg transition-all border border-void-500/40 shadow-neon-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : null}
              {loading ? "Signing in…" : "Sign in"}
            </motion.button>
          </form>

          <p className="text-center text-xs text-muted-foreground mt-6">
            VoidDB v1.0.0 — High-performance document store
          </p>
        </div>
      </motion.div>
    </div>
  );
}
