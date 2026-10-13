"use client";

import { catalogHelp } from "@/lib/catalog-copy";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { ProbeCell } from "./probe-cell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import {
  adapterLabel,
  healthLabel,
  healthTone,
  providerHref,
  providerKindLabel,
  providerStatusLabel,
  catalogStatusTone,
} from "@/lib/catalog-admin";
import { CATALOG_HELP } from "@/lib/catalog-copy";

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  health: string;
  status: string;
  account_count?: number;
};

export default function AdminProvidersPage() {
  const tUI = useTranslations();
  const CATALOG_HELP = catalogHelp(tUI);
  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="text-sm text-ink-secondary">{CATALOG_HELP.providers} {tUI("consoleCopy.healthValuesAreRoutingRecordsNotProofOf")}</p>
      </section>
      <AdminListPanel<Provider>
        headingLevel={1}
        path="/admin/providers"
        title={tUI("admin.providers")}
        emptyTitle={tUI("consoleCopy.noProvidersYet")}
        emptyDetail={tUI("consoleCopy.thePlatformConnectsUpstreamProvidersChannelsCannotCreate")}
        rowHref={(row) => providerHref(String(row.slug || row.id))}
        actions={
          <IfCan action="providers.write">
            <Button asChild size="sm"><Link href="/admin/providers/new">{tUI("consoleCopy.newProvider")}</Link></Button>
          </IfCan>
        }
        columns={[
          { accessorKey: "name", header: tUI("user.colName") },
          {
            accessorKey: "kind",
            header: tUI("partnerBoard.colKind"),
            cell: ({ row }) => providerKindLabel(row.original.kind, tUI),
          },
          {
            accessorKey: "adapter",
            header: tUI("quickstartUi.statProto"),
            cell: ({ row }) => adapterLabel(row.original.adapter, tUI),
          },
          {
            accessorKey: "health",
            header: tUI("consoleCopy.health"),
            cell: ({ row }) => <Badge tone={healthTone(row.original.health)}>{healthLabel(row.original.health, tUI)}</Badge>,
          },
          {
            accessorKey: "status",
            header: tUI("compareUi.status"),
            cell: ({ row }) => (
              <Badge tone={catalogStatusTone(row.original.status)}>{providerStatusLabel(row.original.status, tUI)}</Badge>
            ),
          },
          {
            accessorKey: "account_count",
            header: tUI("adminUi.accountPool"),
            cell: ({ row }) => row.original.account_count ?? 0,
          },
          {
            id: "probe",
            header: tUI("consoleCopy.probe"),
            cell: ({ row }) => <ProbeCell id={String(row.original.id)} />,
          },
        ]}
      />
    </AdminShell>
  );
}
