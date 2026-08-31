"use client";

import { useState } from "react";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Settlement = { id?: string; status?: string; amount_minor?: number; channel_org_id?: string };

export default function ChannelSettlements() {
  const [items, setItems] = useState<Settlement[]>([]);
  const [message, setMessage] = useState("渠道只能看本渠道结算单。P0 打款由平台财务人工完成。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/settlements`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Settlement[];
    setItems(next);
    setMessage(`本渠道结算单 ${next.length} 张`);
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道结算</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">佣金冻结期满后按月出结算单。这里不含其他渠道，也不含 prompt。</p>
      <Button variant="outline" onClick={refresh}>
        刷新结算
      </Button>
      <LedgerTable
        columns={["结算单", "状态", "金额"]}
        emptyTitle="暂无结算单"
        emptyDetail="佣金冻结期满后按月出单。P0 打款由平台财务人工完成。"
        rows={items.map((item) => ({
          key: item.id || "settlement",
          cells: [item.id || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
