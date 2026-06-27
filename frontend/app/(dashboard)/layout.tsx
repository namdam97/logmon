"use client";

import { AuthGuard } from "@/components/auth-guard";
import { DashboardShell } from "@/components/layout/dashboard-shell";
import { WorkspaceProvider } from "@/components/workspace-provider";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthGuard>
      {() => (
        <WorkspaceProvider>
          <DashboardShell>{children}</DashboardShell>
        </WorkspaceProvider>
      )}
    </AuthGuard>
  );
}
