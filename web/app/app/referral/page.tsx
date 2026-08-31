"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Input } from "@/components/ui/input";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ReferralPage() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [copied, setCopied] = useState(false);
  const code = "TH-REF";

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="referral" />
      <section className="rounded-card border border-hairline bg-canvas-raised p-5">
        <h2 className="text-lg font-semibold">{t("refTitle")}</h2>
        <p className="mt-2 text-sm text-ink-secondary">{t("refLead")}</p>
        <div className="mt-4 flex flex-wrap gap-2">
          <Input readOnly value={code} aria-label={t("refAria")} className="max-w-xs font-mono" />
          <Button
            type="button"
            variant="outline"
            onClick={async () => {
              await navigator.clipboard.writeText(code);
              setCopied(true);
            }}
          >
            {copied ? tc("copied") : tc("copy")}
          </Button>
        </div>
      </section>
      <EmptyLedger title={t("refEmpty")} detail={t("refEmptyDetail")} />
    </div>
  );
}
