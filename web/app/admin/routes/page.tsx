"use client";


import { useTranslations } from "next-intl";
import Link from "next/link";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { catalogStatusTone, routeStatusLabel, routeStrategyLabel } from "@/lib/catalog-admin";

type Route = {
  id: string; public_model_id: string; strategy: string; status: string;
  candidates?: { provider_slug?: string; provider_id?: string }[];
};

export default function AdminRoutesPage() {
  const tUI = useTranslations();

  return <AdminShell>
    <AdminListPanel<Route>
        headingLevel={1}
      path="/admin/routes"
      title={tUI("admin.routes")}
      emptyTitle={tUI("consoleCopy.noRouteGroupsYet")}
      emptyDetail={tUI("consoleCopy.publishAModelFirstThenConfigureProvidersAnd")}
      rowHref={(row) => `/admin/routes/${encodeURIComponent(row.id)}`}
      actions={<IfCan action="routes.write"><Button asChild size="sm"><Link href="/admin/routes/new">{tUI("consoleCopy.createRouteGroup")}</Link></Button></IfCan>}
      columns={[
        { accessorKey: "public_model_id", header: tUI("consoleCopy.publicModel389") },
        { accessorKey: "strategy", header: tUI("consoleCopy.strategy390"), cell: ({ row }) => routeStrategyLabel(row.original.strategy, tUI) },
        { id: "providers", header: tUI("admin.providers"), cell: ({ row }) => row.original.candidates?.map((item) => item.provider_slug || item.provider_id).join(" → ") || tUI("consoleCopy.notConfigured") },
        { accessorKey: "status", header: tUI("compareUi.status"), cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{routeStatusLabel(row.original.status, tUI)}</Badge> },
      ]}
    />
  </AdminShell>;
}
