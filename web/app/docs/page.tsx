import { fetchAPI } from "@/lib/api";
import { headers } from "next/headers";

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
    <main className="mx-auto flex max-w-5xl flex-col gap-6 px-6 py-12">
      <h1 className="text-3xl font-semibold">{docs.brand?.name || "TokenHub"} 开发者文档</h1>
      <p className="text-slate-300">
        OEM 站点会展示自己的品牌名、API 域名和模型白名单。下面的示例可以换成测试 Key 后直接跑。
      </p>
      <p className="text-slate-200">可用模型：{(docs.models || []).join("、") || "加载中"}</p>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 font-medium">curl</h2>
        <pre className="overflow-x-auto text-sm text-cyan-100">{docs.examples?.curl}</pre>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 font-medium">Python</h2>
        <pre className="overflow-x-auto text-sm text-cyan-100">{docs.examples?.python}</pre>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 font-medium">Node.js</h2>
        <pre className="overflow-x-auto text-sm text-cyan-100">{docs.examples?.node}</pre>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-2 font-medium">Anthropic Messages</h2>
        <pre className="overflow-x-auto text-sm text-cyan-100">{docs.examples?.messages}</pre>
      </section>
      <p className="text-sm text-slate-400">{docs.notes?.errors}</p>
      <p className="text-sm text-slate-400">{docs.notes?.rate_limit}</p>
      <p className="text-sm text-slate-400">{docs.notes?.webhook}</p>
    </main>
  );
}
