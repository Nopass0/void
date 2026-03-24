/**
 * @fileoverview Global Zustand store for VoidDB Admin.
 * Manages authentication state, active database/collection selection,
 * fetched data, and UI state (sidebar open, active tab, etc.).
 */

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { User, Document, EngineStats, ObjectMeta } from "@/lib/api";

// ── Auth slice ────────────────────────────────────────────────────────────────

interface AuthState {
  /** The currently logged-in user (null if not authenticated). */
  user: User | null;
  /** True while the initial auth check is in progress. */
  authLoading: boolean;
  setUser: (user: User | null) => void;
  setAuthLoading: (v: boolean) => void;
}

// ── Navigation slice ──────────────────────────────────────────────────────────

interface NavState {
  /** Currently selected database name. */
  activeDb: string | null;
  /** Currently selected collection name. */
  activeCol: string | null;
  /** Whether the sidebar is collapsed on mobile. */
  sidebarOpen: boolean;
  /** The active top-level tab: "data" | "blob" | "users" | "settings". */
  activeTab: "data" | "blob" | "users" | "settings" | "dashboard";
  setActiveDb: (db: string | null) => void;
  setActiveCol: (col: string | null) => void;
  setSidebarOpen: (v: boolean) => void;
  setActiveTab: (tab: NavState["activeTab"]) => void;
}

// ── Data slice ────────────────────────────────────────────────────────────────

interface DataState {
  /** List of known database names. */
  databases: string[];
  /** Map of dbName → collection names. */
  collections: Record<string, string[]>;
  /** The currently displayed documents. */
  documents: Document[];
  /** Total count for the current query. */
  docCount: number;
  /** Whether a data fetch is in progress. */
  dataLoading: boolean;
  /** Current query JSON (raw string edited in QueryEditor). */
  queryText: string;
  /** Current page number (0-indexed). */
  page: number;
  /** Page size. */
  pageSize: number;
  setDatabases: (dbs: string[]) => void;
  setCollections: (db: string, cols: string[]) => void;
  setDocuments: (docs: Document[], count: number) => void;
  setDataLoading: (v: boolean) => void;
  setQueryText: (q: string) => void;
  setPage: (p: number) => void;
  setPageSize: (n: number) => void;
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

/**
 * useStore is the single Zustand store for VoidDB Admin.
 * Auth state is persisted to localStorage; everything else is session-only.
 */
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
      sidebarOpen: true,
      activeTab: "dashboard",
      setActiveDb: (activeDb) => set({ activeDb, activeCol: null }),
      setActiveCol: (activeCol) => set({ activeCol }),
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
      setDatabases: (databases) => set({ databases }),
      setCollections: (db, cols) =>
        set((s) => ({ collections: { ...s.collections, [db]: cols } })),
      setDocuments: (documents, docCount) => set({ documents, docCount }),
      setDataLoading: (dataLoading) => set({ dataLoading }),
      setQueryText: (queryText) => set({ queryText }),
      setPage: (page) => set({ page }),
      setPageSize: (pageSize) => set({ pageSize }),

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
