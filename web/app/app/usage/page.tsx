import { ConsolePageHeader } from "@/components/console/page-header";
import UsagePanel from "../usage-panel";

export default function UsagePage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="ANALYTICS"
        title="用量与账单"
        description="只展示当前登录用户的 usage 和账本，不含其他渠道数据。金额单位是 micro-USD。"
      />
      <UsagePanel />
    </div>
  );
}
