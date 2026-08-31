import { Button } from "@/components/ui/button";
import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";

export default function MembersSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="MEMBERS"
        title="成员"
        description="成员共享同一本账和 Key 策略。邀请走邮箱，不在页面上切换渠道。"
      />
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
