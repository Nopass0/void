/**
 * @fileoverview BlobPanel – S3-compatible object storage browser.
 */

"use client";

import React, { useEffect, useCallback, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  FolderOpen, Upload, Trash2, Download, RefreshCw, Plus, File, Loader2
} from "lucide-react";
import { toast } from "sonner";
import { useStore } from "@/store";
import { GlassCard } from "@/components/ui/glass-card";
import { formatBytes } from "@/lib/utils";
import * as api from "@/lib/api";

/**
 * BlobPanel provides a file-browser UI for blob buckets and objects.
 */
export function BlobPanel() {
  const {
    buckets, setBuckets,
    activeBucket, setActiveBucket,
    objects, setObjects,
    blobLoading, setBlobLoading,
  } = useStore();
  const fileRef = useRef<HTMLInputElement>(null);

  const loadBuckets = useCallback(async () => {
    try {
      const bs = await api.listBuckets();
      setBuckets(bs);
    } catch { /* silent */ }
  }, [setBuckets]);

  const loadObjects = useCallback(async (bucket: string) => {
    setBlobLoading(true);
    try {
      const objs = await api.listObjects(bucket);
      setObjects(objs);
    } catch {
      toast.error("Failed to load objects");
    } finally {
      setBlobLoading(false);
    }
  }, [setBlobLoading, setObjects]);

  useEffect(() => { loadBuckets(); }, [loadBuckets]);
  useEffect(() => { if (activeBucket) loadObjects(activeBucket); }, [activeBucket, loadObjects]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    if (!activeBucket || files.length === 0) return;
    for (const file of files) {
      try {
        await api.uploadObject(activeBucket, file.name, file);
        toast.success(`Uploaded ${file.name}`);
      } catch {
        toast.error(`Failed to upload ${file.name}`);
      }
    }
    loadObjects(activeBucket);
  };

  const handleDelete = async (key: string) => {
    if (!activeBucket || !confirm(`Delete ${key}?`)) return;
    try {
      await api.deleteObject(activeBucket, key);
      toast.success("Object deleted");
      loadObjects(activeBucket);
    } catch {
      toast.error("Failed to delete object");
    }
  };

  return (
    <div className="flex gap-4 h-full">
      {/* Bucket list */}
      <GlassCard className="w-48 shrink-0 p-3 flex flex-col gap-2">
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
            Buckets
          </span>
          <button
            onClick={async () => {
              const name = prompt("Bucket name:");
              if (!name) return;
              try {
                await fetch(`${process.env.NEXT_PUBLIC_API_URL}/s3/${name}`, {
                  method: "PUT",
                  headers: { Authorization: `Bearer ${localStorage.getItem("void_access_token")}` },
                });
                toast.success(`Bucket "${name}" created`);
                loadBuckets();
              } catch {
                toast.error("Failed to create bucket");
              }
            }}
            className="text-muted-foreground hover:text-void-400 transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </div>
        {buckets.map((b) => (
          <motion.button
            key={b}
            whileTap={{ scale: 0.97 }}
            onClick={() => setActiveBucket(b)}
            className={`w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-all text-left ${
              activeBucket === b
                ? "bg-void-600/30 text-void-300 border border-void-500/30"
                : "text-muted-foreground hover:text-foreground hover:bg-white/5"
            }`}
          >
            <FolderOpen className="w-3.5 h-3.5 shrink-0" />
            <span className="truncate">{b}</span>
          </motion.button>
        ))}
      </GlassCard>

      {/* Object browser */}
      <div className="flex-1 flex flex-col gap-3">
        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <h2 className="text-sm font-semibold flex-1">
            {activeBucket ? (
              <span className="text-void-300">{activeBucket}</span>
            ) : (
              <span className="text-muted-foreground">Select a bucket</span>
            )}
          </h2>
          <button
            onClick={() => activeBucket && loadObjects(activeBucket)}
            className="text-muted-foreground hover:text-void-400 transition-colors"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
          <motion.button
            whileTap={{ scale: 0.96 }}
            onClick={() => fileRef.current?.click()}
            disabled={!activeBucket}
            className="flex items-center gap-1.5 text-sm bg-void-600/40 hover:bg-void-600/60 text-void-200 px-3 py-2 rounded-lg border border-void-500/30 transition-colors disabled:opacity-40"
          >
            <Upload className="w-4 h-4" />
            Upload
          </motion.button>
          <input
            ref={fileRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleUpload}
          />
        </div>

        {/* Objects table */}
        <GlassCard className="flex-1 p-0 overflow-hidden">
          {blobLoading ? (
            <div className="flex items-center justify-center h-40">
              <Loader2 className="w-6 h-6 animate-spin text-void-400" />
            </div>
          ) : !activeBucket ? (
            <div className="flex flex-col items-center justify-center h-40 text-muted-foreground gap-2">
              <FolderOpen className="w-10 h-10 opacity-20" />
              <p className="text-sm">Select a bucket</p>
            </div>
          ) : objects.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-40 text-muted-foreground gap-2">
              <File className="w-10 h-10 opacity-20" />
              <p className="text-sm">No objects in this bucket</p>
            </div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-void-500/20">
                  <th className="px-4 py-3 text-left text-xs font-semibold text-muted-foreground">Key</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold text-muted-foreground">Size</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold text-muted-foreground">Type</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold text-muted-foreground">Modified</th>
                  <th className="px-4 py-3 w-20" />
                </tr>
              </thead>
              <tbody>
                <AnimatePresence>
                  {objects.map((obj, i) => (
                    <motion.tr
                      key={obj.key}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={{ opacity: 0 }}
                      transition={{ delay: i * 0.03 }}
                      className="border-b border-void-500/10 hover:bg-void-600/10 transition-colors group"
                    >
                      <td className="px-4 py-2.5 font-mono text-xs text-void-300">{obj.key}</td>
                      <td className="px-4 py-2.5 text-xs text-muted-foreground">{formatBytes(obj.size)}</td>
                      <td className="px-4 py-2.5 text-xs text-muted-foreground">{obj.content_type}</td>
                      <td className="px-4 py-2.5 text-xs text-muted-foreground">
                        {new Date(obj.last_modified).toLocaleString()}
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                          <a
                            href={api.getObjectUrl(obj.bucket, obj.key)}
                            download={obj.key}
                            className="text-muted-foreground hover:text-void-400 transition-colors"
                          >
                            <Download className="w-3.5 h-3.5" />
                          </a>
                          <button
                            onClick={() => handleDelete(obj.key)}
                            className="text-muted-foreground hover:text-red-400 transition-colors"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </td>
                    </motion.tr>
                  ))}
                </AnimatePresence>
              </tbody>
            </table>
          )}
        </GlassCard>
      </div>
    </div>
  );
}
