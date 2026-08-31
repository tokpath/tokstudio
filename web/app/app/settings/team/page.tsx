import { Button } from "@/components/ui/button";
import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function TeamSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="TEAM"
        title="团队"
        description="一个账户对应一个结算主体。改名、开票资料在这里维护，不把别人的团队名写进产品文案。"
      />
      <EmptyLedger
        title="默认团队"
        detail="登录后显示你的团队名称与结算主体。未登录时保持占位，避免把环境里的个人信息写进页面。"
        action={
          <Button type="button" variant="outline" disabled>
            重命名（登录后）
          </Button>
        }
      />
    </div>
  );
}
