# `story-agents` — NPX Tool Design

> **Mục tiêu:** CLI tool cài qua `npx`, biến bất kỳ repo nào thành story-driven multi-agent workspace
> **Ngày:** 2026-03-19
> **Status:** DESIGN

---

## 1. Tổng Quan

```bash
npx story-agents init
# → Cài slash commands, agent definitions, config, templates
# → Dùng ngay: /ba:new-story, /dev:gen-tech-spec, /qc:gen-test-cases, ...
```

### 1.1 Triết Lý

- **Zero runtime** — tool chỉ gen files, Claude Code là runtime
- **Config-driven** — mọi thứ tùy chỉnh qua `.agile/config.json`
- **Project-agnostic** — Go, Python, TypeScript, Rust, bất kỳ stack nào
- **MVP first** — 8 commands cốt lõi, mở rộng sau

### 1.2 GSD vs story-agents

| | GSD | story-agents |
|---|-----|-------------|
| **Workflow** | Phase → Plan → Execute (linear) | Story → parallel (BA, Dev, QC, Sec) |
| **Users** | 1 developer + AI orchestra | Multi-user (real team + AI agents) |
| **Contract** | Không | OpenAPI as shared API contract |
| **Roles** | 15 fixed agents | 5 configurable roles |
| **Runtime** | Node.js engine (gsd-tools.cjs) | Zero runtime (Claude Code is engine) |
| **State** | STATE.md + CLI tools | Git tags + story directory |

---

## 2. Package Structure

```
story-agents/
├── package.json
├── bin/
│   └── cli.js                    ← npx entry point
├── src/
│   ├── commands/
│   │   ├── init.js               ← `npx story-agents init`
│   │   └── doctor.js             ← `npx story-agents doctor` (health check)
│   ├── generators/
│   │   ├── slash-commands.js     ← Gen .claude/commands/**/*.md
│   │   ├── agents.js             ← Gen .claude/agents/*.md
│   │   ├── config.js             ← Gen .agile/config.json
│   │   └── templates.js          ← Gen story templates
│   └── adapters/
│       ├── go.js                 ← Go-specific tech-spec prompts
│       ├── typescript.js         ← TS-specific tech-spec prompts
│       ├── python.js             ← Python-specific prompts
│       └── generic.js            ← Fallback
├── templates/
│   ├── commands/                 ← Slash command markdown templates
│   │   ├── ba/
│   │   ├── dev/
│   │   ├── dev-fe/
│   │   ├── qc/
│   │   ├── sec/
│   │   └── sprint/
│   ├── agents/                   ← Agent definition templates
│   ├── story.md.hbs              ← Story template (Handlebars)
│   └── config.json.hbs           ← Config template
└── README.md
```

### 2.1 package.json

```json
{
  "name": "story-agents",
  "version": "0.1.0",
  "description": "Story-driven multi-agent development for Claude Code",
  "bin": {
    "story-agents": "bin/cli.js"
  },
  "keywords": [
    "claude-code", "ai-agents", "agile", "scrum",
    "story-driven", "multi-agent", "openapi"
  ],
  "files": ["bin/", "src/", "templates/"],
  "engines": { "node": ">=18" }
}
```

---

## 3. CLI Commands

```bash
# Cài đặt vào project hiện tại
npx story-agents init

# Cài với options
npx story-agents init --stack go --arch clean-arch-ddd --roles ba,dev-be,qc

# Health check (kiểm tra files đã cài đúng chưa)
npx story-agents doctor

# Update commands (khi tool có version mới)
npx story-agents update
```

### 3.1 `init` Flow

```
npx story-agents init
  │
  ├── 1. Interactive prompts (hoặc --flags):
  │     ? Project name: logmon
  │     ? Backend stack: (go / typescript / python / rust / other)
  │     ? Frontend stack: (nextjs / react / vue / none)
  │     ? Architecture: (clean-arch / clean-arch-ddd-cqrs / layered / hexagonal)
  │     ? Roles: (ba, dev-be, dev-fe, qc, devsecops)  [multi-select]
  │     ? Use OpenAPI contract: (yes / no)
  │
  ├── 2. Generate files:
  │     .claude/
  │       commands/ba/new-story.md
  │       commands/ba/review.md
  │       commands/ba/release.md
  │       commands/dev/gen-tech-spec.md
  │       commands/dev/gen-openapi.md
  │       commands/dev/review.md
  │       commands/dev/sync.md
  │       commands/dev-fe/gen-client.md
  │       commands/dev-fe/gen-tech-spec.md
  │       commands/dev-fe/review.md
  │       commands/dev-fe/sync.md
  │       commands/qc/gen-test-cases.md
  │       commands/qc/gen-scripts.md
  │       commands/qc/bug-report.md
  │       commands/qc/sync.md
  │       commands/sec/review.md
  │       commands/sec/audit.md
  │       commands/sec/sync.md
  │       commands/sprint/init.md
  │       commands/sprint/status.md
  │       agents/ba.md
  │       agents/dev-be.md
  │       agents/dev-fe.md
  │       agents/qc.md
  │       agents/devsecops.md
  │     .agile/
  │       config.json
  │     stories/
  │       .gitkeep
  │
  ├── 3. Update .gitignore (nếu cần)
  │
  └── 4. Print summary:
        ✓ Installed 18 slash commands
        ✓ Installed 5 agent definitions
        ✓ Created .agile/config.json
        ✓ Created stories/ directory

        Ready! Try: /ba:new-story my-feature
```

---

## 4. Config System

### 4.1 `.agile/config.json`

```json
{
  "$schema": "https://story-agents.dev/schema/config.json",
  "version": "0.1.0",
  "project": {
    "name": "logmon",
    "description": "Logging & Monitoring Platform"
  },
  "stack": {
    "backend": "go",
    "frontend": "nextjs",
    "database": "postgresql",
    "api_style": "rest"
  },
  "architecture": {
    "pattern": "clean-arch-ddd-cqrs",
    "layers": ["domain", "app", "ports", "adapters"],
    "bounded_contexts": ["alerting", "slo", "logpipeline", "order", "user"]
  },
  "roles": {
    "ba": { "enabled": true, "model": "opus" },
    "dev-be": { "enabled": true, "model": "sonnet" },
    "dev-fe": { "enabled": true, "model": "sonnet" },
    "qc": { "enabled": true, "model": "opus" },
    "devsecops": { "enabled": true, "model": "sonnet" }
  },
  "conventions": {
    "commit_format": "feat({bc}/US-{id}): {description}",
    "branch_format": "feature/{bc}-{feature}",
    "story_id_prefix": "US",
    "test_framework": "testify",
    "linter": "golangci-lint"
  },
  "openapi": {
    "enabled": true,
    "version": "3.1.0",
    "output_dir": "tech"
  },
  "paths": {
    "stories": "stories",
    "agile": ".agile"
  }
}
```

### 4.2 Config Được Inject Vào Commands

Mỗi slash command đọc `.agile/config.json` khi chạy. Template engine dùng config values:

```markdown
<!-- .claude/commands/dev/gen-tech-spec.md -->
Read .agile/config.json for project context.
Read the story at stories/$ARGUMENTS/story.md.

You are a {{roles.dev-be.model}}-powered Backend Developer.
Tech stack: {{stack.backend}} with {{stack.database}}.
Architecture: {{architecture.pattern}}.
{{#if architecture.bounded_contexts}}
Bounded contexts: {{join architecture.bounded_contexts ", "}}.
{{/if}}

Generate tech-spec.md at stories/$ARGUMENTS/tech/tech-spec.md.
...
```

Nhưng Claude Code không có template engine. Thực tế: prompt chỉ hướng dẫn AI đọc config.json rồi adapt.

---

## 5. Slash Command Specs (MVP — 8 Commands)

### 5.1 `/ba:new-story <path>`

```markdown
# /ba:new-story

You are a Business Analyst AI Agent.

## Task
Create a new user story at `stories/$ARGUMENTS/story.md`.

## Steps
1. Read `.agile/config.json` to understand the project context
2. If the path contains a `/` (e.g., `alerting/create-rule`):
   - First segment = bounded context or feature group
   - Second segment = specific feature
3. Create directory `stories/$ARGUMENTS/` if it doesn't exist
4. Generate story.md using the template below
5. Ask the user clarifying questions about:
   - Who is the persona (user role)?
   - What do they want to achieve?
   - What are the acceptance criteria?
   - What is out of scope?

## Story Template
```yaml
---
id: {{generated US-XXX-NNN}}
title: {{title}}
bounded_context: {{first path segment}}
version: 0.0.0
priority: {{ask user}}
story_points: {{estimate}}
sprint: {{current or unassigned}}
status: draft
created: {{today}}
updated: {{today}}
---
```

## User Story
As a **{{persona}}**, I want to **{{goal}}**,
so that **{{benefit}}**.

## Acceptance Criteria
{{For each AC, use Given/When/Then format}}

## Business Rules
{{Domain-specific rules}}

## UI Requirements (if applicable)
{{UI specs if this story has frontend work}}

## Out of Scope
{{What is NOT included}}

## Rules
- EVERY AC must be testable (clear pass/fail)
- Use Given/When/Then format for all AC
- Do NOT include technical implementation details
- Reference other stories for out-of-scope items
- Story points: 1=trivial, 2=simple, 3=moderate, 5=complex, 8=very complex, 13=epic (should split)
```

### 5.2 `/ba:release <path> <version>`

```markdown
# /ba:release

You are a Business Analyst AI Agent performing a story release.

## Task
Release story at `stories/$ARGUMENTS` (parse path and version from arguments).

## Steps
1. Read the story at `stories/{path}/story.md`
2. Update the `version` field in frontmatter to the new version
3. Update `status` to `released`
4. Update `updated` to today's date
5. Stage and commit the file:
   `git add stories/{path}/story.md`
   `git commit -m "story({path}): release {version}"`
6. Create git tag: `git tag story/{path}/{version}`
7. If this is NOT v1.0.0 (i.e., an update):
   a. Run: `git diff story/{path}/{previous_version}..HEAD -- stories/{path}/story.md`
   b. Summarize what changed (added ACs, modified ACs, changed business rules)
   c. Print: "Dev, QC, and DevSecOps should run their /sync commands"

## Output
Print:
- ✓ Released {path} {version}
- ✓ Git tag: story/{path}/{version}
- Changes since {previous_version}: {summary} (if applicable)
```

### 5.3 `/dev:gen-tech-spec <path>`

```markdown
# /dev:gen-tech-spec

You are a Backend Developer AI Agent.

## Task
Generate a technical specification from a user story.

## Steps
1. Read `.agile/config.json` for project context
2. Read `stories/$ARGUMENTS/story.md`
3. Read `CLAUDE.md` for architecture rules and style guide
4. Generate `stories/$ARGUMENTS/tech/tech-spec.md`

## Tech Spec Structure

The tech-spec MUST include:

### 1. Architecture Decision
- Which bounded context does this belong to?
- What architecture pattern? (read from config)
- Key domain concepts: aggregates, entities, value objects
- CQRS: is this a command (write) or query (read)?

### 2. Implementation Plan
For each layer (based on config.architecture.layers):
- File name and purpose
- Key functions/methods
- Dependencies on other layers

### 3. AC Mapping
Map EVERY acceptance criteria to specific implementation:
| AC | Implementation | Layer |
|----|---------------|-------|

### 4. Domain Events (if cross-BC impact)
List any domain events this feature emits or consumes.

## Rules
- Follow architecture.pattern from config
- Follow layer direction: adapters → ports ← app → domain
- If config.openapi.enabled, note "see openapi.yaml for API contract"
- Do NOT generate code — only specify WHAT to build and WHERE
- Mark "Out of scope" ACs from the story clearly
```

### 5.4 `/dev:gen-openapi <path>`

```markdown
# /dev:gen-openapi

You are a Backend Developer AI Agent.

## Task
Generate OpenAPI spec from story + tech-spec.

## Steps
1. Read `.agile/config.json` (openapi.version, stack.api_style)
2. Read `stories/$ARGUMENTS/story.md` (ACs define the API behavior)
3. Read `stories/$ARGUMENTS/tech/tech-spec.md` (implementation details)
4. Generate `stories/$ARGUMENTS/tech/openapi.yaml`

## Rules
- OpenAPI version from config (default 3.1.0)
- Include request/response schemas with validation rules
- Map each AC to appropriate HTTP status code
- Include error response schemas
- Use $ref for reusable schemas
- This file is the SHARED CONTRACT: Dev-FE and QC will consume it
- Do NOT include internal implementation details
```

### 5.5 `/qc:gen-test-cases <path>`

```markdown
# /qc:gen-test-cases

You are a QC (Quality Assurance) AI Agent.

## Task
Generate test cases from a user story. This runs PARALLEL with Dev — no code dependency needed.

## Steps
1. Read `.agile/config.json` for project context
2. Read `stories/$ARGUMENTS/story.md`
3. If exists, read `stories/$ARGUMENTS/tech/openapi.yaml` (for API contract details)
4. Generate `stories/$ARGUMENTS/test/test-cases.md`

## Test Cases Structure

### Coverage Matrix
| AC | Test Cases | Coverage |

### For each test case:
- **ID**: TC-{NNN}
- **Type**: happy path / negative / boundary / edge case / integration / performance
- **Precondition**: what must be true before test
- **Input**: specific test data
- **Expected**: specific expected result
- **Priority**: critical / high / medium / low

## Rules
- EVERY AC must have ≥ 1 test case
- Include: happy path, at least 1 negative, boundary values
- If AC depends on another story, mark as DEFERRED with dependency link
- Do NOT write code — only test DESIGN
- Test cases should be specific enough that any developer can implement them
```

### 5.6 `/sec:review <path>`

```markdown
# /sec:review

You are a DevSecOps AI Agent.

## Task
Security review of a story and its technical specification.

## Steps
1. Read `.agile/config.json`
2. Read `stories/$ARGUMENTS/story.md`
3. Read `stories/$ARGUMENTS/tech/tech-spec.md` (if exists)
4. Read `stories/$ARGUMENTS/tech/openapi.yaml` (if exists)
5. Generate `stories/$ARGUMENTS/security/review.md`

## Security Review Checklist (OWASP-based)

### Input Validation
- [ ] All user inputs validated (types, lengths, formats)
- [ ] Parameterized queries (no SQL injection)
- [ ] Request body size limits

### Authentication & Authorization
- [ ] Auth required for this endpoint?
- [ ] Role-based access control needed?
- [ ] Token validation

### Data Protection
- [ ] Sensitive data in request/response? (PII, credentials)
- [ ] Encryption at rest/transit needed?
- [ ] Logging — no sensitive data logged?

### API Security
- [ ] Rate limiting needed?
- [ ] Input sanitization
- [ ] Error responses don't leak internals

### Infrastructure
- [ ] New infrastructure components needed?
- [ ] Network isolation requirements
- [ ] Secrets management

## Rules
- Rate each finding: CRITICAL / HIGH / MEDIUM / LOW / INFO
- Provide specific remediation for each finding
- If story is low-risk (e.g., read-only query), state that explicitly
```

### 5.7 `/dev:sync <path>` + `/qc:sync <path>` + `/sec:sync <path>`

```markdown
# /dev:sync (similar pattern for /qc:sync and /sec:sync)

## Task
Detect what needs updating after a story change.

## Steps
1. Read current story: `stories/$ARGUMENTS/story.md`
2. Get previous version tag from story frontmatter
3. Run: `git diff story/{path}/{prev_version}..HEAD -- stories/{path}/story.md`
4. Read current tech-spec (or test-cases, or security review)
5. Analyze: which sections of the artifact are affected by the story diff?

## Output
Print specific sections that need updating, with rationale.
Do NOT auto-update — only REPORT what needs changing.
```

---

## 6. Agent Definitions (`.claude/agents/`)

### 6.1 ba.md

```markdown
---
name: BA (Business Analyst)
model: opus
tools:
  - Read
  - Write
  - Glob
  - Grep
  - WebSearch
  - WebFetch
deny_tools:
  - Edit
  - Bash
---

You are a Business Analyst for the project defined in `.agile/config.json`.

## Your Responsibilities
- Write user stories with Given/When/Then acceptance criteria
- Review stories for gaps, ambiguity, and testability
- Version and release stories via git tags
- Analyze impact when stories change

## You Must NOT
- Write or modify source code
- Run shell commands
- Make technical architecture decisions (that's Dev's job)
- Write test cases (that's QC's job)

## Context Files
Always read these before starting:
- `.agile/config.json` — project configuration
- `CLAUDE.md` — project overview
- Existing stories in `stories/` — for consistency
```

### 6.2 dev-be.md

```markdown
---
name: DEV-BE (Backend Developer)
model: sonnet
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
deny_tools:
  - WebSearch
  - WebFetch
---

You are a Backend Developer for the project defined in `.agile/config.json`.

## Your Responsibilities
- Generate tech specs that map stories to architecture layers
- Generate OpenAPI specs as shared API contracts
- Implement backend code following project architecture
- Write unit and integration tests
- Follow coding standards from CLAUDE.md

## You Must NOT
- Modify frontend code
- Modify infrastructure/deployment code
- Change story files (that's BA's job)
- Search the web during implementation

## Context Files
Always read these before starting:
- `.agile/config.json` — tech stack, architecture, conventions
- `CLAUDE.md` — full guide (architecture rules, style guide, security)
- The relevant story file
```

### 6.3 dev-fe.md, qc.md, devsecops.md

_(Tương tự pattern trên, với role-specific responsibilities và tool permissions.)_

---

## 7. Tech Stack Adapters

Prompt content thay đổi theo `config.stack.backend`:

### 7.1 Go Adapter

Khi `stack.backend = "go"`, tech-spec prompt thêm:

```
## Go-Specific Rules
- Follow Uber Go Style Guide
- Error wrapping: fmt.Errorf("context: %w", err)
- Interface compliance: var _ ports.X = (*Adapter)(nil)
- Table-driven tests with testify/require
- Functional options for constructors
```

### 7.2 TypeScript Adapter

Khi `stack.backend = "typescript"`, tech-spec prompt thêm:

```
## TypeScript-Specific Rules
- Strict mode, no `any`
- Use zod for runtime validation
- Dependency injection via constructor
- Jest for testing
```

### 7.3 Python Adapter

Khi `stack.backend = "python"`:

```
## Python-Specific Rules
- Type hints (PEP 484)
- Pydantic for validation
- pytest for testing
- Follow PEP 8
```

Adapters được chọn tự động dựa trên config và inject vào command prompts.

---

## 8. Implementation Plan

### Phase 1: MVP (tuần 1)

| Task | Deliverable |
|------|------------|
| Scaffold npm package | `package.json`, `bin/cli.js`, basic structure |
| `init` command | Interactive prompts → generate all files |
| 8 core slash commands | BA (3) + Dev-BE (2) + QC (1) + Sec (1) + Dev-sync (1) |
| 5 agent definitions | BA, Dev-BE, Dev-FE, QC, DevSecOps |
| Go adapter | Go-specific prompts for tech-spec |
| Config system | `.agile/config.json` generation + reading |

### Phase 2: Complete Commands (tuần 2)

| Task | Deliverable |
|------|------------|
| Remaining slash commands | Dev-FE (4) + QC (3) + Sec (2) + Sprint (4) = 13 more |
| TypeScript adapter | TS-specific prompts |
| `doctor` command | Health check: verify all files installed correctly |
| `update` command | Update commands to latest version |

### Phase 3: Polish (tuần 3)

| Task | Deliverable |
|------|------------|
| Python adapter | Python-specific prompts |
| Story template customization | User can override story.md template |
| README + docs | npm README, usage guide |
| Publish to npm | `npm publish` |

### Phase 4: Community (sau release)

| Task | Deliverable |
|------|------------|
| Custom role support | Users define their own agent roles |
| Plugin system | Community adapters for other stacks |
| CI integration | GitHub Action that validates story format |

---

## 9. Ví Dụ Sử Dụng

### 9.1 Go + Clean Architecture (LogMon)

```bash
npx story-agents init --stack go --frontend nextjs --arch clean-arch-ddd-cqrs
# → Config cho Go + Next.js + DDD/CQRS

/ba:new-story alerting/create-rule
/ba:release alerting/create-rule v1.0.0
/dev:gen-tech-spec alerting/create-rule    # → DDD layer mapping
/dev:gen-openapi alerting/create-rule      # → OpenAPI 3.1 spec
/qc:gen-test-cases alerting/create-rule    # → Song song với Dev
```

### 9.2 TypeScript + Next.js Full-Stack

```bash
npx story-agents init --stack typescript --frontend nextjs --arch layered
# → Config cho TS full-stack

/ba:new-story auth/login
/dev:gen-tech-spec auth/login              # → Service layer mapping
/dev:gen-openapi auth/login                # → tRPC or REST spec
```

### 9.3 Python + FastAPI

```bash
npx story-agents init --stack python --frontend none --arch hexagonal
# → Config cho Python backend only, no frontend role

/ba:new-story payment/process-order
/dev:gen-tech-spec payment/process-order   # → Hexagonal ports/adapters
```

---

## 10. Tại Sao Không Dùng GSD?

| Điểm | GSD | story-agents |
|------|-----|-------------|
| Installed as | Global CLI + runtime engine | Just files (Claude Code is engine) |
| Maintenance | Complex (Node.js runtime, state machine) | Simple (markdown templates only) |
| Update | Re-install entire package | `npx story-agents update` replaces .md files |
| Customization | Fork entire repo | Edit config.json + override templates |
| Multi-user | Not supported | First-class: each person runs own commands |
| Learning curve | 37 commands to learn | 8 core commands, rest optional |

---

*Design document cho `story-agents` NPX tool. Khi Boss NamDam confirm, bắt đầu implement Phase 1.*
