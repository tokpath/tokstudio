import { ConsolePageHeader } from "@/components/console/page-header";
import { PartnerBoard } from "../partner-board";

export default function PartnerCommissionsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader eyebrow="COMMISSION" title="范围内佣金" description="冻结期满后才可结算。不能改佣金比例。" />
      <PartnerBoard section="commissions" />
    </div>
  );
}
