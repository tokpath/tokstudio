"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Model = { id: string; vendor: string; display_name: string; status: string };

export default function AdminModelsPage() {
  return (
    <AdminShell>
      <AdminListPanel<Model>
        path="/admin/models"
        title="模型"
        columns={[
          { accessorKey: "id", header: "Public ID" },
          { accessorKey: "vendor", header: "Vendor" },
          { accessorKey: "display_name", header: "Name" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
