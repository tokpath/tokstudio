import { ConsolePageHeader } from "@/components/console/page-header";
import WalletPanel from "../wallet-panel";
import { WalletLedger } from "./wallet-ledger";

export default function WalletPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="WALLET"
        title="余额 / 充值"
        description="可用余额可直接调用；预授权占用是 HOLD 中尚未结算的请求。兑换码与支付充值都写进同一本账。"
      />
      <WalletPanel />
      <WalletLedger />
    </div>
  );
}
