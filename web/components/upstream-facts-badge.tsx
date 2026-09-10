"use client";

import { useState } from "react";
import {
  badgeLabel,
  factsJSON,
  hasCompleteUpstreamFacts,
  type UpstreamFacts,
} from "@/lib/upstream-facts";

export function UpstreamFactsBadge({ facts }: { facts: UpstreamFacts }) {
  const [open, setOpen] = useState(false);
  const complete = hasCompleteUpstreamFacts(facts);
  const json = factsJSON(facts);

  return (
    <span
      className="relative inline-flex"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        type="button"
        data-testid="upstream-facts-badge"
        data-complete={complete ? "true" : "false"}
        className="inline-flex max-w-[16rem] items-center rounded-full border border-hairline bg-[color-mix(in_srgb,var(--muted)_14%,transparent)] px-2 py-0.5 font-mono text-[11px] leading-5 text-[color:var(--muted)]"
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
      >
        {badgeLabel(facts)}
      </button>
      {open ? (
        <aside
          role="dialog"
          aria-label="上游事实"
          data-testid="upstream-facts-drawer"
          className="absolute left-0 top-full z-20 mt-1 w-[18rem] rounded-card border border-hairline bg-canvas-raised p-3 text-left shadow-sm"
        >
          <pre className="max-h-48 overflow-auto font-mono text-[11px] leading-5 text-ink">
            {JSON.stringify(json, null, 2)}
          </pre>
        </aside>
      ) : null}
    </span>
  );
}
