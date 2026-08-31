"use client";

import { useEffect, useState } from "react";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type UsageRow = {
  id: string;
  request_id?: string;
  state?: string;
  customer_amount_minor?: number;
  public_model_id?: string;
};

export function ActivityTable() {
  const [rows, setRows] = useState<UsageRow[] | null>(null);
  const [message, setMessage] = useState("登录后可查看请求明细。");

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/usage`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setRows([]);
      setMessage(body.error?.message || "未登录");
      return;
    }
    setRows(body.items || []);
    setMessage("请求明细已刷新");
  }

  useEffect(() => {
    void refresh();
  }, []);

  if (!rows || rows.length === 0) {
    return (
      <div className="flex flex-col gap-3">
        <Button variant="outline" size="sm" className="self-start" onClick={() => void refresh()}>
          刷新
        </Button>
        <EmptyLedger title="暂无数据" detail={message} />
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-card border border-hairline">
      <table className="w-full text-left text-sm">
        <thead className="bg-canvas-raised text-ink-mute">
          <tr>
            <th className="px-4 py-3 font-medium">模型</th>
            <th className="px-4 py-3 font-medium">状态</th>
            <th className="px-4 py-3 font-medium">请求 ID</th>
            <th className="px-4 py-3 font-medium">金额</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} className="border-t border-hairline">
              <td className="px-4 py-3 font-mono text-xs">{row.public_model_id || "—"}</td>
              <td className="px-4 py-3">{row.state || "—"}</td>
              <td className="px-4 py-3 font-mono text-xs text-ink-mute">{row.request_id || row.id}</td>
              <td className="px-4 py-3 font-mono tabular-nums">{row.customer_amount_minor ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
