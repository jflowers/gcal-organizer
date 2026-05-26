# GCal Organizer Development Guidelines

## Overview

A Go CLI tool that automates meeting note organization, calendar
attachment syncing, AI-powered task assignment, and decision
extraction using Google Workspace APIs and Gemini AI.

- **Type**: CLI tool (Cobra)
- **Language**: Go 1.24+ (module `github.com/jflowers/gcal-organizer`, toolchain go1.24.12)
- **Google APIs**: Drive v3, Docs v1, Calendar v3, Tasks v1
- **AI**: Gemini API via `google.golang.org/genai`; Ollama for local AI
- **Browser Automation**: Playwright (TypeScript) via `npx tsx`
- **Authentication**: OAuth2 (Workspace), GCP API Key (Gemini)
- **Secrets**: OS keychain via `github.com/zalando/go-keyring` (macOS Keychain, Linux Secret Service)
- **License**: MIT

## Project Structure

```text
gcal-organizer/
├── cmd/gcal-organizer/          # CLI entry point
├── internal/
│   ├── auth/                    # OAuth2 and API key handling
│   ├── calendar/                # Calendar operations
│   ├── config/                  # Configuration management (YAML + migration)
│   ├── docs/                    # Google Docs parsing + Decisions tab creation
│   ├── drive/                   # Google Drive operations
│   ├── export/                  # Decision markdown export
│   ├── gemini/                  # Gemini AI client
│   ├── logging/                 # Structured logging setup
│   ├── ollama/                  # Local AI via Ollama REST API
│   ├── organizer/               # Main orchestration
│   ├── retry/                   # HTTP retry with backoff
│   ├── secrets/                 # Credential storage (keychain/file)
│   └── ux/                      # Terminal UX utilities
├── pkg/models/                  # Shared data models
├── browser/                     # Browser automation (TypeScript)
├── docs/                        # User documentation
├── man/                         # Man pages
├── specs/                       # Speckit feature specs (NNN-name/)
├── openspec/                    # OpenSpec change artifacts
├── .specify/                    # Spec-driven development artifacts
└── .opencode/                   # OpenCode agent commands
```

## Build & Test Commands

```bash
# Build (CI uses specific target)
go build ./cmd/gcal-organizer

# Test (CI uses -race flag)
go test -race ./...

# Lint
go vet ./...
gofmt -l .

# Module tidiness check (CI step)
go mod tidy
git diff --exit-code go.mod go.sum

# Run CI checks locally (mirrors .github/workflows/ci.yml)
make ci

# Install git hooks
make install-hooks
```

### CI Workflow Structure

| Workflow | File | Trigger |
|----------|------|---------|
| CI | `.github/workflows/ci.yml` | push/PR to main |
| Release | `.github/workflows/release.yml` | tag push |

## Code Style

### Go Conventions
- Standard project layout (cmd/, internal/, pkg/)
- Error handling via explicit return values, not panic
- Use `context.Context` for cancellation and timeouts
- Wrap errors with context using `fmt.Errorf` with `%w`
- Define interfaces at the consumer site (e.g., `organizer` defines `DocsService`, `DriveService`)
- Import grouping: stdlib, external, internal (separated by blank lines)

### Documentation
- README.md, SETUP.md, man pages must be kept current
- New features require documentation before completion

## Testing Conventions

- **Framework**: Go stdlib `testing` package (no assertion libraries)
- **Naming**: `TestFunctionName_Scenario` (e.g., `TestParseEvent_DateTimeFormat`, `TestDo_RetriesOn429`)
- **Style**: Table-driven tests with `t.Run` subtests for cases with multiple inputs
- **Assertions**: Direct comparisons with `t.Errorf`/`t.Fatalf` (no testify/assert)
- **Isolation**: `t.TempDir()` for filesystem tests; struct literal construction for unit tests (no shared global state)
- **Race detection**: CI runs with `-race`; all tests must be race-safe
- **Coverage**: `make test-coverage` generates `coverage.out` and `coverage.html`

## Recent Changes
- 014-local-ai-ollama: Added Go 1.24.0 (toolchain go1.24.12) + `net/http` (Ollama REST API — no SDK), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging), `github.com/charmbracelet/huh` (interactive prompts)
- 013-yaml-config-decision-export: Added Go 1.24.0 (toolchain go1.24.12) + `github.com/spf13/viper` (config), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/zalando/go-keyring` (secrets) — all existing; no new dependencies
- 012-decision-markdown-export: Added Go 1.24.0 (toolchain go1.24.12) + `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging) — all existing; no new dependencies

### 001-gcal-organizer-cli
Core CLI implementation with Google Workspace integration and Gemini AI for action item extraction.

### 002-browser-task-assignment
Browser automation via Playwright for task assignment through Google Docs native UI.

<!-- MANUAL ADDITIONS START -->

## Core Mission (Mission Command)
- **Strategic Architecture:** Engineers shift from manual coding to directing an "infinite supply of junior developers" (AI agents).
- **Outcome Orientation:** Focus on conveying business value and user intent rather than low-level technical sub-tasks.
- **Intent-to-Context:** Treat specs and rules as the medium through which human intent is manifested into code.

## Behavioral Rules

These rules are non-negotiable. Violations are CRITICAL severity.

- **Gatekeeping**: MUST NOT modify quality/governance gates
  (coverage thresholds, CRAP scores, severity definitions,
  CI flags, agent settings, constitution MUST rules, review
  limits, workflow markers). Stop and report instead.
- **Phase boundaries**: MUST NOT cross workflow phase boundaries.
  Spec phases: spec artifacts only. Implement: source code.
  Review: fixes only. Violation = process error, stop immediately.
- **CI parity**: MUST replicate CI checks locally before marking
  tasks complete. Derive commands from `.github/workflows/`.
- **Review council**: MUST run `/review-council` before PR
  submission. Resolve all REQUEST CHANGES. No code changes
  between APPROVE and PR. Exempt: constitution amendments,
  docs-only, emergency hotfixes.
- **Branch protection**: MUST NOT commit directly to `main`.
  All changes via feature branches and PRs.
- **Documentation gate**: Before marking a task complete,
  assess documentation impact: `CHANGELOG.md` for change
  entries, `AGENTS.md` for structural updates (project
  structure, conventions, build commands), `README.md` for
  description changes.
- **Website gate**: MUST file `unbound-force/website` issue
  for user-facing changes before PR merge. Exempt: internal
  refactoring, test-only, CI-only, spec artifacts.
- **Zero-waste**: No orphaned specs, unused standards, or
  aspirational documents that do not map to actionable work.

### PR Review Commands

| Command | When | Scope |
|---------|------|-------|
| `/review-council` | Pre-PR (local) | 5+ Divisor agents |
| `/review-pr [N]` | Post-PR (GitHub) | Single agent, CI analysis |

### Behavioral Constraints (Extended)
- **Neighborhood Rule:** Changes must be audited for negative impacts on adjacent modules or the wider ecosystem.
- **Intent Drift Detection:** Evaluation must detect when the implementation drifts away from the original human-written "Statement of Intent."
- **Automated Governance:** Primary feedback is provided via automated constraints, reserving human energy for high-level security and logic.

### Gatekeeping Value Protection

Agents MUST NOT modify values that serve as quality or
governance gates to make an implementation pass. The
following categories are protected:

1. **Coverage thresholds and CRAP scores** — minimum
   coverage percentages, CRAP score limits, coverage
   ratchets
2. **Severity definitions and auto-fix policies** —
   CRITICAL/HIGH/MEDIUM/LOW boundaries, auto-fix
   eligibility rules
3. **Convention pack rule classifications** —
   MUST/SHOULD/MAY designations on convention pack rules
   (downgrading MUST to SHOULD is prohibited)
4. **CI flags and linter configuration** — `-race`,
   `-count=1`, `govulncheck`, `golangci-lint` rules,
   pinned action SHAs
5. **Agent temperature and tool-access settings** —
   frontmatter `temperature`, `tools.write`, `tools.edit`,
   `tools.bash` restrictions
6. **Constitution MUST rules** — any MUST rule in
   `.specify/memory/constitution.md` or hero constitutions
7. **Review iteration limits and worker concurrency** —
   max review iterations, max concurrent Swarm workers,
   retry limits
8. **Workflow gate markers** — `<!-- spec-review: passed
   -->`, task completion checkboxes used as gates, phase
   checkpoint requirements

**What to do instead**: When an implementation cannot
meet a gate, the agent MUST stop, report which gate is
blocking and why, and let the human decide whether to
adjust the gate or rework the implementation. Modifying
a gate without explicit human authorization is a
constitution violation (CRITICAL severity).

### Workflow Phase Boundaries

Agents MUST NOT cross workflow phase boundaries:

- **Specify/Clarify/Plan/Tasks/Analyze/Checklist** phases:
  spec artifacts ONLY (`specs/NNN-*/` directory). No
  source code, test, agent, command, or config changes.
- **Implement** phase: source code changes allowed,
  guided by spec artifacts.
- **Review** phase: findings and minor fixes only. No new
  features.

A phase boundary violation is treated as a process error.
The agent MUST stop and report the violation rather than
proceeding with out-of-phase changes.

## Technical Guardrails
- **WORM Persistence:** Use Write-Once-Read-Many patterns where data integrity is paramount.

## Council Governance Protocol
- **The Architect:** Must verify that "Intent Driving Implementation" is maintained.
- **The Adversary:** Acts as the primary "Automated Governance" gate for security.
- **The Guard:** Detects "Intent Drift" to ensure the business value remains intact.

**Rule:** A Pull Request is only "Ready for Human" once the `/review-council` command returns an **APPROVE** status.

## Spec-First Development

All changes that modify production code, test code, agent
prompts, embedded assets, or CI configuration **must** be
preceded by a spec workflow. The constitution
(`.specify/memory/constitution.md`) is the highest-
authority document in this project — all work must align
with it.

Two spec workflows are available:

| Workflow | Location | Best For |
|----------|----------|----------|
| **Speckit** | `specs/NNN-name/` | Numbered feature specs with the full pipeline |
| **OpenSpec** | `openspec/changes/name/` | Targeted changes with lightweight artifacts |

**What requires a spec** (no exceptions without explicit
user override):

- New features or capabilities
- Refactoring that changes function signatures, extracts
  helpers, or moves code between packages
- Test additions or assertion strengthening across
  multiple functions
- Agent prompt changes
- CI workflow modifications
- Data model changes (new struct fields, schema updates)

**What is exempt** (may be done directly):

- Constitution amendments (governed by the constitution's
  own Governance section)
- Typo corrections, comment-only changes, single-line
  formatting fixes
- Emergency hotfixes for critical production bugs (must
  be retroactively documented)

When an agent is unsure whether a change is trivial, it
**must** ask the user rather than proceeding without a
spec. The cost of an unnecessary spec is minutes; the
cost of an unplanned change is rework, drift, and broken
CI.

### Website Documentation Gate

When a change affects user-facing behavior, hero
capabilities, CLI commands, or workflows, a GitHub issue
**MUST** be created in the `unbound-force/website`
repository to track required documentation or website
updates. The issue must be created before the
implementing PR is merged.

```bash
gh issue create --repo unbound-force/website \
  --title "docs: <brief description of what changed>" \
  --body "<what changed, why it matters, which pages
          need updating>"
```

**Exempt changes** (no website issue needed):
- Internal refactoring with no user-facing behavior
  change
- Test-only changes
- CI/CD pipeline changes
- Spec artifacts (specs are internal planning documents)

**Examples requiring a website issue**:
- New CLI command or flag added
- Hero capabilities changed (new agent, removed feature)
- Installation steps changed (`uf setup` flow)
- New convention pack added
- Breaking changes to any user-facing workflow

## Specify Workflow

After the following `/speckit.*` commands complete successfully, **commit and push** the resulting artifacts:

- `/speckit.specify` — commit the new spec and checklist
- `/speckit.clarify` — commit the updated spec with clarifications
- `/speckit.plan` — commit plan.md, research.md, data-model.md, quickstart.md, and any AGENTS.md updates
- `/speckit.tasks` — commit tasks.md
- `/speckit.analyze` — commit any analysis artifacts

## Knowledge Retrieval

Agents SHOULD prefer Dewey MCP tools over grep/glob/read
for cross-repo context, design decisions, and
architectural patterns. Dewey provides semantic search
across all indexed Markdown files, specs, and web
documentation — returning ranked results with provenance
metadata that grep cannot match.

### Tool Selection Matrix

| Query Intent | Dewey Tool | When to Use |
|-------------|-----------|-------------|
| Conceptual understanding | `dewey_semantic_search` | "How does X work?" |
| Keyword lookup | `dewey_search` | Known terms, FR numbers |
| Read specific page | `dewey_get_page` | Known document path |
| Relationship discovery | `dewey_find_connections` | "How are X and Y related?" |
| Similar documents | `dewey_similar` | "Find specs like this one" |
| Tag-based discovery | `dewey_find_by_tag` | "All pages tagged #decision" |
| Property queries | `dewey_query_properties` | "All specs with status: draft" |
| Filtered semantic | `dewey_semantic_search_filtered` | Semantic search within source type |
| Graph navigation | `dewey_traverse` | Dependency chain walking |

### When to Fall Back to grep/glob/read

Use direct file operations instead of Dewey when:
- **Dewey is unavailable** — MCP tools return errors or
  are not configured
- **Exact string matching is needed** — searching for a
  specific error message, variable name, or code pattern
- **Specific file path is known** — reading a file you
  already know the path to (use Read directly)
- **Binary/non-Markdown content** — Dewey indexes
  Markdown; use grep for Go source, JSON, YAML, etc.

### Graceful Degradation (3-Tier Pattern)

**Tier 3 (Full Dewey)** — semantic + structured search:
- `dewey_semantic_search` — natural language queries
- `dewey_search` — keyword queries
- `dewey_get_page`, `dewey_find_connections`,
  `dewey_traverse` — structured navigation
- `dewey_find_by_tag`, `dewey_query_properties` —
  metadata queries

**Tier 2 (Graph-only, no embedding model)** — structured
search only:
- `dewey_search` — keyword queries (no embeddings needed)
- `dewey_get_page`, `dewey_traverse`,
  `dewey_find_connections` — graph navigation
- `dewey_find_by_tag`, `dewey_query_properties` —
  metadata queries
- Semantic search unavailable — use exact keyword matches

**Tier 1 (No Dewey)** — direct file access:
- Use Read tool for direct file access
- Use Grep for keyword search across the codebase
- Use Glob for file pattern matching

## Architecture

- **Cobra CLI delegation**: `cmd/gcal-organizer/main.go` wires
  subcommands (`run`, `organize`, `sync-calendar`, `assign-tasks`,
  `install`, `uninstall`). Each subcommand delegates to
  `internal/` service packages.
- **Consumer-site interfaces**: The `organizer` package defines
  interfaces (`DocsService`, `DriveService`, `CalendarService`)
  consumed from `internal/` implementations, enabling test
  doubles without mocking frameworks.
- **Service composition**: `internal/organizer/` orchestrates
  the 4-step workflow by composing calendar, drive, docs, and
  gemini services via dependency injection at construction.
- **Keychain-first secrets**: `internal/secrets/` abstracts OS
  keychain (primary) with filesystem fallback, keeping
  credentials out of config files.
- **Retry with backoff**: `internal/retry/` provides generic
  HTTP retry with exponential backoff for Google API rate limits.

<!-- MANUAL ADDITIONS END -->

## Convention Packs

This repository uses convention packs scaffolded by
unbound-force. Agents MUST read the applicable pack(s)
before writing or reviewing code.

- `.opencode/uf/packs/default.md`
- `.opencode/uf/packs/default-custom.md`
- `.opencode/uf/packs/severity.md`
- `.opencode/uf/packs/content.md`
- `.opencode/uf/packs/content-custom.md`
- `.opencode/uf/packs/go.md`
- `.opencode/uf/packs/go-custom.md`
- `.opencode/uf/packs/typescript.md`
- `.opencode/uf/packs/typescript-custom.md`
