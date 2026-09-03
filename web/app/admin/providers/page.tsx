"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import {
  catalogStatusTone,
  formatCredentialRef,
  formatRpm,
  healthTone,
  providerKindLabel,
  statusWord,
} from "@/lib/catalog-admin";

const selectClass =
  "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

const schema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  slug: z.string().trim().min(1, "请填写标识"),
  adapter: z.string().trim().min(1, "请填写适配器"),
  kind: z.enum(["direct", "aggregator"]),
});

const rotateSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写或点选提供商"),
  secret: z.string().min(1, "请填写上游凭据"),
});

const addAccountSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写或点选提供商"),
  label: z.string().trim().min(1, "请填写标签"),
  secret: z.string().min(1, "请填写账号密文"),
});

const patchSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写或点选提供商"),
  status: z.string().trim().min(1, "请选择状态"),
  rpm_limit: z.string().trim(),
});

type Provider = {
  id: string;
  name: string;
  slug: string;
  kind?: string;
  adapter: string;
  health: string;
  status: string;
  rpm_limit?: number;
  credential_ref?: string;
};

function ProbeCell({ id }: { id: string }) {
  const queryClient = useQueryClient();
  const [result, setResult] = useState("");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <IfCan action="providers.health">
      <Button
        size="sm"
        variant="outline"
        onClick={async (event) => {
          event.stopPropagation();
          const res = await fetch(`${apiBase}/admin/providers/${id}/health-check`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json" },
            body: "{}",
          });
          const body = await res.json();
          setResult(res.ok ? String(body.health ?? "ok") : body.error?.message || "探测失败");
          await queryClient.invalidateQueries();
        }}
      >
        探测
      </Button>
      </IfCan>
      {result ? <span className="text-xs text-ink-secondary">{result}</span> : null}
    </div>
  );
}

function RotateCredentialForm({ selectedID }: { selectedID: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("轮换需要二次确认头。响应和列表只回 credential_ref，不会回显明文。");
  const form = useForm<z.infer<typeof rotateSchema>>({
    resolver: zodResolver(rotateSchema),
    defaultValues: { provider_id: "", secret: "" },
  });

  useEffect(() => {
    if (selectedID) {
      form.setValue("provider_id", selectedID, { shouldValidate: false });
    }
  }, [selectedID, form]);

  return (
    <IfCan action="providers.write">
    <Form {...form}>
      <form className="mt-4 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
        <AdminH2 k="rotateCreds" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">旧密文立即标记 rotated。不要对生产主 Provider 随便试，先建一次性提供商。点上方列表选用后会填入提供商 ID。</p>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <TextField control={form.control} name="provider_id" label="轮换用提供商 ID" placeholder="点列表选用，或填写 provider id" showLabel={false} className="w-72" />
          <TextField control={form.control} name="secret" label="上游凭据" placeholder="轮换用密文" type="password" autoComplete="new-password" showLabel={false} className="w-72" />
          <ConfirmButton
            size="sm"
            title="确认轮换凭据"
            description="旧密文立即标记 rotated。响应不会回显明文。"
            validate={() => form.trigger()}
            onConfirm={form.handleSubmit(async (values) => {
              const res = await fetch(`${apiBase}/admin/providers/${values.provider_id}/credentials`, {
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
              form.reset({ provider_id: values.provider_id, secret: "" });
              setMessage(`已轮换，credential_ref=${body.credential_ref || ""}`);
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

type Account = { id: string; label: string; fingerprint: string; status: string; kind: string };

function AccountPoolPanel({ selectedID }: { selectedID: string }) {
  const queryClient = useQueryClient();
  const [providerID, setProviderID] = useState("");
  const [items, setItems] = useState<Account[]>([]);
  const [message, setMessage] = useState("列表只显示指纹，不回密文。冷却或停用后不会被路由选中。");
  const addForm = useForm<z.infer<typeof addAccountSchema>>({
    resolver: zodResolver(addAccountSchema),
    defaultValues: { provider_id: "", label: "primary", secret: "" },
  });

  async function load(id = providerID) {
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
    if (!selectedID) {
      return;
    }
    setProviderID(selectedID);
    addForm.setValue("provider_id", selectedID, { shouldValidate: false });
    void load(selectedID);
    // load is local to this render; selectedID is the trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedID]);

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
    <section className="mt-4 rounded-card border border-hairline bg-canvas-raised  p-4">
      <AdminH2 k="accountPool" className="mb-4 text-lg font-semibold tracking-tight" />
      <p className="mb-3 text-sm text-ink-secondary">一家提供商可以挂多把上游 Key。列表只显示指纹，不回密文。冷却或停用后不会被路由选中。</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Input className="w-72" value={providerID} onChange={(e) => setProviderID(e.target.value)} aria-label="账号池 provider id" placeholder="点列表选用，或填写提供商 ID" />
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
              <th className="th-eyebrow px-2 py-2 font-medium">状态</th>
              <th className="th-eyebrow px-2 py-2 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2">{item.label}</td>
                <td className="px-2 py-2 font-mono text-[13px]">{item.fingerprint}</td>
                <td className="px-2 py-2">
                  <Badge tone={catalogStatusTone(item.status)}>{statusWord(item.status)}</Badge>
                </td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    <IfCan action="providers.write">
                    <ConfirmButton size="sm" variant="outline" title="确认冷却账号" description="冷却后该账号不会被路由选中。" onConfirm={() => patch(item.id, { cooldown_seconds: 120 }, "已冷却")}>
                      冷却
                    </ConfirmButton>
                    <ConfirmButton size="sm" variant="outline" title="确认停用账号" description="停用后该账号不会被路由选中。" onConfirm={() => patch(item.id, { status: "disabled" }, "已停用")}>
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
      <Form {...addForm}>
        <form className="grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
          <TextField control={addForm.control} name="provider_id" label="添加账号用提供商 ID" />
          <TextField control={addForm.control} name="label" label="账号标签" placeholder="label" />
          <TextField control={addForm.control} name="secret" label="账号密文" type="password" autoComplete="new-password" />
          <IfCan action="providers.write">
          <ConfirmButton
            size="sm"
            title="确认添加账号"
            description="列表只显示指纹，不会回显密文。"
            validate={() => addForm.trigger()}
            onConfirm={addForm.handleSubmit(async (values) => {
              setProviderID(values.provider_id);
              const res = await fetch(`${apiBase}/admin/providers/${values.provider_id}/accounts`, {
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
              addForm.reset({ provider_id: values.provider_id, label: "primary", secret: "" });
              await load(values.provider_id);
              setMessage(`已添加，指纹 ${body.item?.fingerprint || ""}`);
            })}
          >
            添加账号
          </ConfirmButton>
          </IfCan>
        </form>
      </Form>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}

function PatchProviderForm({ selectedID }: { selectedID: string }) {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("改状态和 RPM 都要二次确认。不要改 prd_echo_primary / prd_echo_backup / prd_gemini。");
  const form = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    defaultValues: { provider_id: "", status: "maintenance", rpm_limit: "30" },
  });

  useEffect(() => {
    if (selectedID) {
      form.setValue("provider_id", selectedID, { shouldValidate: false });
    }
  }, [selectedID, form]);

  return (
    <IfCan action="providers.write">
    <Form {...form}>
      <form className="mt-4 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
        <AdminH2 k="editProvider" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">maintenance 会从路由候选里拿掉。RPM 写到 Provider 行，不是账号池单条账号。点上方列表选用后会填入提供商 ID。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <TextField control={form.control} name="provider_id" label="改状态用提供商 ID" />
          <FormField
            control={form.control}
            name="status"
            render={({ field }) => (
              <FormItem>
                <FormLabel>状态</FormLabel>
                <FormControl>
                  <select className={selectClass} aria-label="改状态用状态" {...field}>
                    <option value="active">active · 参与路由</option>
                    <option value="maintenance">maintenance · 从路由拿掉</option>
                    <option value="disabled">disabled · 停用</option>
                  </select>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <TextField control={form.control} name="rpm_limit" label="RPM 上限" placeholder="0 表示未限制" />
        </div>
        <ConfirmButton
          size="sm"
          title="确认保存 Provider"
          description="不要改 prd_echo_primary / prd_echo_backup / prd_gemini。"
          validate={() => form.trigger()}
          onConfirm={form.handleSubmit(async (values) => {
            const payload: Record<string, unknown> = { status: values.status };
            if (values.rpm_limit) {
              payload.rpm_limit = Number(values.rpm_limit);
            }
            const res = await fetch(`${apiBase}/admin/providers/${values.provider_id}`, {
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
            setMessage(`已保存 ${body.item?.id} → ${body.item?.status} / RPM ${body.item?.rpm_limit ?? 0}`);
            await queryClient.invalidateQueries();
          })}
        >
          保存 Provider
        </ConfirmButton>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </form>
    </Form>
    </IfCan>
  );
}

export default function AdminProvidersPage() {
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", slug: "", adapter: "test", kind: "direct" },
  });
  const [createMessage, setCreateMessage] = useState("创建 Provider 需要二次确认头，密钥不会回显。");
  const [selected, setSelected] = useState<Provider | null>(null);

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="text-sm text-ink-secondary">
          提供商是上游进货渠道，不是客户看到的模型名。直连走官方或兼容协议；聚合走 OpenRouter 这类中转，禁止回流 TokenHub。
          同一公开模型可以挂多家提供商。提供商只由平台接入。租户不能自带上游 Key，也不能在渠道里新建提供商。列表每行可探测；探测走上游沙箱、不会计费，也不要二次确认。点一行即可选用，下面的轮换、改状态、账号池会填入该提供商。
        </p>
      </section>
      <AdminListPanel<Provider>
        path="/admin/providers"
        title="提供商"
        emptyTitle="还没有提供商"
        emptyDetail="由平台接入上游。不要在渠道里自建。"
        onRowSelect={setSelected}
        rowSelected={(row) => row.id === selected?.id}
        columns={[
          { accessorKey: "name", header: "名称" },
          {
            accessorKey: "slug",
            header: "标识",
            cell: ({ row }) => <span className="font-mono text-[13px]">{row.original.slug}</span>,
          },
          {
            accessorKey: "kind",
            header: "类型",
            cell: ({ row }) => providerKindLabel(row.original.kind),
          },
          { accessorKey: "adapter", header: "适配器" },
          {
            accessorKey: "health",
            header: "健康",
            cell: ({ row }) => <Badge tone={healthTone(row.original.health)}>{statusWord(row.original.health)}</Badge>,
          },
          {
            accessorKey: "status",
            header: "状态",
            cell: ({ row }) => <Badge tone={catalogStatusTone(row.original.status)}>{statusWord(row.original.status)}</Badge>,
          },
          {
            accessorKey: "rpm_limit",
            header: "RPM",
            cell: ({ row }) => formatRpm(row.original.rpm_limit),
          },
          {
            accessorKey: "credential_ref",
            header: "凭据",
            cell: ({ row }) => (
              <span className="font-mono text-[13px] text-ink-secondary">{formatCredentialRef(row.original.credential_ref)}</span>
            ),
          },
          {
            id: "probe",
            header: "探测",
            cell: ({ row }) => <ProbeCell id={String(row.original.id)} />,
          },
        ]}
      />
      {selected ? (
        <p className="text-sm text-ink-secondary">
          已选 <span className="font-medium text-ink">{selected.name || selected.slug}</span>
          {" · "}
          <span className="font-mono">{selected.slug}</span>
          {" · "}
          {providerKindLabel(selected.kind)}
          {" · "}
          <span className="font-mono">{selected.id}</span>
        </p>
      ) : (
        <p className="text-sm text-ink-secondary">点列表中的一行来选用提供商，不必手抄内部 ID。</p>
      )}
      <RotateCredentialForm selectedID={selected?.id || ""} />
      <IfCan action="providers.write">
      <Form {...form}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-card border border-hairline bg-canvas-raised  p-4" onSubmit={(event) => event.preventDefault()}>
          <AdminH2 k="createProvider" className="text-lg font-semibold tracking-tight" />
          <p className="text-sm text-ink-secondary">创建 Provider 需要二次确认头，密钥不会回显。类型决定它是直连还是聚合进货渠道。</p>
          <TextField control={form.control} name="name" label="名称" />
          <TextField control={form.control} name="slug" label="标识 slug" />
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
          <TextField control={form.control} name="adapter" label="适配器" />
          <ConfirmButton
            size="sm"
            variant="outline"
            title="确认创建 Provider"
            description="密钥不会回显。不要改种子提供商。"
            validate={() => form.trigger()}
            onConfirm={form.handleSubmit(async (values) => {
              await apiClient("POST", "/admin/providers", {
                headers: confirmHeaders,
                body: JSON.stringify(values),
              });
              form.reset({ name: "", slug: "", adapter: "test", kind: "direct" });
              setCreateMessage(`已创建 ${values.slug}`);
            })}
          >
            创建
          </ConfirmButton>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      </IfCan>
      <PatchProviderForm selectedID={selected?.id || ""} />
      <AccountPoolPanel selectedID={selected?.id || ""} />
    </AdminShell>
  );
}
