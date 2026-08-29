"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Key = { id: string; user_id: string; name: string; prefix: string; rpm_limit: number; status: string };

export default function AdminKeysPage() {
  return (
    <AdminShell>
      <AdminListPanel<Key>
        path="/admin/api-keys"
        title="API Key"
        columns={[
          { accessorKey: "prefix", header: "Prefix" },
          { accessorKey: "name", header: "Name" },
          { accessorKey: "user_id", header: "User" },
          { accessorKey: "rpm_limit", header: "RPM" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
