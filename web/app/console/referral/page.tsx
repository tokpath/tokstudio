"use client";

import { Suspense } from "react";

import { ReferralPanel } from "@/components/console/referral-panel";


export default function ReferralPage() {
  return (
    <div className="flex flex-col gap-6">
      <Suspense><ReferralPanel /></Suspense>
    </div>
  );
}
