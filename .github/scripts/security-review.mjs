// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { execFileSync } from 'node:child_process';
import { readFile, writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

export function validateReview(manifest) {
  if (manifest?.version !== 1 || !Array.isArray(manifest.files) || !manifest.files.length || !Array.isArray(manifest.reviews) || manifest.reviews.length > 100) throw new Error('Invalid review manifest.');
  const paths = new Set();
  for (const file of manifest.files) {
    if (!/^(?:services|platform)\/[A-Za-z0-9_./-]+\.go$/.test(file?.path || '') || file.path.split('/').includes('..') || !/^[0-9a-f]{40,64}$/.test(file.blob || '') || paths.has(file.path)) throw new Error('Invalid or duplicate evidence file.');
    paths.add(file.path);
  }
  const seen = new Set();
  for (const review of manifest.reviews) {
    if (!Number.isSafeInteger(review?.number) || review.number < 1 || seen.has(review.number) ||
        !paths.has(review.path) || !/^[a-z]+\/[a-z0-9_/-]+$/.test(review.rule || '') ||
        typeof review.comment !== 'string' || review.comment.length < 80 || review.comment.length > 280 || /[\u0000-\u001f\u007f]/u.test(review.comment)) throw new Error('Invalid or duplicate reviewed alert.');
    seen.add(review.number);
  }
}

function verifyEvidence(files) {
  for (const file of files) {
    const blob = execFileSync('git', ['rev-parse', '--verify', `HEAD:${file.path}`], { encoding: 'utf8' }).trim();
    if (blob !== file.blob) throw new Error(`Evidence changed: ${file.path}; review again before applying.`);
  }
}

/** Only explicitly reviewed false positives can be dismissed; never delete an alert. */
export async function reviewSecurityAlerts({ manifest, env = process.env, fetchImpl = globalThis.fetch, evidence = verifyEvidence } = {}) {
  validateReview(manifest);
  if (!/^[A-Za-z0-9_-]+\/[A-Za-z0-9_.-]+$/.test(env.GITHUB_REPOSITORY || '') || ['.', '..'].includes(env.GITHUB_REPOSITORY.split('/')[1]) ||
      !env.GH_TOKEN || /[\r\n]/u.test(env.GH_TOKEN) || !/^[0-9a-f]{40,64}$/.test(env.GITHUB_SHA || '') || !env.DEFAULT_BRANCH) throw new Error('Invalid review runtime configuration.');
  const applying = env.APPLY_REVIEW === 'true';
  if (!['true', 'false', undefined].includes(env.APPLY_REVIEW)) throw new Error('APPLY_REVIEW must be true or false.');
  evidence(manifest.files);
  const root = `https://api.github.com/repos/${env.GITHUB_REPOSITORY}`;
  async function request(path, body) {
    const response = await fetchImpl(root + path, {
      method: body ? 'PATCH' : 'GET', redirect: 'error', signal: AbortSignal.timeout(30000),
      headers: { Authorization: `Bearer ${env.GH_TOKEN}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28', ...(body ? { 'Content-Type': 'application/json' } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!response.ok || response.redirected) throw new Error(`GitHub review request failed (${response.status}).`);
    const reader = response.body.getReader();
    const chunks = [];
    let size = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > 2 * 1024 * 1024) { await reader.cancel(); throw new Error('Review response is too large.'); }
      chunks.push(value);
    }
    const raw = Buffer.concat(chunks).toString('utf8');
    try { return JSON.parse(raw); } catch { throw new Error('Invalid review API response.'); }
  }
  async function checkHead() {
    const head = await request(`/git/ref/heads/${encodeURIComponent(env.DEFAULT_BRANCH)}`);
    if (head.object?.sha !== env.GITHUB_SHA) throw new Error('Default branch advanced; rerun the review against its current revision.');
  }
  function checkAlert(alert, review) {
    if (alert.number !== review.number || alert.rule?.id !== review.rule || alert.tool?.name !== 'CodeQL' || alert.most_recent_instance?.location?.path !== review.path || alert.most_recent_instance?.ref !== `refs/heads/${env.DEFAULT_BRANCH}` || !['open', 'fixed', 'dismissed'].includes(alert.state)) throw new Error(`Alert #${review.number} changed; review again.`);
  }
  await checkHead();
  const records = [];
  // Validate every selection before performing any mutation.
  for (const review of manifest.reviews) {
    const alert = await request(`/code-scanning/alerts/${review.number}`);
    checkAlert(alert, review);
    records.push({ review, state: alert.state });
  }
  const results = [];
  for (const record of records) {
    const { review } = record;
    let state = record.state;
    let applied = false;
    if (applying && state === 'open') {
      await checkHead();
      const latest = await request(`/code-scanning/alerts/${review.number}`);
      checkAlert(latest, review);
      state = latest.state;
      if (state === 'open') {
        const updated = await request(`/code-scanning/alerts/${review.number}`, { state: 'dismissed', dismissed_reason: 'false positive', dismissed_comment: review.comment });
        if (updated.state !== 'dismissed' || updated.dismissed_reason !== 'false positive') throw new Error(`GitHub did not confirm disposition of #${review.number}.`);
        state = updated.state;
        applied = true;
      }
    }
    results.push({ number: review.number, before: record.state, state, classification: 'reviewed false positive', applied });
  }
  return { revision: env.GITHUB_SHA, applying, results };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (!process.env.REVIEW_PATH) throw new Error('REVIEW_PATH is required.');
    const manifest = JSON.parse(await readFile('.github/security-reviewed-alerts.json', 'utf8'));
    const result = await reviewSecurityAlerts({ manifest });
    await writeFile(process.env.REVIEW_PATH, JSON.stringify(result, null, 2) + '\n', { mode: 0o600 });
    console.log(`Reviewed ${result.results.length} alerts; applied ${result.results.filter(r => r.applied).length} false-positive dispositions.`);
  } catch (error) {
    console.error(`Security review failed: ${error.message}`);
    process.exitCode = 1;
  }
}
