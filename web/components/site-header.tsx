import { BrandMark } from "@/components/brand-mark";
import { Button } from "@/components/button";
import { ThemeToggle } from "@/components/theme-toggle";

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-10 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-14 max-w-[1120px] items-center gap-4 px-6">
        <a href="/" className="flex items-center gap-2 text-ink no-underline">
          <BrandMark />
          <span className="text-2xl font-semibold">TokenHub</span>
        </a>
        <nav className="hidden flex-1 items-center sm:flex" aria-label="公共站">
          <a
            href="#receipt"
            className="rounded-control bg-brand-soft px-3 py-1.5 text-sm text-brand-emphasis no-underline"
          >
            状态
          </a>
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <ThemeToggle />
          <Button href="#receipt" variant="primary" className="hidden sm:inline-flex">
            查看回单
          </Button>
        </div>
      </div>
    </header>
  );
}
