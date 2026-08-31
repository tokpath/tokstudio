import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function WebhooksSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="webhooks" />
      <EmptyLedger title="暂无 Webhook" detail="还没有配置回调地址。支付适配器自己的 webhook 不在这张表里。" />
    </div>
  );
}
