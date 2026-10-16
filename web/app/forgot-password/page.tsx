import { Suspense } from "react";
import { AuthPage } from "@/components/product/auth";
export default function Page() { return <Suspense fallback={<p className="p-8">正在加载…</p>}><AuthPage mode="forgot" /></Suspense>; }
