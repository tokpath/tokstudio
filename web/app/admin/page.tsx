import AdminDashboard from "./dashboard";
import { AdminShell } from "./shell";

export default function AdminConsole() {
  return (
    <AdminShell>
      <p className="text-slate-300">
        平台管理员可以看全部渠道、改归因（必须写原因并进审计），但不能让普通用户自己改归属。套餐审核、支付单和佣金策略在左侧对应页面，不再只暴露接口路径。
      </p>
      <AdminDashboard />
    </AdminShell>
  );
}
