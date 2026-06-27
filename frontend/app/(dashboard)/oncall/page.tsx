"use client";

import {
  currentOnCall,
  listSchedules,
  type OnCallNow,
  type OnCallSchedule,
} from "@/lib/api";
import { useAsync } from "@/lib/use-async";
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

interface ScheduleWithCurrent extends OnCallSchedule {
  current: OnCallNow;
}

async function loadOnCall(): Promise<ScheduleWithCurrent[]> {
  const schedules = await listSchedules();
  const currents = await Promise.all(
    schedules.map((s) =>
      currentOnCall(s.id).catch(() => ({ primary: "—", secondary: "—" }) as OnCallNow),
    ),
  );
  return schedules.map((s, i) => ({ ...s, current: currents[i] }));
}

export default function OnCallPage() {
  const { data, loading, error, reload } = useAsync<ScheduleWithCurrent[]>(loadOnCall);
  const schedules = data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Trực ca</h1>
          <p className="text-sm text-muted-foreground">
            Lịch trực + người đang trực (primary/secondary) theo workspace.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()}>
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
          <CardTitle>Lịch trực</CardTitle>
          <CardDescription>GET /api/v1/oncall/schedules</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : schedules.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có lịch trực nào.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tên</TableHead>
                  <TableHead>Vòng xoay</TableHead>
                  <TableHead>Timezone</TableHead>
                  <TableHead>Primary</TableHead>
                  <TableHead>Secondary</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {schedules.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium">{s.name}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{s.rotation}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{s.timezone}</TableCell>
                    <TableCell>
                      <Badge variant="default">{s.current.primary}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{s.current.secondary}</TableCell>
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
