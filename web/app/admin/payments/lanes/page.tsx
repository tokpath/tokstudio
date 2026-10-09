import { AdminShell } from "../../shell";
import { PaymentLanesPanel } from "@/app/channel/payments/lanes-panel";
import { PaymentsNav } from "../payments-nav";
export default function Page() { return <AdminShell><PaymentsNav scope="admin" active="lanes" /><PaymentLanesPanel scope="admin" /></AdminShell>; }
