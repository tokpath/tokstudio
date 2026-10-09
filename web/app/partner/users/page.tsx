import { redirect } from "next/navigation";

export default function LegacyPartnerPage() {
  redirect("/app/referral?tab=users");
}
