"use client";

import Link from "next/link";
import { forwardRef, useEffect, useState, type ComponentPropsWithoutRef } from "react";
import type { CatalogModel } from "@/lib/catalog";
import { loginHref } from "@/lib/login-next";
import { resolveStartUsingHref } from "@/lib/console-home";
import { useModelHref } from "@/lib/model-use";

type ModelRef = Pick<CatalogModel, "id"> & Partial<CatalogModel>;
type Props = Omit<ComponentPropsWithoutRef<typeof Link>, "href"> & { model: ModelRef };

export const StartUsingLink = forwardRef<HTMLAnchorElement, Props>(function StartUsingLink(
  { model, children, ...props },
  ref,
) {
  const guestHref = loginHref(useModelHref(model));
  const [href, setHref] = useState(guestHref);

  useEffect(() => {
    let cancelled = false;
    void resolveStartUsingHref(model).then((next) => {
      if (!cancelled) {
        setHref(next);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [model]);

  return (
    <Link ref={ref} href={href} {...props}>
      {children}
    </Link>
  );
});
