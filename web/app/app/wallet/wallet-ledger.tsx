"use client";

import { useEffect, useState } from "react";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";

type LedgerRow = {
  id: string;
  entry_type?: string;
  amount_minor?: number;
};

export function WalletLedger() {
  const [rows, setRows] = useState<LedgerRow[] | null>(null);
  const [message, setMessage] = useState("登录后可查看交易流水。");

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/ledger`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setRows([]);
      setMessage(body.error?.message || "未登录");
      return;
    }
    setRows(body.items || []);
    setMessage("流水已刷新");
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">交易记录</h2>
        <Button variant="outline" size="sm" onClick={() => void refresh()}>
          刷新
        </Button>
      </div>
      {!rows || rows.length === 0 ? (
        <EmptyLedger title="暂无交易" detail={message} />
      ) : (
        <ul className="divide-y divide-hairline rounded-card border border-hairline bg-canvas-raised">
          {rows.map((row) => (
            <li key={row.id} className="flex items-center justify-between gap-3 px-4 py-3 text-sm">
              <span className="font-mono text-ink-mute">{row.entry_type || row.id}</span>
              <span className="font-mono tabular-nums">{row.amount_minor ?? "—"}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
