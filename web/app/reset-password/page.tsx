import { Suspense } from "react";
import { EmailActionForm } from "@/components/email-action-form";
export const metadata = { referrer: "no-referrer" as const };
export default function ResetPasswordPage() { return <Suspense><EmailActionForm kind="reset" /></Suspense>; }
