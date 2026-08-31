import { SettingsSubnav } from "@/components/console/settings-nav";

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-6">
      <SettingsSubnav />
      {children}
    </div>
  );
}
