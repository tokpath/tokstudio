import { BrandEditor } from "@/components/brand-editor";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelBrandPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelBrand" />
      <BrandEditor endpoint="/channel/brand" uploadEndpoint="/channel/brand/assets" />
    </div>
  );
}
