import AdminDashboard from "../dashboard";
import { AdminShell } from "../shell";

export default function AdminMetricsPage() {
  return (
    <AdminShell>
      <h1 className="text-2xl font-semibold">服务概况</h1>
      <AdminDashboard />
    </AdminShell>
  );
}
