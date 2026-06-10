"use client";

import { useState } from "react";
import { login, me, type User } from "@/lib/api";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setUser(null);
    try {
      const u = await login({ email, password });
      setUser(u);
    } catch (err) {
      setError(err instanceof Error ? err.message : "unknown error");
    } finally {
      setLoading(false);
    }
  }

  async function refreshMe() {
    setError(null);
    try {
      setUser(await me());
    } catch (err) {
      setError(err instanceof Error ? err.message : "unknown error");
    }
  }

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 px-4 py-16">
      <header>
        <h1 className="text-2xl font-semibold">Đăng nhập LogMon</h1>
        <p className="text-sm text-neutral-500">JWT lưu trong cookie HttpOnly</p>
      </header>

      <form onSubmit={onSubmit} className="flex flex-col gap-3">
        <input
          type="email"
          required
          placeholder="email@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="rounded border border-neutral-300 px-3 py-2 dark:border-neutral-700 dark:bg-neutral-900"
        />
        <input
          type="password"
          required
          placeholder="mật khẩu"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="rounded border border-neutral-300 px-3 py-2 dark:border-neutral-700 dark:bg-neutral-900"
        />
        <button
          type="submit"
          disabled={loading}
          className="rounded bg-neutral-900 px-3 py-2 text-white disabled:opacity-50 dark:bg-white dark:text-neutral-900"
        >
          {loading ? "Đang đăng nhập..." : "Đăng nhập"}
        </button>
      </form>

      <button
        type="button"
        onClick={refreshMe}
        className="rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700"
      >
        Kiểm tra phiên (GET /me)
      </button>

      {error && <p className="text-sm text-red-600">Lỗi: {error}</p>}
      {user && (
        <pre className="overflow-auto rounded bg-neutral-100 p-3 text-sm dark:bg-neutral-900">
          {JSON.stringify(user, null, 2)}
        </pre>
      )}
    </main>
  );
}
