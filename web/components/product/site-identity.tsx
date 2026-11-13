"use client";

import { useI18n } from "@/components/i18n";
import type { Operator } from "@/lib/contracts";

export function SiteIdentity({ brandName, operator }: { brandName: string; operator?: Operator }) {
  const { t } = useI18n();
  return <div className="border-b border-hairline bg-canvas-raised px-5 py-2 text-center text-[11px] leading-5 text-ink-mute">
    {t("品牌：")}{brandName}
    {operator && <><span className="mx-3 text-hairline">/</span>{t("实际经营方：")}{operator.name}{operator.legal_name && <span> · {operator.legal_name}</span>}</>}
  </div>;
}

export function SiteFooter({ operator, brandName, insideWorkspace }: { operator?: Operator; brandName?: string; insideWorkspace?: boolean }) {
  const { t } = useI18n();
  return <footer className={`border-t border-hairline px-5 text-center text-[11px] leading-6 text-ink-mute ${insideWorkspace ? "bg-canvas-raised py-4" : "py-8"}`}>
    {operator?.name || brandName || "TokenHub"} {t("· 模型服务与独立经营")}
    {operator?.contact_email && <span> · <a href={`mailto:${operator.contact_email}`}>{operator.contact_email}</a></span>}
    <p>{t("价格和权益以本次成交条款为准。账单、资金和服务责任均可追溯。")}</p>
  </footer>;
}
