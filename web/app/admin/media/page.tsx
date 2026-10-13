"use client";

import { useTranslations } from "next-intl";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Job = { id: string; kind: string; task_type?: string; status: string; model: string };

export default function AdminMediaPage() {
  const tUI = useTranslations();
  return (
    <AdminShell>
      <AdminListPanel<Job>
        headingLevel={1}
        path="/admin/media"
        exportCSV
        title={tUI("userNav.media")}
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "kind", header: tUI("partnerBoard.colKind") },
          { accessorKey: "task_type", header: tUI("consoleCopy.mode") },
          { accessorKey: "model", header: tUI("adminUi.colModel") },
          { accessorKey: "status", header: tUI("compareUi.status") },
        ]}
      />
    </AdminShell>
  );
}
