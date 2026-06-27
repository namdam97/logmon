// doc_tech — Test pyramid: unit (nhiều/nhanh) → integration → e2e (ít/chậm)
import { writeFileSync } from "node:fs";
import { Diagram } from "../src/builder.mjs";
import { frame, ossBox, renderTree } from "../src/layout-engine.mjs";

const OUT = process.env.OUT ?? new URL("../out/test-pyramid.drawio", import.meta.url);
const d = new Diagram("mesh");

// chiều rộng giảm dần lên đỉnh → gợi hình tháp
const e2e = ossBox("e2e", "E2E (Playwright) — ÍT, CHẬM\nflow người dùng thật (make e2e)", { fill: "#FDECEC", stroke: "#DD344C", w: 320, h: 56, bold: true, fs: 11 });
const integ = ossBox("integ", "Integration — VỪA\nAPI + DB thật (pgx/testcontainers), migrations", { fill: "#FDEEE0", stroke: "#ED7100", w: 560, h: 56, bold: true, fs: 11 });
const unit = ossBox("unit", "Unit — NHIỀU, NHANH\ntable-driven + testify/require, domain/app, go test -race", { fill: "#EAF3EC", stroke: "#5E9B57", w: 800, h: 56, bold: true, fs: 11 });

const pyramid = frame("pyr", "", { dir: "col", gap: 16, align: "center", header: 0, fill: "none", stroke: "none" }, [e2e, integ, unit]);
const note = ossBox("note", "Mục tiêu coverage ≥ 80% (CLAUDE.md/doc_v2/11) · TDD: RED→GREEN→REFACTOR · CI chạy cả 3 tầng (make test, make test-integration, make e2e)",
  { fill: "#FFF8E6", stroke: "#D6B656", w: 820, h: 44, fs: 10 });
const tree = frame("root", "", { dir: "col", gap: 28, align: "center", header: 0, pad: 14, fill: "none", stroke: "none" }, [pyramid, note]);

renderTree(d, tree, [40, 80]);
d.title("Test Pyramid trong LogMon — Unit (nền) → Integration → E2E (đỉnh)");

const res = d.validate();
console.log("testpyramid:", JSON.stringify({ ok: res.ok, advice: res.audit.advice }));
writeFileSync(OUT, d.mxfile("Test pyramid"));
