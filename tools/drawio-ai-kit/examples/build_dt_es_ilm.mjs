// doc_tech — Elasticsearch ILM lifecycle: data stream → hot → warm → cold → frozen → delete
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/es-ilm-lifecycle.drawio", import.meta.url);
const d = new Diagram("pipeline");

const phase = (id, label, fill, stroke) => ossBox(id, label, { fill, stroke, w: 150, h: 88, bold: true, fs: 10 });
const phases = frame("ph", "ILM policy — vòng đời index (rollover tự động)", { dir: "row", gap: 34, align: "center", fill: "#FAFBFC", stroke: "#9AA7B4" }, [
  phase("hot", "HOT\nghi + query nóng\nSSD nhanh\n~7 ngày", "#FDECEC", "#DD344C"),
  phase("warm", "WARM\nread-only\nrollover, force-merge\n~30 ngày", "#FDEEE0", "#ED7100"),
  phase("cold", "COLD\nsearchable snapshot\nlưu rẻ hơn\n~90 ngày", "#EAF2FB", "#3B6FB0"),
  phase("frozen", "FROZEN\nobject store (S3)\ntruy vấn chậm\n~1 năm", "#EEF1F5", "#5A6B7B"),
  phase("delete", "DELETE\nxoá theo\nretention", "#F2F2F2", "#999999"),
]);

const tree = frame("root", "", { dir: "row", gap: 56, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("ds", "logs-* data stream\n\nbacking indices\n(append-only)\nghi qua OTel/ES"),
  icon("es", "elasticsearch", "Elasticsearch"),
  phases,
]);

renderTree(d, tree, [40, 80]);
d.title("Elasticsearch — Data Streams + ILM (hot → warm → cold → frozen → delete)");
d.link("ds", "es", "index", { flow: true });
d.link("es", "hot", "write", { flow: true });
d.link("hot", "warm", "rollover");
d.link("warm", "cold", "age");
d.link("cold", "frozen", "age");
d.link("frozen", "delete", "max age");

const res = d.validate();
console.log("es-ilm:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("ES ILM lifecycle"));
