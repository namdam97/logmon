"use client";

import { useState } from "react";

import {
  listDLQ,
  pipelineStatus,
  retryDLQ,
  type DLQList,
  type PipelineStatus,
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

interface PipelineData {
  status: PipelineStatus;
  dlq: DLQList;
}

async function loadPipeline(): Promise<PipelineData> {
  const [status, dlq] = await Promise.all([pipelineStatus(), listDLQ()]);
  return { status, dlq };
}

function healthVariant(s: string): "default" | "destructive" | "outline" {
  if (s === "up") return "default";
  if (s === "down") return "destructive";
  return "outline";
}

export default function PipelinePage() {
  const { data, loading, error, reload } = useAsync<PipelineData>(loadPipeline);
  const [busy, setBusy] = useState<number | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const status = data?.status;
  const entries = data?.dlq.entries ?? [];

  async function onRetry(id: number) {
    setBusy(id);
    setNotice(null);
    try {
      const res = await retryDLQ([id]);
      setNotice(res.retried.includes(id) ? "Đã retry entry." : "Retry thất bại.");
      await reload();
    } catch (e) {
      setNotice(e instanceof Error ? e.message : "unknown error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Pipeline</h1>
          <p className="text-sm text-muted-foreground">
            Trạng thái log pipeline + dead letter queue của workspace.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void reload()}>
          Làm mới
        </Button>
      </div>

      {(error || notice) && (
        <p
          className={error ? "text-sm text-destructive" : "text-sm text-muted-foreground"}
          role={error ? "alert" : "status"}
        >
          {error ? `Lỗi: ${error}` : notice}
        </p>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Chế độ</CardDescription>
            <CardTitle className="text-2xl">{loading ? "…" : `Mode ${status?.mode ?? "?"}`}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Elasticsearch</CardDescription>
            <CardTitle>
              <Badge variant={healthVariant(status?.health.elasticsearch ?? "unknown")}>
                {status?.health.elasticsearch ?? "—"}
              </Badge>
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Collector / Kafka</CardDescription>
            <CardTitle className="text-sm">
              {status?.health.collector ?? "—"} / {status?.health.kafka ?? "—"}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Data streams</CardDescription>
            <CardTitle className="text-2xl">{status?.dataStreams ?? 0}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Dead Letter Queue</CardTitle>
          <CardDescription>
            Log không nạp được — review rồi retry thủ công (GET /api/v1/pipeline/dlq).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : entries.length === 0 ? (
            <p className="text-sm text-muted-foreground">DLQ trống.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead>Lý do lỗi</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead className="text-right">Hành động</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="font-medium">{e.id}</TableCell>
                    <TableCell className="text-muted-foreground">{e.sourceService ?? "—"}</TableCell>
                    <TableCell className="max-w-md truncate text-muted-foreground" title={e.errorReason}>
                      {e.errorReason}
                    </TableCell>
                    <TableCell>
                      <Badge variant={e.status === "pending" ? "destructive" : "secondary"}>
                        {e.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={e.status !== "pending" || busy === e.id}
                        aria-label={`Retry entry ${e.id}`}
                        onClick={() => void onRetry(e.id)}
                      >
                        {busy === e.id ? "..." : "Retry"}
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
