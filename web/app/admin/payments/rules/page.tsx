import { AdminShell } from "../../shell";
import { PaymentRulesPanel } from "@/app/channel/payments/rules-panel";
import { PaymentsNav } from "../payments-nav";
export default function Page() { return <AdminShell><PaymentsNav scope="admin" active="rules" /><PaymentRulesPanel scope="admin" /></AdminShell>; }
