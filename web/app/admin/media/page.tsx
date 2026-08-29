"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Job = { id: string; kind: string; status: string; model: string };

export default function AdminMediaPage() {
  return (
    <AdminShell>
      <AdminListPanel<Job>
        path="/admin/media"
        title="媒体任务"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "kind", header: "Kind" },
          { accessorKey: "model", header: "Model" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
