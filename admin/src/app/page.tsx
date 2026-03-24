/**
 * Main app shell page.
 * Checks authentication and renders the appropriate panel based on activeTab.
 */

"use client";

import React, { useEffect } from "react";
import { useRouter } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { Loader2 } from "lucide-react";
import { useStore } from "@/store";
import { Sidebar } from "@/components/layout/Sidebar";
import { Header } from "@/components/layout/Header";
import { DashboardPanel } from "@/components/panels/DashboardPanel";
import { DataPanel } from "@/components/panels/DataPanel";
import { BlobPanel } from "@/components/panels/BlobPanel";
import { UsersPanel } from "@/components/panels/UsersPanel";
import { SettingsPanel } from "@/components/panels/SettingsPanel";
import { DocsPanel } from "@/components/panels/DocsPanel";
import { QueryPanel } from "@/components/panels/QueryPanel";
import { LogsPanel } from "@/components/panels/LogsPanel";
import { GlobalContextSuppressor } from "@/components/ui/context-menu";
import * as api from "@/lib/api";

export default function Home() {
  const router = useRouter();
  const { user, setUser, authLoading, setAuthLoading, activeTab } = useStore();

  useEffect(() => {
    if (!api.isLoggedIn()) {
      router.replace("/login");
      setAuthLoading(false);
      return;
    }
    api.getMe()
      .then((me) => { setUser(me); })
      .catch(() => {
        api.logout();
        router.replace("/login");
      })
      .finally(() => setAuthLoading(false));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (authLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="w-8 h-8 animate-spin text-neon-500" />
      </div>
    );
  }

  if (!user) return null;

  return (
    <div className="flex h-screen overflow-hidden">
      <GlobalContextSuppressor />
      <Sidebar />

      <div className="flex-1 flex flex-col overflow-hidden">
        <Header />

        <main className="flex-1 overflow-auto p-6">
          <AnimatePresence mode="wait">
            <motion.div
              key={activeTab}
              initial={{ opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -10 }}
              transition={{ duration: 0.2 }}
              className="h-full"
            >
              {activeTab === "dashboard" && <DashboardPanel />}
              {activeTab === "data" && <DataPanel />}
              {activeTab === "blob" && <BlobPanel />}
              {activeTab === "users" && <UsersPanel />}
              {activeTab === "settings" && <SettingsPanel />}
              {activeTab === "docs" && <DocsPanel />}
              {activeTab === "query" && <QueryPanel />}
              {activeTab === "logs" && <LogsPanel />}
            </motion.div>
          </AnimatePresence>
        </main>
      </div>
    </div>
  );
}
