/**
 * @fileoverview SettingsPanel – connection info and configuration display.
 */

"use client";

import React from "react";
import { Copy, CheckCircle } from "lucide-react";
import { toast } from "sonner";
import { GlassCard } from "@/components/ui/glass-card";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:7700";

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
      <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">{label}</p>
      <div className="relative group">
        <pre className="text-xs font-mono bg-black/40 rounded-lg p-3 overflow-x-auto text-void-300 border border-void-500/20">
          {code}
        </pre>
        <button
          onClick={copy}
          className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-void-400"
        >
          {copied ? <CheckCircle className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
        </button>
      </div>
    </div>
  );
}

/**
 * SettingsPanel shows connection strings and SDK examples.
 */
export function SettingsPanel() {
  const token = typeof window !== "undefined"
    ? localStorage.getItem("void_access_token") ?? "<your-token>"
    : "<your-token>";

  return (
    <div className="space-y-4 max-w-3xl">
      <h2 className="text-lg font-bold gradient-text">Settings & Connection</h2>

      <GlassCard delay={0.05} className="space-y-4">
        <h3 className="font-semibold text-sm">API Endpoint</h3>
        <CodeBlock label="Base URL" code={BASE_URL} />
        <CodeBlock label="Health Check" code={`curl ${BASE_URL}/health`} />
        <CodeBlock
          label="Login"
          code={`curl -X POST ${BASE_URL}/v1/auth/login \\
  -H 'Content-Type: application/json' \\
  -d '{"username":"admin","password":"admin"}'`}
        />
      </GlassCard>

      <GlassCard delay={0.1} className="space-y-4">
        <h3 className="font-semibold text-sm">TypeScript ORM</h3>
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
      </GlassCard>

      <GlassCard delay={0.15} className="space-y-4">
        <h3 className="font-semibold text-sm">S3-Compatible Blob Storage</h3>
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
      </GlassCard>

      <GlassCard delay={0.2} className="space-y-4">
        <h3 className="font-semibold text-sm">Go ORM</h3>
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
      </GlassCard>
    </div>
  );
}
