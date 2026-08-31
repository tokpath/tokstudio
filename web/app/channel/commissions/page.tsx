import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelCommissions from "../commissions-panel";

export default function ChannelCommissionsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="COMMISSION"
        title="佣金 / 结算"
        description="可用额度和已发放记录。聊天不再二次扣渠道，佣金由平台承担。"
      />
      <ChannelCommissions />
    </div>
  );
}
