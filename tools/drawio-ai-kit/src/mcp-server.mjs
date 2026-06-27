#!/usr/bin/env node
// drawio-ai-kit — MCP server (stdio).
// Exposes 6 tools for the AI: search_icon, get_icon_style, validate_diagram, render_diagram,
// get_principles, brand_logo.
// Requires: npm i  (to get @modelcontextprotocol/sdk). If not installed, use src/cli.mjs.

import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import {
  loadCatalog,
  searchIcon,
  getIcon,
  validateDiagram,
  listCategories,
} from "./core.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const catalog = loadCatalog(process.env.DRAWIO_CATALOG);

let renderSeq = 0;
/** Locate the draw.io desktop CLI: env override → PATH → common install locations. */
function findDrawioCli() {
  if (process.env.DRAWIO_CLI && existsSync(process.env.DRAWIO_CLI)) return process.env.DRAWIO_CLI;
  try { const p = execFileSync("/bin/sh", ["-c", "command -v drawio"], { encoding: "utf8" }).trim(); if (p) return p; } catch { /* not on PATH */ }
  for (const p of ["/opt/homebrew/bin/drawio", "/usr/local/bin/drawio", "/usr/bin/drawio",
                   "/Applications/draw.io.app/Contents/MacOS/draw.io"]) if (existsSync(p)) return p;
  return null;
}

const TOOLS = [
  {
    name: "search_icon",
    description:
      "Search the icon catalog by keyword/category. Covers AWS stencils (mxgraph.aws4 family) AND non-AWS brand/tech icons from the OSS packs — Big Data (spark, kafka, airflow, flink, minio…), Database (postgres, mysql, mongodb, redis, clickhouse…), Databricks, CI/CD (jenkins, argocd, terraform, ansible…), Containers & Kubernetes (kubernetes, docker, helm, istio…), Observability (datadog, prometheus, grafana, opentelemetry…), Network & Gateway (nginx, kong, traefik…), AI/ML (pytorch, tensorflow, huggingface, ollama…). Returns the EXACT NAME + full draw.io style ready to paste. ALWAYS search here for ANY component (AWS or third-party) by its name instead of drawing a plain box or recalling stencil names from memory.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "Tool/service name or keyword, e.g. 's3', 'kubernetes', 'spark', 'postgres', 'kafka', 'cloud frame'." },
        category: { type: "string", description: "Filter by category, e.g. 'Compute', 'Storage', 'Big Data', 'Database', 'CI/CD', 'Containers & Kubernetes', 'AI / ML'." },
        kind: { type: "string", enum: ["icon", "group"], description: "icon = service; group = grouping frame (VPC/Region/AZ/AWS Cloud)." },
        limit: { type: "number", description: "Maximum number of results (default 8)." },
      },
      required: ["query"],
    },
  },
  {
    name: "get_icon_style",
    description: "Get the full draw.io style for a stencil by its exact name (e.g. 's3', 'eks').",
    inputSchema: {
      type: "object",
      properties: { name: { type: "string" } },
      required: ["name"],
    },
  },
  {
    name: "validate_diagram",
    description:
      "Validate draw.io XML: whether every resIcon/grIcon exists in the catalog (prevents blank icons caused by fabricated names), whether edges point to valid ids, and a few style lints. ALWAYS call this before returning a diagram to the user.",
    inputSchema: {
      type: "object",
      properties: {
        xml: { type: "string", description: "mxGraphModel XML content." },
        strict: { type: "boolean", description: "true = treat unknown stencils as errors instead of warnings." },
      },
      required: ["xml"],
    },
  },
  {
    name: "get_principles",
    description: "Return design principles for clean draw.io diagrams (grid, spacing, grouping, color by group, routing, labels) + AWS architecture presets + the full list of catalog categories WITH COUNTS (includes the non-AWS OSS packs: Big Data, Database, Databricks, CI/CD, Containers & Kubernetes, Observability, Network & Gateway, AI/ML) so you know which third-party tool icons are available to search_icon.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "render_diagram",
    description:
      "Render draw.io XML to a PNG and return the image so you can SEE the result and self-correct (the vision self-check). ALWAYS render after validate_diagram passes, look at the image, and fix any overlap/alignment/spacing issues before delivering. Requires the draw.io desktop CLI (set DRAWIO_CLI if it is not on PATH).",
    inputSchema: {
      type: "object",
      properties: {
        xml: { type: "string", description: "mxGraphModel / mxfile XML to render." },
        path: { type: "string", description: "Alternatively, an absolute path to a .drawio file (used if xml is omitted)." },
        scale: { type: "number", description: "Render scale (default 2 for crisp output)." },
        page: { type: "number", description: "Page index for multi-page files (default 0)." },
      },
    },
  },
  {
    name: "brand_logo",
    description:
      "Find brand logos (AI/LLM & some brands) as a draw.io 'image' style via lobe-icons. Use for components that do NOT have an AWS icon. Note: many OSS data-infra tools (Kafka/Starburst/MinIO/Dagster...) are not in lobe-icons — in that case load the SVG yourself via scripts/crawl_icons.py --mode base64. Requires python3.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "Brand name, e.g. 'openai', 'spark'." },
        embed: { type: "boolean", description: "Inline SVG base64 (portable, fetches from CDN immediately)." },
        variant: { type: "string", enum: ["color", "mono", "text"] },
      },
      required: ["query"],
    },
  },
];

const server = new Server(
  { name: "drawio-ai-kit", version: "0.1.0" },
  { capabilities: { tools: {} } }
);

server.setRequestHandler(ListToolsRequestSchema, async () => ({ tools: TOOLS }));

server.setRequestHandler(CallToolRequestSchema, async (req) => {
  const { name, arguments: args = {} } = req.params;
  const text = (obj) => ({ content: [{ type: "text", text: typeof obj === "string" ? obj : JSON.stringify(obj, null, 2) }] });
  try {
    switch (name) {
      case "search_icon":
        return text(searchIcon(catalog, args.query, { category: args.category, kind: args.kind, limit: args.limit ?? 8 }));
      case "get_icon_style": {
        const r = getIcon(catalog, args.name);
        return r ? text(r) : text(`Stencil "${args.name}" not found.`);
      }
      case "validate_diagram":
        return text(validateDiagram(catalog, args.xml, { strict: !!args.strict }));
      case "get_principles": {
        const base = join(__dirname, "..", "rules");
        const md = readFileSync(join(base, "principles.md"), "utf8");
        const aws = readFileSync(join(base, "aws-architecture.md"), "utf8");
        const types = readFileSync(join(base, "diagram-types.md"), "utf8");
        const style = readFileSync(join(base, "style-guide.md"), "utf8");
        return text([md, aws, types, style].join("\n\n---\n\n") + "\n\n## Icon groups available in the catalog\n" + JSON.stringify(listCategories(catalog), null, 2));
      }
      case "render_diagram": {
        const cli = findDrawioCli();
        if (!cli) return { content: [{ type: "text", text: "draw.io CLI not found. Install the draw.io desktop app, or set DRAWIO_CLI to its binary path. (On headless Linux, run it under xvfb-run.)" }], isError: true };
        let input = args.path;
        if (input) {
          // Hardening (R1): a caller-supplied path flows into the draw.io CLI. Restrict it to
          // diagram files that actually exist so a prompt-injected path (e.g. ~/.ssh/id_rsa, .env)
          // can't be opened/exfiltrated via the renderer.
          const resolved = resolve(input);
          if (!/\.(drawio|xml)$/i.test(resolved))
            return { content: [{ type: "text", text: "render_diagram: 'path' must point to a .drawio or .xml file." }], isError: true };
          if (!existsSync(resolved))
            return { content: [{ type: "text", text: `render_diagram: file not found: ${resolved}` }], isError: true };
          input = resolved;
        } else {
          if (!args.xml) return { content: [{ type: "text", text: "Provide either 'xml' or 'path'." }], isError: true };
          input = join(tmpdir(), `drawio-ai-${process.pid}-${++renderSeq}.drawio`);
          writeFileSync(input, args.xml.includes("<mxfile") ? args.xml : `<mxfile><diagram id="d">${args.xml}</diagram></mxfile>`);
        }
        const out = join(tmpdir(), `drawio-ai-${process.pid}-${renderSeq}.png`);
        try {
          // Hardening (R2): keep the Chromium sandbox ON by default. Only disable it on headless
          // Linux/CI (where the sandbox can't initialize) by exporting DRAWIO_NO_SANDBOX=1.
          const sandboxArgs = process.env.DRAWIO_NO_SANDBOX === "1" ? ["--no-sandbox"] : [];
          execFileSync(cli, ["-x", "-f", "png", "-s", String(args.scale ?? 2), "-p", String(args.page ?? 0), ...sandboxArgs, "-o", out, input], { encoding: "utf8", timeout: 60000, stdio: ["ignore", "pipe", "pipe"] });
        } catch (e) {
          return { content: [{ type: "text", text: `Render failed: ${e.message}. The draw.io CLI needs a display; on a server use xvfb-run.` }], isError: true };
        }
        if (!existsSync(out)) return { content: [{ type: "text", text: "Render produced no output file." }], isError: true };
        const b64 = readFileSync(out).toString("base64");
        return { content: [
          { type: "text", text: `Rendered to ${out}. Inspect the image for overlaps, misalignment, broken/overlapping edges, and frames that don't hug their content; fix and re-render if needed.` },
          { type: "image", data: b64, mimeType: "image/png" },
        ] };
      }
      case "brand_logo": {
        const script = join(__dirname, "..", "vendor", "aiicons.py");
        const argv = [script, String(args.query), "--json"];
        if (args.embed) argv.push("--embed");
        if (args.variant) argv.push("--variant", args.variant);
        try {
          const out = execFileSync("python3", argv, { encoding: "utf8", timeout: 20000 });
          return text(out.trim() || "No matching logo found.");
        } catch (e) {
          return { content: [{ type: "text", text: `Could not run aiicons.py (requires python3): ${e.message}` }], isError: true };
        }
      }
      default:
        return { content: [{ type: "text", text: `Tool does not exist: ${name}` }], isError: true };
    }
  } catch (err) {
    return { content: [{ type: "text", text: `Error: ${err.message}` }], isError: true };
  }
});

const transport = new StdioServerTransport();
await server.connect(transport);
console.error("drawio-ai-kit MCP server running (stdio). Catalog:", catalog.icons.length, "icons,", catalog.groups.length, "groups.");
