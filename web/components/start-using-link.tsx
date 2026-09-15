"use client";

import Link from "next/link";
import { forwardRef, useEffect, useState, type ComponentPropsWithoutRef } from "react";
import { loginHref } from "@/lib/login-next";
import { playgroundHref, resolveStartUsingHref } from "@/lib/console-home";

type Props = Omit<ComponentPropsWithoutRef<typeof Link>, "href"> & { modelId: string };

export const StartUsingLink = forwardRef<HTMLAnchorElement, Props>(function StartUsingLink(
  { modelId, children, ...props },
  ref,
) {
  const guestHref = loginHref(playgroundHref(modelId));
  const [href, setHref] = useState(guestHref);

  useEffect(() => {
    let cancelled = false;
    void resolveStartUsingHref(modelId).then((next) => {
      if (!cancelled) {
        setHref(next);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [modelId]);

  return (
    <Link ref={ref} href={href} {...props}>
      {children}
    </Link>
  );
});
