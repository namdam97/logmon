"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  acknowledgeAlert,
  listActiveAlerts,
  listAlertRules,
  setAlertRuleEnabled,
  type ActiveAlert,
  type AlertRule,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// severityVariant ánh xạ mức nghiêm trọng → màu badge (critical đỏ, info xám).
function severityVariant(s: string): "destructive" | "default" | "secondary" {
  if (s === "critical") return "destructive";
  if (s === "warning") return "default";
  return "secondary";
}

// statusVariant ánh xạ trạng thái instance → màu badge.
function statusVariant(s: string): "destructive" | "secondary" | "outline" {
  if (s === "firing") return "destructive";
  if (s === "acknowledged") return "secondary";
  return "outline";
}

// alertName lấy tên hiển thị từ labels (alertname là quy ước Prometheus/AM).
function alertName(a: ActiveAlert): string {
  return a.labels.alertname ?? a.labels.service ?? a.fingerprint.slice(0, 12);
}

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<ActiveAlert[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // busyAlert/busyRule giữ id hàng đang gọi API ở mỗi bảng (tách riêng để thao
  // tác trên bảng này không vô hiệu hoá / reset trạng thái nút bảng kia).
  const [busyAlert, setBusyAlert] = useState<string | null>(null);
  const [busyRule, setBusyRule] = useState<string | null>(null);

  // mounted chặn setState sau khi component đã unmount (tránh rò rỉ trong khi
  // request còn bay — điều hướng route hoặc strict-mode double-invoke).
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [a, r] = await Promise.all([listActiveAlerts(), listAlertRules()]);
      if (!mounted.current) return;
      setAlerts(a);
      setRules(r);
    } catch (e) {
      if (mounted.current) {
        setError(e instanceof Error ? e.message : "unknown error");
      }
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function onAcknowledge(id: string) {
    setBusyAlert(id);
    setError(null);
    try {
      const updated = await acknowledgeAlert(id);
      if (mounted.current) {
        setAlerts((prev) => prev.map((a) => (a.id === id ? updated : a)));
      }
    } catch (e) {
      if (mounted.current) {
        setError(e instanceof Error ? e.message : "unknown error");
      }
    } finally {
      if (mounted.current) setBusyAlert(null);
    }
  }

  async function onToggleRule(rule: AlertRule) {
    setBusyRule(rule.id);
    setError(null);
    try {
      const updated = await setAlertRuleEnabled(rule.id, !rule.enabled);
      if (mounted.current) {
        setRules((prev) => prev.map((r) => (r.id === rule.id ? updated : r)));
      }
    } catch (e) {
      if (mounted.current) {
        setError(e instanceof Error ? e.message : "unknown error");
      }
    } finally {
      if (mounted.current) setBusyRule(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Cảnh báo</h1>
          <p className="text-sm text-muted-foreground">
            Alert đang kích hoạt và cấu hình rule của workspace.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()}>
          Làm mới
        </Button>
      </div>

      {error && (
        <p className="text-sm text-destructive" role="alert">
          Lỗi: {error}
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Đang kích hoạt</CardTitle>
          <CardDescription>
            Alert instance từ Alertmanager (GET /api/v1/alerts/active).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : alerts.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Không có alert nào đang kích hoạt.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tên</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead>Kích hoạt lúc</TableHead>
                  <TableHead className="text-right">Hành động</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {alerts.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell className="font-medium">{alertName(a)}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(a.status)}>{a.status}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <time dateTime={a.firedAt}>{a.firedAt}</time>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={a.status !== "firing" || busyAlert === a.id}
                        aria-label={
                          busyAlert === a.id
                            ? `Đang xử lý ${alertName(a)}`
                            : `Tiếp nhận ${alertName(a)}`
                        }
                        onClick={() => void onAcknowledge(a.id)}
                      >
                        {busyAlert === a.id ? "..." : "Tiếp nhận"}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Rule cảnh báo</CardTitle>
          <CardDescription>
            Cấu hình ngưỡng cảnh báo (GET /api/v1/alert-rules).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có rule nào.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tên</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead>Mức độ</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead className="text-right">Hành động</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className="font-medium">{r.name}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {r.service}
                    </TableCell>
                    <TableCell>
                      <Badge variant={severityVariant(r.severity)}>
                        {r.severity}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={r.enabled ? "default" : "outline"}>
                        {r.enabled ? "Bật" : "Tắt"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busyRule === r.id}
                        aria-label={`${r.enabled ? "Tắt" : "Bật"} rule ${r.name}`}
                        onClick={() => void onToggleRule(r)}
                      >
                        {busyRule === r.id ? "..." : r.enabled ? "Tắt" : "Bật"}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
