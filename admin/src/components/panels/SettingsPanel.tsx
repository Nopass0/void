/**
 * @fileoverview SettingsPanel – connection info and configuration display.
 */

"use client";

import React from "react";
import { Copy, CheckCircle } from "lucide-react";
import { toast } from "sonner";
import { Card } from "@/components/ui/glass-card";

const DEFAULT_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700";

interface CodeBlockProps {
  label: string;
  code: string;
}

function CodeBlock({ label, code }: CodeBlockProps) {
  const [copied, setCopied] = React.useState(false);
  const copy = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    toast.success("Copied to clipboard");
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{label}</p>
      <div className="relative group">
        <pre className="text-xs font-mono text-muted-foreground bg-surface-0 rounded-md p-3 overflow-x-auto border border-border">
          {code}
        </pre>
        <button
          onClick={copy}
          className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity btn-ghost !p-1"
        >
          {copied ? <CheckCircle className="w-3.5 h-3.5 text-neon-500" /> : <Copy className="w-3.5 h-3.5" />}
        </button>
      </div>
    </div>
  );
}

export function SettingsPanel() {
  const [autoUrl, setAutoUrl] = React.useState(true);
  const [localOnly, setLocalOnly] = React.useState(false);
  const [origin, setOrigin] = React.useState(DEFAULT_BASE);

  React.useEffect(() => {
    if (typeof window !== "undefined") {
      setOrigin(window.location.origin);
    }
  }, []);

  let BASE_URL = DEFAULT_BASE;
  if (localOnly) {
    BASE_URL = "http://127.0.0.1:7700";
  } else if (autoUrl) {
    BASE_URL = origin;
  }

  const token = typeof window !== "undefined"
    ? localStorage.getItem("void_access_token") ?? "<your-token>"
    : "<your-token>";

  return (
    <div className="space-y-4 max-w-3xl pb-10">
      <h2 className="text-lg font-semibold text-foreground">Settings & Connection</h2>

      <Card className="p-4 space-y-4">
        <h3 className="font-medium text-sm border-b border-border pb-2">Network Preferences</h3>
        
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-foreground">Auto-detect URL</p>
            <p className="text-xs text-muted-foreground">Automatically substitute the current domain for connection scripts.</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input type="checkbox" className="sr-only peer" checked={autoUrl} onChange={(e) => setAutoUrl(e.target.checked)} disabled={localOnly} />
            <div className="w-9 h-5 bg-surface-2 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-neon-500"></div>
          </label>
        </div>

        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-foreground">Local Only Access</p>
            <p className="text-xs text-muted-foreground">Force snippets to use <code>127.0.0.1</code> for local program connections.</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input type="checkbox" className="sr-only peer" checked={localOnly} onChange={(e) => setLocalOnly(e.target.checked)} />
            <div className="w-9 h-5 bg-surface-2 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-neon-500"></div>
          </label>
        </div>
      </Card>

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
          code={`npm install @voiddb/orm
# or
yarn add @voiddb/orm`}
        />
        <CodeBlock
          label="Usage"
          code={`import { VoidClient } from '@voiddb/orm'

const db = new VoidClient({
  url: '${BASE_URL}',
  token: '${token.slice(0, 20)}...',
})

// Insert
const col = db.database('myapp').collection('users')
const { _id } = await col.insert({ name: 'Alice', age: 30 })

// Query
const users = await col.find({
  where: [{ field: 'age', op: 'gte', value: 18 }],
  orderBy: [{ field: 'name', dir: 'asc' }],
  limit: 10,
})

// Update
await col.patch(_id, { age: 31 })`}
        />
      </Card>

      <Card delay={0.15} className="space-y-4">
        <h3 className="font-medium text-sm">S3-Compatible Blob Storage</h3>
        <CodeBlock
          label="AWS CLI"
          code={`aws s3 --endpoint-url ${BASE_URL}/s3 \\
  --no-sign-request \\
  ls s3://my-bucket/`}
        />
        <CodeBlock
          label="AWS SDK (Node.js)"
          code={`import { S3Client, PutObjectCommand } from '@aws-sdk/client-s3'

const s3 = new S3Client({
  endpoint: '${BASE_URL}/s3',
  region: 'void-1',
  credentials: { accessKeyId: 'void', secretAccessKey: '${token.slice(0, 20)}...' },
  forcePathStyle: true,
})

await s3.send(new PutObjectCommand({
  Bucket: 'my-bucket',
  Key: 'hello.txt',
  Body: Buffer.from('Hello VoidDB!'),
}))`}
        />
      </Card>

      <Card delay={0.2} className="space-y-4">
        <h3 className="font-medium text-sm">Go ORM</h3>
        <CodeBlock
          label="Usage"
          code={`import "github.com/voiddb/void/orm/go"

client, _ := voidorm.New(voidorm.Config{
  URL:   "${BASE_URL}",
  Token: os.Getenv("VOID_TOKEN"),
})

col := client.DB("myapp").Collection("users")
id, _ := col.Insert(ctx, voidorm.Doc{"name": "Alice", "age": 30})
docs, _ := col.Find(ctx, voidorm.NewQuery().Where("age", voidorm.Gte, 18))
_ = col.Patch(ctx, id, voidorm.Doc{"age": 31})`}
        />
      </Card>
    </div>
  );
}
