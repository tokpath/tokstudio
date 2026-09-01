"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Field = { key: string; label: string; secret?: boolean; required?: boolean };
type Lane = {
  adapter: string;
  display_name: string;
  brand_color?: string;
  state: string;
  instance_count: number;
  missing_fields?: string[];
  schema?: Field[];
  auto_renew_supported?: boolean;
  checkout_mode?: string;
  pay_currency?: string;
};
type Overview = {
  item?: { lanes?: Lane[]; callback_origin?: string; online_disabled?: boolean; issue_ratio_bps?: number; fen_per_usd?: number };
  channel_type?: string;
  hint?: string;
};

const stateTone: Record<string, "neutral" | "brand" | "success" | "warn"> = {
  none: "neutral",
  configuring: "warn",
  sandbox: "brand",
  live: "success",
  disabled: "warn",
};

export function PaymentLanesPanel() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["/channel/payments/overview"],
    queryFn: async () => {
      const res = await fetch(`${apiBase}/channel/payments/overview`, { credentials: "include" });
      return (await res.json()) as Overview;
    },
  });
  const [open, setOpen] = useState<Lane | null>(null);
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [mode, setMode] = useState("sandbox");
  const [creds, setCreds] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [instanceID, setInstanceID] = useState("");

  const lanes = query.data?.item?.lanes || [];
  const origin = query.data?.item?.callback_origin || "";
  const fields = open?.schema || [];
  const webhookPath = open ? `/v1/payments/${open.adapter}/webhook` : "";

  const stateLabel = useMemo(
    () =>
      ({
        none: "未开通",
        configuring: "配置中",
        sandbox: "沙箱可用",
        live: "已开通",
        disabled: "已停用",
      }) as Record<string, string>,
    [],
  );

  function resetWizard(lane: Lane) {
    setOpen(lane);
    setStep(0);
    setName(`${lane.display_name} 商户`);
    setMode("sandbox");
    setCreds({});
    setInstanceID("");
    setMessage("");
  }

  async function createDraft(): Promise<string> {
    const res = await fetch(`${apiBase}/channel/payments/instances`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ adapter: open?.adapter, name, mode }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "创建失败");
      return "";
    }
    const id = String(body.item?.id || "");
    setInstanceID(id);
    return id;
  }

  async function saveCredentials() {
    const id = instanceID || (await createDraft());
    if (!id) return false;
    const res = await fetch(`${apiBase}/channel/payments/instances/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ credentials: creds, name, mode }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "保存凭证失败");
      return false;
    }
    setMessage("凭证已保存（密钥不会回显）");
    return true;
  }

  async function testConn() {
    const res = await fetch(`${apiBase}/channel/payments/instances/${instanceID}/test`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "连通失败，请检查必填字段");
      return false;
    }
    setMessage("连通成功");
    return true;
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-ink-secondary">{query.data?.hint}</p>
      <div className="grid gap-4 md:grid-cols-3">
        {lanes.map((lane) => (
          <section key={lane.adapter} className="rounded-card border border-hairline bg-canvas-raised p-5">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h2 className="text-lg font-medium" style={{ color: lane.brand_color }}>
                {lane.display_name}
              </h2>
              <Badge tone={stateTone[lane.state] || "neutral"}>{stateLabel[lane.state] || lane.state}</Badge>
            </div>
            <p className="mb-4 text-sm text-ink-secondary">
              {lane.state === "none"
                ? `开通后，本渠道用户充值页会出现${lane.display_name}。`
                : lane.missing_fields?.length
                  ? `还差：${lane.missing_fields.join("、")}`
                  : `已配置 ${lane.instance_count} 个商户。收款币种 ${lane.pay_currency}。`}
            </p>
            <Button size="sm" onClick={() => resetWizard(lane)}>
              {lane.state === "none" ? `开通${lane.display_name}` : "管理凭证"}
            </Button>
          </section>
        ))}
      </div>
      <Dialog open={!!open} onOpenChange={(next) => !next && setOpen(null)}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>开通 {open?.display_name}</DialogTitle>
            <DialogDescription>
              第 {step + 1} / 5 步。凭证按该支付方式自己的字段渲染，后续加本地支付不用改这个向导结构。
            </DialogDescription>
          </DialogHeader>
          {step === 0 ? (
            <div className="grid gap-3">
              <Label htmlFor="ppi-name">内部名称</Label>
              <Input id="ppi-name" value={name} onChange={(e) => setName(e.target.value)} />
              <Label htmlFor="ppi-mode">环境</Label>
              <select
                id="ppi-mode"
                className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
                value={mode}
                onChange={(e) => setMode(e.target.value)}
              >
                <option value="sandbox">沙箱</option>
                <option value="live">生产</option>
              </select>
            </div>
          ) : null}
          {step === 1 ? (
            <div className="grid max-h-80 gap-3 overflow-y-auto">
              {fields.map((field) => (
                <div key={field.key} className="grid gap-1">
                  <Label htmlFor={field.key}>
                    {field.label}
                    {field.required ? " *" : ""}
                  </Label>
                  <Input
                    id={field.key}
                    type={field.secret ? "password" : "text"}
                    placeholder={field.secret ? "已填写则留空不覆盖" : ""}
                    value={creds[field.key] || ""}
                    onChange={(e) => setCreds((prev) => ({ ...prev, [field.key]: e.target.value }))}
                  />
                </div>
              ))}
            </div>
          ) : null}
          {step === 2 ? (
            <div className="grid gap-2 text-sm">
              <p className="text-ink-secondary">回调只读，系统用商户号识别渠道，不必把渠道 ID 写进 URL。</p>
              <p className="th-eyebrow text-ink-mute">ORIGIN</p>
              <code className="rounded-stamp bg-canvas px-3 py-2">{origin || "—"}</code>
              <p className="th-eyebrow text-ink-mute">PATH</p>
              <code className="rounded-stamp bg-canvas px-3 py-2">{webhookPath}</code>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void navigator.clipboard.writeText(`${origin}${webhookPath}`)}
              >
                复制完整 URL
              </Button>
            </div>
          ) : null}
          {step === 3 ? (
            <p className="text-sm text-ink-secondary">限额可留空，使用收款规则里的默认最低/最高金额。多商户时以后再开负载均衡。</p>
          ) : null}
          {step === 4 ? (
            <p className="text-sm text-ink-secondary">先测连通。失败不能上线。沙箱模式可先给本渠道测试账号用。</p>
          ) : null}
          {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
          <DialogFooter>
            {step > 0 ? (
              <Button variant="outline" onClick={() => setStep((n) => n - 1)}>
                上一步
              </Button>
            ) : null}
            {step === 1 ? (
              <ConfirmButton title="保存凭证" description="密钥加密存储，列表不会回显。" onConfirm={async () => { await saveCredentials(); }}>
                保存凭证
              </ConfirmButton>
            ) : null}
            {step === 4 ? (
              <>
                <Button variant="outline" onClick={() => void testConn()}>
                  测连通
                </Button>
                {mode === "live" ? (
                  <ConfirmButton
                    title="上线生产"
                    description="生产通道会在用户充值页展示。请确认商户号和回调已在支付机构后台配好。"
                    onConfirm={async () => {
                      const ok = await testConn();
                      if (!ok || !instanceID) return;
                      const res = await fetch(`${apiBase}/channel/payments/instances/${instanceID}/go-live`, {
                        method: "POST",
                        credentials: "include",
                        headers: confirmHeaders,
                        body: "{}",
                      });
                      const body = await res.json();
                      setMessage(res.ok ? "已上线" : body.error?.message || "上线失败");
                      if (res.ok) {
                        setOpen(null);
                        await queryClient.invalidateQueries({ queryKey: ["/channel/payments/overview"] });
                      }
                    }}
                  >
                    上线
                  </ConfirmButton>
                ) : (
                  <Button
                    onClick={async () => {
                      const ok = await testConn();
                      if (ok) {
                        setOpen(null);
                        await queryClient.invalidateQueries({ queryKey: ["/channel/payments/overview"] });
                      }
                    }}
                  >
                    保存沙箱
                  </Button>
                )}
              </>
            ) : (
              <Button
                onClick={async () => {
                  if (step === 0 && !instanceID) {
                    const id = await createDraft();
                    if (!id) return;
                  }
                  setStep((n) => Math.min(4, n + 1));
                }}
              >
                下一步
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
