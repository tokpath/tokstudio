import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function BillingSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="BILLING"
        title="开票资料"
        description="发票抬头、税号和邮寄地址只用于对账，不出现在公共站。"
      />
      <EmptyLedger title="暂无开票资料" detail="登录后可填写主体名称与税号。空账本不预填任何个人信息。" />
    </div>
  );
}
