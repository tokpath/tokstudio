"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { useState, type ReactNode } from "react";
import { ArrowRight, Check, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";

export function CopyButton({ value, label = "复制", disabled = false }: { value: string; label?: string; disabled?: boolean }) {
  const { t } = useI18n();
  const [state, setState] = useState<"idle" | "copied" | "error">("idle");
  return <span className="inline-flex max-w-full flex-col items-start gap-1"><Button type="button" size="sm" variant="outline" disabled={disabled} onClick={async () => { try { await navigator.clipboard.writeText(value); setState("copied"); } catch { setState("error"); } }}>{state === "copied" ? <Check /> : <Copy />}{state === "copied" ? t("已复制") : t(label)}</Button>{state === "error" && <span role="alert" className="text-xs text-danger">{t("复制失败，请选中文本手动复制。")}</span>}</span>;
}
export function TaskLink({ href, title, children, icon }: { href: string; title: string; children: ReactNode; icon?: ReactNode }) {
  const { t } = useI18n();
  return <Link href={href} className="group flex items-start gap-4 rounded-card border border-hairline bg-canvas-raised p-5 transition-colors hover:border-brand/50"><span className="grid h-10 w-10 shrink-0 place-items-center rounded-control bg-brand-soft text-brand-emphasis">{icon || <ArrowRight className="h-4 w-4" />}</span><span className="min-w-0 flex-1"><span className="font-medium">{t(title)}</span><span className="mt-1 block text-sm leading-6 text-ink-secondary">{children}</span></span><ArrowRight className="mt-2 h-4 w-4 shrink-0 text-ink-mute group-hover:text-brand-emphasis" /></Link>;
}
export function ContextHelp({ title = "了解规则", children }: { title?: string; children: ReactNode }) {
  const { t } = useI18n();
  return <details className="rounded-control border border-hairline bg-canvas/50 px-4 py-3 text-sm"><summary className="cursor-pointer font-medium text-ink-secondary">{t(title)}</summary><div className="mt-3 space-y-3 text-sm leading-7 text-ink-secondary">{children}</div></details>;
}
export function DetailPairs({ items }: { items: { label: string; value: ReactNode }[] }) {
  const { t } = useI18n();
  return <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">{items.map(item => <div key={item.label} className="min-w-0"><dt className="mb-1 text-xs text-ink-mute">{t(item.label)}</dt><dd className="break-words text-sm">{item.value}</dd></div>)}</dl>;
}
export function useCustomerPendingGuard(active: boolean) {
  const { t } = useI18n();
  const blocked = useNavigationBlock(active);
  return blocked ? t("本次操作正在处理或结果待核实，请先完成原操作核实，再离开此页。") : undefined;
}
