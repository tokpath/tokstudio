import type { Metadata } from "next";
import { AuthForm } from "@/components/auth-form";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "注册",
};

export default function RegisterPage() {
  return (
    <PublicShell>
      <AuthForm mode="register" />
    </PublicShell>
  );
}
