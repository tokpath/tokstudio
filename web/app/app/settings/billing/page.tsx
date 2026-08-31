import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function BillingSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="billing" />
      <EmptyLedger title="暂无开票资料" detail="登录后可填写主体名称与税号。空账本不预填任何个人信息。" />
    </div>
  );
}
