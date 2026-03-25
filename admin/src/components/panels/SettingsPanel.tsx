/**
 * @fileoverview SettingsPanel – connection info and operational settings.
 */

"use client";

import React from "react";
import {
  Copy,
  CheckCircle,
  Save,
  RefreshCw,
  Download,
  Trash2,
  Archive,
  CalendarClock,
  FolderCog,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { Card } from "@/components/ui/glass-card";
import * as api from "@/lib/api";
import type { BackupFileInfo, BackupSettings } from "@/lib/api";

const DEFAULT_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700";

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

interface CodeBlockProps {
  label: string;
  code: string;
}

function CodeBlock({ label, code }: CodeBlockProps) {
  const [copied, setCopied] = React.useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    toast.success("Copied to clipboard");
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{label}</p>
      <div className="relative group">
        <pre className="text-xs font-mono text-muted-foreground bg-surface-0 rounded-md p-3 overflow-x-auto border border-border whitespace-pre-wrap break-all">
          {code}
        </pre>
        <button onClick={copy} className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity btn-ghost !p-1">
          {copied ? <CheckCircle className="w-3.5 h-3.5 text-neon-500" /> : <Copy className="w-3.5 h-3.5" />}
        </button>
      </div>
    </div>
  );
}

const PRESETS = [
  { label: "Hourly", cron: "0 * * * *" },
  { label: "Every 6h", cron: "0 */6 * * *" },
  { label: "Daily 02:00", cron: "0 2 * * *" },
  { label: "Weekly Mon 02:00", cron: "0 2 * * 1" },
];

export function SettingsPanel() {
  const [autoUrl, setAutoUrl] = React.useState(true);
  const [localOnly, setLocalOnly] = React.useState(false);
  const [origin, setOrigin] = React.useState(DEFAULT_BASE);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [creatingBackup, setCreatingBackup] = React.useState(false);
  const [refreshingFiles, setRefreshingFiles] = React.useState(false);
  const [settings, setSettings] = React.useState<BackupSettings>({
    dir: "./backups",
    retain: 14,
    schedule_cron: "",
  });
  const [files, setFiles] = React.useState<BackupFileInfo[]>([]);

  React.useEffect(() => {
    if (typeof window !== "undefined") {
      setOrigin(window.location.origin);
    }
  }, []);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [loadedSettings, loadedFiles] = await Promise.all([
        api.getBackupSettings(),
        api.listBackupFiles(),
      ]);
      setSettings(loadedSettings);
      setFiles(loadedFiles);
    } catch {
      toast.error("Failed to load backup settings");
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshFiles = React.useCallback(async () => {
    setRefreshingFiles(true);
    try {
      setFiles(await api.listBackupFiles());
    } catch {
      toast.error("Failed to refresh backups");
    } finally {
      setRefreshingFiles(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  let BASE_URL = DEFAULT_BASE;
  if (localOnly) {
    BASE_URL = "http://127.0.0.1:7700";
  } else if (autoUrl) {
    BASE_URL = origin;
  }

  const token = typeof window !== "undefined"
    ? localStorage.getItem("void_access_token") ?? "<your-token>"
    : "<your-token>";

  const saveSettings = async () => {
    setSaving(true);
    try {
      const updated = await api.updateBackupSettings(settings);
      setSettings(updated);
      toast.success("Backup settings saved");
    } catch (err: any) {
      toast.error(err?.response?.data?.error || "Failed to save backup settings");
    } finally {
      setSaving(false);
    }
  };

  const createBackup = async () => {
    setCreatingBackup(true);
    try {
      await api.createBackupFile();
      toast.success("Backup created");
      await refreshFiles();
    } catch (err: any) {
      toast.error(err?.response?.data?.error || "Failed to create backup");
    } finally {
      setCreatingBackup(false);
    }
  };

  const deleteBackup = async (name: string) => {
    if (!window.confirm(`Delete backup "${name}"?`)) {
      return;
    }
    try {
      await api.deleteBackupFile(name);
      toast.success(`Deleted ${name}`);
      await refreshFiles();
    } catch (err: any) {
      toast.error(err?.response?.data?.error || "Failed to delete backup");
    }
  };

  return (
    <div className="space-y-4 max-w-6xl pb-10">
      <div>
        <h2 className="text-lg font-semibold text-foreground">Settings & Operations</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configure backup retention and schedules, create server-side archives, and grab connection snippets.
        </p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[1.1fr_0.9fr] gap-4">
        <Card className="p-4 space-y-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h3 className="font-medium text-sm flex items-center gap-2">
                <FolderCog className="w-4 h-4 text-neon-500" />
                Backup Policy
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                These settings are persisted to the server config and applied live.
              </p>
            </div>
            {loading && <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1.5 md:col-span-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Backup Directory
              </label>
              <input
                value={settings.dir}
                onChange={(e) => setSettings((prev) => ({ ...prev, dir: e.target.value }))}
                className="input-field text-sm"
                placeholder="./backups"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Max Backups
              </label>
              <input
                type="number"
                min={0}
                value={settings.retain}
                onChange={(e) => setSettings((prev) => ({ ...prev, retain: Number(e.target.value || "0") }))}
                className="input-field text-sm"
              />
              <p className="text-xs text-muted-foreground">`0` means keep all backups forever.</p>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Next Scheduled Run
              </label>
              <div className="input-field text-sm flex items-center">
                <CalendarClock className="w-4 h-4 mr-2 text-neon-500" />
                <span>{formatDate(settings.next_run)}</span>
              </div>
            </div>

            <div className="space-y-1.5 md:col-span-2">
              <label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Schedule Cron
              </label>
              <input
                value={settings.schedule_cron}
                onChange={(e) => setSettings((prev) => ({ ...prev, schedule_cron: e.target.value }))}
                className="input-field text-sm font-mono"
                placeholder="0 2 * * *"
              />
              <p className="text-xs text-muted-foreground">
                Standard 5-field cron: `minute hour day month weekday`.
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            {PRESETS.map((preset) => (
              <button
                key={preset.cron}
                onClick={() => setSettings((prev) => ({ ...prev, schedule_cron: preset.cron }))}
                className="btn-ghost text-xs"
              >
                {preset.label}
              </button>
            ))}
            <button
              onClick={() => setSettings((prev) => ({ ...prev, schedule_cron: "" }))}
              className="btn-ghost text-xs text-red-400"
            >
              Disable Schedule
            </button>
          </div>

          <div className="flex flex-wrap items-center gap-3 pt-1">
            <button onClick={saveSettings} disabled={saving || loading} className="btn-primary text-sm">
              {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              Save Backup Settings
            </button>
            <button onClick={createBackup} disabled={creatingBackup || loading} className="btn-secondary text-sm">
              {creatingBackup ? <Loader2 className="w-4 h-4 animate-spin" /> : <Archive className="w-4 h-4" />}
              Create Backup Now
            </button>
          </div>
        </Card>

        <Card className="p-4 space-y-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h3 className="font-medium text-sm flex items-center gap-2">
                <Archive className="w-4 h-4 text-neon-500" />
                Stored Backups
              </h3>
              <p className="text-xs text-muted-foreground mt-1">
                Download or delete server-side backup archives.
              </p>
            </div>
            <button onClick={() => { void refreshFiles(); }} className="btn-ghost !p-1.5" title="Refresh backups">
              <RefreshCw className={refreshingFiles ? "w-4 h-4 animate-spin" : "w-4 h-4"} />
            </button>
          </div>

          <div className="space-y-2">
            {files.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
                No backup archives found in `{settings.dir}`.
              </div>
            ) : (
              files.map((file) => (
                <div key={file.name} className="rounded-lg border border-border bg-surface-1 px-3 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-foreground break-all">{file.name}</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {formatBytes(file.size)} · {formatDate(file.modified_at)}
                      </p>
                    </div>
                    <div className="flex gap-2 shrink-0">
                      <button
                        onClick={() => { void api.downloadBackupFile(file.name); }}
                        className="btn-ghost !p-2"
                        title="Download backup"
                      >
                        <Download className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => { void deleteBackup(file.name); }}
                        className="btn-ghost !p-2 text-red-400"
                        title="Delete backup"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      <Card className="p-4 space-y-4">
        <h3 className="font-medium text-sm border-b border-border pb-2">Network Preferences</h3>

        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-foreground">Auto-detect URL</p>
            <p className="text-xs text-muted-foreground">Automatically substitute the current domain for connection scripts.</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input type="checkbox" className="sr-only peer" checked={autoUrl} onChange={(e) => setAutoUrl(e.target.checked)} disabled={localOnly} />
            <div className="w-9 h-5 bg-surface-2 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-neon-500" />
          </label>
        </div>

        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-foreground">Local Only Access</p>
            <p className="text-xs text-muted-foreground">Force snippets to use <code>127.0.0.1</code> for local program connections.</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input type="checkbox" className="sr-only peer" checked={localOnly} onChange={(e) => setLocalOnly(e.target.checked)} />
            <div className="w-9 h-5 bg-surface-2 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-neon-500" />
          </label>
        </div>
      </Card>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <Card delay={0.05} className="space-y-4">
          <h3 className="font-medium text-sm">API Endpoint</h3>
          <CodeBlock label="Base URL" code={BASE_URL} />
          <CodeBlock label="Health Check" code={`curl ${BASE_URL}/health`} />
          <CodeBlock
            label="Login"
            code={`curl -X POST ${BASE_URL}/v1/auth/login \\
  -H 'Content-Type: application/json' \\
  -d '{"username":"admin","password":"admin"}'`}
          />
        </Card>

        <Card delay={0.1} className="space-y-4">
          <h3 className="font-medium text-sm">TypeScript ORM</h3>
          <CodeBlock
            label="Install"
            code={`npm install @voiddb/orm\n# or\nyarn add @voiddb/orm`}
          />
          <CodeBlock
            label="Usage"
            code={`import { VoidClient } from '@voiddb/orm'\n\nconst db = new VoidClient({\n  url: '${BASE_URL}',\n  token: '${token.slice(0, 20)}...',\n})\n\nconst col = db.database('myapp').collection('users')\nawait col.insert({ name: 'Alice', age: 30 })\nconst users = await col.find({\n  where: [{ field: 'age', op: 'gte', value: 18 }],\n  order_by: [{ field: 'name', dir: 'asc' }],\n  limit: 10,\n})`}
          />
        </Card>
      </div>
    </div>
  );
}
