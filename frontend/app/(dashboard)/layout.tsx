"use client";

import { AuthGuard } from "@/components/auth-guard";
import { DashboardShell } from "@/components/layout/dashboard-shell";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthGuard>{() => <DashboardShell>{children}</DashboardShell>}</AuthGuard>
  );
}
