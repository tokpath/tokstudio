import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function middleware(request: NextRequest) {
  const legacyTabs: Record<string,string> = {"/partner":"overview","/partner/commissions":"commissions","/partner/settlements":"settlements","/partner/users":"users"};
  const legacyTab = legacyTabs[request.nextUrl.pathname.replace(/\/$/,"")];
  if (legacyTab) {
    const url = request.nextUrl.clone();
    url.pathname = "/app/referral";
    const page = Number(url.searchParams.get("page"));
    url.search = "";
    if (legacyTab !== "overview") url.searchParams.set("tab",legacyTab);
    if (Number.isInteger(page) && page > 1 && page <= 100000) url.searchParams.set("page",String(page));
    return NextResponse.redirect(url);
  }
  const headers = new Headers(request.headers);
  headers.set("x-tokenhub-host", request.headers.get("host") || "localhost");
  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|api/).*)"],
};
