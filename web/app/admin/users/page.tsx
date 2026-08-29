"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type User = { id: string; email: string; channel_org_id: string; status: string };

export default function AdminUsersPage() {
  return (
    <AdminShell>
      <AdminListPanel<User>
        path="/admin/users"
        title="用户/项目"
        columns={[
          { accessorKey: "email", header: "Email" },
          { accessorKey: "channel_org_id", header: "Channel" },
          { accessorKey: "status", header: "Status" },
        ]}
      />
    </AdminShell>
  );
}
