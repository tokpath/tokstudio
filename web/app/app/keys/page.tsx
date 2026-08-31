import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ConsolePageHeader } from "@/components/console/page-header";
import KeysPanel from "../keys-panel";

export default function KeysPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="API KEYS"
        title="API Key"
        description="完整 Key 可长期查看。轮换、复制、禁用、过期都会写审计日志。空白名单不限制模型。"
        actions={
          <Button asChild variant="outline">
            <Link href="/app/docs">接入示例</Link>
          </Button>
        }
      />
      <KeysPanel />
    </div>
  );
}
