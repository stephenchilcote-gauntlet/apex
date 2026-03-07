/**
 * playwright-visual-judge — LLM-powered visual assertions for Playwright tests.
 *
 * Uses Claude Haiku to evaluate screenshots against yes/no questions with
 * severity tiers: CRITICAL checks hard-fail tests, ADVISORY checks emit warnings.
 *
 * Two-call prompt caching strategy:
 *   Call 1: screenshot + "analyze in detail" → detailed description (cached image)
 *   Call 2: cached image + analysis + structured rubric → JSON results
 */

import Anthropic from '@anthropic-ai/sdk';
import { Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const DEFAULT_MODEL = 'claude-haiku-4-5-20251001';
const DEFAULT_ARTIFACT_DIR = 'tests/artifacts/visual';

// ── Check Types ──────────────────────────────────────────────────────────

export interface VisualCheck {
  question: string;
  severity: 'critical' | 'advisory';
}

export function critical(question: string): VisualCheck {
  return { question, severity: 'critical' };
}

export function advisory(question: string): VisualCheck {
  return { question, severity: 'advisory' };
}

// ── Result Types ─────────────────────────────────────────────────────────

export interface CheckResult {
  passed: boolean;
  severity: string;
  reason: string;
  evidence: string;
}

export interface AssertVisualOptions {
  fullPage?: boolean;
  testName?: string;
}

// ── VisualJudge ──────────────────────────────────────────────────────────

export class VisualJudge {
  private client: Anthropic;
  private model: string;
  private artifactDir: string;

  constructor(options?: {
    apiKey?: string;
    model?: string;
    artifactDir?: string;
  }) {
    const key = options?.apiKey || process.env.ANTHROPIC_API_KEY || '';
    if (!key) {
      throw new Error(
        'No API key provided. Pass apiKey option or set ANTHROPIC_API_KEY.'
      );
    }
    this.client = new Anthropic({ apiKey: key });
    this.model = options?.model || DEFAULT_MODEL;
    this.artifactDir = options?.artifactDir || DEFAULT_ARTIFACT_DIR;
  }

  async assertVisual(
    page: Page,
    checks: VisualCheck[],
    options?: AssertVisualOptions
  ): Promise<Record<string, CheckResult>> {
    if (!checks.length) {
      throw new Error('checks list is required');
    }

    const screenshot = await page.screenshot({
      fullPage: options?.fullPage ?? false,
    });
    const b64 = screenshot.toString('base64');
    const severityMap = new Map(checks.map((c) => [c.question, c.severity]));

    // Call 1: image analysis (primes prompt cache)
    const analysis = await this.analyzeScreenshot(b64);

    // Call 2: structured evaluation against rubric
    const raw = await this.evaluateChecks(b64, analysis, checks);

    // Parse and save
    const results = this.parseJsonResults(checks, raw, severityMap);
    const artifactPath = this.saveArtifacts(
      options?.testName || '',
      screenshot,
      analysis,
      raw,
      results
    );

    // Report
    this.reportResults(results, artifactPath);

    return results;
  }

  // ── LLM Calls ───────────────────────────────────────────────────────

  private async analyzeScreenshot(b64: string): Promise<string> {
    const response = await this.client.messages.create({
      model: this.model,
      max_tokens: 1024,
      messages: [
        {
          role: 'user',
          content: [
            {
              type: 'image',
              source: {
                type: 'base64',
                media_type: 'image/png',
                data: b64,
              },
              cache_control: { type: 'ephemeral' },
            },
            {
              type: 'text',
              text: 'Analyze this screenshot of a web application in detail. Describe everything you see: layout, colors, typography, spacing, alignment, components, text content, visual hierarchy, and overall quality. Be thorough.',
            },
          ],
        },
      ],
    });
    const block = response.content[0];
    if (block.type !== 'text') {
      throw new Error('Unexpected response type from analysis call');
    }
    return block.text;
  }

  private async evaluateChecks(
    b64: string,
    analysis: string,
    checks: VisualCheck[]
  ): Promise<string> {
    const checksJson = JSON.stringify(
      checks.map((c, i) => ({
        id: i + 1,
        severity: c.severity,
        question: c.question,
      })),
      null,
      2
    );

    const response = await this.client.messages.create({
      model: this.model,
      max_tokens: 2048,
      messages: [
        {
          role: 'user',
          content: [
            {
              type: 'image',
              source: {
                type: 'base64',
                media_type: 'image/png',
                data: b64,
              },
              cache_control: { type: 'ephemeral' },
            },
            {
              type: 'text',
              text: 'Analyze this screenshot of a web application in detail.',
            },
          ],
        },
        {
          role: 'assistant',
          content: analysis,
        },
        {
          role: 'user',
          content:
            'Based on your analysis of the screenshot, evaluate each check below.\n\n' +
            `Checks:\n${checksJson}\n\n` +
            'Respond with a JSON object in this EXACT format (no markdown, no extra text):\n' +
            '{\n' +
            '  "checks": [\n' +
            '    {\n' +
            '      "id": 1,\n' +
            '      "status": "pass" or "fail",\n' +
            '      "evidence": "Quote visible text or describe the specific element you see that supports your answer",\n' +
            '      "reason": "Brief explanation"\n' +
            '    }\n' +
            '  ]\n' +
            '}\n\n' +
            'Rules:\n' +
            '- Only mark "pass" if the criterion is clearly satisfied in the screenshot.\n' +
            '- For "evidence", cite specific visible text or describe exact UI elements you observe.\n' +
            '- Return ONLY the JSON object, nothing else.',
        },
      ],
    });
    const block = response.content[0];
    if (block.type !== 'text') {
      throw new Error('Unexpected response type from evaluation call');
    }
    return block.text;
  }

  // ── Parsing ─────────────────────────────────────────────────────────

  private parseJsonResults(
    checks: VisualCheck[],
    raw: string,
    severityMap: Map<string, string>
  ): Record<string, CheckResult> {
    const results: Record<string, CheckResult> = {};

    // Try JSON parse first
    try {
      let cleaned = raw.trim();
      if (cleaned.startsWith('```')) {
        cleaned = cleaned.replace(/^```(?:json)?\s*/, '');
        cleaned = cleaned.replace(/\s*```$/, '');
      }
      const data = JSON.parse(cleaned);
      const checkResults: Array<{
        status?: string;
        reason?: string;
        evidence?: string;
      }> = data.checks || [];

      for (let i = 0; i < checks.length; i++) {
        const check = checks[i];
        if (i < checkResults.length) {
          const cr = checkResults[i];
          const passed = (cr.status || '').toLowerCase() === 'pass';
          results[check.question] = {
            passed,
            severity: severityMap.get(check.question) || 'advisory',
            reason: cr.reason || '',
            evidence: cr.evidence || '',
          };
        } else {
          results[check.question] = {
            passed: false,
            severity: severityMap.get(check.question) || 'advisory',
            reason: 'No result returned by LLM for this check',
            evidence: '',
          };
        }
      }
      return results;
    } catch {
      // Fall through to line-based parsing
    }

    // Fallback: line-based parsing
    const lines = raw
      .trim()
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean);

    for (let i = 0; i < checks.length; i++) {
      const check = checks[i];
      let matched = false;
      const prefix = `${i + 1}.`;

      for (const line of lines) {
        if (line.startsWith(prefix)) {
          const rest = line.slice(prefix.length).trim();
          if (rest.toUpperCase().startsWith('PASS')) {
            const reason = rest.slice(4).replace(/^:/, '').trim();
            results[check.question] = {
              passed: true,
              severity: severityMap.get(check.question) || 'advisory',
              reason,
              evidence: '',
            };
            matched = true;
          } else if (rest.toUpperCase().startsWith('FAIL')) {
            const reason = rest.slice(4).replace(/^:/, '').trim();
            results[check.question] = {
              passed: false,
              severity: severityMap.get(check.question) || 'advisory',
              reason,
              evidence: '',
            };
            matched = true;
          }
          break;
        }
      }

      if (!matched) {
        results[check.question] = {
          passed: false,
          severity: severityMap.get(check.question) || 'advisory',
          reason: 'Could not parse result from LLM output',
          evidence: '',
        };
      }
    }

    return results;
  }

  // ── Artifacts ───────────────────────────────────────────────────────

  private saveArtifacts(
    testName: string,
    screenshot: Buffer,
    analysis: string,
    rawResponse: string,
    results: Record<string, CheckResult>
  ): string {
    const safeName = testName
      ? testName.replace(/[^\w\-.]/g, '_')
      : 'unnamed';
    const ts = Math.floor(Date.now() / 1000);
    const dir = path.join(this.artifactDir, `${safeName}_${ts}`);

    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'screenshot.png'), screenshot);
    fs.writeFileSync(path.join(dir, 'analysis.txt'), analysis);
    fs.writeFileSync(path.join(dir, 'llm_raw.txt'), rawResponse);
    fs.writeFileSync(
      path.join(dir, 'results.json'),
      JSON.stringify(results, null, 2)
    );

    return dir;
  }

  // ── Reporting ───────────────────────────────────────────────────────

  private reportResults(
    results: Record<string, CheckResult>,
    artifactDir: string
  ): void {
    console.log(`\n  📸 Screenshot: ${path.join(artifactDir, 'screenshot.png')}`);

    const criticalFailures: Array<[string, CheckResult]> = [];
    const advisoryFailures: Array<[string, CheckResult]> = [];

    for (const [question, r] of Object.entries(results)) {
      const icon = r.passed
        ? '✓'
        : r.severity === 'critical'
          ? '✗'
          : '⚠';
      const tag = `[${r.severity.toUpperCase()}]`;
      console.log(`  ${icon} ${tag} ${question}`);
      console.log(`      → ${r.reason}`);
      if (r.evidence) {
        console.log(`      evidence: ${r.evidence}`);
      }

      if (!r.passed) {
        if (r.severity === 'critical') {
          criticalFailures.push([question, r]);
        } else {
          advisoryFailures.push([question, r]);
        }
      }
    }

    for (const [question, r] of advisoryFailures) {
      console.warn(
        `[ADVISORY VISUAL] ${question}: ${r.reason} (screenshot: ${path.join(artifactDir, 'screenshot.png')})`
      );
    }

    if (criticalFailures.length > 0) {
      const msgLines = [
        `Visual assertion CRITICAL failure: ${criticalFailures.length} critical check(s) failed`,
        `Screenshot: ${path.join(artifactDir, 'screenshot.png')}`,
        `Results: ${path.join(artifactDir, 'results.json')}`,
        '',
      ];
      for (const [question, r] of criticalFailures) {
        msgLines.push(`  ✗ ${question}`);
        msgLines.push(`    → ${r.reason}`);
        if (r.evidence) {
          msgLines.push(`    evidence: ${r.evidence}`);
        }
      }
      throw new Error(msgLines.join('\n'));
    }
  }
}
