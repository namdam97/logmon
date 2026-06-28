"use client";

import { topology, type TopologyGraph, type TopologyNode } from "@/lib/api";
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

// statusVariant: ánh xạ sức khoẻ node sang màu badge.
function statusVariant(
  status: TopologyNode["status"],
): "destructive" | "default" | "secondary" {
  if (status === "unhealthy") return "destructive";
  if (status === "degraded") return "default";
  return "secondary";
}

function ratePct(n: number): string {
  return `${(n * 100).toFixed(2)}%`;
}

export default function TopologyPage() {
  const { data, loading, error, reload } = useAsync<TopologyGraph>(topology);
  const nodes = data?.nodes ?? [];
  const edges = data?.edges ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Topology</h1>
          <p className="text-sm text-muted-foreground">
            Bản đồ phụ thuộc service suy từ traces (cửa sổ 1 giờ).
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
          <CardTitle>Health map</CardTitle>
          <CardDescription>
            Sức khoẻ từng service theo tỉ lệ lỗi outbound (GET /api/v1/topology).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-sm text-muted-foreground">Đang tải...</p>
          ) : nodes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Chưa có dữ liệu topology (cần traces có attribute peer.service).
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Service</TableHead>
                  <TableHead>Trạng thái</TableHead>
                  <TableHead className="text-right">Lượt gọi</TableHead>
                  <TableHead className="text-right">Lỗi</TableHead>
                  <TableHead className="text-right">Tỉ lệ lỗi</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.map((n) => (
                  <TableRow key={n.service}>
                    <TableCell className="font-medium">{n.service}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(n.status)}>{n.status}</Badge>
                    </TableCell>
                    <TableCell className="text-right">{n.callCount}</TableCell>
                    <TableCell className="text-right">{n.errorCount}</TableCell>
                    <TableCell className="text-right">{ratePct(n.errorRate)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Phụ thuộc</CardTitle>
          <CardDescription>Cạnh gọi service → service trong cửa sổ quan sát.</CardDescription>
        </CardHeader>
        <CardContent>
          {edges.length === 0 ? (
            <p className="text-sm text-muted-foreground">Chưa có cạnh phụ thuộc.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Từ</TableHead>
                  <TableHead>Đến</TableHead>
                  <TableHead className="text-right">Lượt gọi</TableHead>
                  <TableHead className="text-right">Tỉ lệ lỗi</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {edges.map((e) => (
                  <TableRow key={`${e.source}->${e.target}`}>
                    <TableCell className="font-medium">{e.source}</TableCell>
                    <TableCell className="text-muted-foreground">{e.target}</TableCell>
                    <TableCell className="text-right">{e.callCount}</TableCell>
                    <TableCell className="text-right">
                      <Badge variant={e.errorRate > 0.05 ? "destructive" : "secondary"}>
                        {ratePct(e.errorRate)}
                      </Badge>
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
