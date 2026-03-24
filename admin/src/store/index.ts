/**
 * @fileoverview Global Zustand store for VoidDB Admin.
 * Manages authentication state, active database/collection selection,
 * fetched data, and UI state (sidebar open, active tab, etc.).
 */

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { User, Document, EngineStats, ObjectMeta, Schema } from "@/lib/api";

// ── Auth slice ────────────────────────────────────────────────────────────────

interface AuthState {
  user: User | null;
  authLoading: boolean;
  setUser: (user: User | null) => void;
  setAuthLoading: (v: boolean) => void;
}

// ── Navigation slice ──────────────────────────────────────────────────────────

interface NavState {
  activeDb: string | null;
  activeCol: string | null;
  activeSchema: Schema | null;
  sidebarOpen: boolean;
  activeTab: "data" | "blob" | "users" | "settings" | "dashboard" | "docs" | "query" | "logs";
  setActiveDb: (db: string | null) => void;
  setActiveCol: (col: string | null) => void;
  setActiveSchema: (schema: Schema | null) => void;
  setSidebarOpen: (v: boolean) => void;
  setActiveTab: (tab: NavState["activeTab"]) => void;
}

// ── Data slice ────────────────────────────────────────────────────────────────

interface SortSpec {
  field: string;
  dir: "asc" | "desc";
}

interface DataState {
  databases: string[];
  collections: Record<string, string[]>;
  documents: Document[];
  docCount: number;
  dataLoading: boolean;
  queryText: string;
  page: number;
  pageSize: number;
  /** Column sort state for the document table. */
  sortBy: SortSpec | null;
  /** Selected document IDs for bulk operations. */
  selectedIds: Set<string>;
  setDatabases: (dbs: string[]) => void;
  setCollections: (db: string, cols: string[]) => void;
  setDocuments: (docs: Document[], count: number) => void;
  setDataLoading: (v: boolean) => void;
  setQueryText: (q: string) => void;
  setPage: (p: number) => void;
  setPageSize: (n: number) => void;
  setSortBy: (s: SortSpec | null) => void;
  setSelectedIds: (ids: Set<string>) => void;
  toggleSelectedId: (id: string) => void;
  clearSelectedIds: () => void;
}

// ── Stats slice ───────────────────────────────────────────────────────────────

interface StatsState {
  stats: EngineStats | null;
  statsHistory: EngineStats[];
  setStats: (s: EngineStats) => void;
}

// ── Blob slice ────────────────────────────────────────────────────────────────

interface BlobState {
  buckets: string[];
  activeBucket: string | null;
  objects: ObjectMeta[];
  blobLoading: boolean;
  setBuckets: (bs: string[]) => void;
  setActiveBucket: (b: string | null) => void;
  setObjects: (objs: ObjectMeta[]) => void;
  setBlobLoading: (v: boolean) => void;
}

// ── Combined store ────────────────────────────────────────────────────────────

type VoidStore = AuthState & NavState & DataState & StatsState & BlobState;

export const useStore = create<VoidStore>()(
  persist(
    (set, _get) => ({
      // Auth
      user: null,
      authLoading: true,
      setUser: (user) => set({ user }),
      setAuthLoading: (authLoading) => set({ authLoading }),

      // Nav
      activeDb: null,
      activeCol: null,
      activeSchema: null,
      sidebarOpen: true,
      activeTab: "dashboard",
      setActiveDb: (activeDb) => set({ activeDb, activeCol: null, activeSchema: null }),
      setActiveCol: (activeCol) => set({ activeCol, activeSchema: null }),
      setActiveSchema: (activeSchema) => set({ activeSchema }),
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      setActiveTab: (activeTab) => set({ activeTab }),

      // Data
      databases: [],
      collections: {},
      documents: [],
      docCount: 0,
      dataLoading: false,
      queryText: "{}",
      page: 0,
      pageSize: 25,
      sortBy: null,
      selectedIds: new Set<string>(),
      setDatabases: (databases) => set({ databases }),
      setCollections: (db, cols) =>
        set((s) => ({ collections: { ...s.collections, [db]: cols } })),
      setDocuments: (documents, docCount) => set({ documents, docCount }),
      setDataLoading: (dataLoading) => set({ dataLoading }),
      setQueryText: (queryText) => set({ queryText }),
      setPage: (page) => set({ page }),
      setPageSize: (pageSize) => set({ pageSize }),
      setSortBy: (sortBy) => set({ sortBy }),
      setSelectedIds: (selectedIds) => set({ selectedIds }),
      toggleSelectedId: (id) =>
        set((s) => {
          const next = new Set(s.selectedIds);
          if (next.has(id)) next.delete(id);
          else next.add(id);
          return { selectedIds: next };
        }),
      clearSelectedIds: () => set({ selectedIds: new Set<string>() }),

      // Stats
      stats: null,
      statsHistory: [],
      setStats: (stats) =>
        set((s) => ({
          stats,
          statsHistory: [...s.statsHistory.slice(-59), stats],
        })),

      // Blob
      buckets: [],
      activeBucket: null,
      objects: [],
      blobLoading: false,
      setBuckets: (buckets) => set({ buckets }),
      setActiveBucket: (activeBucket) => set({ activeBucket }),
      setObjects: (objects) => set({ objects }),
      setBlobLoading: (blobLoading) => set({ blobLoading }),
    }),
    {
      name: "void-admin-store",
      storage: createJSONStorage(() =>
        typeof window !== "undefined" ? localStorage : { getItem: () => null, setItem: () => {}, removeItem: () => {} }
      ),
      partialize: (state) => ({
        user: state.user,
        activeDb: state.activeDb,
        activeCol: state.activeCol,
        activeTab: state.activeTab,
        pageSize: state.pageSize,
      }),
    }
  )
);
