import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelPlans from "../plans-panel";

export default function ChannelPlansPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="PLANS" title="套餐" description="低价或高风险配额会进平台审核。归属会被后端写成当前渠道。" />
      <ChannelPlans />
    </div>
  );
}
