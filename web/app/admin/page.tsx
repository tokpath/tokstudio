import AdminDashboard from "./dashboard";
import { AdminShell } from "./shell";

export default function AdminConsole() {
  return (
    <AdminShell>
      <p className="text-slate-300">
        平台管理员可以看全部渠道、改归因（必须写原因并进审计），但不能让普通用户自己改归属。渠道低价或高风险媒体配额的套餐会出现在审核队列，接口是
        <code> GET/POST /admin/plans </code> 与 <code> POST /admin/plans/:id/review </code>。
      </p>
      <AdminDashboard />
    </AdminShell>
  );
}
