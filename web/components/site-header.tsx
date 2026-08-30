import { BrandMark } from "@/components/brand-mark";
import { ThemeToggle } from "@/components/theme-toggle";

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-10 border-b border-hairline bg-canvas">
      <div className="mx-auto flex h-14 max-w-[1120px] items-center justify-between px-6">
        <a href="/" className="flex items-center gap-2 text-ink no-underline">
          <BrandMark />
          <span className="text-lg font-semibold tracking-tight">TokenHub</span>
        </a>
        <ThemeToggle />
      </div>
    </header>
  );
}
