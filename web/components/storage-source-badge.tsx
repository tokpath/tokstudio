"use client";

import {
  STORAGE_LABEL_S3,
  STORAGE_LABEL_UNAVAILABLE,
  STORAGE_SOURCE_TITLE,
  storageBadge,
  type StorageSource,
} from "@/lib/storage-source";

export function StorageSourceBadge({ storage }: { storage?: StorageSource | null }) {
  const view = storageBadge(storage);
  const toneClass =
    view.tone === "ok"
      ? "text-[color:var(--success)]"
      : view.tone === "unavailable"
        ? "text-ink-mute"
        : "text-[color:var(--muted)]";

  return (
    <span className="inline-flex items-center gap-1.5" data-testid="storage-source">
      <span className="th-eyebrow text-ink-mute">{STORAGE_SOURCE_TITLE}</span>
      <span
        data-testid="storage-source-badge"
        data-ok={view.ok ? "true" : "false"}
        data-tone={view.tone}
        data-silent-success="forbidden"
        className={`inline-flex items-center rounded-full border border-hairline px-2 py-0.5 font-mono text-[11px] leading-5 ${toneClass} ${
          view.tone === "unavailable"
            ? "bg-[color-mix(in_srgb,var(--ink-mute)_12%,transparent)]"
            : "bg-[color-mix(in_srgb,var(--muted)_14%,transparent)]"
        }`}
      >
        {view.label.replace(/[✓✔☑✅]/g, "") || (view.ok ? STORAGE_LABEL_S3 : STORAGE_LABEL_UNAVAILABLE)}
      </span>
    </span>
  );
}
