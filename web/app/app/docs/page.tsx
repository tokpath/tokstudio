import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ConsolePageHeader } from "@/components/console/page-header";
import ExamplesPanel from "../examples-panel";

export default function ConsoleDocsPage() {
  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="DOCS"
        title="接入文档"
        description="示例会带上当前品牌的 Base URL 和模型白名单，不会写入完整 API Key。"
        actions={
          <Button asChild variant="outline">
            <Link href="/docs">打开公共文档</Link>
          </Button>
        }
      />
      <ExamplesPanel />
    </div>
  );
}
