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
    <div className="flex flex-col items-start gap-2 px-12 py-12">
      <IconStamp icon={icon} />
      <p className="mt-2 text-sm text-ink">{title}</p>
      <p className="text-sm text-ink-mute">{detail}</p>
    </div>
  );
}
