"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";

const schema = z.object({
  name: z.string().trim().min(1, "请填写名称"),
  slug: z.string().trim().min(1, "请填写 slug"),
  adapter: z.string().trim().min(1, "请填写 adapter"),
});

const rotateSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写 provider id"),
  secret: z.string().min(1, "请填写上游凭据"),
});

const addAccountSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写 provider id"),
  label: z.string().trim().min(1, "请填写标签"),
  secret: z.string().min(1, "请填写账号密文"),
});

const patchSchema = z.object({
  provider_id: z.string().trim().min(1, "请填写 provider id"),
  status: z.string().trim().min(1, "请填写状态"),
  rpm_limit: z.string().trim(),
});

type Provider = {
  id: string;
  name: string;
  slug: string;
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
      <Button
        size="sm"
        variant="outline"
        onClick={async () => {
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
      {result ? <span className="text-xs text-slate-400">{result}</span> : null}
    </div>
  );
}

function RotateCredentialForm() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("轮换需要二次确认头。响应和列表只回 credential_ref，不会回显明文。");
  const form = useForm<z.infer<typeof rotateSchema>>({
    resolver: zodResolver(rotateSchema),
    defaultValues: { provider_id: "", secret: "" },
  });

  return (
    <Form {...form}>
      <form className="mt-4 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4" onSubmit={(event) => event.preventDefault()}>
        <h2 className="mb-3 text-xl font-medium">凭据轮换</h2>
        <p className="mb-3 text-sm text-slate-400">旧密文立即标记 rotated。不要对生产主 Provider 随便试，先建一次性提供商。</p>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <TextField control={form.control} name="provider_id" label="轮换 provider id" placeholder="轮换用 provider id" showLabel={false} className="w-72" />
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
              form.reset();
              setMessage(`已轮换，credential_ref=${body.credential_ref || ""}`);
              await queryClient.invalidateQueries();
            })}
          >
            轮换凭据
          </ConfirmButton>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </form>
    </Form>
  );
}

type Account = { id: string; label: string; fingerprint: string; status: string; kind: string };

function AccountPoolPanel() {
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
    <section className="mt-4 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4">
      <h2 className="mb-3 text-xl font-medium">账号池</h2>
      <p className="mb-3 text-sm text-slate-400">列表只显示指纹，不回密文。冷却或停用后不会被路由选中。</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Input className="w-72" value={providerID} onChange={(e) => setProviderID(e.target.value)} aria-label="账号池 provider id" placeholder="账号池 provider id" />
        <Button size="sm" variant="outline" onClick={() => load()}>
          读取账号
        </Button>
      </div>
      <div className="mb-3 overflow-x-auto">
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-white/10 text-slate-400">
              <th className="px-2 py-2 font-medium">Label</th>
              <th className="px-2 py-2 font-medium">Fingerprint</th>
              <th className="px-2 py-2 font-medium">Status</th>
              <th className="px-2 py-2 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-white/10/80">
                <td className="px-2 py-2">{item.label}</td>
                <td className="px-2 py-2">{item.fingerprint}</td>
                <td className="px-2 py-2">{item.status}</td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    <ConfirmButton size="sm" variant="outline" title="确认冷却账号" description="冷却后该账号不会被路由选中。" onConfirm={() => patch(item.id, { cooldown_seconds: 120 }, "已冷却")}>
                      冷却
                    </ConfirmButton>
                    <ConfirmButton size="sm" variant="outline" title="确认停用账号" description="停用后该账号不会被路由选中。" onConfirm={() => patch(item.id, { status: "disabled" }, "已停用")}>
                      停用
                    </ConfirmButton>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Form {...addForm}>
        <form className="grid max-w-xl gap-2" onSubmit={(event) => event.preventDefault()}>
          <TextField control={addForm.control} name="provider_id" label="添加账号 provider id" />
          <TextField control={addForm.control} name="label" label="账号标签" placeholder="label" />
          <TextField control={addForm.control} name="secret" label="账号密文" type="password" autoComplete="new-password" />
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
              addForm.reset();
              await load(values.provider_id);
              setMessage(`已添加，指纹 ${body.item?.fingerprint || ""}`);
            })}
          >
            添加账号
          </ConfirmButton>
        </form>
      </Form>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </section>
  );
}

function PatchProviderForm() {
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("改状态和 RPM 都要二次确认。不要改 prd_echo_primary / prd_echo_backup / prd_gemini。");
  const form = useForm<z.infer<typeof patchSchema>>({
    resolver: zodResolver(patchSchema),
    defaultValues: { provider_id: "", status: "maintenance", rpm_limit: "30" },
  });

  return (
    <Form {...form}>
      <form className="mt-4 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4" onSubmit={(event) => event.preventDefault()}>
        <h2 className="mb-3 text-xl font-medium">改 Provider 状态</h2>
        <p className="mb-3 text-sm text-slate-400">maintenance 会从路由候选里拿掉。RPM 写到 Provider 行，不是账号池单条账号。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <TextField control={form.control} name="provider_id" label="改状态用 provider id" />
          <TextField control={form.control} name="status" label="改状态用状态" placeholder="改状态用状态 maintenance" />
          <TextField control={form.control} name="rpm_limit" label="改状态用 RPM" placeholder="改状态用 RPM 30" />
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
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </form>
    </Form>
  );
}

export default function AdminProvidersPage() {
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", slug: "", adapter: "test" },
  });
  const [createMessage, setCreateMessage] = useState("创建 Provider 需要二次确认头，密钥不会回显。");

  return (
    <AdminShell>
      <p className="text-sm text-slate-400">列表每行可探测。探测走上游沙箱、不会计费，也不要二次确认。</p>
      <AdminListPanel<Provider>
        path="/admin/providers"
        title="提供商"
        columns={[
          { accessorKey: "slug", header: "Slug" },
          { accessorKey: "adapter", header: "Adapter" },
          { accessorKey: "health", header: "Health" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "rpm_limit", header: "RPM" },
          { accessorKey: "credential_ref", header: "Cred Ref" },
          {
            id: "probe",
            header: "探测",
            cell: ({ row }) => <ProbeCell id={String(row.original.id)} />,
          },
        ]}
      />
      <RotateCredentialForm />
      <Form {...form}>
        <form className="mt-4 grid max-w-xl gap-2 rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-4" onSubmit={(event) => event.preventDefault()}>
          <p className="text-sm text-slate-400">创建 Provider 需要二次确认头，密钥不会回显。</p>
          <TextField control={form.control} name="name" label="name" />
          <TextField control={form.control} name="slug" label="slug" />
          <TextField control={form.control} name="adapter" label="adapter" />
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
              form.reset({ name: "", slug: "", adapter: "test" });
              setCreateMessage(`已创建 ${values.slug}`);
            })}
          >
            创建
          </ConfirmButton>
          <p className="text-sm text-slate-300">{createMessage}</p>
        </form>
      </Form>
      <PatchProviderForm />
      <AccountPoolPanel />
    </AdminShell>
  );
}
