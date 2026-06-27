#!/usr/bin/env node
// drawio-ai-kit CLI — runs immediately, no MCP SDK required.
//   drawio-ai search <query> [--category C] [--limit N] [--kind icon|group]
//   drawio-ai style <name>
//   drawio-ai validate <file.drawio|file.xml> [--strict]
//   drawio-ai categories
//   drawio-ai principles

import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import {
  loadCatalog,
  searchIcon,
  getIcon,
  validateDiagram,
  auditAesthetics,
  listCategories,
} from "./core.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));

function parseFlags(args) {
  const flags = {};
  const positional = [];
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    if (a.startsWith("--")) {
      const key = a.slice(2);
      const next = args[i + 1];
      if (next && !next.startsWith("--")) {
        flags[key] = next;
        i++;
      } else flags[key] = true;
    } else positional.push(a);
  }
  return { flags, positional };
}

function out(obj) {
  process.stdout.write(JSON.stringify(obj, null, 2) + "\n");
}

const [cmd, ...rest] = process.argv.slice(2);
const { flags, positional } = parseFlags(rest);
const catalog = loadCatalog(flags.catalog);

switch (cmd) {
  case "search": {
    const q = positional.join(" ");
    if (!q) { console.error("A query is required. Example: drawio-ai search s3"); process.exit(1); }
    out(searchIcon(catalog, q, {
      category: flags.category,
      limit: flags.limit ? Number(flags.limit) : 8,
      kind: flags.kind,
    }));
    break;
  }
  case "style": {
    const name = positional[0];
    const icon = name ? getIcon(catalog, name) : null;
    if (!icon) { console.error(`Stencil "${name}" not found in catalog.`); process.exit(1); }
    out(icon);
    break;
  }
  case "validate": {
    const f = positional[0];
    if (!f) { console.error("A file is required. Example: drawio-ai validate diagram.drawio"); process.exit(1); }
    const xml = readFileSync(f, "utf8");
    const res = validateDiagram(catalog, xml, { strict: !!flags.strict });
    out(res);
    process.exit(res.ok ? 0 : 2);
    break;
  }
  case "audit": {
    const f = positional[0];
    if (!f) { console.error("A file is required. Example: drawio-ai audit diagram.drawio"); process.exit(1); }
    out(auditAesthetics(readFileSync(f, "utf8")));
    break;
  }
  case "logo": {
    const q = positional.join(" ");
    if (!q) { console.error("A brand is required. Example: drawio-ai logo openai"); process.exit(1); }
    const script = join(__dirname, "..", "vendor", "aiicons.py");
    const argv = [script, q, "--json"];
    if (flags.embed) argv.push("--embed");
    if (flags.variant) argv.push("--variant", String(flags.variant));
    try { process.stdout.write(execFileSync("python3", argv, { encoding: "utf8" })); }
    catch (e) { console.error("python3 is required to run aiicons.py:", e.message); process.exit(1); }
    break;
  }
  case "categories":
    out(listCategories(catalog));
    break;
  case "principles": {
    const base = join(__dirname, "..", "rules");
    for (const f of ["principles.md", "aws-architecture.md", "diagram-types.md"]) {
      process.stdout.write(readFileSync(join(base, f), "utf8") + "\n\n---\n\n");
    }
    break;
  }
  case "types": {
    const { listTypes } = await import("./types.mjs");
    out(listTypes());
    break;
  }
  default:
    console.error(
`drawio-ai-kit CLI
  search <query> [--category C] [--limit N] [--kind icon|group]
  style <name>
  validate <file> [--strict]
  audit <file>
  logo <brand> [--embed] [--variant color|mono|text]
  categories
  types
  principles
  [--catalog <path>]  override the default catalog (catalog/aws.json)`
    );
    process.exit(cmd ? 1 : 0);
}
