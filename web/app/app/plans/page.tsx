import { ConsolePageHeader } from "@/components/console/page-header";
import PlansPanel from "../plans-panel";

export default function PlansPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="PLANS"
        title="套餐"
        description="扣减顺序：即将过期的赠送 → 当期套餐 → 现金钱包。金额单位是 micro-USD。"
      />
      <PlansPanel />
    </div>
  );
}
