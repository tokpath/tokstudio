import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelAttribution from "../attribution-panel";

export default function ChannelAttributionPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="ATTRIBUTION" title="额度" description="按推广码和代理层级点数。渠道不能改别人的归属。" />
      <ChannelAttribution />
    </div>
  );
}
