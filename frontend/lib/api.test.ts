import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  acknowledgeAlert,
  listActiveAlerts,
  listAlertRules,
  readCookie,
  setAlertRuleEnabled,
  setWorkspaceID,
  topology,
} from "./api";

// fakeResponse dựng đối tượng tối thiểu giống Response để mock fetch.
function fakeResponse(body: unknown, ok = true, status = 200) {
  return {
    ok,
    status,
    json: () => Promise.resolve(body),
  } as Response;
}

describe("readCookie", () => {
  beforeEach(() => {
    vi.stubGlobal("document", {
      cookie: "lm_csrf=tok-123; other=xyz",
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("trả về giá trị khi cookie tồn tại", () => {
    expect(readCookie("lm_csrf")).toBe("tok-123");
  });

  it("trả về undefined khi cookie không tồn tại", () => {
    expect(readCookie("missing")).toBeUndefined();
  });
});

describe("alerting API client", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("document", { cookie: "lm_csrf=csrf-tok" });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("listActiveAlerts gọi GET /alerts/active và trả mảng data", async () => {
    const alerts = [{ id: "a1", status: "firing" }];
    fetchMock.mockResolvedValue(fakeResponse({ success: true, data: alerts }));

    const got = await listActiveAlerts();

    expect(got).toEqual(alerts);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/v1/alerts/active");
    expect(init.method ?? "GET").toBe("GET");
    expect(init.credentials).toBe("include");
    // GET là safe method → KHÔNG gửi CSRF header.
    expect(init.headers["X-CSRF-Token"]).toBeUndefined();
  });

  it("acknowledgeAlert POST kèm X-CSRF-Token đọc từ cookie", async () => {
    const inst = { id: "a1", status: "acknowledged" };
    fetchMock.mockResolvedValue(fakeResponse({ success: true, data: inst }));

    const got = await acknowledgeAlert("a1");

    expect(got).toEqual(inst);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/v1/alerts/a1/acknowledge");
    expect(init.method).toBe("POST");
    expect(init.headers["X-CSRF-Token"]).toBe("csrf-tok");
  });

  it("listAlertRules gọi GET /alert-rules", async () => {
    fetchMock.mockResolvedValue(fakeResponse({ success: true, data: [] }));

    const got = await listAlertRules();

    expect(got).toEqual([]);
    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/alert-rules");
  });

  it("setAlertRuleEnabled(true) POST .../enable kèm CSRF", async () => {
    const rule = { id: "r1", enabled: true };
    fetchMock.mockResolvedValue(fakeResponse({ success: true, data: rule }));

    await setAlertRuleEnabled("r1", true);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/v1/alert-rules/r1/enable");
    expect(init.method).toBe("POST");
    expect(init.headers["X-CSRF-Token"]).toBe("csrf-tok");
  });

  it("setAlertRuleEnabled(false) POST .../disable", async () => {
    fetchMock.mockResolvedValue(
      fakeResponse({ success: true, data: { id: "r1", enabled: false } }),
    );

    await setAlertRuleEnabled("r1", false);

    expect(fetchMock.mock.calls[0][0]).toContain("/api/v1/alert-rules/r1/disable");
  });

  it("ném lỗi khi envelope success=false", async () => {
    fetchMock.mockResolvedValue(
      fakeResponse({ success: false, data: null, error: "boom" }, false, 502),
    );

    await expect(listActiveAlerts()).rejects.toThrow("boom");
  });
});

describe("topology API client", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("document", { cookie: "" });
    const store: Record<string, string> = {};
    vi.stubGlobal("localStorage", {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v;
      },
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("topology gọi GET /topology kèm header workspace", async () => {
    setWorkspaceID("ws-9");
    const graph = { nodes: [], edges: [], generatedAt: "2026-06-28T00:00:00Z" };
    fetchMock.mockResolvedValue(fakeResponse({ success: true, data: graph }));

    const got = await topology();

    expect(got).toEqual(graph);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/v1/topology");
    expect(init.method ?? "GET").toBe("GET");
    expect(init.headers["X-Workspace-ID"]).toBe("ws-9");
  });
});
