// doc_tech — Thanos components (long-term metrics, global query, downsampling)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, icon, ossBox, renderTree } from "../src/layout-engine.mjs";
import { GRAFANA_ICON } from "./_grafana_icon.mjs";

const OUT = process.env.OUT ?? new URL("../out/thanos-components.drawio", import.meta.url);
const d = new Diagram("hubspoke");
d.c.byName.set(GRAFANA_ICON.name, { ...GRAFANA_ICON, kind: "icon" });
d.c.validNames.add(GRAFANA_ICON.name);
const c = (id, label) => ossBox(id, label, { fill: "#F3EEF8", stroke: "#6D4FB3", w: 168, h: 56, bold: true, fs: 10 });

const prom = frame("pg", "Mỗi cụm Prometheus", { dir: "col", gap: 10, fill: "#FAFBFC", stroke: "#9AA7B4" }, [
  icon("prom", "prometheus", "Prometheus\n(scrape, TSDB local)"),
  c("sidecar", "Thanos Sidecar\nupload block → object store\n+ phục vụ query gần"),
]);
const store = ossBox("obj", "Object Storage\n(S3 / GCS / MinIO)\nlưu metric dài hạn", { fill: "#FFF8E6", stroke: "#D6B656", w: 184, h: 76, bold: true, fs: 10 });
const back = frame("bg", "Thành phần Thanos", { dir: "col", gap: 14, fill: "#FAFBFC", stroke: "#9AA7B4" }, [
  c("sg", "Store Gateway\nđọc block từ object store"),
  c("compactor", "Compactor\ncompaction + downsampling\n(5m, 1h)"),
  c("ruler", "Ruler (rule/alert\ntrên dữ liệu global)"),
]);
const querier = c("querier", "Querier\nfan-out + DEDUP\n→ global view (PromQL)");

const tree = frame("root", "", { dir: "row", gap: 56, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  prom, store, back,
  frame("q", "", { dir: "col", gap: 24, header: 0, fill: "none", stroke: "none" }, [querier, icon("grafana", "grafana", "Grafana")]),
]);
renderTree(d, tree, [40, 80]);
d.title("Thanos — metrics dài hạn, global query, downsampling (planned, doc_v2/04+10)");
d.link("sidecar", "obj", "upload blocks", { flow: true });
d.link("obj", "sg", "read", { dir: "LR" });
d.link("compactor", "obj", "compact/downsample", { dash: true });
d.link("querier", "sidecar", "StoreAPI", { role: "fanout" });
d.link("querier", "sg", "StoreAPI", { role: "fanout" });
d.link("querier", "ruler", "StoreAPI", { role: "fanout" });
d.link("grafana", "querier", "PromQL");

const res = d.validate();
console.log("thanos:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Thanos components"));
