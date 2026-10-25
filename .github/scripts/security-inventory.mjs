// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import { pathToFileURL } from 'node:url';

const MAX_PAGES = 100;
const MAX_BYTES = 8 * 1024 * 1024;
const text = (value, max = 5000) => typeof value === 'string'
  ? value.replace(/[\u0000-\u001f\u007f-\u009f]/gu, ' ').slice(0, max) : null;

/** Read-only inventory. Never follow response URLs or export credentials. */
export async function securityInventory({ env = process.env, fetchImpl = globalThis.fetch } = {}) {
  const repository = env.GITHUB_REPOSITORY;
  if (!/^[A-Za-z0-9_-]+\/[A-Za-z0-9_.-]+$/.test(repository || '') || ['.', '..'].includes(repository.split('/')[1])) {
    throw new Error('A valid GITHUB_REPOSITORY is required.');
  }
  if (!env.GH_TOKEN || /[\r\n]/u.test(env.GH_TOKEN)) throw new Error('GH_TOKEN is required.');
  if (!env.DEFAULT_BRANCH || /[\r\n]/u.test(env.DEFAULT_BRANCH)) throw new Error('DEFAULT_BRANCH is required.');
  const alerts = [];
  const seen = new Set();
  for (let page = 1; page <= MAX_PAGES; page++) {
    const url = new URL(`https://api.github.com/repos/${repository}/code-scanning/alerts`);
    url.search = new URLSearchParams({ per_page: '100', page: String(page), ref: `refs/heads/${env.DEFAULT_BRANCH}` }).toString();
    const response = await fetchImpl(url, {
      headers: { Authorization: `Bearer ${env.GH_TOKEN}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28' },
      redirect: 'error', signal: AbortSignal.timeout(30000),
    });
    if (!response.ok || response.redirected) throw new Error(`Code scanning inventory request failed (${response.status}).`);
    const reader = response.body.getReader();
    const chunks = [];
    let size = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_BYTES) { await reader.cancel(); throw new Error('Inventory response is too large.'); }
      chunks.push(value);
    }
    const items = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (!Array.isArray(items) || items.length > 100) throw new Error('Invalid inventory response.');
    for (const alert of items) {
      if (!Number.isSafeInteger(alert?.number) || alert.number < 1 || seen.has(alert.number) ||
          !['open', 'fixed', 'dismissed'].includes(alert.state) || typeof alert.rule?.id !== 'string') {
        throw new Error('Invalid or duplicate alert in inventory.');
      }
      seen.add(alert.number);
      const instance = alert.most_recent_instance;
      alerts.push({
        number: alert.number, state: alert.state,
        url: `https://github.com/${repository}/security/code-scanning/${alert.number}`,
        rule: { id: text(alert.rule.id, 300), severity: text(alert.rule.security_severity_level, 100), description: text(alert.rule.description) },
        tool: text(alert.tool?.name, 300),
        location: instance?.location ? {
          path: text(instance.location.path, 2048),
          startLine: instance.location.start_line, endLine: instance.location.end_line,
        } : null,
        message: text(instance?.message?.text),
      });
    }
    if (items.length < 100) return { repository, branch: env.DEFAULT_BRANCH, observedAt: new Date().toISOString(), alerts };
  }
  throw new Error('Inventory pagination limit reached; refusing to publish incomplete results.');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (!process.env.INVENTORY_PATH) throw new Error('INVENTORY_PATH is required.');
    const result = await securityInventory();
    await mkdir(dirname(process.env.INVENTORY_PATH), { recursive: true });
    await writeFile(process.env.INVENTORY_PATH, JSON.stringify(result, null, 2) + '\n', { mode: 0o600 });
    console.log(`Read ${result.alerts.length} alerts; ${result.alerts.filter(a => a.state === 'open').length} open.`);
  } catch (error) {
    console.error(`Security inventory failed: ${error.message}`);
    process.exitCode = 1;
  }
}
