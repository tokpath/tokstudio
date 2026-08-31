import { ConsolePageHeader } from "@/components/console/page-header";
import ChannelUsers from "../users-panel";

export default function ChannelUsersPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="USERS" title="本渠道用户" description="邮箱可能已脱敏。归因在注册时写死，这里不含其他渠道。" />
      <ChannelUsers />
    </div>
  );
}
