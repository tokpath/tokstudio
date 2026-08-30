import { fetchAPI } from "@/lib/api";
import { headers } from "next/headers";
import { Badge } from "@/components/ui/badge";
import { Card, CardTitle } from "@/components/ui/card";

type DocsContext = {
  brand?: { name: string; api_domain: string };
  models?: string[];
  examples?: { curl: string; python: string; node: string; messages?: string; video?: string };
  notes?: { errors?: string; rate_limit?: string; webhook?: string };
};

export default async function DocsPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  let docs: DocsContext = {};
  try {
    docs = await fetchAPI<DocsContext>("/v1/public/docs-context", { host });
  } catch {
    docs = {};
  }
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-8 px-6 py-14">
      <div>
        <Badge tone="brand">Developer docs</Badge>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight">{docs.brand?.name || "TokenHub"} 开发者文档</h1>
        <p className="mt-3 max-w-2xl text-slate-300">
          OEM 站点会展示自己的品牌名、API 域名和模型白名单。下面的示例可以换成测试 Key 后直接跑。
        </p>
        <p className="mt-4 text-sm text-slate-400">Base URL · {docs.brand?.api_domain || "api.tokenhub.local"}</p>
        <p className="mt-2 text-slate-200">可用模型：{(docs.models || []).join("、") || "加载中"}</p>
      </div>
      <CodeCard title="curl" code={docs.examples?.curl} />
      <CodeCard title="Python" code={docs.examples?.python} />
      <CodeCard title="Node.js" code={docs.examples?.node} />
      <CodeCard title="Anthropic Messages" code={docs.examples?.messages} />
      {docs.examples?.video ? <CodeCard title="视频任务" code={docs.examples.video} /> : null}
      <div className="space-y-2 text-sm text-slate-400">
        <p>{docs.notes?.errors}</p>
        <p>{docs.notes?.rate_limit}</p>
        <p>{docs.notes?.webhook}</p>
      </div>
    </main>
  );
}

function CodeCard({ title, code }: { title: string; code?: string }) {
  return (
    <Card className="p-0 overflow-hidden">
      <div className="flex items-center justify-between border-b border-white/10 px-5 py-3">
        <CardTitle className="mb-0 text-base">{title}</CardTitle>
        <Badge tone="neutral">copy-ready</Badge>
      </div>
      <pre className="th-code overflow-x-auto bg-black/40 p-5 text-sm leading-7 text-cyan-100">{code || "加载中"}</pre>
    </Card>
  );
}
