import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelSettlements from "../settlements-panel";

export default function ChannelSettlementsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="SETTLEMENTS" title="结算" description="佣金冻结期满后按月出结算单。P0 打款由平台财务人工完成。" />
      <ChannelSettlements />
    </div>
  );
}
