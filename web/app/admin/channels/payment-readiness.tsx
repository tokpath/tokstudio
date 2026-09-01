"use client";

import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Badge } from "@/components/ui/badge";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { useState } from "react";

type Lane = { adapter: string; display_name?: string; state: string; instance_count?: number };

export function ChannelPaymentReadiness({ channelID }: { channelID: string }) {
  const [message, setMessage] = useState("");
  const query = useQuery({
    queryKey: ["/admin/channels", channelID, "payments"],
    queryFn: async () => {
      const res = await fetch(`${apiBase}/admin/channels/${channelID}/payments`, { credentials: "include" });
      return res.json();
    },
    enabled: !!channelID,
  });
  const lanes: Lane[] = query.data?.item?.lanes || [];
  const label: Record<string, string> = { none: "未开通", configuring: "配置中", sandbox: "沙箱", live: "已开通", disabled: "停用" };

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <h3 className="mb-3 text-lg font-medium">收款就绪</h3>
      <p className="mb-3 text-sm text-ink-secondary">不展示密钥。紧急停用后，该渠道用户充值页不再出现在线支付按钮。</p>
      <ul className="mb-4 grid gap-2 md:grid-cols-3">
        {lanes.map((lane) => (
          <li key={lane.adapter} className="flex items-center justify-between rounded-stamp border border-hairline px-3 py-2 text-sm">
            <span>{lane.display_name || lane.adapter}</span>
            <Badge tone={lane.state === "live" ? "success" : lane.state === "sandbox" ? "brand" : "neutral"}>
              {label[lane.state] || lane.state}
            </Badge>
          </li>
        ))}
      </ul>
      <ConfirmButton
        size="sm"
        variant="outline"
        title="紧急停用在线支付"
        description="用户将看不到该渠道已开通的在线支付按钮。已填凭证保留。"
        onConfirm={async () => {
          const res = await fetch(`${apiBase}/admin/channels/${channelID}/payments/disable`, {
            method: "POST",
            credentials: "include",
            headers: confirmHeaders,
            body: "{}",
          });
          const body = await res.json();
          setMessage(res.ok ? "已停用该渠道在线支付" : body.error?.message || "操作失败");
        }}
      >
        紧急停用在线支付
      </ConfirmButton>
      {message ? <p className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
    </section>
  );
}
