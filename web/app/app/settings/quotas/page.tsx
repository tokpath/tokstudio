import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function QuotasSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="quotas" />
      <EmptyLedger title="暂无配额记录" detail="没有套餐或赠送时，这里是空的。调用限额请到 API Key 页设置。" />
    </div>
  );
}
