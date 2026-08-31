import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function WebhooksSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="WEBHOOKS"
        title="Webhook"
        description="媒体完成、支付到账按 event_id 幂等投递。验签头是 X-Tokenhub-Signature。"
      />
      <EmptyLedger title="暂无 Webhook" detail="还没有配置回调地址。支付适配器自己的 webhook 不在这张表里。" />
    </div>
  );
}
