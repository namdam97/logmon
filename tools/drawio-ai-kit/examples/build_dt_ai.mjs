// doc_tech — GĐ5 AI Incident Automation flow (HolmesGPT + WeKnora, MCP, human-in-loop)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, endpoint, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/ai-automation-flow.drawio", import.meta.url);
const d = new Diagram("pipeline");

const aiBox = (id, label) => ossBox(id, label, { fill: "#E8F7F1", stroke: "#01A88D", w: 184, h: 64, bold: true, fs: 10 });

const ai = frame("ai", "AI service (Python — NGOÀI Go core, ADR-032)", { dir: "col", gap: 16, fill: "#EAFBF4", stroke: "#01A88D" }, [
  aiBox("holmes", "HolmesGPT\nđiều tra / RCA tự động\n(đọc metrics·logs·traces)"),
  aiBox("weknora", "WeKnora (RAG)\nrunbooks · postmortem cũ\n→ ngữ cảnh khắc phục"),
]);
const gate = ossBox("hil", "Human-in-loop (L2)\nDuyệt / từ chối\nhành động đề xuất", { fill: "#FFF8E6", stroke: "#D6B656", w: 176, h: 64, bold: true, fs: 10 });

const tree = frame("root", "", { dir: "row", gap: 56, align: "center", header: 0, pad: 12, fill: "none", stroke: "none" }, [
  endpoint("core", "LogMon Go core\n\nincident BC\nphát IncidentOpened\n(SEV1/2)"),
  ossBox("mcp", "MCP / webhook\n/ event\n(ranh giới tích hợp)", { fill: "#EEF1F5", stroke: "#5A6B7B", w: 150, h: 76, fs: 10 }),
  ai,
  ossBox("rca", "RCA + đề xuất\nhành động\n(có độ tin cậy)", { fill: "#F3EEF8", stroke: "#8C4FFF", w: 164, h: 64, bold: true, fs: 10 }),
  gate,
  endpoint("apply", "Áp dụng\n\nincident: cập nhật\nnotification: gửi\n→ giảm MTTR"),
]);

renderTree(d, tree, [40, 80]);
d.title("GĐ5 — AI Incident Automation: điều tra → RAG → đề xuất → human-in-loop → giảm MTTR (planned)");
d.link("core", "mcp", "incident event", { flow: true });
d.link("mcp", "holmes", "trigger điều tra", { flow: true });
d.link("holmes", "weknora", "truy hồi ngữ cảnh");
d.link("weknora", "rca", "tổng hợp", { flow: true });
d.link("rca", "hil", "đề xuất", { flow: true });
d.link("hil", "apply", "đã duyệt", { flow: true });

const res = d.validate();
console.log("ai:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("AI incident automation"));
