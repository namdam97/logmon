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

// _workspaceKey lưu workspace đang chọn (localStorage) để mọi request tenant-scoped
// đính kèm header X-Workspace-ID (multi-tenancy GĐ3.6 — BE bắt buộc header này).
const _workspaceKey = "logmon_workspace_id";

// getWorkspaceID đọc workspace đang chọn (undefined nếu chưa chọn / SSR).
export function getWorkspaceID(): string | undefined {
  if (typeof localStorage === "undefined") return undefined;
  return localStorage.getItem(_workspaceKey) ?? undefined;
}

// setWorkspaceID lưu workspace đang chọn.
export function setWorkspaceID(id: string): void {
  if (typeof localStorage !== "undefined") localStorage.setItem(_workspaceKey, id);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const ws = getWorkspaceID();
  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    // Gửi/nhận cookie auth (HttpOnly) cho mọi request qua biên origin.
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      // BE bỏ qua header này trên route không tenant-scoped (login/me/workspaces).
      ...(ws ? { "X-Workspace-ID": ws } : {}),
      ...(init?.headers ?? {}),
    },
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

// ── Workspaces & RBAC (GĐ3.6) ─────────────────────────────────────────────────

export interface Workspace {
  id: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

export interface Member {
  workspaceId: string;
  userId: string;
  role: string; // viewer | editor | admin | platform_admin
  joinedAt: string;
}

// listWorkspaces trả workspace mà user là thành viên (KHÔNG cần X-Workspace-ID).
export function listWorkspaces(): Promise<Workspace[]> {
  return request<Workspace[]>("/api/v1/workspaces");
}

export function createWorkspace(name: string, slug?: string): Promise<Workspace> {
  return mutate<Workspace>("/api/v1/workspaces", { body: JSON.stringify({ name, slug }) });
}

export function listMembers(workspaceId: string): Promise<Member[]> {
  return request<Member[]>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members`);
}

export function addMember(workspaceId: string, email: string, role: string): Promise<Member> {
  return mutate<Member>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/members`, {
    body: JSON.stringify({ email, role }),
  });
}

// ── SLO (GĐ3.1) ───────────────────────────────────────────────────────────────

export interface SLO {
  id: string;
  workspaceId: string;
  name: string;
  service: string;
  objective: number; // 99.9
  window: string; // 30d
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface SLOCompliance {
  id: string;
  name: string;
  service: string;
  objective: number;
  currentSLI: number;
  errorBudgetRemaining: number; // %
  burnRate: number;
}

export function listSLOs(): Promise<SLO[]> {
  return request<SLO[]>("/api/v1/slos");
}

export function sloCompliance(): Promise<SLOCompliance[]> {
  return request<SLOCompliance[]>("/api/v1/slos/compliance");
}

// ── Incidents (GĐ3.3) ─────────────────────────────────────────────────────────

export interface Incident {
  id: string;
  workspaceId: string;
  title: string;
  description?: string;
  service: string;
  severity?: string; // SEV1..SEV4
  status: string; // open|triaged|assigned|mitigating|resolved|postmortem_pending|closed
  assignee?: string;
  createdAt: string;
  resolvedAt?: string;
  mttaSeconds?: number;
  mttrSeconds?: number;
}

export function listIncidents(activeOnly = false): Promise<Incident[]> {
  return request<Incident[]>(`/api/v1/incidents${activeOnly ? "?active=true" : ""}`);
}

export function getIncident(id: string): Promise<Incident> {
  return request<Incident>(`/api/v1/incidents/${encodeURIComponent(id)}`);
}

export function transitionIncident(
  id: string,
  action: "triage" | "assign" | "mitigate" | "resolve" | "close",
  body?: Record<string, unknown>,
): Promise<Incident> {
  return mutate<Incident>(`/api/v1/incidents/${encodeURIComponent(id)}/${action}`, {
    body: JSON.stringify(body ?? {}),
  });
}

// ── On-call (GĐ3.4) ───────────────────────────────────────────────────────────

export interface OnCallSchedule {
  id: string;
  name: string;
  rotation: string; // daily|weekly
  participants: string[];
  timezone: string;
}

export interface OnCallNow {
  primary: string;
  secondary: string;
}

export function listSchedules(): Promise<OnCallSchedule[]> {
  return request<OnCallSchedule[]>("/api/v1/oncall/schedules");
}

export function currentOnCall(scheduleId: string): Promise<OnCallNow> {
  return request<OnCallNow>(`/api/v1/oncall/schedules/${encodeURIComponent(scheduleId)}/current`);
}

// ── Notification channels (GĐ3.2) ─────────────────────────────────────────────

export interface Channel {
  id: string;
  name: string;
  type: string; // slack|email|pagerduty|teams|webhook|in_app
  enabled: boolean;
  createdAt: string;
}

export function listChannels(): Promise<Channel[]> {
  return request<Channel[]>("/api/v1/notifications/channels");
}

export function testChannel(id: string): Promise<{ sent: boolean }> {
  return mutate<{ sent: boolean }>(`/api/v1/notifications/channels/${encodeURIComponent(id)}/test`);
}

// ── Pipeline management (GĐ3.7) ───────────────────────────────────────────────

export interface PipelineStatus {
  mode: string;
  health: { elasticsearch: string; collector: string; kafka: string };
  dataStreams: number;
  updatedAt: string;
}

export interface DLQList {
  entries: {
    id: number;
    rawMessage: string;
    errorReason: string;
    sourceService?: string;
    retryCount: number;
    status: string;
    createdAt: string;
  }[];
  counts: Record<string, number>;
}

export function pipelineStatus(): Promise<PipelineStatus> {
  return request<PipelineStatus>("/api/v1/pipeline/status");
}

export function listDLQ(status?: string): Promise<DLQList> {
  const q = status ? `?status=${encodeURIComponent(status)}` : "";
  return request<DLQList>(`/api/v1/pipeline/dlq${q}`);
}

export function retryDLQ(ids: number[]): Promise<{ retried: number[]; failed: Record<string, string> }> {
  return mutate<{ retried: number[]; failed: Record<string, string> }>("/api/v1/pipeline/dlq/retry", {
    body: JSON.stringify({ ids }),
  });
}
