import { ConsolePageHeader } from "@/components/console/page-header";
import { PartnerBoard } from "../partner-board";

export default function PartnerSettlementsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="SETTLEMENTS" title="范围内结算" description="平台财务打款后才会出现在这里。" />
      <PartnerBoard section="settlements" />
    </div>
  );
}
