// API client cho LogMon backend. Khớp với response envelope chuẩn:
// { success, data, error?, meta? }.

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export interface Envelope<T> {
  success: boolean;
  data: T | null;
  error?: string;
}

export interface User {
  id: string;
  email: string;
  createdAt: string;
}

export interface RegisterInput {
  email: string;
  password: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  const body = (await res.json()) as Envelope<T>;
  if (!res.ok || !body.success || body.data === null) {
    throw new Error(body.error ?? `request failed with status ${res.status}`);
  }
  return body.data;
}

export function registerUser(input: RegisterInput): Promise<User> {
  return request<User>("/api/v1/users", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getUser(id: string): Promise<User> {
  return request<User>(`/api/v1/users/${encodeURIComponent(id)}`);
}
