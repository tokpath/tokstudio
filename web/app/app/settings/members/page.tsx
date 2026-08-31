import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function MembersSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="members" />
      <EmptyLedger
        title="暂无其他成员"
        detail="目前只有你自己。邀请发送后会出现在这张表里。"
        action={
          <Button type="button" variant="outline" disabled>
            邀请成员（登录后）
          </Button>
        }
      />
    </div>
  );
}
