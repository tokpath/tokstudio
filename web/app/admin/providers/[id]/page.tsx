"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { ProbeCell } from "../probe-cell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { modelEditHref } from "@/lib/catalog";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import {
  PROVIDER_STATUSES,
  adapterLabel,
  protocolOptions,
  catalogStatusTone,
  formatCredentialRef,
  healthLabel,
  healthTone,
  providerKindLabel,
  providerStatusLabel,
  statusWord,
  type MappedPublicModel,
} from "@/lib/catalog-admin";

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  base_url?: string;
  health: string;
  status: string;
  timeout_ms?: number;
  credential_ref?: string;
  models?: MappedPublicModel[];
};

type ItemResponse = { item?: Provider; error?: { message?: string } };
type Account = { id: string; label: string; fingerprint: string; status: string; kind: string };

const patchSchema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  kind: z.enum(["direct", "aggregator"]),
  adapter: z.string().trim().min(1, "请选择协议"),
  base_url: z.string().trim(),
  status: z.string().trim().min(1, "请选择状态"),
  timeout_ms: z.string().trim(),
});

const rotateSchema = z.object({
  secret: z.string().min(1, "请填写上游 API Key"),
});

const addAccountSchema = z.object({
  label: z.string().trim().min(1, "请填写标签"),
  secret: z.string().min(1, "请填写账号密文"),
});

function accountKindLabel(kind?: string): string {
  switch ((kind || "").trim()) {
    case "adapter_secret":
      return "适配器密钥";
    case "api_key":
      return "API Key";
    default:
      return kind?.trim() || "—";
  }
}

export default function AdminProviderDetailPage() {
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const routeID = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("维护中会从路由候选里拿掉。不要改 echo-primary / echo-backup / gemini-flash。");
  const query = useQuery({
    queryKey: ["/admin/providers", routeID],
    queryFn: () => apiClient<ItemResponse>("GET", `/admin/providers/${encodeURIComponent(routeID)}`),
  });
  const item = query.data?.item;
  const providerID = item?.id || "";
  const form = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    values: {
      name: item?.name || "",
      kind: item?.kind === "aggregator" ? "aggregator" : "direct",
      adapter: item?.adapter || "openai",
      base_url: item?.base_url || "",
      status: item?.status || "active",
      timeout_ms: item?.timeout_ms ? String(item.timeout_ms) : "",
    },
  });

  return (
    <AdminShell>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href="/admin/providers" className="text-sm text-brand-emphasis no-underline hover:underline">
            返回列表
          </Link>
          <h2 className="mt-3 text-lg font-semibold tracking-tight">提供商详情</h2>
          <p className="mt-1 text-sm text-ink-secondary">
            {item ? `${item.name} · ${item.slug} · ${providerKindLabel(item.kind)} · ${adapterLabel(item.adapter)}` : routeID}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {item ? <ProbeCell id={item.id} /> : null}
          <IfCan action="providers.write">
            {editing ? (
              <>
                <Button size="sm" variant="outline" onClick={() => setEditing(false)}>
                  取消
                </Button>
                <ConfirmButton
                  size="sm"
                  title="确认保存提供商"
                  description="维护中会从路由拿掉。不要改 echo-primary / echo-backup / gemini-flash。"
                  validate={() => form.trigger()}
                  onConfirm={form.handleSubmit(async (values) => {
                    const payload: Record<string, unknown> = {
                      name: values.name,
                      kind: values.kind,
                      adapter: values.adapter,
                      status: values.status,
                    };
                    if (values.base_url) {
                      payload.base_url = values.base_url;
                    }
                    if (values.timeout_ms) {
                      payload.timeout_ms = Number(values.timeout_ms);
                    }
                    const res = await fetch(`${apiBase}/admin/providers/${providerID}`, {
                      method: "PATCH",
                      credentials: "include",
                      headers: confirmHeaders,
                      body: JSON.stringify(payload),
                    });
                    const body = await res.json();
                    if (!res.ok) {
                      setMessage(body.error?.message || "保存失败");
                      return;
                    }
                    setMessage(`已保存 ${body.item?.slug || body.item?.id} → ${providerStatusLabel(body.item?.status)}`);
                    setEditing(false);
                    await queryClient.invalidateQueries({ queryKey: ["/admin/providers", routeID] });
                  })}
                >
                  保存提供商
                </ConfirmButton>
              </>
            ) : (
              <Button size="sm" onClick={() => setEditing(true)}>
                编辑
              </Button>
            )}
          </IfCan>
        </div>
      </div>
      {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}

      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="editProvider" className="mb-3 text-lg font-semibold tracking-tight" />
        {editing ? (
          <Form {...form}>
            <form className="grid max-w-xl gap-3" onSubmit={(event) => event.preventDefault()}>
              <p className="text-sm text-ink-secondary">
                改状态、协议和上游地址。标识创建后不要改。超时只影响调用这家上游时等多久。
              </p>
              <TextField control={form.control} name="name" label="名称" />
              <FormField
                control={form.control}
                name="kind"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>类型</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="类型" {...field}>
                        <option value="direct">直连 · 官方或兼容协议</option>
                        <option value="aggregator">聚合 · OpenRouter 等，禁止回流 TokenHub</option>
                      </select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="adapter"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>协议</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="协议" {...field}>
                        {protocolOptions(field.value).map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <TextField control={form.control} name="base_url" label="上游地址" placeholder="https://api.openai.com/v1" />
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>状态</FormLabel>
                    <FormControl>
                      <select className={selectClass} aria-label="状态" {...field}>
                        {PROVIDER_STATUSES.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <TextField control={form.control} name="timeout_ms" label="上游超时（毫秒）" placeholder="默认 30000" />
            </form>
          </Form>
        ) : (
          <dl className="grid max-w-3xl gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-ink-secondary">健康</dt>
              <dd className="mt-1">
                <Badge tone={healthTone(item?.health)}>{healthLabel(item?.health)}</Badge>
              </dd>
            </div>
            <div>
              <dt className="text-ink-secondary">状态</dt>
              <dd className="mt-1">
                <Badge tone={catalogStatusTone(item?.status)}>{providerStatusLabel(item?.status)}</Badge>
              </dd>
            </div>
            <div>
              <dt className="text-ink-secondary">凭据</dt>
              <dd className="mt-1">{formatCredentialRef(item?.credential_ref)}</dd>
            </div>
            <div>
              <dt className="text-ink-secondary">上游超时</dt>
              <dd className="mt-1">{item?.timeout_ms ? `${item.timeout_ms} ms` : "默认 30000 ms"}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-ink-secondary">上游地址</dt>
              <dd className="mt-1 font-mono text-[13px]">{item?.base_url || "未填写（沙箱可空）"}</dd>
            </div>
          </dl>
        )}
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>

      <MappedModelsPanel models={item?.models || []} />
      <SyncProviderPanel providerID={providerID} />
      <RotateCredentialForm providerID={providerID} />
      <AccountPoolPanel providerID={providerID} />
    </AdminShell>
  );
}

function MappedModelsPanel({ models }: { models: MappedPublicModel[] }) {
  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="mappedModels" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        一家提供商可关联多个公开模型。左侧为客户看到的公开 ID，右侧为该上游识别的模型名。如需增删关联，请前往模型页操作。
      </p>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="th-eyebrow px-2 py-2 font-medium">公开模型</th>
              <th className="th-eyebrow px-2 py-2 font-medium">厂商</th>
              <th className="th-eyebrow px-2 py-2 font-medium">上游模型名</th>
              <th className="th-eyebrow px-2 py-2 font-medium">映射状态</th>
            </tr>
          </thead>
          <tbody>
            {models.length === 0 ? (
              <tr>
                <td className="px-2 py-3 text-ink-secondary" colSpan={4}>
                  还没有挂模型。创建提供商不会自动带模型，需要在模型页把公开模型挂上来。
                </td>
              </tr>
            ) : (
              models.map((model) => (
                <tr key={`${model.public_id}-${model.upstream_model_id}`} className="border-b border-hairline/80">
                  <td className="px-2 py-2">
                    {model.public_id ? (
                      <Link className="font-mono text-[13px] text-brand-emphasis no-underline hover:underline" href={modelEditHref(model.public_id)}>
                        {model.public_id}
                      </Link>
                    ) : (
                      "—"
                    )}
                    {model.display_name ? <span className="ml-2 text-ink-secondary">{model.display_name}</span> : null}
                  </td>
                  <td className="px-2 py-2">{model.vendor || "—"}</td>
                  <td className="px-2 py-2 font-mono text-[13px]">{model.upstream_model_id || "—"}</td>
                  <td className="px-2 py-2">
                    <Badge tone={catalogStatusTone(model.status)}>{statusWord(model.status)}</Badge>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function SyncProviderPanel({ providerID }: { providerID: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("同步只会生成待审核草稿，不会直接出现在客户目录。");
  return (
    <IfCan action="models.attach">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <AdminH2 k="syncUpstream" className="mb-3 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">从这家上游拉模型清单，结果进入待审核。创建人不能审核或发布自己同步出来的模型。</p>
        <ConfirmButton
          size="sm"
          variant="outline"
          title="确认同步上游"
          description="同步结果只进入 draft，不会自动上架。"
          onConfirm={async () => {
            const res = await fetch(`${apiBase}/admin/providers/${providerID}/sync`, {
              method: "POST",
              credentials: "include",
              headers: confirmHeaders,
              body: "{}",
            });
            const body = await res.json();
            if (!res.ok) {
              setMessage(body.error?.message || "同步失败");
              return;
            }
            const count = Array.isArray(body.item?.items) ? body.item.items.length : 0;
            setMessage(`已同步 ${count} 条草稿`);
            await queryClient.invalidateQueries();
          }}
        >
          同步上游
        </ConfirmButton>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
    </IfCan>
  );
}

function RotateCredentialForm({ providerID }: { providerID: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("旧 Key 立即作废。响应和列表只显示「已配置」，不会回显明文。");
  const form = useForm<z.infer<typeof rotateSchema>>({
    resolver: zodResolver(rotateSchema),
    defaultValues: { secret: "" },
  });

  return (
    <IfCan action="providers.write">
      <Form {...form}>
        <form className="rounded-card border border-hairline bg-canvas-raised p-6" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="rotateCreds" className="mb-3 text-lg font-semibold tracking-tight" />
          <p className="mb-3 text-sm text-ink-secondary">
            凭据轮换用于更换该上游的 API Key。旧密文将立即标记为 rotated，路由改用新 Key。此密钥不同于用户在 TokenHub 控制台使用的 API Key。请勿在生产主提供商上随意测试。
          </p>
          <div className="mb-3 flex max-w-xl flex-wrap items-end gap-2">
            <TextField control={form.control} name="secret" label="新的上游 API Key" placeholder="不会回显明文" type="password" autoComplete="new-password" />
            <ConfirmButton
              size="sm"
              title="确认轮换凭据"
              description="旧密文立即标记 rotated。响应不会回显明文。"
              validate={() => form.trigger()}
              onConfirm={form.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/providers/${providerID}/credentials`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ secret: values.secret }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "轮换失败");
                  return;
                }
                form.reset({ secret: "" });
                setMessage("已轮换，凭据状态变为已配置。");
                await queryClient.invalidateQueries();
              })}
            >
              轮换凭据
            </ConfirmButton>
          </div>
          <p className="text-sm text-ink-secondary">{message}</p>
        </form>
      </Form>
    </IfCan>
  );
}

function AccountPoolPanel({ providerID }: { providerID: string }) {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<Account[]>([]);
  const [message, setMessage] = useState("列表只显示指纹，不回密文。冷却或停用后不会被路由选中。");
  const addForm = useForm<z.infer<typeof addAccountSchema>>({
    resolver: zodResolver(addAccountSchema),
    defaultValues: { label: "primary", secret: "" },
  });

  async function load(id = providerID) {
    if (!id) {
      return;
    }
    const res = await fetch(`${apiBase}/admin/providers/${id}/accounts`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "读取账号失败");
      setItems([]);
      return;
    }
    const raw = JSON.stringify(body);
    if (raw.includes("ciphertext") || raw.includes("\"secret\"")) {
      setMessage("账号列表泄漏了密文，已拒绝展示");
      setItems([]);
      return;
    }
    setItems(body.items || []);
    setMessage(`已读取 ${body.items?.length ?? 0} 条，只含指纹`);
  }

  useEffect(() => {
    if (!providerID) {
      return;
    }
    void load(providerID);
    // load is local to this render; providerID is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerID]);

  async function patch(accountID: string, payload: Record<string, unknown>, okText: string) {
    const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts/${accountID}`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify(payload),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "更新失败");
      return;
    }
    await load();
    setMessage(`${okText} ${body.item?.fingerprint || accountID} → ${body.item?.status}`);
    await queryClient.invalidateQueries();
  }

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="accountPool" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        一家提供商可以挂多把上游 Key，路由会挑还能用的那把。列表只显示指纹，不回密文。
      </p>
      <div className="mb-3">
        <Button size="sm" variant="outline" onClick={() => load()}>
          读取账号
        </Button>
      </div>
      <div className="mb-3 overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="th-eyebrow px-2 py-2 font-medium">标签</th>
              <th className="th-eyebrow px-2 py-2 font-medium">指纹</th>
              <th className="th-eyebrow px-2 py-2 font-medium">种类</th>
              <th className="th-eyebrow px-2 py-2 font-medium">状态</th>
              <th className="th-eyebrow px-2 py-2 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <tr key={row.id} className="border-b border-hairline/80">
                <td className="px-2 py-2">{row.label}</td>
                <td className="px-2 py-2 font-mono text-[13px]">{row.fingerprint}</td>
                <td className="px-2 py-2">{accountKindLabel(row.kind)}</td>
                <td className="px-2 py-2">
                  <Badge tone={catalogStatusTone(row.status)}>{statusWord(row.status)}</Badge>
                </td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    <IfCan action="providers.write">
                      <ConfirmButton size="sm" variant="outline" title="确认冷却账号" description="冷却后该账号不会被路由选中。" onConfirm={() => patch(row.id, { cooldown_seconds: 120 }, "已冷却")}>
                        冷却
                      </ConfirmButton>
                      <ConfirmButton size="sm" variant="outline" title="确认停用账号" description="停用后该账号不会被路由选中。" onConfirm={() => patch(row.id, { status: "disabled" }, "已停用")}>
                        停用
                      </ConfirmButton>
                    </IfCan>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <IfCan action="providers.write">
        <Form {...addForm}>
          <form className="grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
            <TextField control={addForm.control} name="label" label="账号标签" placeholder="primary" />
            <TextField control={addForm.control} name="secret" label="账号密文" type="password" autoComplete="new-password" />
            <ConfirmButton
              size="sm"
              title="确认添加账号"
              description="列表只显示指纹，不会回显密文。"
              validate={() => addForm.trigger()}
              onConfirm={addForm.handleSubmit(async (values) => {
                const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ secret: values.secret, label: values.label, kind: "api_key" }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "添加失败");
                  return;
                }
                if (body.item?.secret || body.item?.ciphertext) {
                  setMessage("添加响应泄漏了密文");
                  return;
                }
                addForm.reset({ label: "primary", secret: "" });
                await load(providerID);
                setMessage(`已添加，指纹 ${body.item?.fingerprint || ""}`);
              })}
            >
              添加账号
            </ConfirmButton>
          </form>
        </Form>
      </IfCan>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
