"use client";

import { useEffect, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { useQuery } from "@tanstack/react-query";
import { AdminShell } from "../shell";
import { BrandEditor } from "@/components/brand-editor";
import { Button } from "@/components/ui/button";
import { useViewer } from "@/components/rbac/viewer-context";
import { canWrite } from "@/lib/rbac";
import { readAllPages } from "@/lib/api-pages";
import { OEMCreateDialog } from "../channels/oem-create-dialog";

type Brand = { id: string; name: string; primary_domain: string };
export default function AdminBrandsPage() {
  const t = useTranslations("oemDelivery");
  const viewer = useViewer();
  const router = useRouter();
  const desired = useSearchParams().get("brand");
  const [create, setCreate] = useState(false);
  const query = useQuery({ queryKey: [viewer.userId, "/admin/brands", "all"], queryFn: () => readAllPages<Brand>("/admin/brands") });
  const selected = query.data?.items.find(item => item.id === desired) || (!desired ? query.data?.items[0] : undefined);
  useEffect(() => { if (!desired && selected) router.replace(`/admin/brands?brand=${encodeURIComponent(selected.id)}`); }, [desired, selected, router]);
  return <AdminShell>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6"><h1 className="text-xl font-semibold">{t("brandSettings")}</h1><div className="mt-4 flex flex-wrap gap-3"><select aria-label={t("selectBrand")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" disabled={query.isPending || query.isError} value={selected?.id || ""} onChange={event => router.replace(`/admin/brands?brand=${encodeURIComponent(event.target.value)}`)}><option value="">{t("selectBrand")}</option>{query.data?.items.map(brand => <option key={brand.id} value={brand.id}>{brand.name} · {brand.primary_domain}</option>)}</select>{canWrite("channels.write", viewer) ? <Button onClick={() => setCreate(true)}>{t("createTitle")}</Button> : null}<Button variant="outline" onClick={() => void query.refetch()}>{t("refresh")}</Button></div>{query.isError || (desired && !selected && !query.isPending) ? <p role="alert" className="mt-3">{t("loadFailed")}</p> : null}</section>
    {selected ? <section className="rounded-card border border-hairline bg-canvas-raised p-6"><BrandEditor key={`${viewer.userId}:${selected.id}`} endpoint={`/admin/brands/${encodeURIComponent(selected.id)}`} uploadEndpoint={`/admin/brands/${encodeURIComponent(selected.id)}/assets`} confirmWrites readOnly={!canWrite("brands.write", viewer)} /></section> : null}
    <OEMCreateDialog open={create} onOpenChange={setCreate} />
  </AdminShell>;
}
