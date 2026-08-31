import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function TeamSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="team" />
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
