"use client";

import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string };

export default function AdminChannelsPage() {
  return (
    <AdminShell>
      <AdminListPanel<Channel>
        path="/admin/channels"
        title="渠道 / 代理商"
        columns={[
          { accessorKey: "code", header: "Code" },
          { accessorKey: "type", header: "Type" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "brand_id", header: "Brand" },
        ]}
      />
    </AdminShell>
  );
}
