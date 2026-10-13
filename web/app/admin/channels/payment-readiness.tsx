"use client";

import { networkUnavailable } from "@/lib/confirm";
import { useTranslations } from "next-intl";
import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Badge } from "@/components/ui/badge";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { useState } from "react";
import { IfCan } from "@/components/rbac/if-can";

type Lane = { adapter: string; display_name?: string; state: string; instance_count?: number };

export function ChannelPaymentReadiness({ channelID }: { channelID: string }) {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);
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
  const label: Record<string, string> = { none: tUI("consoleCopy.notActivated"), configuring: tUI("oemDelivery.phase.configuring"), sandbox: tUI("consoleCopy.sandbox"), live: tUI("consoleCopy.activated"), disabled: tUI("staff.disable") };

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <h3 className="mb-3 text-lg font-medium">{tUI("consoleCopy.paymentReadiness")}</h3>
      <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.secretsAreHiddenAnEmergencyShutdownRemovesOnline")}</p>
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
      <IfCan action="channels.payments.disable">
      <ConfirmButton
        size="sm"
        variant="outline"
        title={tUI("consoleCopy.emergencyOnlinePaymentShutdown")}
        description={tUI("consoleCopy.onlinePaymentButtonsForThisChannelWillBe")}
        onConfirm={async () => {
                    try {
          const res = await fetch(`${apiBase}/admin/channels/${channelID}/payments/disable`, {
            method: "POST",
            credentials: "include",
            headers: confirmHeaders,
            body: "{}",
          });
          const body = await res.json();
          setMessage(res.ok ? tUI("consoleCopy.onlinePaymentsDisabledForThisChannel") : body.error?.message || tUI("user.actFail"));
                    const __ok = res.ok;
                    return __ok;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
      >
        {tUI("consoleCopy.emergencyOnlinePaymentShutdown")}</ConfirmButton>
      </IfCan>
      {message ? <p className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
    </section>
  );
}
