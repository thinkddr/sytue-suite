// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { appendFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

/** Probe only Dependabot read access; never publish a token or an alert body. */
export async function selectDependabotCredential({ env = process.env, fetchImpl = globalThis.fetch } = {}) {
  if (!/^[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9_.-]{1,100}$/.test(env.GITHUB_REPOSITORY || '') || ['.', '..'].includes(env.GITHUB_REPOSITORY.split('/')[1]) || ![undefined, '', 'https://api.github.com'].includes(env.GITHUB_API_URL)) {
    throw new Error('Invalid GitHub repository or API origin.');
  }
  const candidates = [];
  for (const [credential, value] of [['primary', env.ALERT_PRIMARY_TOKEN], ['connector', env.ALERT_CONNECTOR_TOKEN]]) {
    if (value === undefined || value === '') continue;
    if (typeof value !== 'string' || !value.trim() || /[\r\n]/u.test(value)) throw new Error('Invalid configured credential.');
    if (!candidates.some(c => c.value === value)) candidates.push({ credential, value });
  }
  if (!candidates.length) throw new Error('Configure a credential with Dependabot alerts: read for this repository.');
  const url = `https://api.github.com/repos/${env.GITHUB_REPOSITORY}/dependabot/alerts?state=open&per_page=1`;
  for (const { credential, value } of candidates) {
    let response;
    try {
      response = await fetchImpl(url, { method: 'GET', redirect: 'error', signal: AbortSignal.timeout(15000), headers: { Authorization: `Bearer ${value}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28' } });
    } catch { throw new Error('Dependabot permission probe could not reach the GitHub API.'); }
    if (response.redirected) throw new Error('Dependabot permission probe refused a redirect.');
    await response.body?.cancel();
    if (response.status === 200) return credential;
    if (![401, 403].includes(response.status)) throw new Error(`Dependabot permission probe failed (${response.status}); credentials were not retried.`);
  }
  throw new Error('Configured credentials cannot read Dependabot alerts (401/403). Grant Dependabot alerts: read for this repository; no alert inventory was accepted.');
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    if (!process.env.GITHUB_OUTPUT) throw new Error('GITHUB_OUTPUT is required.');
    const credential = await selectDependabotCredential();
    await appendFile(process.env.GITHUB_OUTPUT, `credential=${credential}\n`);
    console.log(`Dependabot read access verified with the ${credential} credential.`);
  } catch (error) { console.error(`Security credential check failed: ${error.message}`); process.exitCode = 1; }
}
