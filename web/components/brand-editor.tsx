"use client";

import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { useEffect, useRef, useState } from "react";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import type { AssetKind, Brand } from "@/lib/brand";
import { describeAssetLimit, inspectLocalAsset, publicAssetURL } from "@/lib/brand";
import { StorageSourceBadge } from "@/components/storage-source-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { applyStorageFact, type StorageSource } from "@/lib/storage-source";

const KINDS: AssetKind[] = ["logo", "logo_dark", "favicon", "og_image"];

export function BrandEditor({
  endpoint,
  uploadEndpoint,
  confirmWrites = false,
  readOnly = false,
}: {
  endpoint: string;
  uploadEndpoint: string;
  confirmWrites?: boolean;
  readOnly?: boolean;
}) {
  const t = useTranslations("oemDelivery");
  const [domains,setDomains]=useState({primary_domain:"",api_domain:"",admin_domain:""});
  const loadVersion = useRef(0);
  const [brand, setBrand] = useState<Brand | undefined>();
  const [name, setName] = useState("");
  const [color, setColor] = useState("#2150D6");
  const [message, setMessage] = useState("");
  const [customizable, setCustomizable] = useState(true);
  const [storage, setStorage] = useState<StorageSource | undefined>();

  async function load() {
    const version = ++loadVersion.current;
    setBrand(undefined);
    try {
    const res = await fetch(`${apiBase}${endpoint}`, { credentials: "include" });
    const body = await res.json();
    if (version !== loadVersion.current) return;
    const nextStorage = applyStorageFact(body, res.ok);
    if (nextStorage) {
      setStorage(nextStorage);
    }
    if (!res.ok) {
      setMessage(body.error?.message || "读取品牌失败");
      return;
    }
    const item = (body.brand || body.item) as Brand;
    setBrand(item);
    setDomains({primary_domain:item?.primary_domain||"",api_domain:item?.api_domain||"",admin_domain:item?.admin_domain||""});
    setName(item?.name || "");
    setColor(item?.theme?.brand || item?.theme?.primary || "#2150D6");
    if (typeof body.customizable === "boolean") {
      setCustomizable(body.customizable);
    }
    } catch { if (version === loadVersion.current) setMessage(t("loadFailed")); }
  }

  useEffect(() => {
    void load();
    return () => { loadVersion.current++; };
  }, [endpoint]);

  async function save() {
    try {
    const headers: Record<string, string> = { "Content-Type": "application/json", ...(confirmWrites ? confirmHeaders : {}) };
    const res = await fetch(`${apiBase}${endpoint}`, {
      method: "PATCH",
      credentials: "include",
      headers,
      body: JSON.stringify({ name, ...domains, theme: { brand: color, brand_emphasis: color } }),
    });
    const body = await res.json();
    setMessage(res.ok ? "已保存名称和主色" : body.error?.message || "保存失败");
    if (res.ok) await load();
    return res.ok;
    } catch { setMessage(t("resultUnknown")); return false; }
  }

  async function upload(kind: AssetKind, file: File) {
    try {
    const local = await inspectLocalAsset(kind, file);
    if (local) {
      setMessage(local);
      return;
    }
    const form = new FormData();
    form.set("kind", kind);
    form.set("file", file);
    const res = await fetch(`${apiBase}${uploadEndpoint}`, { method: "POST", credentials: "include", body: form });
    const body = await res.json();
    const nextStorage = applyStorageFact(body, res.ok);
    if (nextStorage) {
      setStorage(nextStorage);
    }
    setMessage(res.ok ? `已上传 ${kind} ${body.item?.width_px}×${body.item?.height_px}` : body.error?.message || "存储不可用");
    if (res.ok) await load();
    } catch { setMessage(t("resultUnknown")); }
  }

  const locked = readOnly || !customizable || !brand;

  return (
    <div className="space-y-6">
      {locked ? <p className="text-sm text-ink-secondary">{t("readOnly")}</p> : null}
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm text-ink-secondary">
          站点名
          <Input className="mt-1" value={name} disabled={locked} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="text-sm text-ink-secondary">
          章的颜色
          <Input className="mt-1" type="color" value={color} disabled={locked} onChange={(e) => setColor(e.target.value)} />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <span className="inline-flex h-9 items-center rounded-stamp bg-brand px-3 text-sm text-on-brand">主按钮预览</span>
        <span className="text-sm text-brand-emphasis">链字预览</span>
        {brand?.logo_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={publicAssetURL(brand.logo_url)} alt="" className="h-6 max-w-24 object-contain" />
        ) : null}
        <StorageSourceBadge storage={storage} />
        {brand?.logo_url ? (
          <a
            className="text-sm text-brand-emphasis"
            href={publicAssetURL(brand.logo_url)}
            target="_blank"
            rel="noreferrer"
          >
            下载 Logo
          </a>
        ) : null}
      </div>
      <div className="grid gap-3 sm:grid-cols-3">{(["primary_domain","api_domain","admin_domain"] as const).map((key,index)=><label key={key} className="text-sm">{t(["primary","api","admin"][index])}<Input className="mt-1" disabled={locked} value={domains[key]} onChange={event=>setDomains(current=>({...current,[key]:event.target.value.trim().toLowerCase()}))}/></label>)}</div>
      {!locked ? <ConfirmButton title={t("brandSettings")} description={[brand?.name||"",...(["primary_domain","api_domain","admin_domain"] as const).map(key=>`${brand?.[key]} → ${domains[key]}`)].join("\n")} validate={()=>{const valid=name.trim()&&Object.values(domains).every(host=>/^[a-zA-Z0-9.-]+$/.test(host));if(!valid)setMessage(t("fillBrand"));return Boolean(valid)}} onConfirm={save}>{t("brandSettings")}</ConfirmButton>:null}
      <div className="grid gap-4 md:grid-cols-2">
        {KINDS.map((kind) => (
          <label key={kind} className="rounded-card border border-hairline p-4 text-sm">
            <p className="font-medium text-ink">{kind}</p>
            <p className="mt-1 text-ink-mute">{describeAssetLimit(kind)}</p>
            <Input
              className="mt-3"
              type="file"
              disabled={locked}
              accept={kind === "favicon" ? "image/png,image/x-icon,.ico" : "image/png,image/webp,image/svg+xml,image/jpeg"}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void upload(kind, file);
              }}
            />
          </label>
        ))}
      </div>
      <p role="status" aria-live="polite" className="text-sm text-ink-secondary">{message}</p>
    </div>
  );
}
