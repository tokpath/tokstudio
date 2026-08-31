import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ConnectedAppsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="apps" />
      <EmptyLedger title="暂无已连接应用" detail="还没有第三方客户端拿过你的授权。" />
    </div>
  );
}
