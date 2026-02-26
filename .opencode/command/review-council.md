---
description: Run the three-reviewer governance council to audit codebase compliance.
---

# Command: /review-council

## Description

Review the current codebase for compliance with the Behavioral Constraints in `AGENTS.md` using the review council (The Adversary, The Architect, The Guard).

## Instructions

1. Delegate the review to all three council agents in parallel using the Task tool:
   - `reviewer-adversary` — audits for security, resilience, efficiency, and constraint violations
   - `reviewer-architect` — audits for architectural alignment, coding conventions, and plan adherence
   - `reviewer-guard` — audits for intent drift, neighborhood impact, and zero-waste compliance

   For each agent, instruct it to review the current changes and return its verdict (**APPROVE** or **REQUEST CHANGES**) along with all findings.

2. Collect all **REQUEST CHANGES** findings from the three reviewers. If all three return **APPROVE**, report the result and stop.

3. If there are **REQUEST CHANGES**, address the findings by making the necessary code fixes. Then re-run all three reviewers to verify the fixes. Repeat this loop until all three return **APPROVE** or the process has exceeded 3 iterations.

4. If 3 iterations are exceeded, ask the user whether to continue or stop.

5. Provide a final report to the user:
   - What was found in each iteration
   - What was fixed
   - If stopped early, the current set of outstanding **REQUEST CHANGES**
   - If there were persistent circular **REQUEST CHANGES** (fixes for one reviewer cause failures in another), report those with additional detail so the user can make an informed decision

## Verdict

The council returns **APPROVE** only when all three reviewers return **APPROVE**. Any single **REQUEST CHANGES** means the council verdict is **REQUEST CHANGES**.
