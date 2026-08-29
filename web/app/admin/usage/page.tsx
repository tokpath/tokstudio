"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Usage = { id: string; request_id: string; state: string; customer_amount?: number };

export default function AdminUsagePage() {
  return (
    <AdminShell>
      <AdminListPanel<Usage>
        path="/admin/usage"
        title="用量 / 账单"
        columns={[
          { accessorKey: "request_id", header: "Request" },
          { accessorKey: "state", header: "State" },
          { accessorKey: "id", header: "ID" },
        ]}
      />
    </AdminShell>
  );
}
