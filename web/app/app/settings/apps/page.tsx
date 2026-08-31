import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function ConnectedAppsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="APPS"
        title="已连接应用"
        description="OAuth 应用会列出名称和授权范围。撤销后立即失效，审计里留一条。"
      />
      <EmptyLedger title="暂无已连接应用" detail="还没有第三方客户端拿过你的授权。" />
    </div>
  );
}
