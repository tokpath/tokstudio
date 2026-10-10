"use client";
import { Suspense } from "react";
import { CustomerList } from "@/components/customer-list";
import { AdminShell } from "../shell";
export default function AdminUsersPage(){return <AdminShell><Suspense fallback={<p>正在读取客户…</p>}><CustomerList surface="admin"/></Suspense></AdminShell>;}
