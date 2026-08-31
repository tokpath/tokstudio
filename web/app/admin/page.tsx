import AdminDashboard from "./dashboard";
import { AdminShell } from "./shell";

export default function AdminConsole() {
  return (
    <AdminShell>
      <p className="text-ink-secondary">
        平台管理员可以看全部渠道、改归因（必须写原因并进审计），但不能让普通用户自己改归属。套餐审核、价格、支付、指标和佣金策略都在导航里。
      </p>
      <AdminDashboard />
    </AdminShell>
  );
}
