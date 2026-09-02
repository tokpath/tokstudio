"use client";

import { useState } from "react";
import { AdminShell } from "../shell";
import { BrandEditor } from "@/components/brand-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

export default function AdminBrandsPage() {
  const [brandID, setBrandID] = useState("brd_oem");
  const [name, setName] = useState("");
  const [primary, setPrimary] = useState("");
  const [api, setAPI] = useState("");
  const [admin, setAdmin] = useState("");
  const [message, setMessage] = useState("");

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="oemBrand" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-4 text-sm text-ink-secondary">创建 C 渠道品牌，再上传 Logo。Logo ≤128KiB，短边 64–1024px。</p>
        <div className="mb-4 grid gap-3 sm:grid-cols-2">
          <Input placeholder="站点名" value={name} onChange={(e) => setName(e.target.value)} />
          <Input placeholder="primary_domain" value={primary} onChange={(e) => setPrimary(e.target.value)} />
          <Input placeholder="api_domain" value={api} onChange={(e) => setAPI(e.target.value)} />
          <Input placeholder="admin_domain" value={admin} onChange={(e) => setAdmin(e.target.value)} />
        </div>
        <IfCan action="brands.write">
        <Button
          size="sm"
          onClick={async () => {
            const res = await fetch(`${apiBase}/admin/brands`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json", ...confirmHeaders },
              body: JSON.stringify({ name, primary_domain: primary, api_domain: api, admin_domain: admin }),
            });
            const body = await res.json();
            if (res.ok) {
              setBrandID(body.item.id);
              setMessage(`已创建 ${body.item.id}`);
            } else {
              setMessage(body.error?.message || "创建失败");
            }
          }}
        >
          创建品牌
        </Button>
        </IfCan>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <div className="mb-4 flex flex-wrap gap-2">
          <Input className="w-56" value={brandID} onChange={(e) => setBrandID(e.target.value)} aria-label="品牌 ID" />
        </div>
        <IfCan action="brands.write">
        <BrandEditor
          key={brandID}
          endpoint={`/admin/brands/${brandID}`}
          uploadEndpoint={`/admin/brands/${brandID}/assets`}
          confirmWrites
        />
        </IfCan>
      </section>
    </AdminShell>
  );
}
