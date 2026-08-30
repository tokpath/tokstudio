"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { adminGroups, channelSections, partnerSections, portalLinks, userSections } from "@/lib/nav";

type Item = { href: string; label: string; group: string };

export function CommandPalette({
  open,
  onOpenChange,
  labels,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  labels: Record<string, string>;
}) {
  const router = useRouter();
  const [q, setQ] = useState("");

  const items = useMemo<Item[]>(() => {
    const portals = portalLinks.map((item) => ({ href: item.href, label: labels[item.key] || item.key, group: "门户" }));
    const user = userSections.map((item) => ({ href: `/app${item.href}`, label: item.label, group: "用户" }));
    const channel = channelSections.map((item) => ({ href: `/channel${item.href}`, label: item.label, group: "渠道" }));
    const partner = partnerSections.map((item) => ({ href: `/partner${item.href}`, label: item.label, group: "分销" }));
    const admin = adminGroups.flatMap((group) =>
      group.items.map((item) => ({ href: item.href, label: labels[item.key] || item.key, group: "管理" })),
    );
    return [...portals, ...user, ...channel, ...partner, ...admin];
  }, [labels]);

  const filtered = items.filter((item) => {
    const hay = `${item.label} ${item.href} ${item.group}`.toLowerCase();
    return hay.includes(q.trim().toLowerCase());
  });

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        onOpenChange(!open);
      }
      if (event.key === "Escape") {
        onOpenChange(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onOpenChange, open]);

  useEffect(() => {
    if (!open) {
      setQ("");
    }
  }, [open]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-[80] flex items-start justify-center bg-scrim px-4 pt-[12vh]" onClick={() => onOpenChange(false)}>
      <div
        className="w-full max-w-xl overflow-hidden rounded-stamp border border-hairline bg-canvas-raised"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-label="快速跳转"
      >
        <input
          autoFocus
          value={q}
          onChange={(event) => setQ(event.target.value)}
          placeholder="搜索页面、模型、账单、渠道…"
          className="h-12 w-full border-b border-hairline bg-transparent px-4 text-sm text-ink placeholder:text-ink-mute"
        />
        <ul className="th-scrollbar max-h-80 overflow-auto p-2">
          {filtered.slice(0, 16).map((item) => (
            <li key={`${item.group}-${item.href}`}>
              <button
                className="flex w-full items-center justify-between rounded-control px-3 py-2 text-left text-sm text-ink hover:bg-brand-soft"
                onClick={() => {
                  onOpenChange(false);
                  router.push(item.href);
                }}
              >
                <span>{item.label}</span>
                <span className="text-xs text-ink-mute">{item.group}</span>
              </button>
            </li>
          ))}
          {filtered.length === 0 ? <li className="px-3 py-6 text-center text-sm text-ink-mute">没有匹配的入口</li> : null}
        </ul>
        <p className="border-t border-hairline px-4 py-2 text-xs text-ink-mute">Enter 跳转 · Esc 关闭 · ⌘K / Ctrl+K 打开</p>
      </div>
    </div>
  );
}
