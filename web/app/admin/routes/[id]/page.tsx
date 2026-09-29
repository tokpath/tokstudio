"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useFieldArray, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { AdminSelectField } from "@/components/admin-select-field";
import { ConfirmButton, confirmFormSubmit } from "@/components/confirm-button";
import { ProviderSlugCombobox } from "@/components/provider-slug-combobox";
import { PublicModelCombobox } from "@/components/public-model-combobox";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { IfCan } from "@/components/rbac/if-can";
import { AdminShell } from "../../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { type AdminModel, modelEditHref } from "@/lib/catalog";
import { routeStrategyOptions, routeStatusOptions } from "@/lib/catalog-admin";
import { perTokenToPerMillion } from "@/lib/token-price";

type Route = {
  id: string; public_model_id: string; strategy: string; status: string;
  candidates?: { provider_id: string; provider_slug: string; upstream_model_id: string; weight: number }[];
};
const schema = z.object({
  public_model_id: z.string().trim().min(1, "请选择模型"),
  strategy: z.enum(["priority", "weight", "price", "health"]),
  status: z.enum(["inactive", "active"]),
  candidates: z.array(z.object({
    provider_id: z.string().trim().min(1, "请选择提供商"),
    upstream_model_id: z.string().trim().min(1, "请填写上游模型标识"),
    weight: z.string().trim().regex(/^\d+$/, "权重须为正整数"),
  })),
}).refine((value) => value.status !== "active" || value.candidates.length > 0, {
  message: "启用前至少配置一个提供商", path: ["status"],
});
type Fields = z.infer<typeof schema>;

type UpstreamModel = { upstream_model_id: string; display_name: string; unit_costs: Record<string, string>; status: string };
function hasCost(costs: Record<string, string>, kind: string) {
  if (kind === "image") return !!costs.image_count;
  if (kind === "video") return !!costs.video_second;
  if (kind === "audio") return !!costs.audio_second;
  if (kind === "embedding") return !!costs.input;
  return !!costs.input && !!costs.output;
}

function UpstreamModelSelect({ control, name, providerID, kind }: {
  control: ReturnType<typeof useForm<Fields>>["control"];
  name: `candidates.${number}.upstream_model_id`;
  providerID: string;
  kind: string;
}) {
  const selected = useWatch({ control, name });
  const query = useQuery({
    queryKey: ["/admin/providers", providerID, "upstream-models"],
    queryFn: () => apiClient<{ items?: UpstreamModel[] }>("GET", `/admin/providers/${encodeURIComponent(providerID)}/upstream-models`),
    enabled: !!providerID,
  });
  const options = (query.data?.items ?? []).filter((item) => item.status !== "inactive" && hasCost(item.unit_costs, kind)).map((item) => ({
    value: item.upstream_model_id,
    label: `${item.display_name && item.display_name !== item.upstream_model_id ? `${item.display_name} · ` : ""}${item.upstream_model_id}${item.unit_costs.input !== undefined ? ` · ${perTokenToPerMillion(item.unit_costs.input)}/${perTokenToPerMillion(item.unit_costs.output ?? "")} 美元/M` : ""}`,
  }));
  const selectedUnavailable = !!selected && !options.some((item) => item.value === selected);
  return <div>
    <AdminSelectField control={control} name={name} label="上游模型" options={[
      { value: "", label: providerID ? "选择已配置成本价的模型" : "先选提供商" },
      ...(selectedUnavailable ? [{ value: selected, label: `${selected}（已停用或未报价）` }] : []),
      ...options,
    ]} />
    {selectedUnavailable && !query.isLoading ? <p className="mt-1 text-sm text-ink-secondary">当前模型不可用，请更换或移除候选。</p> : null}
    {providerID && !query.isLoading && options.length === 0 ? <p className="mt-1 text-sm text-ink-secondary">这家提供商没有已定价的模型，请先到提供商详情补价。</p> : null}
  </div>;
}

export default function AdminRouteEditPage() {
  const params = useParams<{ id: string }>();
  const routeID = Array.isArray(params.id) ? params.id[0] : params.id || "";
  const isNew = routeID === "new";
  const router = useRouter();
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const [loadedID, setLoadedID] = useState("");
  const routeQuery = useQuery({
    queryKey: ["/admin/routes", routeID],
    queryFn: () => apiClient<{ item?: Route; error?: { message?: string } }>("GET", `/admin/routes/${encodeURIComponent(routeID)}`),
    enabled: !!routeID && !isNew,
  });
  const modelsQuery = useQuery({ queryKey: ["/admin/models"], queryFn: () => apiClient<{ items?: AdminModel[] }>("GET", "/admin/models") });
  const routesQuery = useQuery({ queryKey: ["/admin/routes"], queryFn: () => apiClient<{ items?: Route[] }>("GET", "/admin/routes"), enabled: isNew });
  const providersQuery = useQuery({
    queryKey: ["/admin/providers"],
    queryFn: () => apiClient<{ items?: { id: string; slug: string; name: string }[] }>("GET", "/admin/providers"),
  });
  const selectable = useMemo(() => {
    const used = new Set((routesQuery.data?.items ?? []).map((item) => item.public_model_id));
    return (modelsQuery.data?.items ?? []).filter((item) => item.status === "published" && item.config_ready !== false && !used.has(item.id));
  }, [modelsQuery.data?.items, routesQuery.data?.items]);
  const form = useForm<Fields>({
    resolver: zodResolver(schema),
    defaultValues: { public_model_id: "", strategy: "priority", status: "inactive", candidates: [] },
  });
  const candidates = useFieldArray({ control: form.control, name: "candidates" });
  const strategy = form.watch("strategy");
  const status = form.watch("status");
  const chosenModel = form.watch("public_model_id");
  const chosenCandidates = form.watch("candidates");
  const modelKind = (modelsQuery.data?.items ?? []).find((item) => item.id === chosenModel)?.kind || "text";

  useEffect(() => {
    if (!isNew) return;
    const model = new URLSearchParams(window.location.search).get("model");
    if (model) form.setValue("public_model_id", model);
  }, [isNew, form]);
  useEffect(() => {
    const route = routeQuery.data?.item;
    if (!route || route.id === loadedID) return;
    form.reset({
      public_model_id: route.public_model_id,
      strategy: route.strategy as Fields["strategy"], status: route.status as Fields["status"],
      candidates: (route.candidates ?? []).map((item) => ({
        provider_id: item.provider_id || item.provider_slug,
        upstream_model_id: item.upstream_model_id || "",
        weight: String(item.weight || 1),
      })),
    });
    setLoadedID(route.id);
  }, [routeQuery.data?.item, loadedID, form]);

  async function save(values: Fields): Promise<boolean> {
    try {
      const body = {
        ...(isNew ? { public_model_id: values.public_model_id } : {}),
        strategy: values.strategy, status: values.status,
        candidates: values.candidates.map((item, index) => ({
          provider_id: item.provider_id, upstream_model_id: item.upstream_model_id,
          priority: index + 1, weight: Number(item.weight),
        })),
      };
      const res = await fetch(`${apiBase}/admin/routes${isNew ? "" : `/${encodeURIComponent(routeID)}`}`, {
        method: isNew ? "POST" : "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify(body),
      });
      const result = await res.json();
      if (!res.ok) { setMessage(result.error?.message || "保存失败"); return false; }
      setMessage(values.status === "active" ? "路由组已启用" : "路由组已保存");
      await queryClient.invalidateQueries({ queryKey: ["/admin/routes"] });
      if (isNew) router.replace(`/admin/routes/${encodeURIComponent(result.item?.id)}`);
      return true;
    } catch { setMessage(confirmNetworkUnavailable); return false; }
  }

  return <AdminShell>
    <Link className="text-sm text-brand-emphasis hover:underline" href="/admin/routes">返回路由组</Link>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <h2 className="text-lg font-semibold">{isNew ? "创建路由组" : `路由组 ${routeID}`}</h2>
      <p className="mt-1 text-sm text-ink-secondary">在这里把已发布模型接到上游，并决定选路顺序。保存为停用时不会接收请求。</p>
      {routeQuery.data?.error ? <p className="mt-2 text-sm text-danger">{routeQuery.data.error.message}</p> : null}
    </section>
    <IfCan action="routes.write">
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <Form {...form}>
          <form className="grid max-w-3xl gap-5" onSubmit={(event) => event.preventDefault()}>
            {isNew ? <PublicModelCombobox control={form.control} name="public_model_id" options={selectable} /> :
              <p className="text-sm">公开模型：<Link className="font-mono text-brand-emphasis hover:underline" href={modelEditHref(routeQuery.data?.item?.public_model_id || "")}>{routeQuery.data?.item?.public_model_id}</Link></p>}
            <AdminSelectField control={form.control} name="strategy" label="选路策略" options={routeStrategyOptions(strategy)} />
            <div>
              <div className="mb-3 flex items-center justify-between gap-2">
                <h3 className="text-base font-semibold">提供商候选</h3>
                <Button type="button" size="sm" variant="outline" onClick={() => candidates.append({ provider_id: "", upstream_model_id: "", weight: "1" })}>添加提供商</Button>
              </div>
              {candidates.fields.length === 0 ? <p className="text-sm text-ink-secondary">尚未配置。可以先保存停用的路由组。</p> : null}
              <div className="grid gap-3">
                {candidates.fields.map((field, index) => <div key={field.id} className="rounded-control border border-hairline p-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <ProviderSlugCombobox control={form.control} name={`candidates.${index}.provider_id`} label="提供商" options={providersQuery.data?.items ?? []} />
                    <UpstreamModelSelect control={form.control} name={`candidates.${index}.upstream_model_id`}
                      providerID={(providersQuery.data?.items ?? []).find((item) => item.slug === chosenCandidates[index]?.provider_id || item.id === chosenCandidates[index]?.provider_id)?.id || ""}
                      kind={modelKind} />
                    {strategy === "weight" ? <TextField control={form.control} name={`candidates.${index}.weight`} label="权重" /> : null}
                  </div>
                  <div className="mt-3 flex gap-2">
                    <Button type="button" size="sm" variant="outline" disabled={index === 0} onClick={() => candidates.swap(index, index - 1)}>上移</Button>
                    <Button type="button" size="sm" variant="outline" disabled={index === candidates.fields.length - 1} onClick={() => candidates.swap(index, index + 1)}>下移</Button>
                    <Button type="button" size="sm" variant="ghost" onClick={() => candidates.remove(index)}>移除</Button>
                  </div>
                </div>)}
              </div>
            </div>
            <AdminSelectField control={form.control} name="status" label="状态" options={routeStatusOptions(status)} />
            <ConfirmButton
              size="sm" title={status === "active" ? "确认启用路由组" : "确认保存路由组"}
              description={`${form.watch("public_model_id")}：${status === "active" ? "启用后新请求会按此候选池选路。" : "保存为停用，不会接收请求。"}`}
              error={message} validate={() => { setMessage(""); return form.trigger(); }}
              onConfirm={confirmFormSubmit(form.handleSubmit, save)}
            >{isNew ? "创建路由组" : "保存路由组"}</ConfirmButton>
            {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
          </form>
        </Form>
      </section>
    </IfCan>
  </AdminShell>;
}
