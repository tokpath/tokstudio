import { getTranslations } from "next-intl/server";
import AdminDashboard from "../dashboard";
import { AdminShell } from "../shell";

export default async function AdminMetricsPage() {
  const tUI = await getTranslations();
  return (
    <AdminShell>
      <h1 className="text-2xl font-semibold">{tUI("consoleCopy.serviceOverview")}</h1>
      <AdminDashboard />
    </AdminShell>
  );
}
