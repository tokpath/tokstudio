"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Route = { id: string; public_model_id: string; strategy: string; status: string };

export default function AdminRoutesPage() {
  return (
    <AdminShell>
      <AdminListPanel<Route>
        path="/admin/routes"
        title="路由组"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "public_model_id", header: "Model" },
          { accessorKey: "strategy", header: "Strategy" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
