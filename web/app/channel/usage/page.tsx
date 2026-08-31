import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelUsage from "../usage-panel";

export default function ChannelUsagePage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="USAGE" title="用量" description="Token 和媒体秒数按账务 usage 汇总，不含 prompt 原文。" />
      <ChannelUsage />
    </div>
  );
}
