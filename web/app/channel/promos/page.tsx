import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelPromos from "../promos-panel";

export default function ChannelPromosPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="PROMOS" title="推广" description="推广链接只属于本渠道。用户注册时服务端会固化归因。" />
      <ChannelPromos />
    </div>
  );
}
