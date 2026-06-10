"use client";

import { useEffect, useState } from "react";

import { me, type User } from "@/lib/api";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function ProfilePage() {
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    me()
      .then(setUser)
      .catch((e) => setError(e instanceof Error ? e.message : "unknown error"));
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Hồ sơ</h1>
        <p className="text-sm text-muted-foreground">
          Tài khoản đang đăng nhập.
        </p>
      </div>

      <Card className="max-w-lg">
        <CardHeader>
          <CardTitle>Thông tin tài khoản</CardTitle>
          <CardDescription>Lấy từ GET /api/v1/me</CardDescription>
        </CardHeader>
        <CardContent>
          {error && <p className="text-sm text-destructive">Lỗi: {error}</p>}
          {user && (
            <dl className="grid grid-cols-3 gap-y-3 text-sm">
              <dt className="text-muted-foreground">ID</dt>
              <dd className="col-span-2 font-mono">{user.id}</dd>
              <dt className="text-muted-foreground">Email</dt>
              <dd className="col-span-2">{user.email}</dd>
              <dt className="text-muted-foreground">Tạo lúc</dt>
              <dd className="col-span-2">{user.createdAt}</dd>
            </dl>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
