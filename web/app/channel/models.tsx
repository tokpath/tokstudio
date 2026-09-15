"use client";

import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { ListResourceView } from "@/components/console/list-resource-view";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";

type ChannelModel = { public_id?: string; display_name?: string; vendor?: string; status?: string; enabled?: boolean };

export default function ChannelModels() {
  const list = useListResource<ChannelModel>({
    load: () => fetchListItems(`${apiBase}/channel/models`),
  });

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">本渠道模型</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">
        所有租户的模型资源都只能从平台目录出发。渠道不能自建提供商或模型，也不能引入目录外的模型。
      </p>
      <Button variant="outline" onClick={() => void list.reload()}>
        刷新模型
      </Button>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle="暂无已授权模型"
        emptyDetail="平台还没有给本渠道授权模型。开通后会出现在这里。"
        onRetry={() => void list.reload()}
      >
        <ul className="mt-4 grid gap-2 text-sm">
          {list.snapshot.items.map((item) => (
            <li key={item.public_id} className="flex flex-wrap justify-between gap-2 border-b border-hairline py-2">
              <span>
                {item.display_name || item.public_id} <span className="font-mono text-ink-secondary">{item.public_id}</span>
              </span>
              <span className="text-ink-secondary">
                {item.vendor} · {item.status} · {item.enabled ? "已授权" : "未授权"}
              </span>
            </li>
          ))}
        </ul>
      </ListResourceView>
    </Card>
  );
}
