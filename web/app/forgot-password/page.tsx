import { Suspense } from "react";
import { EmailActionForm } from "@/components/email-action-form";
export const metadata = { referrer: "no-referrer" as const };
export default function ForgotPasswordPage() { return <Suspense><EmailActionForm kind="forgot" /></Suspense>; }
