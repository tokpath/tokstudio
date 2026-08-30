import type { Metadata } from "next";
import { AuthForm } from "@/components/auth-form";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "登录",
};

export default function LoginPage() {
  return (
    <PublicShell>
      <AuthForm mode="login" />
    </PublicShell>
  );
}
