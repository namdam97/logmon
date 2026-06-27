// doc_tech — Kafka buffer Mode B cho log pipeline (KRaft, topic/partition, consumer group, DLQ)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, stage, icon, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/kafka-mode-b.drawio", import.meta.url);
const d = new Diagram("pipeline");

const produce = stage("p", 0, "Producer", [
  icon("otel", "opentelemetry", "OTel gateway\n(producer)"),
]);
const buffer = stage("b", 1, "Buffer (Kafka, KRaft)", [
  icon("kafka", "kafka", "topic: logs\nN partitions (theo key)"),
  ossBox("dlq", "topic: logs.DLQ\n(message lỗi parse/index)", { fill: "#FDECEC", stroke: "#DD344C", w: 176, h: 50, fs: 10 }),
]);
const consume = stage("c", 2, "Consumer group", [
  ossBox("idx", "ES indexer ×N\n(consumer group,\noffset commit sau khi index)", { fill: "#EAF2FB", stroke: "#3B6FB0", w: 184, h: 64, fs: 10, bold: true }),
]);
const store = stage("s", 3, "Store", [
  icon("es", "elasticsearch", "Elasticsearch\n(data streams)"),
]);

const tree = frame("root", "", { dir: "row", gap: 50, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("src", "Logs (OTLP)\n\nMode B = bật khi\ntải cao / ES chậm\n(ADR-018)"),
  frame("core", "Log pipeline — Mode B (Kafka buffer)", { dir: "row", gap: 44, fill: "#FAFBFC", stroke: "#9AA7B4" }, [produce, buffer, consume, store]),
]);
renderTree(d, tree, [40, 80]);
d.title("Kafka — buffer Mode B của log pipeline: hấp thụ burst, decouple ingest↔index, DLQ (planned, ADR-018)");
d.link("src", "otel", "logs", { flow: true });
d.link("otel", "kafka", "produce", { flow: true });
d.link("kafka", "idx", "consume (pull)", { flow: true });
d.link("idx", "es", "bulk index", { flow: true });
d.link("idx", "dlq", "lỗi → DLQ", { dash: true });
d.link("dlq", "idx", "retry/reprocess", { dash: true });

const res = d.validate();
console.log("kafka:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Kafka Mode B"));
