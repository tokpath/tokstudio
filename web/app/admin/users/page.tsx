"use client";
import { useTranslations } from "next-intl";
import { Suspense } from "react";
import { CustomerList } from "@/components/customer-list";
import { AdminShell } from "../shell";
export default function AdminUsersPage(){
  const tUI = useTranslations();return <AdminShell><Suspense fallback={<p>{tUI("paymentTasks.loadingCustomer")}</p>}><CustomerList surface="admin"/></Suspense></AdminShell>;}
