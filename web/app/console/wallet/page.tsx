import { RecoveryHistory } from "./recovery-history";
import WalletPanel from "../wallet-panel";
import { WalletLedger } from "./wallet-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function WalletPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="wallet" />
      <WalletPanel />
      <RecoveryHistory />
      <WalletLedger />
    </div>
  );
}
