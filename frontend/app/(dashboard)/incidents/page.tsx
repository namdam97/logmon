"use client";

import { useState } from "react";

import {
  listIncidents,
  transitionIncident,
  type Incident,
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

function severityVariant(s?: string): "destructive" | "default" | "secondary" {
  if (s === "SEV1" || s === "SEV2") return "destructive";
  if (s === "SEV3") return "default";
  return "secondary";
}

function statusVariant(s: string): "destructive" | "default" | "secondary" | "outline" {
  switch (s) {
    case "open":
      return "destructive";
    case "closed":
      return "outline";
    case "resolved":
    case "postmortem_pending":
      return "secondary";
    default:
      return "default";
  }
}

const _activeStatuses = new Set(["open", "triaged", "assigned", "mitigating"]);
const _closableStatuses = new Set(["resolved", "postmortem_pending"]);

function mins(seconds?: number): string {
  if (!seconds) return "—";
  return `${Math.round(seconds / 60)}m`;
}

export default function IncidentsPage() {
  const { data, loading, error, reload } = useAsync<Incident[]>(() => listIncidents());
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const incidents = data ?? [];

  async function act(id: string, action: "resolve" | "close") {
    setBusy(id);
    setActionError(null);
    try {
      await transitionIncident(id, action);
      await reload();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "unknown error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Sự cố</h1>
          <p className="text-sm text-muted-foreground">
            Bảng điều phối incident: trạng thái, mức độ, MTTA/MTTR.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()}>
          Làm mới
        </Button>
      </div>

      {(error || actionError) && (
        <p className="text-sm text-destructive" role="alert">
          Lỗi: {error ?? actionError}
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Danh sách sự cố</CardTitle>
          <CardDescription>GET /api/v1/incidents</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : incidents.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có sự cố nào.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tiêu đề</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead>Mức độ</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead className="text-right">MTTA</TableHead>
                  <TableHead className="text-right">MTTR</TableHead>
                  <TableHead className="text-right">Hành động</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {incidents.map((i) => (
                  <TableRow key={i.id}>
                    <TableCell className="font-medium">{i.title}</TableCell>
                    <TableCell className="text-muted-foreground">{i.service}</TableCell>
                    <TableCell>
                      <Badge variant={severityVariant(i.severity)}>{i.severity ?? "—"}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(i.status)}>{i.status}</Badge>
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">{mins(i.mttaSeconds)}</TableCell>
                    <TableCell className="text-right text-muted-foreground">{mins(i.mttrSeconds)}</TableCell>
                    <TableCell className="text-right">
                      {_activeStatuses.has(i.status) && (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={busy === i.id}
                          aria-label={`Giải quyết ${i.title}`}
                          onClick={() => void act(i.id, "resolve")}
                        >
                          {busy === i.id ? "..." : "Giải quyết"}
                        </Button>
                      )}
                      {_closableStatuses.has(i.status) && (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy === i.id}
                          aria-label={`Đóng ${i.title}`}
                          onClick={() => void act(i.id, "close")}
                        >
                          {busy === i.id ? "..." : "Đóng"}
                        </Button>
                      )}
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
