"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Audit = { id: string; action: string; resource_type: string; resource_id: string };

export default function AdminAuditPage() {
  return (
    <AdminShell>
      <AdminListPanel<Audit>
        path="/admin/audit-logs"
        title="审计日志"
        columns={[
          { accessorKey: "action", header: "Action" },
          { accessorKey: "resource_type", header: "Resource" },
          { accessorKey: "resource_id", header: "ID" },
        ]}
      />
    </AdminShell>
  );
}
