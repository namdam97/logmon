"use client";

import { useState } from "react";
import { registerUser, type User } from "@/lib/api";

export default function HomePage() {
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
      const created = await registerUser({ email, password });
      setUser(created);
    } catch (err) {
      setError(err instanceof Error ? err.message : "unknown error");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 px-4 py-16">
      <header>
        <h1 className="text-2xl font-semibold">LogMon</h1>
        <p className="text-sm text-neutral-500">Walking skeleton — đăng ký user</p>
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
          minLength={8}
          placeholder="mật khẩu (>= 8 ký tự)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="rounded border border-neutral-300 px-3 py-2 dark:border-neutral-700 dark:bg-neutral-900"
        />
        <button
          type="submit"
          disabled={loading}
          className="rounded bg-neutral-900 px-3 py-2 text-white disabled:opacity-50 dark:bg-white dark:text-neutral-900"
        >
          {loading ? "Đang gửi..." : "Đăng ký"}
        </button>
      </form>

      {error && <p className="text-sm text-red-600">Lỗi: {error}</p>}
      {user && (
        <pre className="overflow-auto rounded bg-neutral-100 p-3 text-sm dark:bg-neutral-900">
          {JSON.stringify(user, null, 2)}
        </pre>
      )}
    </main>
  );
}
