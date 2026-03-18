# LogMon AI Agent Team — Agile/Scrum Multi-Agent System

> **Dự án:** LogMon — Logging & Monitoring Platform
> **Inspired by:** [get-shit-done](https://github.com/gsd-build/get-shit-done) (GSD)
> **Ngày thiết kế:** 2026-03-18
> **Status:** DRAFT — Chờ Boss NamDam confirm

---

## 1. Tổng Quan

### 1.1 Vấn Đề

Một developer dùng Claude Code cho dự án lớn gặp 3 vấn đề:

| Vấn đề | Triệu chứng | Hậu quả |
|--------|-------------|---------|
| **Context rot** | AI "quên" architecture rules sau ~50K tokens | Code vi phạm DDD boundaries, chất lượng giảm dần |
| **Single-role bottleneck** | 1 AI làm tất cả: requirements, code, test, deploy | Thiếu kiểm tra chéo, bugs lọt qua |
| **No structure** | Mỗi session bắt đầu lại từ đầu | Không có tiến độ tích lũy giữa các sessions |

### 1.2 Giải Pháp

5 AI Agents chuyên biệt, giao tiếp qua **file-based bus** (`.agile/`), làm việc theo **Scrum sprints**:

```
┌─────────────────────────────────────────────────────────┐
│                    SCRUM MASTER (User)                    │
│              Điều phối qua slash commands                 │
└───────────┬──────────┬──────────┬──────────┬────────────┘
            │          │          │          │
    ┌───────▼──┐ ┌─────▼────┐ ┌──▼─────┐ ┌─▼────────┐ ┌──────────┐
    │    BA    │ │  DEV-BE  │ │ DEV-FE │ │    QA    │ │ DevSecOps│
    │ Analyst  │ │ Backend  │ │Frontend│ │ Testing  │ │ Infra+Sec│
    └────┬─────┘ └────┬─────┘ └───┬────┘ └────┬─────┘ └────┬─────┘
         │            │           │            │             │
         └────────────┴───────────┴────────────┴─────────────┘
                              │
                    ┌─────────▼─────────┐
                    │   .agile/         │
                    │   File-Based Bus  │
                    │   (Communication) │
                    └───────────────────┘
```

### 1.3 Nguyên Tắc Thiết Kế (từ GSD)

| Nguyên tắc | Mô tả |
|------------|-------|
| **File-based communication** | Agents KHÔNG giao tiếp trực tiếp. Tất cả qua files trong `.agile/` |
| **Fresh context per agent** | Mỗi agent spawn với fresh context window, tự đọc files cần thiết |
| **Thin orchestrator** | User (Scrum Master) chỉ chạy commands, agents tự coordinate qua files |
| **Principle of least privilege** | Mỗi agent chỉ có tools cần thiết cho role của mình |
| **Defense in depth** | Nhiều layers kiểm tra: BA validates requirements, QA validates code, DevSecOps validates security |

---

## 2. Năm AI Agents

### 2.1 BA (Business Analyst)

**Nhiệm vụ:** Phân tích requirements, viết user stories, định nghĩa acceptance criteria, maintain product backlog.

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Opus (cần reasoning sâu cho domain analysis) |
| **Tools** | Read, Write, Glob, Grep, WebSearch, WebFetch |
| **KHÔNG có** | Edit, Bash (không sửa code, không chạy commands) |
| **Đọc** | `doc/logmon.md`, `.agile/PRODUCT-BACKLOG.md`, `.agile/STATE.md` |
| **Viết** | `.agile/PRODUCT-BACKLOG.md`, `.agile/sprints/{N}/stories/*.md`, `.agile/sprints/{N}/SPRINT-BACKLOG.md` |

**Quy tắc:**
- Mọi user story PHẢI có format: `As a [persona], I want [goal], so that [benefit]`
- Mọi story PHẢI có Acceptance Criteria dạng Given/When/Then
- Stories PHẢI map tới Bounded Context (alerting, slo, logpipeline, order, user)
- PHẢI gắn REQ-ID traceable (REQ-ALT-001, REQ-SLO-001, ...)
- PHẢI estimate Story Points (1, 2, 3, 5, 8, 13) dựa trên complexity
- KHÔNG viết technical implementation details — chỉ business requirements

**Output format (User Story):**
```markdown
---
id: US-ALT-001
title: Tạo Alert Rule mới
bounded_context: alerting
priority: high
story_points: 5
requirements: [REQ-ALT-001, REQ-ALT-002]
sprint: 1
status: ready
---

## User Story
As a **SRE**, I want to **create a new alert rule with PromQL expression**,
so that **I get notified when a service exceeds error threshold**.

## Acceptance Criteria
- [ ] **AC1:** Given valid PromQL expression, When SRE submits rule, Then rule is saved and evaluation starts within 15s
- [ ] **AC2:** Given invalid PromQL expression, When SRE submits rule, Then validation error is returned with specific reason
- [ ] **AC3:** Given rule with severity=critical, When alert fires, Then Slack notification is sent within 5 minutes
- [ ] **AC4:** Given duplicate rule name, When SRE submits, Then conflict error is returned

## Notes
- PromQL validation nên check syntax trước khi save
- Cần support 3 severity levels: critical, warning, info
```

---

### 2.2 DEV-BE (Backend Developer)

**Nhiệm vụ:** Implement Go backend theo Clean Architecture + DDD + CQRS. Viết code, unit tests, integration tests.

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet (balanced speed/quality cho code generation) |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch (không research, chỉ implement) |
| **Đọc** | User stories từ BA, `CLAUDE.md`, `doc/logmon.md` Section 8-9, `.agile/sprints/{N}/tasks/BE-*.md` |
| **Viết** | `backend/internal/**/*.go`, `.agile/sprints/{N}/tasks/BE-*-DONE.md` |

**Quy tắc:**
- PHẢI tuân thủ layer direction: `adapters → ports ← app → domain`
- PHẢI verify interface compliance: `var _ ports.X = (*Adapter)(nil)`
- PHẢI viết unit tests cho domain logic (table-driven, `require` not `assert`)
- PHẢI viết integration tests cho adapters
- Mỗi task = 1 atomic git commit với message format: `feat(alerting/BE-001): implement CreateRule command handler`
- Error handling theo decision matrix (Section 9.2)
- KHÔNG sửa frontend code
- KHÔNG sửa infrastructure code

**Output format (Task Done):**
```markdown
---
task_id: BE-ALT-001
story_id: US-ALT-001
status: done
files_created:
  - backend/internal/alerting/domain/alert_rule.go
  - backend/internal/alerting/domain/errors.go
  - backend/internal/alerting/app/command/create_rule.go
  - backend/internal/alerting/ports/repository.go
files_modified: []
tests_added:
  - backend/internal/alerting/domain/alert_rule_test.go
  - backend/internal/alerting/app/command/create_rule_test.go
test_results: "ok  alerting/domain 0.003s | ok  alerting/app/command 0.012s"
commits:
  - "feat(alerting/BE-ALT-001): implement AlertRule aggregate and CreateRule command"
deviations: []
---

## Summary
Implemented AlertRule aggregate root with CreateRule command handler.
Domain validates PromQL expression format and severity enum.
All 12 test cases pass (8 domain + 4 command handler).

## Verification
- `go test ./internal/alerting/...` — all pass
- `go vet ./internal/alerting/...` — no issues
- Interface compliance: `var _ ports.AlertRuleRepository = (*PostgresAlertRepo)(nil)` — verified
```

---

### 2.3 DEV-FE (Frontend Developer)

**Nhiệm vụ:** Implement Next.js frontend. UI components, pages, API client, TypeScript types.

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Đọc** | User stories từ BA, UI specs, `.agile/sprints/{N}/tasks/FE-*.md` |
| **Viết** | `frontend/**/*.{tsx,ts,css}`, `.agile/sprints/{N}/tasks/FE-*-DONE.md` |

**Quy tắc:**
- TypeScript strict mode, NO `any` types
- Components dùng shadcn/ui + TailwindCSS
- API client layer trong `services/` — KHÔNG gọi fetch trực tiếp từ components
- Mỗi page PHẢI có loading state, error state, empty state
- KHÔNG sửa backend code
- KHÔNG sửa infrastructure code
- Responsive design (mobile-first)

---

### 2.4 QA (Quality Assurance)

**Nhiệm vụ:** Verify code quality, test coverage, acceptance criteria. Review output của DEV-BE và DEV-FE.

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Opus (cần reasoning sâu cho verification) |
| **Tools** | Read, Bash, Glob, Grep |
| **KHÔNG có** | Write, Edit (READ-ONLY — không sửa code, chỉ report) |
| **Đọc** | User stories (AC), code từ DEV-BE/DEV-FE, test results, `*-DONE.md` files |
| **Viết** | `.agile/sprints/{N}/reviews/QA-REPORT.md` (chỉ file duy nhất) |

**Quy tắc:**
- Verify TỪNG Acceptance Criteria (pass/fail với evidence)
- Chạy `go test ./...` và `pnpm test` — report kết quả
- Kiểm tra DDD layer violations (domain import infrastructure?)
- Kiểm tra error handling (có `_ = err` nào không?)
- Kiểm tra test coverage (domain logic PHẢI có unit tests)
- KHÔNG tự sửa code — chỉ report bugs với file:line reference
- Nếu có issues: tạo BUG entries trong QA-REPORT.md

**Output format (QA Report):**
```markdown
---
sprint: 1
phase: review
stories_verified: [US-ALT-001, US-ALT-002]
verdict: PASS_WITH_ISSUES
bugs_found: 2
coverage_backend: 78%
coverage_frontend: 65%
---

## Acceptance Criteria Verification

### US-ALT-001: Tạo Alert Rule mới
| AC | Status | Evidence |
|----|--------|----------|
| AC1: Valid PromQL → saved | PASS | `go test -run TestCreateRule/valid_expression` — PASS |
| AC2: Invalid PromQL → error | PASS | `go test -run TestCreateRule/invalid_expression` — PASS |
| AC3: Critical → Slack | FAIL | SlackNotifier adapter chưa implement |
| AC4: Duplicate name → error | PASS | `go test -run TestCreateRule/duplicate_name` — PASS |

## Bugs Found

### BUG-001: Missing SlackNotifier implementation
- **Severity:** High
- **File:** `backend/internal/alerting/adapters/slack/notifier.go` — file chưa tồn tại
- **Story:** US-ALT-001 AC3
- **Action Required:** DEV-BE implement SlackNotifier adapter

### BUG-002: Missing error wrap context
- **Severity:** Low
- **File:** `backend/internal/alerting/app/command/create_rule.go:45`
- **Issue:** `return err` không wrap context, vi phạm Section 9.2
- **Expected:** `return fmt.Errorf("create rule: %w", err)`

## DDD Layer Check
- [ ] domain/ imports stdlib only: PASS
- [ ] adapters/ implements ports/ interfaces: PASS
- [ ] No cross-BC imports: PASS
- [x] Interface compliance verified: PASS

## Test Coverage
- `alerting/domain`: 92% — GOOD
- `alerting/app/command`: 85% — GOOD
- `alerting/adapters`: 0% — NEEDS WORK (integration tests missing)
```

---

### 2.5 DevSecOps

**Nhiệm vụ:** Infrastructure setup, Docker/K8s config, CI/CD pipeline, security audit, deployment.

| Thuộc tính | Giá trị |
|-----------|---------|
| **Model** | Sonnet |
| **Tools** | Read, Write, Edit, Bash, Glob, Grep |
| **KHÔNG có** | WebSearch, WebFetch |
| **Đọc** | `doc/logmon.md` Section 9.7 (Security), Section 13 (Deploy), infra configs |
| **Viết** | `infra/**/*`, `.github/workflows/*`, `.agile/sprints/{N}/reviews/SECURITY-AUDIT.md` |

**Quy tắc:**
- PHẢI audit security theo OWASP Go-SCP checklist (Section 9.7)
- Docker images PHẢI có: healthcheck, non-root user, minimal base image
- CI pipeline PHẢI chạy: `go test`, `golangci-lint`, `pnpm test`, `docker build`
- KHÔNG sửa backend business logic code
- KHÔNG sửa frontend component code
- Secrets LUÔN qua environment variables

**Security Audit Checklist:**
```markdown
## Security Audit — Sprint {N}

### Input Validation
- [ ] Tất cả request structs có validator tags
- [ ] Parameterized queries (không string concatenation)

### Authentication
- [ ] bcrypt cho password hashing
- [ ] JWT với HttpOnly + Secure + SameSite cookies
- [ ] Generic error messages (không lộ field nào sai)

### HTTP Security
- [ ] HSTS header trên mọi response
- [ ] X-Content-Type-Options: nosniff
- [ ] X-Frame-Options: DENY
- [ ] TLS 1.2+ minimum

### Secrets
- [ ] Không hardcode credentials trong source
- [ ] .env không commit vào git
- [ ] crypto/rand cho token generation

### Infrastructure
- [ ] Docker images non-root
- [ ] Resource limits trên mọi container
- [ ] Network isolation configured
```

---

## 3. Scrum Workflow

### 3.1 Sprint Lifecycle

```
Sprint Duration: 2 tuần (có thể adjust)
Sprint Goal: Hoàn thành 1-2 Bounded Contexts hoặc 1 epic

┌──────────────────────────────────────────────────────────────────┐
│                        SPRINT LIFECYCLE                          │
│                                                                  │
│  ┌─────────┐   ┌──────────┐   ┌─────────┐   ┌────────┐        │
│  │ BACKLOG │──▶│ PLANNING │──▶│ EXECUTE │──▶│ REVIEW │──┐     │
│  │ REFINE  │   │          │   │         │   │        │  │     │
│  └─────────┘   └──────────┘   └─────────┘   └────────┘  │     │
│      BA            ALL         BE+FE+DSO      QA+DSO     │     │
│                                                           │     │
│                                              ┌────────┐   │     │
│                                              │ RETRO  │◀──┘     │
│                                              │        │         │
│                                              └────────┘         │
│                                                 ALL             │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 Ceremonies (Slash Commands)

| Command | Ceremony | Agents Involved | Output |
|---------|----------|----------------|--------|
| `/sprint:init` | Sprint Initialization | BA | `PRODUCT-BACKLOG.md`, Sprint directory |
| `/sprint:refine` | Backlog Refinement | BA | Updated stories with AC, story points |
| `/sprint:plan` | Sprint Planning | BA → DEV-BE, DEV-FE, DevSecOps | `SPRINT-BACKLOG.md`, task breakdown |
| `/sprint:execute` | Sprint Execution | DEV-BE, DEV-FE, DevSecOps (parallel) | Code + `*-DONE.md` files |
| `/sprint:review` | Sprint Review | QA, DevSecOps | `QA-REPORT.md`, `SECURITY-AUDIT.md` |
| `/sprint:retro` | Sprint Retrospective | ALL (sequential) | `RETRO.md` |
| `/sprint:status` | Daily Standup | — (reads STATE.md) | Progress report |

### 3.3 Chi Tiết Từng Ceremony

#### `/sprint:init` — Khởi Tạo Sprint Mới

```
User chạy: /sprint:init

1. BA Agent spawn (Opus)
   ├── Đọc: doc/logmon.md, CLAUDE.md, PRODUCT-BACKLOG.md, STATE.md
   ├── Hỏi user: Sprint Goal là gì? Focus BC nào?
   ├── Viết: .agile/sprints/sprint-{N}/ directory
   ├── Viết: SPRINT-BACKLOG.md (draft)
   └── Return: Sprint {N} initialized, {X} stories proposed
```

#### `/sprint:refine` — Backlog Refinement

```
User chạy: /sprint:refine

1. BA Agent spawn (Opus)
   ├── Đọc: PRODUCT-BACKLOG.md, current sprint stories
   ├── Viết/update: stories/*.md với Acceptance Criteria
   ├── Estimate story points
   ├── Prioritize (MoSCoW: Must/Should/Could/Won't)
   └── Return: {X} stories refined, total {Y} story points
```

#### `/sprint:plan` — Sprint Planning

```
User chạy: /sprint:plan

1. BA Agent spawn (Opus) — Task Decomposition
   ├── Đọc: SPRINT-BACKLOG.md, refined stories
   ├── Viết: Task breakdown per agent role
   │   ├── tasks/BE-{BC}-{NNN}.md (backend tasks)
   │   ├── tasks/FE-{BC}-{NNN}.md (frontend tasks)
   │   └── tasks/INFRA-{NNN}.md   (devops tasks)
   ├── Assign waves (dependency analysis):
   │   Wave 0: Infrastructure setup (DevSecOps)
   │   Wave 1: Domain + Ports (DEV-BE) — no dependencies
   │   Wave 2: Adapters + HTTP (DEV-BE) + API Client (DEV-FE) — depends Wave 1
   │   Wave 3: UI Pages (DEV-FE) — depends Wave 2
   │   Wave 4: Integration (QA verification)
   └── Return: {X} tasks, {Y} waves, estimated {Z} story points

2. User confirms plan (hoặc adjust)
```

#### `/sprint:execute` — Sprint Execution

```
User chạy: /sprint:execute

Orchestrator đọc SPRINT-BACKLOG.md, xác định wave hiện tại:

Wave 0 — Infrastructure (sequential):
  └── DevSecOps Agent spawn
      ├── Đọc: tasks/INFRA-*.md
      ├── Setup: Docker Compose, Prometheus config, DB migrations
      ├── Viết: tasks/INFRA-*-DONE.md
      └── Git commit: "infra(sprint-1/INFRA-001): docker compose for alerting BC"

Wave 1 — Domain + Ports (parallel):
  ├── DEV-BE Agent #1 spawn (alerting domain)
  │   ├── Đọc: tasks/BE-ALT-001.md, US-ALT-001.md, CLAUDE.md
  │   ├── Code: domain/alert_rule.go, domain/errors.go, ports/repository.go
  │   ├── Tests: domain/alert_rule_test.go
  │   ├── Viết: tasks/BE-ALT-001-DONE.md
  │   └── Git commit: "feat(alerting/BE-ALT-001): AlertRule aggregate root"
  │
  └── DEV-BE Agent #2 spawn (slo domain) [PARALLEL]
      ├── Đọc: tasks/BE-SLO-001.md, US-SLO-001.md, CLAUDE.md
      ├── Code: domain/slo.go, domain/error_budget.go, ports/repository.go
      └── Git commit: "feat(slo/BE-SLO-001): SLO aggregate with ErrorBudget"

Wave 2 — Adapters + API (parallel, after Wave 1):
  ├── DEV-BE Agent spawn (adapters)
  │   ├── VERIFY: Wave 1 commits exist (git log check)
  │   ├── Code: adapters/postgres/repo.go, adapters/http/handler.go
  │   └── Git commit: "feat(alerting/BE-ALT-002): PostgreSQL adapter + HTTP handler"
  │
  └── DEV-FE Agent spawn (API client) [PARALLEL]
      ├── Code: services/alerting-api.ts, types/alert.ts
      └── Git commit: "feat(frontend/FE-ALT-001): alerting API client"

Wave 3 — UI Pages (after Wave 2):
  └── DEV-FE Agent spawn
      ├── VERIFY: API client exists
      ├── Code: app/alerts/page.tsx, components/AlertRuleForm.tsx
      └── Git commit: "feat(frontend/FE-ALT-002): alert management page"

Sau mỗi wave: Orchestrator check tất cả *-DONE.md files trước khi bắt đầu wave tiếp theo.
```

#### `/sprint:review` — Sprint Review

```
User chạy: /sprint:review

1. QA Agent spawn (Opus) — READ-ONLY
   ├── Đọc: Tất cả *-DONE.md files, stories/*.md (AC), source code
   ├── Chạy: go test ./..., pnpm test
   ├── Verify: Từng Acceptance Criteria (pass/fail)
   ├── Check: DDD layer violations, error handling, test coverage
   ├── Viết: reviews/QA-REPORT.md
   └── Return: PASS / PASS_WITH_ISSUES / FAIL

2. DevSecOps Agent spawn — Security Audit
   ├── Đọc: Source code, Docker configs, CI pipeline
   ├── Chạy: golangci-lint, security-specific checks
   ├── Check: OWASP checklist (Section 9.7)
   ├── Viết: reviews/SECURITY-AUDIT.md
   └── Return: PASS / ISSUES_FOUND

3. Nếu FAIL hoặc ISSUES_FOUND:
   ├── User review issues
   ├── Spawn DEV-BE/DEV-FE để fix (targeted)
   └── Re-run /sprint:review
```

#### `/sprint:retro` — Sprint Retrospective

```
User chạy: /sprint:retro

Orchestrator tổng hợp từ tất cả agents (sequential, mỗi agent đọc sprint artifacts):

1. BA Agent: "Requirements đủ rõ không? AC có bị thiếu không?"
2. DEV-BE Agent: "Architecture decisions nào tốt? Technical debt nào?"
3. DEV-FE Agent: "UI patterns nào hiệu quả? Component reuse?"
4. QA Agent: "Bugs pattern nào lặp lại? Test gaps?"
5. DevSecOps Agent: "Security issues? Infrastructure improvements?"

Output: RETRO.md với format:
  - What went well (keep doing)
  - What didn't go well (stop doing)
  - Action items for next sprint
  - Velocity metrics (story points completed / planned)
```

---

## 4. File-Based Communication Bus

### 4.1 Cấu Trúc `.agile/`

```
.agile/
├── PRODUCT-BACKLOG.md              ← BA maintains (all stories, prioritized)
├── STATE.md                        ← Auto-updated (current sprint, wave, progress)
├── config.json                     ← Settings (sprint length, max agents, etc.)
│
├── sprints/
│   ├── sprint-01/
│   │   ├── SPRINT-BACKLOG.md       ← Stories selected cho sprint này
│   │   ├── SPRINT-GOAL.md          ← Sprint goal + Definition of Done
│   │   │
│   │   ├── stories/                ← BA output
│   │   │   ├── US-ALT-001.md
│   │   │   ├── US-ALT-002.md
│   │   │   └── US-SLO-001.md
│   │   │
│   │   ├── tasks/                  ← Task plans + results
│   │   │   ├── BE-ALT-001.md       ← Task plan (input cho DEV-BE)
│   │   │   ├── BE-ALT-001-DONE.md  ← Task result (output từ DEV-BE)
│   │   │   ├── FE-ALT-001.md
│   │   │   ├── FE-ALT-001-DONE.md
│   │   │   ├── INFRA-001.md
│   │   │   └── INFRA-001-DONE.md
│   │   │
│   │   ├── reviews/                ← QA + DevSecOps output
│   │   │   ├── QA-REPORT.md
│   │   │   └── SECURITY-AUDIT.md
│   │   │
│   │   └── RETRO.md                ← Sprint retrospective
│   │
│   └── sprint-02/
│       └── ...
│
└── knowledge/                      ← Accumulated learnings
    ├── architecture-decisions.md   ← ADRs made during sprints
    ├── bug-patterns.md             ← QA: recurring bug patterns
    └── security-findings.md        ← DevSecOps: security patterns
```

### 4.2 Communication Flow

```
BA viết story ──────────────────────────────────────▶ stories/US-ALT-001.md
                                                            │
BA viết task plan ──────────────────────────────────▶ tasks/BE-ALT-001.md
                                                            │
DEV-BE đọc task + story ◀──────────────────────────────────┘
DEV-BE viết code ──────────────────────────────────▶ backend/internal/alerting/**
DEV-BE viết result ────────────────────────────────▶ tasks/BE-ALT-001-DONE.md
                                                            │
QA đọc story (AC) + code + result ◀────────────────────────┘
QA chạy tests, verify AC ─────────────────────────▶ reviews/QA-REPORT.md
                                                            │
DevSecOps đọc code + QA report ◀───────────────────────────┘
DevSecOps audit security ──────────────────────────▶ reviews/SECURITY-AUDIT.md
```

**Quy tắc giao tiếp:**
1. Agents KHÔNG BAO GIỜ đọc/sửa files của nhau đang viết
2. Communication là ONE-WAY: viết xong → agent tiếp theo đọc
3. Mỗi agent CHỈ viết vào files thuộc scope của mình
4. Orchestrator kiểm tra `*-DONE.md` existence trước khi spawn wave tiếp

### 4.3 STATE.md Format

```markdown
---
current_sprint: 1
sprint_goal: "Implement Alerting BC core (domain + adapters + basic UI)"
current_wave: 2
total_waves: 4
status: executing
---

## Sprint 1 Progress

| Wave | Status | Tasks | Completed | Agent(s) |
|------|--------|-------|-----------|----------|
| 0 | done | 2 | 2 | DevSecOps |
| 1 | done | 3 | 3 | DEV-BE x2 |
| 2 | executing | 4 | 1 | DEV-BE, DEV-FE |
| 3 | pending | 2 | 0 | DEV-FE |
| 4 | pending | 1 | 0 | QA |

## Story Points
- Planned: 21
- Completed: 13
- Remaining: 8

## Velocity
- Sprint 1 (in progress): ~13 SP (projected: 21)
```

---

## 5. Sprint Plan cho LogMon

### 5.1 Product Backlog (High-Level)

| Sprint | Focus | BCs | Epic |
|--------|-------|-----|------|
| **Sprint 1** | Foundation + Alerting Core | `shared`, `alerting` | Infrastructure setup, AlertRule CRUD, domain events |
| **Sprint 2** | Alerting Notifications + SLO | `alerting`, `slo` | Slack/Email adapters, SLO definition, error budget |
| **Sprint 3** | Log Pipeline + Order/User | `logpipeline`, `order`, `user` | Pipeline mode switching, CRUD services |
| **Sprint 4** | Frontend Dashboard | `frontend` | Alert management, SLO dashboard, log viewer |
| **Sprint 5** | Integration + Monitoring Stack | `infra` | Prometheus, ELK, Grafana, Docker Compose full stack |
| **Sprint 6** | CI/CD + Production Readiness | `infra` | GitHub Actions, security hardening, load testing |

### 5.2 Sprint 1 Chi Tiết (Ví Dụ)

**Sprint Goal:** *"Setup foundation infrastructure và implement Alerting bounded context core — domain logic, persistence, REST API."*

**Stories:**

| ID | Story | SP | Wave |
|----|-------|----|------|
| US-INFRA-001 | Setup Docker Compose cho dev environment | 3 | 0 |
| US-INFRA-002 | Setup PostgreSQL schema + migrations | 2 | 0 |
| US-ALT-001 | Tạo Alert Rule mới | 5 | 1-2 |
| US-ALT-002 | Liệt kê Alert Rules theo service | 3 | 1-2 |
| US-ALT-003 | Alert evaluation engine (mock) | 5 | 2 |
| US-ALT-004 | Alert management UI page | 3 | 3 |

**Wave Breakdown:**

```
Wave 0 — Infrastructure (DevSecOps):
  INFRA-001: Docker Compose (postgres, backend, frontend)
  INFRA-002: DB migrations, Go project structure

Wave 1 — Domain + Ports (DEV-BE, parallel):
  BE-ALT-001: AlertRule aggregate root + domain errors + value objects
  BE-ALT-002: AlertRuleRepository port interface + AlertReadModel port

Wave 2 — Adapters + App (DEV-BE + DEV-FE, parallel):
  BE-ALT-003: PostgreSQL adapter (implements AlertRuleRepository)
  BE-ALT-004: HTTP handler (Gin routes for CRUD)
  BE-ALT-005: CreateRule + ListRules command/query handlers
  FE-ALT-001: API client + TypeScript types for alerting

Wave 3 — UI (DEV-FE):
  FE-ALT-002: Alert management page (list + create form)
  FE-ALT-003: Error/loading/empty states

Wave 4 — Verification (QA + DevSecOps):
  QA-001: Full AC verification, test coverage report
  SEC-001: Security audit (input validation, SQL injection, error messages)
```

---

## 6. Deviation Handling

Khi agent gặp vấn đề ngoài scope task:

| Rule | Tình huống | Action |
|------|-----------|--------|
| **R1** | Bug nhỏ trong code đang sửa | Auto-fix, ghi vào DONE.md deviations |
| **R2** | Thiếu function/type cần thiết trong cùng BC | Auto-thêm, ghi vào DONE.md |
| **R3** | Blocking issue (DB connection, build fail) | Auto-fix nếu < 10 phút, otherwise escalate |
| **R4** | Cần thay đổi architecture / cross-BC change | **STOP** — escalate to User (Scrum Master) |

---

## 7. Agent Spawn Configuration

### 7.1 config.json

```json
{
  "sprint_length_weeks": 2,
  "max_concurrent_agents": 3,
  "models": {
    "ba": "opus",
    "dev_be": "sonnet",
    "dev_fe": "sonnet",
    "qa": "opus",
    "devsecops": "sonnet"
  },
  "auto_advance_waves": true,
  "require_qa_pass_before_merge": true,
  "require_security_audit": true,
  "commit_format": "{type}({bc}/{task_id}): {description}",
  "bounded_contexts": [
    "alerting", "slo", "logpipeline", "order", "user", "shared"
  ]
}
```

### 7.2 Agent CLAUDE.md Context

Mỗi agent khi spawn sẽ được inject context riêng:

```
BA Agent context:
  - CLAUDE.md (project overview + architecture rules)
  - doc/logmon.md Section 1-6 (system overview, components, data flows)
  - .agile/PRODUCT-BACKLOG.md
  - .agile/STATE.md

DEV-BE Agent context:
  - CLAUDE.md (FULL — architecture rules + style guide + security)
  - doc/logmon.md Section 7-9 (project structure, backend details, coding rules)
  - Relevant story file (stories/US-XXX.md)
  - Task file (tasks/BE-XXX.md)

DEV-FE Agent context:
  - CLAUDE.md (project overview + frontend-relevant sections)
  - Relevant story file
  - Task file (tasks/FE-XXX.md)
  - Existing component inventory (glob frontend/components/**)

QA Agent context:
  - CLAUDE.md (architecture rules + style guide + security)
  - doc/logmon.md Section 9 (ALL coding rules)
  - All stories for current sprint (stories/*.md)
  - All DONE files (tasks/*-DONE.md)
  - Source code being reviewed

DevSecOps Agent context:
  - CLAUDE.md (security section)
  - doc/logmon.md Section 9.7, 9.9, 13 (security, infrastructure, deploy)
  - infra/ directory
  - .github/workflows/
```

---

## 8. Definition of Done

### Per Sprint
- [ ] Tất cả stories trong sprint backlog: AC verified by QA
- [ ] QA Report: PASS hoặc PASS_WITH_ISSUES (no blockers)
- [ ] Security Audit: PASS (no critical/high issues)
- [ ] `go test ./...` — all pass
- [ ] `pnpm test` — all pass
- [ ] `golangci-lint run` — no errors
- [ ] Docker Compose stack starts successfully
- [ ] All code committed with proper message format
- [ ] STATE.md updated
- [ ] RETRO.md completed

### Per Agent Output
| Agent | Definition of Done |
|-------|-------------------|
| BA | Story có: AC (Given/When/Then), REQ-ID, story points, BC assignment |
| DEV-BE | Code compiles, tests pass, interface compliance verified, DONE.md written, git committed |
| DEV-FE | No TypeScript errors, tests pass, responsive, DONE.md written, git committed |
| QA | Every AC verified (pass/fail + evidence), coverage reported, bugs filed with file:line |
| DevSecOps | OWASP checklist completed, no critical issues, Docker works, CI pipeline passes |

---

## 9. Quick Start

```bash
# 1. Initialize agile directory
mkdir -p .agile/sprints/sprint-01/{stories,tasks,reviews}
mkdir -p .agile/knowledge

# 2. Chạy Sprint Init
/sprint:init
# → BA Agent tạo PRODUCT-BACKLOG.md, SPRINT-BACKLOG.md draft

# 3. Refine backlog
/sprint:refine
# → BA Agent viết stories với AC

# 4. Plan sprint
/sprint:plan
# → BA Agent tạo task breakdown, wave assignment

# 5. Execute sprint
/sprint:execute
# → DevSecOps (Wave 0) → DEV-BE (Wave 1-2) → DEV-FE (Wave 2-3) → parallel

# 6. Review sprint
/sprint:review
# → QA Agent verifies AC → DevSecOps audits security

# 7. Retrospective
/sprint:retro
# → All agents contribute lessons learned

# 8. Next sprint
/sprint:init
# → Repeat cycle
```

---

*Tài liệu này được thiết kế dựa trên [get-shit-done](https://github.com/gsd-build/get-shit-done) architecture, adapted cho 5-role AI Agent team theo Agile/Scrum methodology, tối ưu cho dự án LogMon.*
