/**
 * Login page for VoidDB Admin — Neon-style dark design.
 */

"use client";

import React, { useState } from "react";
import { motion } from "framer-motion";
import { Zap, Loader2, Eye, EyeOff } from "lucide-react";
import { toast } from "sonner";
import { useRouter } from "next/navigation";
import axios from "axios";
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
    } catch (error) {
      if (axios.isAxiosError(error) && error.response?.status === 401) {
        toast.error("Invalid username or password");
      } else {
        toast.error("Login request failed");
      }
      setLoading(false);
      return;
    }

    try {
      const me = await api.getMe();
      setUser(me);
      router.replace("/");
    } catch {
      api.logout();
      toast.error("Signed in, but failed to load the current user");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-surface-0">
      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.96 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className="relative w-full max-w-sm"
      >
        <div className="rounded-lg p-8 bg-surface-2 border border-border">
          {/* Logo */}
          <div className="flex flex-col items-center mb-8">
            <div className="p-3 rounded-lg bg-neon-500/10 mb-4">
              <Zap className="w-7 h-7 text-neon-500" />
            </div>
            <h1 className="text-2xl font-bold text-foreground">VoidDB</h1>
            <p className="text-sm text-muted-foreground mt-1">Admin Console</p>
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
                className="input-field"
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
                  className="input-field pr-10"
                  placeholder="••••••••"
                  autoComplete="current-password"
                  required
                />
                <button
                  type="button"
                  onClick={() => setShowPw(!showPw)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                >
                  {showPw ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <motion.button
              whileTap={{ scale: 0.98 }}
              type="submit"
              disabled={loading}
              className="btn-primary w-full justify-center py-2.5"
            >
              {loading && <Loader2 className="w-4 h-4 animate-spin" />}
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
