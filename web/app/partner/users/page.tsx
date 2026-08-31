import { ConsolePageHeader } from "@/components/console/page-header";
import { PartnerBoard } from "../partner-board";

export default function PartnerUsersPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="USERS" title="范围内用户" description="邮箱已脱敏。2 级只看直接引流。" />
      <PartnerBoard section="users" />
    </div>
  );
}
