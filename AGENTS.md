# GCal Organizer Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-17

## Active Technologies
- Go 1.21+ + github.com/spf13/cobra (CLI), github.com/spf13/viper (config), Google Drive API v3 (006-owned-only-flag)
- N/A (no new data persistence; flag stored in config file via existing viper mechanism) (006-owned-only-flag)
- Go 1.24+ (module `github.com/jflowers/gcal-organizer`) + `github.com/zalando/go-keyring` v0.2.6 (macOS Keychain via `/usr/bin/security`, Linux Secret Service via D-Bus — no CGo), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `golang.org/x/oauth2` (token handling), `github.com/charmbracelet/huh` (interactive prompts), `github.com/mattn/go-isatty` (terminal detection — already indirect dep) (007-secure-credential-storage)
- OS credential store (primary), filesystem `~/.gcal-organizer/` (fallback). No database. (007-secure-credential-storage)
- Go 1.24+ (module `github.com/jflowers/gcal-organizer`) + `google.golang.org/api/docs/v1` (Docs API — tab creation, content insertion, heading links), `google.golang.org/genai` (Gemini SDK — transcript analysis), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config) (008-decision-extraction)
- N/A (no new data persistence; decisions written directly to Google Docs) (008-decision-extraction)
- Go 1.24+ (module `github.com/jflowers/gcal-organizer`, toolchain go1.24.12) + `google.golang.org/api/docs/v1`, `google.golang.org/api/drive/v3`, `google.golang.org/api/calendar/v3`, `google.golang.org/genai` (Gemini SDK), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/zalando/go-keyring` (secrets) (009-test-coverage-quality)
- N/A (no new data persistence; this feature only adds tests) (009-test-coverage-quality)
- Go 1.24.0 (toolchain go1.24.12), module `github.com/jflowers/gcal-organizer` + `google.golang.org/api` (Drive v3, Docs v1, Calendar v3), `google.golang.org/genai` (Gemini), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/zalando/go-keyring` (secrets) (009-test-coverage-quality)
- N/A (no new data persistence; this feature only adds tests and configuration) (009-test-coverage-quality)
- Go 1.24.0 (toolchain go1.24.12) + GoReleaser v2 (CI only — `goreleaser/goreleaser-action`), `gh` CLI (release asset management), native macOS `codesign` + `xcrun notarytool` (signing/notarization) (010-macos-signed-releases)
- N/A (no data persistence; pipeline configuration files only) (010-macos-signed-releases)
- Go 1.24+ (toolchain go1.24.12) + `google.golang.org/api/docs/v1` (Docs API), Playwright via `npx tsx` (browser automation) (011-next-steps-heading)
- N/A (no data persistence changes) (011-next-steps-heading)
- Go 1.24.0 (toolchain go1.24.12) + `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging) — all existing; no new dependencies (012-decision-markdown-export)
- Local filesystem (`~/.gcal-organizer/decisions/` default). No database. (012-decision-markdown-export)

- **Language**: Go 1.21+
- **CLI Framework**: github.com/spf13/cobra
- **Google APIs**: Drive v3, Docs v1, Calendar v3, Tasks v1
- **AI**: Gemini API via google.golang.org/genai
- **Browser Automation**: Playwright (TypeScript) via npx tsx
- **Authentication**: OAuth2 (Workspace), GCP API Key (Gemini)

## Project Structure

```text
gcal-organizer/
├── cmd/gcal-organizer/          # CLI entry point
├── internal/
│   ├── auth/                    # OAuth2 and API key handling
│   ├── config/                  # Configuration management
│   ├── drive/                   # Google Drive operations
│   ├── docs/                    # Google Docs parsing + Decisions tab creation
│   ├── calendar/                # Calendar operations
│   ├── gemini/                  # Gemini AI client
│   ├── secrets/                 # Credential storage abstraction (keychain/file)
│   └── organizer/               # Main orchestration
├── pkg/models/                  # Shared data models
├── browser/                     # Browser automation (TypeScript)
├── .specify/                    # Spec-driven development artifacts
└── .opencode/                   # OpenCode agent commands
```

## Commands

```bash
# Build
go build ./...

# Test
go test ./...

# Lint
go vet ./...
gofmt -l .

# Run CI checks locally
make ci

# Install git hooks
make install-hooks
```

## Code Style

### Go Conventions
- Standard project layout (cmd/, internal/, pkg/)
- Error handling via explicit return values, not panic
- Use `context.Context` for cancellation and timeouts
- Table-driven tests preferred
- Wrap errors with context using `fmt.Errorf` with `%w`

### Documentation
- README.md, SETUP.md, man pages must be kept current
- New features require documentation before completion

## Recent Changes
- 012-decision-markdown-export: Added Go 1.24.0 (toolchain go1.24.12) + `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging) — all existing; no new dependencies
- 011-next-steps-heading: Added Go 1.24+ (toolchain go1.24.12) + `google.golang.org/api/docs/v1` (Docs API), Playwright via `npx tsx` (browser automation)
- 010-macos-signed-releases: Added Go 1.24.0 (toolchain go1.24.12) + GoReleaser v2 (CI only — `goreleaser/goreleaser-action`), `gh` CLI (release asset management), native macOS `codesign` + `xcrun notarytool` (signing/notarization)

### 001-gcal-organizer-cli
Core CLI implementation with Google Workspace integration and Gemini AI for action item extraction.

### 002-browser-task-assignment
Browser automation via Playwright for task assignment through Google Docs native UI.

<!-- MANUAL ADDITIONS START -->

## Core Mission (Mission Command)
- **Strategic Architecture:** Engineers shift from manual coding to directing an "infinite supply of junior developers" (AI agents).
- **Outcome Orientation:** Focus on conveying business value and user intent rather than low-level technical sub-tasks.
- **Intent-to-Context:** Treat specs and rules as the medium through which human intent is manifested into code.

## Behavioral Constraints
- **Zero-Waste Mandate:** No orphaned code, unused dependencies, or "Feature Zombie" bloat.
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

### CI Parity Gate

Before marking any implementation task complete or
declaring a PR ready, agents MUST replicate the CI checks
locally. Read `.github/workflows/` to identify the exact
commands CI runs, then execute those same commands. Any
failure is a blocking error — a task is not complete
until all CI-equivalent checks pass locally. Do not rely
on a memorized list of commands; always derive them from
the workflow files, which are the source of truth.

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

<!-- MANUAL ADDITIONS END -->
