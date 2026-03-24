/**
 * @fileoverview BlobPanel – S3-compatible object storage browser.
 */

"use client";

import React, { useEffect, useCallback, useRef } from "react";
import { AnimatePresence, motion } from "framer-motion";
import {
  FolderOpen, Upload, Trash2, Download, RefreshCw, Plus, File, Loader2, Copy, Link
} from "lucide-react";
import { toast } from "sonner";
import { ContextMenu, type ContextMenuEntry } from "@/components/ui/context-menu";
import { useStore } from "@/store";
import { Card } from "@/components/ui/glass-card";
import { formatBytes } from "@/lib/utils";
import * as api from "@/lib/api";

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
    if (fileRef.current) {
      fileRef.current.value = "";
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
      <Card className="w-48 shrink-0 p-3 flex flex-col gap-1">
        <div className="flex items-center justify-between mb-1.5 px-1">
          <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
            Buckets
          </span>
          <button
            onClick={async () => {
              const name = prompt("Bucket name:");
              if (!name) return;
              try {
                await api.createBucket(name);
                toast.success(`Bucket "${name}" created`);
                loadBuckets();
              } catch {
                toast.error("Failed to create bucket");
              }
            }}
            className="text-muted-foreground hover:text-neon-500 transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </div>
        {buckets.map((b) => (
          <button
            key={b}
            onClick={() => setActiveBucket(b)}
            className={`w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-all text-left ${
              activeBucket === b
                ? "bg-surface-4 text-foreground"
                : "text-muted-foreground hover:text-foreground hover:bg-surface-3"
            }`}
          >
            <FolderOpen className="w-3.5 h-3.5 shrink-0" />
            <span className="truncate">{b}</span>
          </button>
        ))}
      </Card>

      {/* Object browser */}
      <div className="flex-1 flex flex-col gap-3">
        {/* Toolbar */}
        <div className="flex items-center gap-3">
          <h2 className="text-sm font-medium flex-1">
            {activeBucket ? (
              <span className="text-neon-500">{activeBucket}</span>
            ) : (
              <span className="text-muted-foreground">Select a bucket</span>
            )}
          </h2>
          <button
            onClick={() => activeBucket && loadObjects(activeBucket)}
            className="btn-ghost"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => fileRef.current?.click()}
            disabled={!activeBucket}
            className="btn-primary text-sm disabled:opacity-40"
          >
            <Upload className="w-4 h-4" />
            Upload
          </button>
          <input
            ref={fileRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleUpload}
          />
        </div>

        {/* Objects table */}
        <div className="flex-1 rounded-lg border border-border bg-surface-2 overflow-hidden">
          {blobLoading ? (
            <div className="flex items-center justify-center h-40">
              <Loader2 className="w-5 h-5 animate-spin text-neon-500" />
            </div>
          ) : !activeBucket ? (
            <div className="flex flex-col items-center justify-center h-40 text-muted-foreground gap-2">
              <FolderOpen className="w-8 h-8 opacity-20" />
              <p className="text-sm">Select a bucket</p>
            </div>
          ) : objects.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-40 text-muted-foreground gap-2">
              <File className="w-8 h-8 opacity-20" />
              <p className="text-sm">No objects in this bucket</p>
            </div>
          ) : (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Key</th>
                  <th>Size</th>
                  <th>Type</th>
                  <th>Modified</th>
                  <th className="w-20" />
                </tr>
              </thead>
              <tbody>
                <AnimatePresence>
                  {objects.map((obj, i) => {
                    const objMenu: ContextMenuEntry[] = [
                      { label: "Download", icon: <Download className="w-3.5 h-3.5" />, onClick: () => { window.open(api.getObjectUrl(obj.bucket, obj.key), "_blank"); } },
                      { label: "Copy URL", icon: <Link className="w-3.5 h-3.5" />, onClick: () => { navigator.clipboard.writeText(api.getObjectUrl(obj.bucket, obj.key)); toast.success("URL copied"); } },
                      { label: "Copy key", icon: <Copy className="w-3.5 h-3.5" />, onClick: () => { navigator.clipboard.writeText(obj.key); toast.success("Key copied"); } },
                      { separator: true },
                      { label: "Delete", icon: <Trash2 className="w-3.5 h-3.5" />, danger: true, onClick: () => handleDelete(obj.key) },
                    ];
                    return (
                    <ContextMenu key={obj.key} items={objMenu} as="tr">
                      <motion.tr
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        exit={{ opacity: 0 }}
                        transition={{ delay: i * 0.02 }}
                        className="group"
                      >
                        <td className="font-mono text-xs text-neon-400">{obj.key}</td>
                        <td className="text-xs text-muted-foreground">{formatBytes(obj.size)}</td>
                        <td className="text-xs text-muted-foreground">{obj.content_type}</td>
                        <td className="text-xs text-muted-foreground">
                          {new Date(obj.last_modified).toLocaleString()}
                        </td>
                        <td>
                          <div className="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                            <a
                              href={api.getObjectUrl(obj.bucket, obj.key)}
                              download={obj.key}
                              className="text-muted-foreground hover:text-neon-500 transition-colors"
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
                    </ContextMenu>
                    );
                  })}
                </AnimatePresence>
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
