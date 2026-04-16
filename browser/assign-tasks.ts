#!/usr/bin/env npx tsx
/**
 * assign-tasks.ts - Browser automation for Google Docs task assignment
 * 
 * Uses Playwright to:
 * 1. Open a Google Doc with existing Chrome profile (via CDP)
 * 2. Find checkboxes in "Next steps" (or "Suggested next steps") section
 * 3. Click the "Assign as a task" canvas widget for each checkbox
 * 4. Fill in the assignee email and confirm
 * 
 * Usage: npx tsx assign-tasks.ts --doc <docId> --assignments <json>
 * 
 * Input JSON format:
 * [
 *   { "checkboxIndex": 0, "email": "user@example.com", "text": "Task description" },
 *   ...
 * ]
 * 
 * Output JSON format:
 * {
 *   "success": true,
 *   "results": [
 *     { "checkboxIndex": 0, "email": "user@example.com", "status": "assigned" },
 *     { "checkboxIndex": 1, "email": "other@example.com", "status": "skipped", "reason": "already assigned" }
 *   ]
 * }
 */

import { chromium, Browser, BrowserContext, Page } from 'playwright';

interface Assignment {
    checkboxIndex: number;
    email: string;
    text: string;
}

interface AssignmentResult {
    checkboxIndex: number;
    email: string;
    status: 'assigned' | 'skipped' | 'failed';
    reason?: string;
}

interface ScriptOutput {
    success: boolean;
    results: AssignmentResult[];
    error?: string;
}

/**
 * Section heading candidates to search for, in priority order (FR-005).
 * The browser tries each in order; the first match found is used.
 */
const NEXT_STEPS_HEADINGS = ['Next steps', 'Suggested next steps'] as const;

// Parse command line arguments
function parseArgs(): { docId: string; assignments: Assignment[]; chromeProfilePath: string } {
    const args = process.argv.slice(2);
    let docId = '';
    let assignmentsJson = '';
    // Default to the dedicated automation profile; the Go CLI always passes
    // --profile explicitly, so this default is only a safety fallback.
    const home = process.env.HOME || process.env.USERPROFILE || '';
    let chromeProfilePath = `${home}/.gcal-organizer/chrome-data`;

    for (let i = 0; i < args.length; i++) {
        if (args[i] === '--doc' && args[i + 1]) {
            docId = args[i + 1];
            i++;
        } else if (args[i] === '--assignments' && args[i + 1]) {
            assignmentsJson = args[i + 1];
            i++;
        } else if (args[i] === '--profile' && args[i + 1]) {
            chromeProfilePath = args[i + 1];
            i++;
        }
    }

    if (!docId) {
        throw new Error('--doc argument is required');
    }

    // Validate docId is a safe Google Docs ID (alphanumeric, hyphens, underscores).
    // Prevents open-redirect / SSRF if a crafted value is passed by an attacker
    // with local CLI access, since docId is interpolated directly into the URL
    // and the browser session holds Google authentication cookies.
    if (!/^[a-zA-Z0-9_-]{10,}$/.test(docId)) {
        throw new Error(`Invalid document ID format: "${docId}". Expected alphanumeric/underscore/hyphen, min 10 chars.`);
    }

    if (!assignmentsJson) {
        throw new Error('--assignments argument is required');
    }

    let assignments: Assignment[];
    try {
        assignments = JSON.parse(assignmentsJson);
    } catch (e) {
        throw new Error(`Failed to parse assignments JSON: ${e}`);
    }

    return { docId, assignments, chromeProfilePath };
}

// Maximum total time the script is allowed to run before aborting.
const SCRIPT_TIMEOUT_MS = 10 * 60 * 1000; // 10 minutes

// Set a hard kill-switch so a hung script never blocks the parent process.
const scriptTimer = setTimeout(() => {
    process.stderr.write('[assign] ERROR: Script exceeded 10-minute timeout, aborting\n');
    process.stdout.write(JSON.stringify({
        success: false,
        results: [],
        error: 'Script exceeded 10-minute timeout',
    }));
    process.exit(1);
}, SCRIPT_TIMEOUT_MS);
// Prevent the timer from keeping the process alive on normal exit.
scriptTimer.unref();

// Main execution
async function main(): Promise<void> {
    const output: ScriptOutput = { success: false, results: [] };

    try {
        const { docId, assignments, chromeProfilePath } = parseArgs();

        if (assignments.length === 0) {
            output.success = true;
            console.log(JSON.stringify(output));
            return;
        }

        // Connect to Chrome via CDP - Chrome must be running with --remote-debugging-port=9222
        let browser: Browser;
        try {
            browser = await chromium.connectOverCDP('http://127.0.0.1:9222');
        } catch (cdpError) {
            throw new Error(
                `Could not connect to Chrome on port 9222.\n\n` +
                `Chrome must be running with remote debugging enabled.\n` +
                `To start Chrome with debugging:\n\n` +
                `  /Applications/Google\\ Chrome.app/Contents/MacOS/Google\\ Chrome --remote-debugging-port=9222 &\n\n` +
                `Then re-run this command.`
            );
        }

        const contexts = browser.contexts();
        if (contexts.length === 0) {
            throw new Error('No browser contexts found. Chrome may not have any windows open.');
        }
        const context = contexts[0];

        const page = context.pages()[0] || await context.newPage();
        const log = (msg: string) => process.stderr.write(`[assign] ${msg}\n`);

        try {
            // Navigate to the document
            const docUrl = `https://docs.google.com/document/d/${docId}/edit`;
            await page.goto(docUrl, { waitUntil: 'domcontentloaded', timeout: 60000 });

            // Wait for document to fully load - try multiple selectors
            const editorSelectors = [
                '.kix-appview-editor',
                '.kix-page',
                '.docs-editor',
                '[role="textbox"]',
                '.kix-paragraphrenderer',
            ];

            let editorFound = false;
            for (const selector of editorSelectors) {
                try {
                    await page.waitForSelector(selector, { timeout: 15000 });
                    editorFound = true;
                    break;
                } catch {
                    // Try the next selector
                }
            }

            if (!editorFound) {
                const title = await page.title();
                throw new Error(`Document editor not found. Page title: "${title}". The page may require login or the document may not exist.`);
            }

            // Give the doc time to render
            await page.waitForTimeout(2000);

            // Navigate to the checkbox section by searching for the section header.
            // Try each candidate heading in priority order (FR-005).
            const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
            log('Navigating to checkbox section...');
            log(`  KEY: ${modifier}+f (open find)`);
            await page.keyboard.press(`${modifier}+f`);
            await page.waitForTimeout(500);

            const navFindInput = page.locator('.docs-findinput-input, input[aria-label="Find in document"]').first();
            let sectionFound = false;

            for (const heading of NEXT_STEPS_HEADINGS) {
                try {
                    await navFindInput.waitFor({ state: 'visible', timeout: 3000 });
                    log(`  FILL: find input ← "${heading}"`);
                    await navFindInput.fill('');
                    await navFindInput.fill(heading);
                    await page.waitForTimeout(500);
                } catch {
                    log(`  TYPE: "${heading}" (fallback)`);
                    await page.keyboard.type(heading, { delay: 20 });
                    await page.waitForTimeout(500);
                }

                // Check if the heading was found in the document
                const matchInfo = await page.evaluate(() => {
                    const countEl = document.querySelector('.docs-findinput-count');
                    if (countEl) {
                        const text = countEl.textContent || '';
                        const match = text.match(/(\d+)\s+of\s+(\d+)/i);
                        if (match) {
                            return { current: parseInt(match[1]), total: parseInt(match[2]) };
                        }
                    }
                    return null;
                });

                if (matchInfo && matchInfo.total > 0) {
                    log(`  Found "${heading}" (${matchInfo.current} of ${matchInfo.total})`);
                    sectionFound = true;
                    break;
                }

                log(`  "${heading}" not found, trying next candidate...`);
            }

            if (!sectionFound) {
                log('  WARNING: No matching section heading found in document (FR-007)');
            }

            log('  KEY: Enter (jump to match)');
            await page.keyboard.press('Enter');
            await page.waitForTimeout(500);
            log('  KEY: Escape (close find bar)');
            await page.keyboard.press('Escape');
            await page.waitForTimeout(500);
            log('Navigated to checkbox section');

            // Process each assignment
            for (const assignment of assignments) {
                const result = await processAssignment(page, assignment);
                output.results.push(result);

                // Delay between assignments for UI to settle
                await page.waitForTimeout(1000);
            }

            output.success = true;
        } finally {
            // Disconnect from CDP without closing the user's Chrome.
            // Swallow any error in case Chrome was already closed.
            try { browser.close(); } catch { /* already closed */ }
        }

    } catch (error) {
        output.error = error instanceof Error ? error.message : String(error);
    }

    console.log(JSON.stringify(output));
}

async function processAssignment(page: Page, assignment: Assignment): Promise<AssignmentResult> {
    const result: AssignmentResult = {
        checkboxIndex: assignment.checkboxIndex,
        email: assignment.email,
        status: 'failed',
    };

    const log = (msg: string) => process.stderr.write(`[assign] ${msg}\n`);

    try {
        // Strip control characters that the Go side (unicode.IsControl/IsPrint) would also remove:
        //  \x00-\x1f  C0 controls (NUL, CR, VT, etc.)
        //  \x7f       DEL
        //  \x80-\x9f  C1 controls (Go's unicode.IsControl covers these too)
        //  \u200b-\u200d  Zero-width spaces
        //  \u2028     Line separator
        //  \u2029     Paragraph separator
        //  \ufeff     BOM / zero-width no-break space
        // This mirrors the Go-side sanitization in internal/docs/service.go.
        const sanitized = assignment.text
            .replace(/[\x00-\x1f\x7f\x80-\x9f\u200b-\u200d\u2028\u2029\ufeff]/g, '')
            .trim();
        log(`--- Processing: "${sanitized.substring(0, 30)}..." → ${assignment.email}`);

        // ============================================================
        // Phase 1: Find the text with a unique search match
        // ============================================================
        // Start with 20 chars and increase until the find bar shows "1 of 1"
        // to avoid landing on the wrong occurrence of the text.

        const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
        log(`  KEY: ${modifier}+f (open find)`);
        await page.keyboard.press(`${modifier}+f`);
        await page.waitForTimeout(500);

        const findInput = page.locator('.docs-findinput-input, input[aria-label="Find in document"]').first();
        try {
            await findInput.waitFor({ state: 'visible', timeout: 3000 });
        } catch {
            // Find bar didn't appear
        }

        let searchLen = 20;
        const maxLen = sanitized.length;
        let searchText = '';
        let isUnique = false;

        while (searchLen <= maxLen && !isUnique) {
            searchText = sanitized.substring(0, searchLen).trim();

            try {
                await findInput.fill('');
                await findInput.fill(searchText);
            } catch {
                await page.keyboard.type(searchText, { delay: 20 });
            }
            await page.waitForTimeout(600);
            log(`  FILL: find input ← "${searchText}" (${searchText.length} chars)`);

            // Read the match count from the find bar (e.g., "1 of 3" or "1 of 1")
            const matchInfo = await page.evaluate(() => {
                // Google Docs shows match count in .docs-findinput-count
                const countEl = document.querySelector('.docs-findinput-count');
                if (countEl) {
                    const text = countEl.textContent || '';
                    // Format: "1 of 3" or "0 of 0" or similar
                    const match = text.match(/(\d+)\s+of\s+(\d+)/i);
                    if (match) {
                        return { current: parseInt(match[1]), total: parseInt(match[2]) };
                    }
                }
                return null;
            });

            if (matchInfo) {
                log(`  Match count: ${matchInfo.current} of ${matchInfo.total}`);
                if (matchInfo.total === 1) {
                    isUnique = true;
                } else if (matchInfo.total === 0) {
                    log(`  WARNING: No matches found for "${searchText}"`);
                    break;
                } else {
                    // Multiple matches — increase search length
                    searchLen += 10;
                    log(`  Multiple matches, increasing search to ${Math.min(searchLen, maxLen)} chars`);
                }
            } else {
                // Can't read the match count element (UI not ready or element absent).
                // Treat as ambiguous (non-unique) and increase search length rather than
                // proceeding blindly — a wrong checkbox assignment is worse than a failure.
                searchLen += 10;
                log(`  Could not read match count, extending search to ${Math.min(searchLen, maxLen)} chars`);
            }
        }

        // After exhausting the search text, fail the assignment rather than
        // risk landing on the wrong checkbox occurrence in the document.
        if (!isUnique) {
            log(`  ERROR: Could not find a unique match for "${sanitized.substring(0, 40)}..." — skipping`);
            await page.keyboard.press('Escape');
            result.status = 'failed';
            result.reason = 'Could not find a unique match for checkbox text — too many duplicates';
            return result;
        }

        // Press Enter to jump to the match
        log('  KEY: Enter (jump to match)');
        await page.keyboard.press('Enter');
        await page.waitForTimeout(500);

        // Close find bar — cursor stays at the found text
        log('  KEY: Escape (close find bar)');
        await page.keyboard.press('Escape');
        await page.waitForTimeout(500);

        // Now cursor is in the text body of the checkbox line.
        // The "Assign as a task" widget icon (a canvas overlay with NO DOM presence)
        // should now be visible to the left of the checkbox.

        // ============================================================
        // Phase 2: Get the line start position
        // ============================================================
        // Press Home to move cursor to the start of the current visual line.
        // With the shortened search text (20 chars), this should land us on
        // the first line of the checkbox item where the widget icon is.

        log('  KEY: Home (go to line start)');
        await page.keyboard.press('Home');
        await page.waitForTimeout(300);

        // Read cursor position at line start
        const lineStart = await page.evaluate(() => {
            const caret = document.querySelector('.kix-cursor-caret');
            if (caret) {
                const rect = caret.getBoundingClientRect();
                if (rect.height > 0 && rect.top > 0) {
                    return { x: rect.x, y: rect.y + rect.height / 2, h: rect.height };
                }
            }
            return null;
        });

        if (!lineStart || lineStart.y <= 0) {
            result.reason = 'Could not find cursor position after Home key';
            return result;
        }

        log(`  Cursor at (${lineStart.x.toFixed(0)}, ${lineStart.y.toFixed(0)})`);

        // Move cursor back into text so it stays on this line and widget remains visible
        log('  KEY: End (move back into text)');
        await page.keyboard.press('End');
        await page.waitForTimeout(200);

        // ============================================================
        // Phase 3: Hover to find the widget, then click it
        // ============================================================
        // The widget icon is rendered on canvas to the LEFT of the checkbox.
        // From browser inspection:
        //   - Text starts at ~475px, checkbox at ~455px, widget at ~443px
        //   - Widget is about 32px left of text start (lineStart.x)
        //   - Checkbox is about 20px left of text start
        // We must click the WIDGET, not the CHECKBOX (clicking checkbox toggles it!)
        //
        // Strategy: Hover at positions to the left, detect when tooltip
        // "Assign as a task" appears in the DOM, then click at that position.

        let widgetClicked = false;

        // Try hovering at various X offsets left of the line start
        // Widget ≈ 32px left, checkbox ≈ 20px left
        // We want to hit the widget (32-40px left) and avoid the checkbox (15-25px left)
        const hoverOffsets = [-35, -40, -45, -32, -50, -30, -55];

        for (const offset of hoverOffsets) {
            const hoverX = lineStart.x + offset;
            if (hoverX < 0) continue;

            log(`  HOVER: mouse.move(${hoverX.toFixed(0)}, ${lineStart.y.toFixed(0)}) [offset ${offset}]`);
            await page.mouse.move(hoverX, lineStart.y);
            await page.waitForTimeout(400);

            // Check if a tooltip containing "Assign" appeared in the DOM
            const tooltipFound = await page.evaluate(() => {
                // Check for tooltip elements
                const allEls = document.querySelectorAll('[data-tooltip], [aria-label], .docs-material-tooltip, [role="tooltip"]');
                for (const el of allEls) {
                    const tooltip = (el as HTMLElement).dataset?.tooltip || '';
                    const ariaLabel = el.getAttribute('aria-label') || '';
                    const text = el.textContent || '';
                    if (tooltip.toLowerCase().includes('assign') ||
                        ariaLabel.toLowerCase().includes('assign') ||
                        text.toLowerCase().includes('assign as a task')) {
                        const rect = el.getBoundingClientRect();
                        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2, source: 'tooltip' };
                    }
                }
                // Also check for any newly appeared overlay or popup near the specified area
                return null;
            });

            if (tooltipFound) {
                log(`  TOOLTIP FOUND at offset ${offset}!`);
                log(`  CLICK: mouse.click(${hoverX.toFixed(0)}, ${lineStart.y.toFixed(0)}) [widget]`);
                await page.mouse.click(hoverX, lineStart.y);
                await page.waitForTimeout(800);
                widgetClicked = true;
                break;
            }
        }

        // If tooltip detection didn't work, try blind click at the most likely position
        // (widget is ~35px left of line start, which is in between our attempts above)
        if (!widgetClicked) {
            log('No tooltip detected, trying blind click at widget position...');
            // Try each position, check if assignee popover appeared after each
            for (const offset of [-35, -40, -32, -45, -28]) {
                const clickX = lineStart.x + offset;
                if (clickX < 0) continue;

                log(`  CLICK: mouse.click(${clickX.toFixed(0)}, ${lineStart.y.toFixed(0)}) [blind, offset ${offset}]`);
                await page.mouse.click(clickX, lineStart.y);
                await page.waitForTimeout(800);

                // Check if the assignee popover appeared
                const popoverAppeared = await checkForAssigneePopover(page);
                if (popoverAppeared) {
                    log('  Assignee popover detected after click!');
                    widgetClicked = true;
                    break;
                }

                // If we accidentally toggled the checkbox, undo it
                log(`  KEY: ${modifier}+z (undo safety)`);
                await page.keyboard.press(`${modifier}+z`);
                await page.waitForTimeout(300);
            }
        }

        if (!widgetClicked) {
            result.reason = 'Could not find or click the Assign as task widget';
            return result;
        }

        // ============================================================
        // Phase 4: Fill in the assignee email
        // ============================================================
        // From DOM inspection:
        //   - Input class: kix-task-bubble-assignee-input-field
        //   - Input aria-label: "Open assignee picker"
        //   - Popover dialog: role="dialog" aria-label="Assign a task"

        const emailInputSelectors = [
            'input.kix-task-bubble-assignee-input-field',
            'input[aria-label="Open assignee picker"]',
            'input[aria-label*="ssignee"]',
            'input[aria-label*="Assignee"]',
            'input[type="email"]',
        ];

        let emailInput = null;
        for (const sel of emailInputSelectors) {
            try {
                const el = page.locator(sel).first();
                if (await el.isVisible({ timeout: 2000 })) {
                    emailInput = el;
                    log(`Found email input via: ${sel}`);
                    break;
                }
            } catch {
                // Try next
            }
        }

        if (!emailInput) {
            // CRITICAL: Do NOT type into the document body! That corrupts the doc.
            result.reason = 'Assignee input not found — widget may not have opened';
            return result;
        }

        log(`  FILL: assignee input ← "${assignment.email}"`);
        await emailInput.fill(assignment.email);
        await page.waitForTimeout(500);

        // ============================================================
        // Phase 5: Confirm the assignment by pressing Tab x3 + Enter
        // ============================================================
        // After filling the email, Tab three times to navigate through
        // the popover fields, then Enter to confirm the assignment.

        log('  KEY: Tab (1/3)');
        await page.keyboard.press('Tab');
        await page.waitForTimeout(300);
        log('  KEY: Tab (2/3)');
        await page.keyboard.press('Tab');
        await page.waitForTimeout(300);
        log('  KEY: Tab (3/3)');
        await page.keyboard.press('Tab');
        await page.waitForTimeout(300);
        log('  KEY: Enter (confirm assignment)');
        await page.keyboard.press('Enter');
        await page.waitForTimeout(1500);
        log('  Assignment confirmed');

        result.status = 'assigned';
        log(`✓ Successfully assigned to ${assignment.email}`);
    } catch (error) {
        result.reason = error instanceof Error ? error.message : String(error);
        log(`✗ Error: ${result.reason}`);
    }

    return result;
}

/**
 * Check if the assignee popover/dialog has appeared.
 * This is used to confirm that clicking the widget actually opened the UI.
 */
async function checkForAssigneePopover(page: Page): Promise<boolean> {
    const selectors = [
        '[role="dialog"][aria-label="Assign a task"]',
        'input.kix-task-bubble-assignee-input-field',
        'input[aria-label="Open assignee picker"]',
        '.docs-material-button-fill-primary',
        '.kix-task-bubble-assign-action-bar',
    ];

    for (const sel of selectors) {
        try {
            const el = page.locator(sel).first();
            if (await el.isVisible({ timeout: 300 })) {
                return true;
            }
        } catch {
            // Try next
        }
    }
    return false;
}

main().catch((error) => {
    console.log(JSON.stringify({
        success: false,
        results: [],
        error: error instanceof Error ? error.message : String(error),
    }));
    process.exit(1);
});
