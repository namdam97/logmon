"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { me, type User } from "@/lib/api";

type State = { status: "loading" } | { status: "ready"; user: User };

// AuthGuard kiểm tra phiên qua GET /me; chưa đăng nhập → điều hướng /login.
export function AuthGuard({
  children,
}: {
  children: (user: User) => React.ReactNode;
}) {
  const router = useRouter();
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    let active = true;
    me()
      .then((user) => active && setState({ status: "ready", user }))
      .catch(() => active && router.replace("/login"));
    return () => {
      active = false;
    };
  }, [router]);

  if (state.status === "loading") {
    return (
      <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
        Đang tải phiên...
      </div>
    );
  }
  return <>{children(state.user)}</>;
}
