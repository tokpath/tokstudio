import { AdminShell } from "../shell";
import { PaymentOrdersPanel } from "./orders-panel";
import { PaymentsNav } from "./payments-nav";
export default function Page() { return <AdminShell><PaymentsNav scope="admin" active="orders" /><PaymentOrdersPanel /></AdminShell>; }
