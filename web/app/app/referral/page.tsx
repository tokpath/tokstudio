"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { ConsolePageHeader } from "@/components/console/page-header";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { Input } from "@/components/ui/input";

export default function ReferralPage() {
  const [copied, setCopied] = useState(false);
  const code = "TH-REF";

  return (
    <div className="flex flex-col gap-6">
      <ConsolePageHeader
        eyebrow="REFERRAL"
        title="推荐计划"
        description="把兑换码发给下线用户。归因在注册时写死，推荐人不在页面上改渠道。"
      />
      <section className="rounded-card border border-hairline bg-canvas-raised p-5">
        <h2 className="text-lg font-semibold">推荐码</h2>
        <p className="mt-2 text-sm text-ink-secondary">登录后会显示你自己的推广码。未登录时只展示占位，不会写入别人的邮箱或团队名。</p>
        <div className="mt-4 flex flex-wrap gap-2">
          <Input readOnly value={code} aria-label="推荐码" className="max-w-xs font-mono" />
          <Button
            type="button"
            variant="outline"
            onClick={async () => {
              await navigator.clipboard.writeText(code);
              setCopied(true);
            }}
          >
            {copied ? "已复制" : "复制"}
          </Button>
        </div>
      </section>
      <EmptyLedger title="暂无推荐记录" detail="还没有通过你的码完成注册的用户。账本空着，不编造人数。" />
    </div>
  );
}
