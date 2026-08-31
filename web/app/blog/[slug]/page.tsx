import Link from "next/link";
import { notFound } from "next/navigation";
import { headers } from "next/headers";
import { Button } from "@/components/ui/button";
import { getTranslations } from "next-intl/server";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { loadSite } from "@/lib/site-content";

export default async function BlogPostPage({ params }: { params: Promise<{ slug: string }> }) {
  const t = await getTranslations("blogUi");
  const { slug } = await params;
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const post = (site.blog || []).find((p) => p.href === `/blog/${slug}` || p.href.endsWith(`/${slug}`));
  if (!post) notFound();

  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-8 px-6 py-20">
      <I18nPublicHero
        id="article"
        primaryHref="/blog"
        secondaryHref="/docs"
        title={post.title}
        description={post.summary || undefined}
      />
      <article className="space-y-4 text-sm leading-relaxed text-ink-secondary">
        <p>{post.summary}</p>
        <p>{t("note")}</p>
      </article>
      <div className="flex flex-wrap gap-3">
        <Button asChild variant="outline">
            <Link href="/docs">{t("docs")}</Link>
          </Button>
          {post.source ? (
            <Button asChild variant="ghost">
              <a href={post.source} rel="noreferrer">
                {t("source")}
              </a>
          </Button>
        ) : null}
      </div>
    </main>
  );
}
