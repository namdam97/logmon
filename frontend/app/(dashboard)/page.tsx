"use client";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

// Số liệu mẫu — chưa nối API metrics (cần endpoint slo/alerting ở GĐ sau).
const STATS = [
  { label: "Services", value: "—", hint: "đang giám sát" },
  { label: "Alerts (24h)", value: "—", hint: "đang kích hoạt" },
  { label: "Error budget", value: "—", hint: "còn lại" },
  { label: "Log throughput", value: "—", hint: "sự kiện/giây" },
];

export default function OverviewPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Tổng quan</h1>
        <p className="text-sm text-muted-foreground">
          Bảng điều khiển observability của LogMon.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {STATS.map((s) => (
          <Card key={s.label}>
            <CardHeader className="pb-2">
              <CardDescription>{s.label}</CardDescription>
              <CardTitle className="text-3xl">{s.value}</CardTitle>
            </CardHeader>
            <CardContent className="text-xs text-muted-foreground">
              {s.hint}
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Bắt đầu</CardTitle>
          <CardDescription>
            Walking skeleton — các widget số liệu sẽ nối với metrics/alerting BC ở
            giai đoạn sau.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  );
}
