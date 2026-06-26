"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Activity, Bell, Gauge, LogOut, User } from "lucide-react";

import { cn } from "@/lib/utils";
import { logout as apiLogout } from "@/lib/api";
import { Button } from "@/components/ui/button";

const NAV = [
  { href: "/", label: "Tổng quan", icon: Gauge },
  { href: "/alerts", label: "Cảnh báo", icon: Bell },
  { href: "/profile", label: "Hồ sơ", icon: User },
];

export function DashboardShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();

  // Gọi backend xoá cookie HttpOnly rồi điều hướng về /login. Dù lỗi mạng vẫn
  // điều hướng (tránh kẹt người dùng ở trạng thái nửa-vời).
  async function logout() {
    try {
      await apiLogout();
    } finally {
      router.push("/login");
    }
  }

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-60 flex-col border-r bg-card md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-5">
          <Activity className="h-5 w-5" />
          <span className="font-semibold">LogMon</span>
        </div>
        <nav className="flex-1 space-y-1 p-3">
          {NAV.map(({ href, label, icon: Icon }) => {
            const active = pathname === href;
            return (
              <Link
                key={href}
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                  active
                    ? "bg-secondary font-medium text-secondary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                )}
              >
                <Icon className="h-4 w-4" />
                {label}
              </Link>
            );
          })}
        </nav>
      </aside>

      <div className="flex flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b px-6">
          <span className="text-sm text-muted-foreground">
            Observability Admin
          </span>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" aria-label="Thông báo">
              <Bell className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="sm" onClick={logout}>
              <LogOut className="h-4 w-4" />
              Đăng xuất
            </Button>
          </div>
        </header>
        <main className="flex-1 p-6">{children}</main>
      </div>
    </div>
  );
}
