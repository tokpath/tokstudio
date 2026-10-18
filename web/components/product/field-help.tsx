"use client";

import { CircleHelp } from "lucide-react";
import type { ReactNode } from "react";
import { useI18n } from "@/components/i18n";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

/** Business help is separate from submission and follows the enclosing form lock. */
export function FieldHelp({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useI18n();
  return <Dialog>
    <DialogTrigger asChild>
      <button type="button" aria-label={t("了解{title}", { title })} className="ml-1 inline-flex size-7 max-sm:size-11 shrink-0 items-center justify-center rounded-full align-middle text-ink-mute hover:bg-brand-soft hover:text-brand-emphasis focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-emphasis" onClick={event => event.stopPropagation()}>
        <CircleHelp className="size-4" aria-hidden="true" />
      </button>
    </DialogTrigger>
    <DialogContent className="max-h-[85dvh] w-[calc(100%_-_2rem)] max-w-md overflow-y-auto" onClick={event => event.stopPropagation()}>
      <DialogHeader><DialogTitle className="pr-7">{title}</DialogTitle><DialogDescription asChild><div className="whitespace-pre-line text-left leading-7">{children}</div></DialogDescription></DialogHeader>
    </DialogContent>
  </Dialog>;
}
