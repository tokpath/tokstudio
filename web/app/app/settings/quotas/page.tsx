import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function QuotasSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="QUOTAS"
        title="用量配额"
        description="RPM、并发和模型白名单写在 API Key 上。这里只汇总当前账户还剩多少额度。"
      />
      <EmptyLedger title="暂无配额记录" detail="没有套餐或赠送时，这里是空的。调用限额请到 API Key 页设置。" />
    </div>
  );
}
