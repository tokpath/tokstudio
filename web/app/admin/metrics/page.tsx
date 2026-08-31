import AdminDashboard from "../dashboard";
import { AdminShell } from "../shell";

export default function AdminMetricsPage() {
  return (
    <AdminShell>
      <p className="text-ink-secondary">按 Provider、模型、渠道看成功率、延迟、收入和毛利。数据来自网关和账务接口，不直连业务表。</p>
      <AdminDashboard />
    </AdminShell>
  );
}
