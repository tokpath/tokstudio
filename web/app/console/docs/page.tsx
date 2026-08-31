import Link from "next/link";
import { Button } from "@/components/ui/button";
import ExamplesPanel from "../examples-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ConsoleDocsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="docs" />
      <ExamplesPanel />
    </div>
  );
}
