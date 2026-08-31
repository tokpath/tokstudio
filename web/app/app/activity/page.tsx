import { ConsolePageHeader } from "@/components/console/page-header";
import { ActivityTable } from "./activity-table";

export default function ActivityPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="ACTIVITY"
        title="请求明细"
        description="每一行是一次可解释的路由回单：模型、状态、金额。没有请求时保持空账本，不编造演示数据。"
      />
      <ActivityTable />
    </div>
  );
}
