"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { CONSOLE_ENTRY_PATH, resolveConsoleHref } from "@/lib/console-home";

export function ConsoleEntryButton() {
  const tc = useTranslations("chrome");
  const [href, setHref] = useState(CONSOLE_ENTRY_PATH);

  useEffect(() => {
    let cancelled = false;
    void resolveConsoleHref().then((next) => {
      if (!cancelled) setHref(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <Button asChild size="sm">
      <Link href={href}>{tc("console")}</Link>
    </Button>
  );
}

export function ConsoleEnter() {
  const tc = useTranslations("chrome");

  useEffect(() => {
    let cancelled = false;
    void resolveConsoleHref().then((next) => {
      if (!cancelled) window.location.replace(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <main className="flex min-h-svh items-center justify-center px-6 text-ink-secondary">{tc("entering")}</main>
  );
}
