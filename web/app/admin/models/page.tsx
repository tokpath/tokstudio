"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { apiClient } from "@/lib/client";
import { type AdminModel, modelEditHref, vendorLabel } from "@/lib/catalog";
import { catalogStatusTone } from "@/lib/catalog-admin";

export default function AdminModelsPage() {
  const t = useTranslations("modelService");
  return (
    <AdminShell>
      <AdminListPanel<AdminModel>
        path="/admin/models"
        title={t("listTitle")}
        emptyTitle={t("emptyTitle")}
        emptyDetail={t("emptyDetail")}
        rowHref={(row) => modelEditHref(row.id)}
        actions={
          <IfCan action="models.write">
            <Button asChild size="sm"><Link href="/admin/models/new">{t("create")}</Link></Button>
          </IfCan>
        }
        columns={[
          { accessorKey: "display_name", header: t("modelName") },
          { accessorKey: "id", header: t("callID"), cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.id}</span> },
          { accessorKey: "vendor", header: t("vendor"), cell: ({ row }) => vendorLabel(row.original.vendor) },
          { accessorKey: "status", header: t("status"), cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{row.original.status === "published" ? (row.original.config_ready === false ? t("state.not_configured") : t("ready")) : t("draft")}</Badge> },
          {
            id: "route",
            header: t("access"),
            cell: ({ row }) => {
              return t(`state.${row.original.service_readiness?.runtime_state || "unknown"}`);
            },
          },
        ]}
      />
    </AdminShell>
  );
}
