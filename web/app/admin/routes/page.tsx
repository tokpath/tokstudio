"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { AdminSelectField } from "@/components/admin-select-field";
import { ConfirmButton } from "@/components/confirm-button";
import { ProviderSlugCombobox } from "@/components/provider-slug-combobox";
import { PublicModelCombobox } from "@/components/public-model-combobox";
import { Form } from "@/components/ui/form";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { ScrollTable } from "@/components/ui/scroll-table";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { IfCan } from "@/components/rbac/if-can";
import { routeStatusLabel, routeStatusOptions, routeStrategyLabel, routeStrategyOptions } from "@/lib/catalog-admin";
import { CATALOG_HELP, CATALOG_LABEL } from "@/lib/catalog-copy";
import { type AdminModel, vendorLabel } from "@/lib/catalog";

type RouteCandidate = { provider_id?: string; provider_slug?: string; priority?: number; weight?: number };
type Route = {
  id: string;
  public_model_id: string;
  vendor?: string;
  strategy: string;
  status: string;
  candidates?: RouteCandidate[];
};

const createSchema = z.object({
  public_model_id: z.string().trim().min(1, "请选择公开模型标识"),
  strategy: z.string().trim().min(1, "请选择选路策略"),
  status: z.string().trim().min(1, "请选择路由状态"),
  provider_id: z.string().trim(),
});

const patchSchema = z.object({
  route_id: z.string().trim().min(1, "请填写路由 ID"),
  strategy: z.string().trim(),
  status: z.string().trim(),
});

function vendorOf(route: Route): string {
  if (route.vendor?.trim()) {
    return route.vendor.trim();
  }
  const id = route.public_model_id || "";
  const i = id.indexOf("/");
  return i > 0 ? id.slice(0, i) : "unknown";
}

function formatCandidates(candidates?: RouteCandidate[]): string {
  if (!candidates?.length) {
    return "—";
  }
  return [...candidates]
    .sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0))
    .map((c) => c.provider_slug || c.provider_id || "?")
    .join(" → ");
}

export default function AdminRoutesPage() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("创建与修改策略均需二次确认。请勿修改 rg_echo，该路由为文本网关默认路由。");
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { public_model_id: "", strategy: "priority", status: "active", provider_id: "" },
  });
  const patchForm = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    defaultValues: { route_id: "", strategy: "priority", status: "active" },
  });
  const query = useQuery({
    queryKey: ["/admin/routes"],
    queryFn: () => apiClient<{ items?: Route[]; error?: { message?: string } }>("GET", "/admin/routes"),
  });
  const providersQuery = useQuery({
    queryKey: ["/admin/providers"],
    queryFn: () => apiClient<{ items?: { id: string; name: string; slug: string }[] }>("GET", "/admin/providers"),
  });
  const providerOptions = providersQuery.data?.items ?? [];
  const modelsQuery = useQuery({
    queryKey: ["/admin/models"],
    queryFn: () => apiClient<{ items?: AdminModel[] }>("GET", "/admin/models"),
  });
  const modelOptions = modelsQuery.data?.items ?? [];
  const grouped = useMemo(() => {
    const items = query.data?.items ?? [];
    const map = new Map<string, Route[]>();
    for (const route of items) {
      const vendor = vendorOf(route);
      const list = map.get(vendor) ?? [];
      list.push(route);
      map.set(vendor, list);
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [query.data?.items]);

  function openEdit(route: Route) {
    patchForm.reset({ route_id: route.id, strategy: route.strategy, status: route.status });
    setEditOpen(true);
  }

  const columns = [
    { id: "id", header: "ID", cell: (route: Route) => <span className="font-mono text-[13px]">{route.id}</span> },
    {
      id: "model",
      header: CATALOG_LABEL.publicModelId,
      cell: (route: Route) => {
        const model = modelOptions.find((item) => item.id === route.public_model_id);
        return (
          <div>
            <p>{model?.display_name || route.public_model_id}</p>
            {model?.display_name ? (
              <p className="font-mono text-[12px] text-ink-secondary">{route.public_model_id}</p>
            ) : null}
          </div>
        );
      },
    },
    { id: "vendor", header: CATALOG_LABEL.vendor, cell: (route: Route) => vendorLabel(vendorOf(route)) },
    { id: "strategy", header: CATALOG_LABEL.routeStrategy, cell: (route: Route) => routeStrategyLabel(route.strategy) },
    {
      id: "candidates",
      header: CATALOG_LABEL.providerPool,
      cell: (route: Route) => (
        <span className="font-mono text-[12px] text-ink-secondary">{formatCandidates(route.candidates)}</span>
      ),
    },
    { id: "status", header: CATALOG_LABEL.routeStatus, cell: (route: Route) => routeStatusLabel(route.status) },
    {
      id: "actions",
      header: "操作",
      cell: (route: Route) => (
        <IfCan action="routes.write">
          <Button
            size="sm"
            variant="outline"
            onClick={(event) => {
              event.stopPropagation();
              openEdit(route);
            }}
          >
            改策略
          </Button>
        </IfCan>
      ),
    },
  ];

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">路由组</h2>
            <p className="mt-1 text-sm text-ink-secondary">{CATALOG_HELP.routes}</p>
          </div>
          <IfCan action="routes.write">
            <div className="flex shrink-0 items-center gap-2">
              <Button size="sm" onClick={() => setCreateOpen(true)}>
                创建路由
              </Button>
            </div>
          </IfCan>
        </div>
        {query.data?.error ? <p className="mb-4 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <div className="space-y-6">
          {grouped.length === 0 ? (
            <ScrollTable
              columns={columns}
              rows={[]}
              getRowId={(route) => route.id}
              empty={
                <EmptyState
                  title={query.data?.error ? "暂时看不到路由组" : "还没有路由组"}
                  detail={query.data?.error ? "登录平台管理员后可以看到数据。" : "给已有公开模型创建路由后会出现在这里。"}
                />
              }
            />
          ) : (
            grouped.map(([vendor, routes]) => (
              <div key={vendor}>
                <h3 className="mb-2 text-sm font-semibold tracking-tight text-ink">
                  {CATALOG_LABEL.vendor} {vendorLabel(vendor)}
                </h3>
                <ScrollTable columns={columns} rows={routes} getRowId={(route) => route.id} />
              </div>
            ))
          )}
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建路由</DialogTitle>
            <DialogDescription>选择已有公开模型和提供商。上游模型标识在模型页填写，这里只排提供商池。</DialogDescription>
          </DialogHeader>
          <Form {...createForm}>
            <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
              <PublicModelCombobox control={createForm.control} name="public_model_id" options={modelOptions} />
              <AdminSelectField
                control={createForm.control}
                name="strategy"
                label={CATALOG_LABEL.routeStrategy}
                options={routeStrategyOptions(createForm.watch("strategy"))}
              />
              <AdminSelectField
                control={createForm.control}
                name="status"
                label={CATALOG_LABEL.routeStatus}
                options={routeStatusOptions(createForm.watch("status"))}
              />
              <ProviderSlugCombobox
                control={createForm.control}
                name="provider_id"
                label={CATALOG_LABEL.provider}
                options={providerOptions}
                placeholder="可选，写入提供商池第一家"
              />
              <ConfirmButton
                size="sm"
                title="确认创建路由"
                description="请勿修改 rg_echo。选路策略只决定走哪家提供商、谁优先。"
                validate={() => createForm.trigger()}
                onConfirm={createForm.handleSubmit(async (values) => {
                  const body: Record<string, unknown> = {
                    public_model_id: values.public_model_id,
                    strategy: values.strategy || "priority",
                    status: values.status || "active",
                  };
                  if (values.provider_id) {
                    body.candidates = [{ provider_id: values.provider_id, priority: 1, weight: 1 }];
                  }
                  const res = await fetch(`${apiBase}/admin/routes`, {
                    method: "POST",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify(body),
                  });
                  const json = await res.json();
                  if (!res.ok) {
                    setMessage(json.error?.message || "创建失败");
                    return;
                  }
                  createForm.reset();
                  setMessage(`已创建 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
                  setCreateOpen(false);
                  await queryClient.invalidateQueries();
                })}
              >
                创建路由
              </ConfirmButton>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>改选路策略</DialogTitle>
            <DialogDescription>只改选路策略或路由状态。请勿修改 rg_echo，否则会影响默认选路。</DialogDescription>
          </DialogHeader>
          <Form {...patchForm}>
            <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
              <p className="text-sm text-ink-secondary">
                路由 ID <span className="font-mono">{patchForm.watch("route_id") || "请从列表点「改策略」"}</span>
              </p>
              <AdminSelectField
                control={patchForm.control}
                name="strategy"
                label={CATALOG_LABEL.routeStrategy}
                options={routeStrategyOptions(patchForm.watch("strategy"))}
              />
              <AdminSelectField
                control={patchForm.control}
                name="status"
                label={CATALOG_LABEL.routeStatus}
                options={routeStatusOptions(patchForm.watch("status"))}
              />
              <ConfirmButton
                size="sm"
                title="确认保存策略"
                description="请勿修改 rg_echo。"
                validate={() => patchForm.trigger()}
                onConfirm={patchForm.handleSubmit(async (values) => {
                  const res = await fetch(`${apiBase}/admin/routes/${values.route_id}`, {
                    method: "PATCH",
                    credentials: "include",
                    headers: confirmHeaders,
                    body: JSON.stringify({ strategy: values.strategy, status: values.status }),
                  });
                  const json = await res.json();
                  if (!res.ok) {
                    setMessage(json.error?.message || "保存失败");
                    return;
                  }
                  setMessage(`已保存 ${json.item?.id} → ${json.item?.strategy} / ${json.item?.status}`);
                  setEditOpen(false);
                  await queryClient.invalidateQueries();
                })}
              >
                保存策略
              </ConfirmButton>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </AdminShell>
  );
}
