# Feature Specification: Local AI via Ollama with IBM Granite

**Feature Branch**: `014-local-ai-ollama`
**Created**: 2026-04-20
**Status**: Draft
**Input**: Add local AI via Ollama with IBM Granite models for transcript sensitivity gating, local task assignment, and local-only mode. Based on exploration of Dewey's Ollama integration pattern and IBM Granite model capabilities.

## Overview

gcal-organizer currently sends all meeting transcripts to Google's Gemini cloud AI for decision extraction and task assignment. This presents two problems: (1) sensitive meeting content -- HR concerns, legal matters, compensation discussions -- is sent to a cloud service without any screening, and (2) users who prefer to keep meeting data local have no option to do so.

This feature adds a local AI layer using Ollama with IBM Granite models. A sensitivity classifier (Granite Guardian) screens every transcript before any processing occurs. If a transcript is classified as sensitive, it is skipped entirely -- no cloud AI calls, no document modifications, no local exports. For task assignment, a smaller Granite model replaces Gemini, keeping that data local by default. An optional local-only mode runs everything through Granite, eliminating all cloud AI dependencies.

When Ollama is configured, it is a hard requirement: if Ollama is not available (not installed, not running, or models not pulled), the pipeline stops with an actionable error rather than silently falling back to cloud processing. This is a deliberate security posture -- if the user has opted into local AI screening, bypassing it would violate their intent.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Sensitivity Gate (Priority: P1)

As a meeting organizer who processes transcripts from meetings that sometimes involve sensitive topics (HR concerns, legal matters, compensation), I want the system to automatically detect sensitive content and skip processing those transcripts so that sensitive information is never sent to cloud AI services or written to local files.

**Why this priority**: This is the core safety feature. Without it, all transcripts are processed indiscriminately, including those containing private HR discussions, legal matters, or personnel actions. This must be the first capability delivered because it gates all other processing.

**Independent Test**: Can be tested by providing transcripts with known sensitive content (e.g., performance review language, termination discussion) and verifying the pipeline skips them while logging the classification result.

**Acceptance Scenarios**:

1. **Given** a meeting transcript that discusses an employee performance concern, **When** the sensitivity classifier runs, **Then** the transcript is classified as sensitive with category "hr" and a score above the configured threshold, and all processing (decision extraction, task assignment, document modification, local export) is skipped for that transcript.
2. **Given** a meeting transcript about a routine sprint planning session, **When** the sensitivity classifier runs, **Then** the transcript is classified as not sensitive and processing proceeds normally.
3. **Given** a sensitive transcript is skipped, **When** the user reviews the log output, **Then** they see the category and score of the classification along with the Google Doc URL for the skipped document (e.g., "Skipped sensitive transcript: category=hr, score=0.92, doc=https://docs.google.com/document/d/abc123/edit").
4. **Given** the `--dry-run` flag is set, **When** a transcript is classified as sensitive, **Then** the classification result and Google Doc URL are logged, but processing proceeds anyway so the user can see the full dry-run output for all transcripts.
5. **Given** the sensitivity threshold is configured at 0.7, **When** a transcript receives a score of 0.65, **Then** it is NOT classified as sensitive and processing proceeds normally.
6. **Given** the sensitivity threshold is configured at 0.7, **When** a transcript receives a score of 0.70, **Then** it IS classified as sensitive (threshold is inclusive: score >= threshold).
7. **Given** the user has set the `--verbose` flag, **When** a transcript is classified (sensitive or not), **Then** the classification reasoning is included in the log output at debug level.

---

### User Story 2 - Local Task Assignment (Priority: P2)

As a user who processes meeting transcripts with action items, I want task assignment (identifying who is responsible for each action item) to run locally on my machine so that action item content is not sent to a cloud AI service.

**Why this priority**: Task assignment is a structured extraction task (read checkbox items, identify assignees) that does not require the nuanced reasoning of cloud AI. Running it locally keeps action item data on the user's machine and eliminates the dependency on a cloud API key for this specific function.

**Independent Test**: Can be tested by providing a set of checkbox items with known assignees and verifying the local model returns the correct assignments in the same format as the current cloud-based extraction.

**Acceptance Scenarios**:

1. **Given** a document with checkbox action items that include named assignees (e.g., "Jay will schedule the follow-up"), **When** task assignment runs, **Then** the local model correctly identifies "Jay" as the assignee, returning results in the same structured format as the current cloud-based extraction.
2. **Given** a document with checkbox items that have ambiguous or group assignees (e.g., "The team will discuss"), **When** task assignment runs, **Then** the local model correctly returns no assignee for those items.
3. **Given** the system is running and the local AI service is available, **When** task assignment is requested, **Then** the local model is used instead of the cloud AI service.

---

### User Story 3 - Local-Only Mode (Priority: P2)

As a privacy-conscious user who does not want any meeting data sent to cloud AI services, I want a configuration option that runs all AI processing locally so that my meeting transcripts, action items, and decisions never leave my machine.

**Why this priority**: This completes the local AI vision. Users in regulated industries or with strict data policies need assurance that no transcript data reaches external services. This builds on US1 (sensitivity gate) and US2 (local assignments) by adding local decision extraction.

**Independent Test**: Can be tested by enabling local-only mode, running the full pipeline, and verifying that no cloud AI API calls are made while decisions and assignments are still extracted (with potentially lower quality for decisions).

**Acceptance Scenarios**:

1. **Given** local-only mode is enabled in the configuration, **When** the pipeline processes a meeting transcript, **Then** decision extraction uses the local model instead of the cloud AI service, and the same decision categories (made, deferred, open) are produced.
2. **Given** local-only mode is enabled, **When** the pipeline processes action items, **Then** task assignment uses the local model (same as US2 behavior).
3. **Given** local-only mode is enabled, **When** the pipeline processes any transcript, **Then** no network calls are made to any cloud AI service.
4. **Given** local-only mode is enabled and the local AI service is not available, **When** the user runs the pipeline, **Then** the system stops with an error explaining that local-only mode requires the local AI service to be running, with actionable fix instructions.
5. **Given** local-only mode is disabled (default), **When** the pipeline processes a transcript, **Then** decision extraction uses the cloud AI service (existing behavior) while task assignment and sensitivity gating use the local model.

---

### User Story 4 - Setup and Diagnostics (Priority: P3)

As a user setting up gcal-organizer for the first time (or diagnosing issues), I want the setup and diagnostic tools to check whether the local AI service and required models are properly installed so that I can resolve issues before running the pipeline.

**Why this priority**: Diagnostics make the other stories usable in practice. Without clear setup guidance and health checks, users would encounter cryptic errors when the local AI service is misconfigured.

**Independent Test**: Can be tested by running the diagnostic command with and without the local AI service installed, running, and with models present/missing, verifying appropriate pass/warn/fail output for each state.

**Acceptance Scenarios**:

1. **Given** the local AI service is installed, running, and all required models are present, **When** the user runs the diagnostic command, **Then** all local AI checks pass.
2. **Given** the local AI service binary is not installed, **When** the user runs the diagnostic command, **Then** the check fails with the message "Install the local AI service: brew install ollama".
3. **Given** the binary is installed but the service is not running, **When** the user runs the diagnostic command, **Then** the check fails with a message to start the service.
4. **Given** the service is running but a required model has not been pulled, **When** the user runs the diagnostic command, **Then** the check fails with a message identifying which model is missing and the command to pull it.
5. **Given** the user runs the initial setup command, **When** the local AI service is installed and running but models are not yet pulled, **Then** the setup prompts the user interactively: "Local AI models are needed for transcript screening. Pull them now? (Y/n)" and pulls the models if confirmed.
6. **Given** local AI is not configured (disabled in configuration), **When** the user runs the diagnostic command, **Then** the local AI checks are skipped entirely (not reported as failures).

---

### Edge Cases

- What happens when a transcript exceeds the local model's context window? The system truncates the transcript with a warning log and classifies/processes the truncated version. The truncation point preserves the beginning and end of the transcript (where sensitive topics are most likely to be introduced or summarized).
- What happens when the local AI service crashes mid-request? The request times out (configurable, default 120 seconds), the system treats it as a hard failure and stops the pipeline with an actionable error.
- What happens when the sensitivity score is exactly at the threshold? The transcript is treated as sensitive (threshold comparison is >=, not >).
- What happens when the local model returns malformed output (not valid structured data)? The system retries once. If the retry also fails, it stops the pipeline with an error describing the malformed response.
- What happens when both local AI and cloud AI configuration exist but local-only mode is off? The sensitivity gate and task assignment use the local model. Decision extraction uses the cloud AI service. Both systems coexist.
- What happens during `--dry-run` with a sensitive transcript? The sensitivity classification runs and is logged (with category, score, and document URL), but processing proceeds anyway so the user sees the complete dry-run output. The log clearly indicates the transcript would be skipped in a real run.
- What happens when the local AI service is configured but the user has not yet run the setup command? The diagnostic command fails for the local AI checks. The pipeline hard-stops on first run with instructions to install and configure the local AI service.
- What happens when the local AI service endpoint is configured to a remote address (not localhost)? The system logs a warning at startup: "Ollama endpoint is not localhost -- sensitive transcripts will be sent over the network." The sensitivity privacy guarantee only holds for local endpoints (localhost, 127.0.0.1, ::1).

## Requirements *(mandatory)*

### Functional Requirements

**Sensitivity Gate**:

- **FR-001**: The system MUST classify every meeting transcript for sensitivity before any other processing (decision extraction, task assignment, document modification, or local export) occurs.
- **FR-002**: The sensitivity classification MUST produce four outputs: a binary determination (sensitive or not sensitive), a category (one of: hr, legal, financial, health, termination, none), a confidence score (0.0 to 1.0), and a reasoning explanation.
- **FR-003**: When the confidence score is greater than or equal to the configured threshold, the system MUST skip all processing for that transcript.
- **FR-004**: When a transcript is skipped due to sensitivity, the system MUST log the category, score, and Google Doc URL at the standard log level. The reasoning MUST only be logged at the verbose/debug level.
- **FR-005**: The sensitivity threshold MUST be configurable with a default value of 0.7.
- **FR-006**: The sensitivity gate MUST be independently enable/disable-able via configuration, separate from the local-only mode setting.
- **FR-007**: When the `--dry-run` flag is active, the sensitivity classifier MUST run and log results (including category, score, and Google Doc URL), but processing MUST proceed regardless of the classification result.

**Local AI Service Availability**:

- **FR-008**: When local AI is configured (enabled in configuration), the local AI service MUST be available (installed, running, required models present) before the pipeline proceeds. If the service is unavailable, the system MUST stop with an actionable error message listing the specific fix steps.
- **FR-009**: The system MUST NOT fall back to cloud AI when local AI is configured but unavailable. This is a hard stop, not a graceful degradation.
- **FR-010**: The system MUST verify model availability by querying the local AI service's model listing before the first use in each run.
- **FR-011**: The system MUST communicate with the local AI service via its HTTP interface for both text generation and model listing.
- **FR-012**: The system MUST NOT add a direct client library dependency for the local AI service. Communication MUST use standard HTTP.

**Local Task Assignment**:

- **FR-013**: When local AI is configured, the system MUST use the local model for task assignment (assignee extraction from action items) instead of the cloud AI service.
- **FR-014**: The local task assignment MUST produce results in the same structured format as the existing cloud-based extraction (assignee name per action item, or null when no clear assignee exists).
- **FR-015**: The local model MUST apply the same assignment rules as the current cloud extraction: only return an assignee when a single named individual is clearly responsible; return null for groups, shared responsibility, non-subject mentions, and vague references.

**Local-Only Mode**:

- **FR-016**: The system MUST support a configuration option for local-only mode that prevents all cloud AI API calls.
- **FR-017**: In local-only mode, decision extraction MUST use the local model instead of the cloud AI service, producing the same three decision categories (made, deferred, open) with text, timestamp, and context fields.
- **FR-018**: In local-only mode, if the local AI service is unavailable, the system MUST stop with an error. The system MUST NOT fall back to cloud AI.
- **FR-019**: In non-local-only mode (default), decision extraction MUST continue to use the cloud AI service while sensitivity gating and task assignment use the local model.

**Configuration**:

- **FR-020**: The local AI configuration MUST be a nested section in the application's configuration file with settings for: enabled toggle, service endpoint, sensitivity model name, sensitivity threshold, assignment model name, and local-only toggle.
- **FR-021**: Each configurable value MUST have a sensible default: enabled=true, endpoint=http://localhost:11434, sensitivity model=granite-guardian, threshold=0.7, assignment model=granite3.2:8b, local-only=false.
- **FR-022**: The generation request timeout MUST be configurable with a default of 120 seconds.

**Doctor/Init**:

- **FR-023**: The diagnostic command MUST check whether the local AI service binary is installed on the system.
- **FR-024**: The diagnostic command MUST check whether the local AI service is currently running and accepting requests.
- **FR-025**: The diagnostic command MUST check whether the sensitivity model has been pulled and is available.
- **FR-026**: The diagnostic command MUST check whether the assignment model has been pulled and is available.
- **FR-027**: When local AI is disabled in configuration, the diagnostic command MUST skip all local AI checks entirely.
- **FR-028**: The setup command MUST prompt the user interactively to pull required models when the local AI service is running but models are not yet present.
- **FR-029**: The setup documentation MUST list the local AI service as a prerequisite with installation instructions.

### Key Entities

- **Sensitivity Result**: The output of the sensitivity classifier for a single transcript. Contains a binary determination (sensitive/not), a category (hr, legal, financial, health, termination, none), a confidence score (0.0-1.0), and a reasoning explanation string.
- **Local AI Configuration**: A nested configuration section controlling the local AI integration. Contains an enabled toggle, service endpoint URL, sensitivity model name, sensitivity threshold, assignment model name, local-only toggle, and request timeout.
- **Model Availability Status**: The result of querying the local AI service for available models. Used during startup validation and diagnostic checks to confirm required models are pulled and ready.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of transcripts containing sensitive content (HR, legal, financial, health, termination discussions) are detected and skipped before any cloud AI call or local file write occurs, as validated by a test suite of representative sensitive transcript samples.
- **SC-002**: Task assignment results from the local model match the cloud model's accuracy on a test set of 20+ action items with known assignees, achieving at least 90% agreement on assignee identification.
- **SC-003**: In local-only mode, zero network requests are made to cloud AI services during a complete pipeline run, verifiable by monitoring outbound network traffic.
- **SC-004**: When the local AI service is configured but unavailable, the pipeline stops within 5 seconds with a clear error message that includes specific fix steps (install, start, pull commands).
- **SC-005**: The diagnostic command accurately reports the status of all four local AI checks (binary installed, service running, sensitivity model pulled, assignment model pulled) in under 3 seconds.
- **SC-006**: A user following the setup documentation can go from zero local AI to a fully configured system (service installed, models pulled, configuration set) in under 10 minutes.

## Assumptions

- The local AI service runs as a separate process on the user's machine, accessible via HTTP on localhost. The service lifecycle (start/stop) is managed by the user or their system's service manager, not by gcal-organizer.
- The sensitivity classification model is designed for safety and risk classification tasks and can be prompted to identify workplace-sensitive content categories (HR, legal, financial, health, termination) with reasonable accuracy.
- The task assignment model (8B parameters) has sufficient reasoning capability to extract named assignees from structured action item text, matching the quality of the current cloud-based extraction for this specific structured task.
- The task assignment model can handle the same prompt format currently used for cloud-based extraction, producing compatible structured output.
- Decision extraction in local-only mode will produce lower-quality results than the cloud AI service, particularly for nuanced reasoning and justification. This is an acceptable tradeoff for users who prioritize data locality over extraction quality.
- The local AI service is available as a standard package manager installation (e.g., `brew install` on macOS) and supports pulling models from a public registry.
- The typical meeting transcript fits within the local model's context window. Transcripts that exceed the window are truncated rather than rejected.
- Users who configure local AI intend for it to be a hard requirement -- falling back to cloud AI when local is unavailable would violate their privacy expectations.
