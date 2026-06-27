// drawio-ai-kit — THEME (design tokens / style system).
// LIGHT-ONLY (solid) palette: PALE solid fills + SOLID dark text. Rationale: the kit's box text and
// the catalog icon labels carry FIXED dark fontColors, so `light-dark()` fills (which flip dark when
// the VIEWER is in dark mode) produced dark-text-on-dark-fill = unreadable. Solid light fills are
// readable in EVERY viewer/theme. (Edge labels keep a solid white bg; the diagram title in
// builder.text stays theme-aware since it sits on the canvas, not on a frame.)
// AWS icons still carry the strong category color; frames stay pale; edges are clean 2px blue.

export const THEME = {
  // Per-stage frame tints — PALE solid, cohesive progression (green → orange → amber → purple → blue).
  stages: [
    "#eaf3ec", // 1 green   (ingest)
    "#fff3e9", // 2 orange  (process)
    "#fff8e6", // 3 amber   (store)
    "#f3eef8", // 4 purple  (serve)
    "#e9eef4", // 5 blue-grey
  ],
  stageStroke: ["#82B366", "#D79B00", "#D6B656", "#9673A6", "#6C8EBF"],

  base: "#ffffff",        // plain box / OSS component
  baseStroke: "#5A6B7B",
  endpoint: "#eaf3ff",    // source / consumer card (entry/exit)
  endpointStroke: "#6C8EBF",
  band: "#eef1f5",        // cross-cutting band (governance/security/ops)
  bandStroke: "#8593A3",
  // AWS-convention container colours (matches the common reference scheme).
  subnetPublic: "#eef5e6",  subnetPublicStroke: "#7AA116",   // public subnet — AWS green
  subnetPrivate: "#e6f4f4", subnetPrivateStroke: "#00A4A6",  // private subnet — AWS teal
  regionStroke: "#147EBA",                     // Region border — AWS blue (kept distinct from teal subnets)
  vpcStroke: "#8C4FFF",                        // VPC border — AWS networking purple
  accountStroke: "#C2487A",                    // Account border — magenta (like the reference)
  azStroke: "#2F9491",                         // Availability Zone / Local Zone — muted teal-green (dashed)
  note: "#fbe7d4",        // emphasis / callout note
  noteStroke: "#D79B00",
  onprem: "#eef1f5",      // on-premise / external site frame
  onpremStroke: "#666666",

  fontColor: "#1B2733",   // solid dark text (readable on the pale solid fills)
  edge: {
    stroke: "#2D6A9F",    // calm blue — the signature edge color
    strokeWidth: 2,
    fontColor: "#1B2733",
    labelBg: "#FFFFFF",
  },
  fonts: { title: 14, label: 11, small: 10 },
  gaps: { layer: 50, item: 16 },
};

export const stageFill = (i) => THEME.stages[i % THEME.stages.length];
export const stageStroke = (i) => THEME.stageStroke[i % THEME.stageStroke.length];
