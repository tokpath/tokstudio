import { ConsolePageHeader } from "@/components/console/page-header";
import SettingsPanel from "../settings-panel";

export default function AccountSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="ACCOUNT"
        title="账户"
        description="改显示名、界面语言和登录密码。渠道归属不能自己改。"
      />
      <SettingsPanel />
    </div>
  );
}
