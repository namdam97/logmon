// API client cho LogMon backend. Khớp với response envelope chuẩn:
// { success, data, error?, meta? }.

// Mặc định rỗng = URL tương đối (same-origin): gọi /api/... trên chính origin
// của FE, được Next rewrite (dev/e2e) hoặc Nginx (prod) proxy sang userservice.
// Same-origin là điều kiện để CSRF double-submit hoạt động (JS đọc cookie
// lm_csrf). Đặt NEXT_PUBLIC_API_BASE_URL nếu muốn gọi thẳng origin khác.
const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

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
    // Gửi/nhận cookie auth (HttpOnly) cho mọi request qua biên origin.
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  const body = (await res.json()) as Envelope<T>;
  if (!res.ok || !body.success || body.data === null) {
    throw new Error(body.error ?? `request failed with status ${res.status}`);
  }
  return body.data;
}

// readCookie đọc giá trị cookie không-HttpOnly (vd lm_csrf) từ document.cookie.
// Trả undefined nếu không có hoặc đang chạy ngoài trình duyệt (SSR).
export function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  for (const part of document.cookie.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return v.join("=");
  }
  return undefined;
}

// mutate là request cho method thay đổi trạng thái (POST/PUT/DELETE): tự đính
// kèm CSRF token (double-submit) đọc từ cookie lm_csrf vào header X-CSRF-Token.
// Thiếu token → ném lỗi rõ ràng thay vì gửi request chắc chắn bị 403 (tránh
// che giấu sự cố CSRF: phân biệt "chưa đăng nhập" với "cookie bị mất").
function mutate<T>(path: string, init?: RequestInit): Promise<T> {
  const token = readCookie("lm_csrf");
  if (!token) {
    throw new Error("Thiếu CSRF token phiên — vui lòng tải lại trang.");
  }
  return request<T>(path, {
    method: "POST",
    ...init,
    headers: {
      "X-CSRF-Token": token,
      ...(init?.headers ?? {}),
    },
  });
}

export function registerUser(input: RegisterInput): Promise<User> {
  return request<User>("/api/v1/users", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export type LoginInput = RegisterInput;

// login đặt cookie HttpOnly phía server và trả về user vừa xác thực.
export function login(input: LoginInput): Promise<User> {
  return request<User>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// me trả về user đang đăng nhập dựa trên cookie hiện tại.
export function me(): Promise<User> {
  return request<User>("/api/v1/me");
}

// logout yêu cầu server xoá cookie phiên (HttpOnly nên client không tự xoá được).
// /auth/logout là POST mutating KHÔNG được miễn CSRF → phải đi qua mutate() để
// đính kèm X-CSRF-Token, nếu không server trả 403 và phiên không bị xoá.
export function logout(): Promise<{ loggedOut: boolean }> {
  return mutate<{ loggedOut: boolean }>("/api/v1/auth/logout");
}

// ── Alerting ────────────────────────────────────────────────────────────────

// ActiveAlert khớp instanceResponse của backend (GET /api/v1/alerts/active).
export interface ActiveAlert {
  id: string;
  fingerprint: string;
  status: string; // firing | acknowledged | resolved
  firedAt: string;
  acknowledgedAt?: string;
  acknowledgedBy?: string;
  resolvedAt?: string;
  labels: Record<string, string>;
}

// AlertRule khớp ruleResponse của backend (GET /api/v1/alert-rules).
export interface AlertRule {
  id: string;
  workspaceId: string;
  name: string;
  expression: string;
  service: string;
  severity: string; // critical | warning | info
  forDuration: string;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  enabled: boolean;
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

// listActiveAlerts trả về các alert instance đang active (GET — miễn CSRF).
export function listActiveAlerts(): Promise<ActiveAlert[]> {
  return request<ActiveAlert[]>("/api/v1/alerts/active");
}

// acknowledgeAlert đánh dấu một instance đã được tiếp nhận (POST — cần CSRF).
export function acknowledgeAlert(id: string): Promise<ActiveAlert> {
  return mutate<ActiveAlert>(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`);
}

// listAlertRules trả về toàn bộ alert rule của workspace (GET — miễn CSRF).
export function listAlertRules(): Promise<AlertRule[]> {
  return request<AlertRule[]>("/api/v1/alert-rules");
}

// setAlertRuleEnabled bật/tắt một rule (POST enable|disable — cần CSRF).
export function setAlertRuleEnabled(
  id: string,
  enabled: boolean,
): Promise<AlertRule> {
  const action = enabled ? "enable" : "disable";
  return mutate<AlertRule>(`/api/v1/alert-rules/${encodeURIComponent(id)}/${action}`);
}
