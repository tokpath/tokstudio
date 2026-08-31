import { headers } from "next/headers";
import Link from "next/link";
import { PublicPageHero } from "@/components/public-section";
import { loadSite } from "@/lib/site-content";

export default async function BlogPage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const posts = site.blog || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="BLOG"
        title="博客"
        description="对齐 ofox 博客列表。文章来自公开站快照，由服务端返回。"
        primaryHref="/docs"
        primaryLabel="先看文档"
        secondaryHref="/quickstart"
        secondaryLabel="快速开始"
      />
      <ul className="flex flex-col gap-3">
        {posts.map((post) => (
          <li key={post.href}>
            <Link
              href={post.href}
              className="block rounded-card border border-hairline bg-canvas-raised p-5 no-underline hover:bg-brand-soft/30"
            >
              <p className="th-eyebrow text-ink-mute">{post.date || "LOG"}</p>
              <h2 className="mt-2 text-lg font-semibold text-ink">{post.title}</h2>
              {post.summary ? <p className="mt-2 text-sm text-ink-secondary">{post.summary}</p> : null}
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}
