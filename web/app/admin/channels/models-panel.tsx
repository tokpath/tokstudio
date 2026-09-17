"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { canWrite } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";

type ChannelModel = { public_id: string; display_name: string; vendor: string; status: string; enabled: boolean };
type ModelsResponse = { items?: ChannelModel[]; error?: { message?: string } };

export function ChannelModelsPanel({ channelID }: { channelID: string }) {
  const queryClient = useQueryClient();
  const [granting, setGranting] = useState(false);
  const [enabledIDs, setEnabledIDs] = useState<string[]>([]);
  const [message, setMessage] = useState("只能勾选平台目录里已有的模型。不能在租户里新建提供商或模型。");
  const viewer = useViewer();
  const modelsQuery = useQuery({
    queryKey: ["/admin/channels", channelID, "models"],
    queryFn: () => apiClient<ModelsResponse>("GET", `/admin/channels/${channelID}/models`),
  });
  const items = modelsQuery.data?.items ?? [];
  const canGrant = canWrite("models.grant", viewer);
  const selected = useMemo(() => new Set(enabledIDs), [enabledIDs]);

  function startGrant() {
    setEnabledIDs(items.filter((item) => item.enabled).map((item) => item.public_id));
    setGranting(true);
  }

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-lg font-medium">平台模型白名单</h3>
        {canGrant && granting ? (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setGranting(false)}>
              取消
            </Button>
            <ConfirmButton
              size="sm"
              title="确认授权平台模型"
              description="只会开关平台目录中已有的模型，不会创建提供商或新模型。"
              onConfirm={async () => {
                    try {
                const res = await fetch(`${apiBase}/admin/channels/${channelID}/models`, {
                  method: "PATCH",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    items: items.map((item) => ({ public_id: item.public_id, enabled: selected.has(item.public_id) })),
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "保存失败");
                  return false;
                }
                setMessage(`已按平台目录更新白名单，共 ${body.items?.length ?? 0} 个模型`);
                setGranting(false);
                await queryClient.invalidateQueries({ queryKey: ["/admin/channels", channelID, "models"] });
                    return true;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
            >
              保存白名单
            </ConfirmButton>
          </div>
        ) : canGrant ? (
          <Button size="sm" onClick={startGrant}>
            从平台目录授权
          </Button>
        ) : null}
      </div>
      <p className="mb-3 text-sm text-ink-secondary">
        租户不能自己添加提供商和模型。这里只展示平台目录，并把已启用模型授权给该渠道。要新增模型请到平台
        <Link href="/admin/models" className="mx-1 text-brand-emphasis no-underline hover:underline">
          模型
        </Link>
        或
        <Link href="/admin/providers" className="mx-1 text-brand-emphasis no-underline hover:underline">
          提供商
        </Link>
        页。
      </p>
      {modelsQuery.data?.error ? (
        <p className="text-sm text-ink-secondary">{modelsQuery.data.error.message}</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-ink-secondary">登录平台管理员后可以看到目录授权。</p>
      ) : (
        <ul className="grid gap-2 text-sm">
          {items.map((model) => {
            const checked = granting ? selected.has(model.public_id) : model.enabled;
            return (
              <li key={model.public_id} className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline py-2">
                <label className="flex min-w-0 items-center gap-2">
                  {granting ? (
                    <input
                      type="checkbox"
                      aria-label={`授权 ${model.public_id}`}
                      checked={checked}
                      onChange={(event) => {
                        setEnabledIDs((current) => {
                          const next = new Set(current);
                          if (event.target.checked) {
                            next.add(model.public_id);
                          } else {
                            next.delete(model.public_id);
                          }
                          return Array.from(next);
                        });
                      }}
                    />
                  ) : null}
                  <span>
                    {model.display_name} <span className="font-mono text-ink-secondary">{model.public_id}</span>
                  </span>
                </label>
                <span className="text-ink-secondary">
                  {model.vendor} · {model.status} · {checked ? "已授权" : "未授权"}
                </span>
              </li>
            );
          })}
        </ul>
      )}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
