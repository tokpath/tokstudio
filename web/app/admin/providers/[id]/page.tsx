"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { safeReturnHref } from "@/lib/return-context";
import { useViewer } from "@/components/rbac/viewer-context";
import { useParams, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { AdminShell } from "../../shell";
import { ProbeCell } from "../probe-cell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { modelEditHref, vendorLabel } from "@/lib/catalog";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";
import { CATALOG_LABEL } from "@/lib/catalog-copy";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import {
  PROVIDER_STATUSES,
  adapterLabel,
  protocolOptions,
  catalogStatusTone,
  healthLabel,
  healthTone,
  modelStatusLabel,
  providerKindLabel,
  providerStatusLabel,
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
  health_checked_at?: string;
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
  const t = useTranslations("modelService");
  const viewer = useViewer();
  const searchParams = useSearchParams();
  const originModel = searchParams.get("model");
  const returnTo = searchParams.get("return_to");
  const params = useParams<{ id: string }>();
  const raw = params.id;
  const routeID = decodeURIComponent(Array.isArray(raw) ? raw[0] : raw || "");
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [message, setMessage] = useState("");
  const query = useQuery({
    queryKey: [viewer.userId, "/admin/providers", routeID],
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
      {originModel ? <Link className="text-brand-emphasis underline" href={safeReturnHref(returnTo, modelEditHref(originModel))}>{t("returnModel")}</Link> : null}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href="/admin/providers" className="text-sm text-brand-emphasis no-underline hover:underline">
            返回列表
          </Link>
          <h2 className="mt-3 text-lg font-semibold tracking-tight">提供商详情</h2>
          <p className="mt-1 text-sm text-ink-secondary">
            {item ? `${item.name} · ${providerKindLabel(item.kind)} · ${adapterLabel(item.adapter)}` : routeID}
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
                  description={t("providerHint")}
                  validate={() => form.trigger()}
                  onConfirm={confirmFormSubmit(form.handleSubmit, async (values) => {
                    try {
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
                      return false;
                    }
                    setMessage(`已保存 ${body.item?.slug || body.item?.id} → ${providerStatusLabel(body.item?.status)}`);
                    setEditing(false);
                    await queryClient.invalidateQueries({ queryKey: [viewer.userId, "/admin/providers", routeID] });
                    return true;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
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
                <Badge tone={healthTone(item?.health)}>{item?.health_checked_at && Date.now() - new Date(item.health_checked_at).getTime() <= 86400000 ? healthLabel(item.health) : t("state.unknown")}</Badge>
              </dd>
            </div>
            <div>
              <dt className="text-ink-secondary">状态</dt>
              <dd className="mt-1">
                <Badge tone={catalogStatusTone(item?.status)}>{providerStatusLabel(item?.status)}</Badge>
              </dd>
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

      <AccountPoolPanel providerID={providerID} />
      {providerID ? <ProviderModelsPanel providerID={providerID} /> : null}
      <MappedModelsPanel models={item?.models || []} />
    </AdminShell>
  );
}

type UpstreamModel = {
  upstream_model_id: string;
  display_name: string;
  unit_costs: Record<string, string>;
  price_source: string;
  status: string;
};

function costSummary(costs: Record<string, string>) {
  const parts: string[] = [];
  if (costs.input) parts.push(`输入 ${perTokenToPerMillion(costs.input)} 美元/M`);
  if (costs.output) parts.push(`输出 ${perTokenToPerMillion(costs.output)} 美元/M`);
  if (costs.image_count) parts.push(`图片 ${costs.image_count} 美元/张`);
  if (costs.video_second) parts.push(`视频 ${costs.video_second} 美元/秒`);
  if (costs.audio_second) parts.push(`音频 ${costs.audio_second} 美元/秒`);
  return parts.join(" · ") || "待补价";
}

function ProviderModelsPanel({ providerID }: { providerID: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const [search, setSearch] = useState("");
  const [editingID, setEditingID] = useState<string | null>(null);
  const [modelID, setModelID] = useState("");
  const [input, setInput] = useState("");
  const [output, setOutput] = useState("");
  const [image, setImage] = useState("");
  const [video, setVideo] = useState("");
  const [audio, setAudio] = useState("");
  const queryKey = ["/admin/providers", providerID, "upstream-models"];
  const query = useQuery({
    queryKey,
    queryFn: () => apiClient<{ items?: UpstreamModel[] }>("GET", `/admin/providers/${providerID}/upstream-models`),
  });
  const models = (query.data?.items ?? []).filter((item) =>
    `${item.upstream_model_id} ${item.display_name}`.toLowerCase().includes(search.toLowerCase()));

  function edit(item?: UpstreamModel) {
    setEditingID(item?.upstream_model_id ?? "");
    setModelID(item?.upstream_model_id ?? "");
    setInput(perTokenToPerMillion(item?.unit_costs.input ?? ""));
    setOutput(perTokenToPerMillion(item?.unit_costs.output ?? ""));
    setImage(item?.unit_costs.image_count ?? "");
    setVideo(item?.unit_costs.video_second ?? "");
    setAudio(item?.unit_costs.audio_second ?? "");
    setMessage("");
  }

  async function discover() {
    try {
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models/discover`, {
        method: "POST", credentials: "include", headers: confirmHeaders,
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || "上游不支持模型发现，请人工添加"); return; }
      setMessage(`发现 ${body.items?.length ?? 0} 个上游模型；未返回报价的模型可人工补价。`);
      await queryClient.invalidateQueries({ queryKey });
    } catch { setMessage(confirmNetworkUnavailable); }
  }

  async function save() {
    try {
      const unitCosts = { ...millionDim(input, output) } as Record<string, string>;
      if (image.trim()) unitCosts.image_count = image.trim();
      if (video.trim()) unitCosts.video_second = video.trim();
      if (audio.trim()) unitCosts.audio_second = audio.trim();
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models`, {
        method: "PUT", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ upstream_model_id: modelID.trim(), unit_costs: unitCosts }),
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || "保存成本价失败"); return; }
      setEditingID(null);
      setMessage("已保存上游模型成本价");
      await queryClient.invalidateQueries({ queryKey });
    } catch (err) { setMessage(err instanceof Error ? err.message : confirmNetworkUnavailable); }
  }

  async function setEnabled(item: UpstreamModel, enabled: boolean) {
    try {
      const res = await fetch(`${apiBase}/admin/providers/${providerID}/upstream-models/status`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ upstream_model_id: item.upstream_model_id, enabled }),
      });
      const body = await res.json();
      if (!res.ok) { setMessage(body.error?.message || "更新状态失败"); return false; }
      setMessage(enabled ? "上游模型已启用" : "上游模型已停用，后续请求不会选用它");
      await queryClient.invalidateQueries({ queryKey });
      await queryClient.invalidateQueries({ queryKey: ["/admin/routes"] });
      return true;
    } catch { setMessage(confirmNetworkUnavailable); return false; }
  }

  const fieldClass = "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm";
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6">
    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 className="text-lg font-semibold">支持的模型及价格</h3>
      <IfCan action="providers.write"><div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={discover}>探测上游</Button>
        <Button size="sm" variant="outline" onClick={() => edit()}>人工添加</Button>
      </div></IfCan>
    </div>
    <p className="mb-3 text-sm text-ink-secondary">上游接口有报价则自动读取；没有报价时人工补齐成本价，路由组才能选择。</p>
    <input className={`${fieldClass} mb-3 max-w-sm`} aria-label="搜索上游模型" placeholder="搜索上游模型" value={search} onChange={(event) => setSearch(event.target.value)} />
    <div className="max-h-96 overflow-auto">
      {models.length === 0 ? <p className="py-3 text-sm text-ink-secondary">暂无上游模型。可以探测或人工添加。</p> :
        <table className="min-w-full text-left text-sm"><thead><tr className="border-b border-hairline text-ink-secondary">
          <th className="px-2 py-2">上游模型</th><th className="px-2 py-2">成本价</th><th className="px-2 py-2">状态</th><th className="px-2 py-2">来源</th><th className="px-2 py-2">操作</th>
        </tr></thead><tbody>{models.map((item) => <tr key={item.upstream_model_id} className="border-b border-hairline/80">
          <td className="px-2 py-2"><span className="font-mono">{item.upstream_model_id}</span>{item.display_name && item.display_name !== item.upstream_model_id ? <span className="ml-2 text-ink-secondary">{item.display_name}</span> : null}</td>
          <td className="px-2 py-2 font-mono">{costSummary(item.unit_costs)}</td>
          <td className="px-2 py-2">{item.status === "inactive" ? "已停用" : "已启用"}</td>
          <td className="px-2 py-2">{item.price_source === "manual" ? "人工" : item.price_source === "detected" ? "上游" : "待补价"}</td>
          <td className="px-2 py-2"><IfCan action="providers.write"><div className="flex gap-1"><Button size="sm" variant="ghost" onClick={() => edit(item)}>编辑报价</Button><ConfirmButton size="sm" variant="ghost" title={item.status === "inactive" ? "启用上游模型" : "停用上游模型"} description={item.status === "inactive" ? "启用后可重新参与符合条件的路由。" : "停用后新请求不再选用该上游模型。"} onConfirm={() => setEnabled(item, item.status === "inactive")}>{item.status === "inactive" ? "启用" : "停用"}</ConfirmButton></div></IfCan></td>
        </tr>)}</tbody></table>}
    </div>
    {editingID !== null ? <div className="mt-4 grid max-w-3xl gap-3 rounded-control border border-hairline p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm">上游模型标识（调用时使用）<input className={fieldClass} value={modelID} disabled={!!editingID} onChange={(event) => setModelID(event.target.value)} /></label>
        <label className="text-sm">输入成本（美元/M token）<input className={fieldClass} value={input} onChange={(event) => setInput(event.target.value)} /></label>
        <label className="text-sm">输出成本（美元/M token）<input className={fieldClass} value={output} onChange={(event) => setOutput(event.target.value)} /></label>
        <label className="text-sm">图片成本（美元/张）<input className={fieldClass} value={image} onChange={(event) => setImage(event.target.value)} /></label>
        <label className="text-sm">视频成本（美元/秒）<input className={fieldClass} value={video} onChange={(event) => setVideo(event.target.value)} /></label>
        <label className="text-sm">音频成本（美元/秒）<input className={fieldClass} value={audio} onChange={(event) => setAudio(event.target.value)} /></label>
      </div>
      <div className="flex gap-2"><Button size="sm" onClick={save}>保存报价</Button><Button size="sm" variant="outline" onClick={() => setEditingID(null)}>取消</Button></div>
    </div> : null}
    {message ? <p className="mt-3 text-sm text-ink-secondary">{message}</p> : null}
  </section>;
}

function MappedModelsPanel({ models }: { models: MappedPublicModel[] }) {
  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="mappedModels" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        这里只读展示路由组使用此提供商的模型及上游标识。修改请到路由组。
      </p>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.publicModelId}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.vendor}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">{CATALOG_LABEL.upstreamModelId}</th>
              <th className="th-eyebrow px-2 py-2 font-medium">状态</th>
            </tr>
          </thead>
          <tbody>
            {models.length === 0 ? (
              <tr>
                <td className="px-2 py-3 text-ink-secondary" colSpan={4}>
                  还没有接到公开模型。先发布模型，再到路由组配置。
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
                  <td className="px-2 py-2">{vendorLabel(model.vendor)}</td>
                  <td className="px-2 py-2 font-mono text-[13px]">{model.upstream_model_id || "—"}</td>
                  <td className="px-2 py-2">
                    <Badge tone={catalogStatusTone(model.status)}>{modelStatusLabel(model.status)}</Badge>
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

function AccountPoolPanel({ providerID }: { providerID: string }) {
  const queryClient = useQueryClient();
  const [items, setItems] = useState<Account[]>([]);
  const [message, setMessage] = useState("正在加载账号…");
  const [loading, setLoading] = useState(false);
  const addForm = useForm<z.infer<typeof addAccountSchema>>({
    resolver: zodResolver(addAccountSchema),
    defaultValues: { label: "primary", secret: "" },
  });

  async function load(id = providerID) {
    if (!id) {
      return;
    }
    setLoading(true);
    try {
      const res = await fetch(`${apiBase}/admin/providers/${encodeURIComponent(id)}/accounts`, { credentials: "include" });
      const body = await res.json();
      if (!res.ok) {
        setMessage(body.error?.message || "加载账号失败");
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
      setMessage(`已加载 ${body.items?.length ?? 0} 个账号`);
    } catch {
      setMessage(confirmNetworkUnavailable);
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!providerID) {
      return;
    }
    void load(providerID);
    // load is local to this render; providerID is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [providerID]);

  async function patch(accountID: string, payload: Record<string, unknown>, okText: string): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts/${accountID}`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify(payload),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "更新失败");
      return false;
    }
    await load();
    setMessage(`${okText} ${body.item?.fingerprint || accountID} → ${body.item?.status}`);
    await queryClient.invalidateQueries();
    return true;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <AdminH2 k="accountPool" className="mb-3 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">
        一家提供商可以挂多把上游 Key，路由会挑还能用的那把。列表只显示指纹，不回密文。
      </p>
      <p role="status" className="mb-3 text-sm text-ink-secondary">{message}</p>
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
            {items.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-2 py-3 text-ink-secondary">
                  {loading ? "正在加载账号…" : "暂无账号，请在下方添加。"}
                </td>
              </tr>
            ) : items.map((row) => (
              <tr key={row.id} className="border-b border-hairline/80">
                <td className="px-2 py-2">{row.label}</td>
                <td className="px-2 py-2 font-mono text-[13px]">{row.fingerprint}</td>
                <td className="px-2 py-2">{accountKindLabel(row.kind)}</td>
                <td className="px-2 py-2">
                  <Badge tone={catalogStatusTone(row.status)}>{modelStatusLabel(row.status)}</Badge>
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
              onConfirm={confirmFormSubmit(addForm.handleSubmit, async (values) => {
                try {
                const res = await fetch(`${apiBase}/admin/providers/${providerID}/accounts`, {
                  method: "POST",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({ secret: values.secret, label: values.label, kind: "api_key" }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "添加失败");
                  return false;
                }
                if (body.item?.secret || body.item?.ciphertext) {
                  setMessage("添加响应泄漏了密文");
                  return false;
                }
                addForm.reset({ label: "primary", secret: "" });
                await load(providerID);
                setMessage(`已添加，指纹 ${body.item?.fingerprint || ""}`);
                return true;
                } catch {
                  setMessage(confirmNetworkUnavailable);
                  return false;
                }
})}
            >
              添加账号
            </ConfirmButton>
          </form>
        </Form>
      </IfCan>
    </section>
  );
}
