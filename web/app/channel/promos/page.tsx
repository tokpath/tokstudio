import { getTranslations } from "next-intl/server";
import ChannelPromos from "../promos-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import Link from "next/link";

export default async function ChannelPromosPage() {
  const tUI = await getTranslations();
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPromos" actions={<Link href="/channel/attribution" className="text-sm text-brand-emphasis underline">{tUI("consoleCopy.referralAttribution")}</Link>} />
      <ChannelPromos />
    </div>
  );
}
