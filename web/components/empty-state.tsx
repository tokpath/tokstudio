import type { LucideIcon } from "lucide-react";
import { Inbox } from "lucide-react";
import { IconStamp } from "@/components/icon-stamp";

export function EmptyState({
  title,
  detail,
  icon = Inbox,
}: {
  title: string;
  detail: string;
  icon?: LucideIcon;
}) {
  return (
    <div className="flex flex-col items-start px-12 py-12">
      <IconStamp icon={icon} />
      <p className="mt-4 text-sm font-medium text-ink">{title}</p>
      <p className="mt-2 max-w-md text-sm leading-relaxed text-ink-mute">{detail}</p>
    </div>
  );
}
