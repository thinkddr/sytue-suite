// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { createCipheriv, createPublicKey, publicEncrypt, randomBytes, constants } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

class InventoryError extends Error {
  constructor(status, kind) { super(kind); this.status = status; this.kind = kind; }
}
const text = value => typeof value === 'string' ? value.slice(0, 2000) : null;
const number = value => Number.isSafeInteger(value) && value > 0 ? value : null;
const repoName = value => typeof value === 'string' && /^[A-Za-z0-9-]+\/[A-Za-z0-9_.-]+$/.test(value) && !['.', '..'].includes(value.split('/')[1]);

/** Only projected metadata is retained; secret-scanning values never enter the report. */
export function projectAlert(source, repo, alert) {
  if (!number(alert?.number) || !repoName(repo)) throw new Error('Invalid alert identity.');
  const common = { number: alert.number, state: text(alert.state), url: `https://github.com/${repo}/security/${source}/${alert.number}` };
  if (source === 'code-scanning') return { ...common, rule: text(alert.rule?.id), severity: text(alert.rule?.security_severity_level || alert.rule?.severity), tool: text(alert.tool?.name), ref: text(alert.most_recent_instance?.ref), commit: text(alert.most_recent_instance?.commit_sha), path: text(alert.most_recent_instance?.location?.path), line: number(alert.most_recent_instance?.location?.start_line) };
  if (source === 'dependabot') return { ...common, severity: text(alert.security_advisory?.severity), ghsa: text(alert.security_advisory?.ghsa_id), summary: text(alert.security_advisory?.summary), ecosystem: text(alert.dependency?.package?.ecosystem), package: text(alert.dependency?.package?.name), manifest: text(alert.dependency?.manifest_path), vulnerableRange: text(alert.security_vulnerability?.vulnerable_version_range), fixedVersion: text(alert.security_vulnerability?.first_patched_version?.identifier) };
  if (source === 'secret-scanning') return { ...common, secretType: text(alert.secret_type), validity: text(alert.validity) };
  throw new Error('Unsupported inventory source.');
}

export function encryptInventory(report, pem) {
  const publicKey = createPublicKey(pem);
  if (publicKey.asymmetricKeyType !== 'rsa' || publicKey.asymmetricKeyDetails.modulusLength < 2048) throw new Error('A public RSA key of at least 2048 bits is required.');
  const key = randomBytes(32), iv = randomBytes(12);
  const cipher = createCipheriv('aes-256-gcm', key, iv);
  const ciphertext = Buffer.concat([cipher.update(JSON.stringify(report), 'utf8'), cipher.final()]);
  const wrappedKey = publicEncrypt({ key: publicKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' }, key);
  return { version: 1, algorithm: 'RSA-OAEP-SHA256+AES-256-GCM', wrappedKey: wrappedKey.toString('base64'), iv: iv.toString('base64'), tag: cipher.getAuthTag().toString('base64'), ciphertext: ciphertext.toString('base64') };
}

export async function accountInventory({ env = process.env, fetchImpl = globalThis.fetch } = {}) {
  if (!repoName(env.GITHUB_REPOSITORY) || !env.SECURITY_ALERTS_TOKEN?.trim() || /[\r\n]/.test(env.SECURITY_ALERTS_TOKEN) || !Number.isSafeInteger(Number(env.EXPECTED_REPOSITORIES)) || Number(env.EXPECTED_REPOSITORIES) < 1 || Number(env.EXPECTED_REPOSITORIES) > 1000) throw new Error('Invalid inventory runtime configuration.');
  const owner = env.GITHUB_REPOSITORY.split('/')[0];
  let rateLimited = false;
  async function request(path) {
    if (rateLimited) throw new InventoryError(429, 'rate_limited');
    let response;
    try { response = await fetchImpl(`https://api.github.com${path}`, { method: 'GET', redirect: 'error', signal: AbortSignal.timeout(30000), headers: { Accept: 'application/vnd.github+json', Authorization: `Bearer ${env.SECURITY_ALERTS_TOKEN}`, 'X-GitHub-Api-Version': '2022-11-28' } }); }
    catch { throw new InventoryError(null, 'network_error'); }
    if (response.redirected) throw new InventoryError(response.status, 'unexpected_redirect');
    const reader = response.body.getReader(); let size = 0; const chunks = [];
    for (;;) { const {done,value} = await reader.read(); if (done) break; size += value.byteLength; if (size > 4 * 1024 * 1024) { await reader.cancel(); throw new InventoryError(response.status, 'oversized_response'); } chunks.push(value); }
    let data; try { data = JSON.parse(Buffer.concat(chunks).toString('utf8')); } catch { throw new InventoryError(response.status, 'invalid_json'); }
    if (!response.ok) {
      if (response.status === 429 || response.headers.get('retry-after') || response.headers.get('x-ratelimit-remaining') === '0') { rateLimited = true; throw new InventoryError(response.status, 'rate_limited'); }
      const disabled = typeof data?.message === 'string' && /(?:not enabled|are disabled|is disabled)/i.test(data.message);
      throw new InventoryError(response.status, disabled ? 'not_enabled' : [401,403,404].includes(response.status) ? 'access_denied' : 'api_error');
    }
    return { data, link: response.headers.get('link') };
  }
  async function list(path, params = {}) {
    const values = [], seen = new Set(); let after;
    for (let page = 1; page <= 20; page++) {
      const query = new URLSearchParams({ ...params, per_page: '100', ...(path.endsWith('/dependabot/alerts') ? (after ? {after} : {}) : {page: String(page)}) });
      const {data,link} = await request(`${path}?${query}`);
      if (!Array.isArray(data) || data.length > 100) throw new InventoryError(200, 'invalid_list');
      for (const item of data) { const id = item?.id ?? item?.number; if (!number(id) || seen.has(id)) throw new InventoryError(200, 'duplicate_or_invalid_identity'); seen.add(id); values.push(item); }
      if (path.endsWith('/dependabot/alerts')) {
        const match = link?.match(/<([^<>]+)>;\s*rel="next"/);
        if (!match) return values;
        const next = new URL(match[1]);
        if (next.origin !== 'https://api.github.com' || next.pathname !== path || next.username || next.password || next.hash || next.searchParams.getAll('after').length !== 1) throw new InventoryError(200, 'invalid_pagination');
        const cursor = next.searchParams.get('after');
        if (!cursor || cursor.length > 2048 || /[\s\u0000-\u001f]/.test(cursor) || cursor === after) throw new InventoryError(200, 'invalid_pagination');
        after = cursor;
      } else if (data.length < 100) return values;
    }
    throw new InventoryError(200, 'pagination_limit');
  }
  const report = { observedAt: new Date().toISOString(), owner, expectedRepositories: Number(env.EXPECTED_REPOSITORIES), complete: true, repositories: [] };
  let repositories;
  try { repositories = await list('/user/repos', { affiliation: 'owner', sort: 'full_name' }); }
  catch (error) { return {...report, complete:false, discoveryError:{status:error.status ?? null, kind:error.kind || 'invalid_metadata'}}; }
  const owned = repositories.filter(r => r.owner?.login?.toLowerCase() === owner.toLowerCase() && r.full_name !== env.GITHUB_REPOSITORY);
  if (owned.length !== report.expectedRepositories) report.complete = false;
  for (const repository of owned) {
    if (!repoName(repository.full_name) || repository.full_name.split('/')[0].toLowerCase() !== owner.toLowerCase() || typeof repository.default_branch !== 'string' || !repository.default_branch || /[\u0000-\u001f]/.test(repository.default_branch)) { report.complete = false; continue; }
    const record = { repository: repository.full_name, branch: repository.default_branch, archived: !!repository.archived, sources: {} };
    for (const source of ['code-scanning','dependabot','secret-scanning']) {
      try {
        const alerts = await list(`/repos/${repository.full_name}/${source}/alerts`, {state:'open', ...(source === 'code-scanning' ? {ref:`refs/heads/${repository.default_branch}`} : {})});
        record.sources[source] = { status:'available', alerts:alerts.filter(a=>a.state==='open').map(a=>projectAlert(source,repository.full_name,a)) };
      } catch (error) { record.sources[source] = { status:error.kind || 'invalid_metadata', httpStatus:error.status ?? null }; if (error.kind !== 'not_enabled') report.complete = false; }
    }
    report.repositories.push(record);
  }
  return report;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    // Validate the recipient before accessing any private metadata.
    encryptInventory({}, process.env.INVENTORY_PUBLIC_KEY);
    if (!process.env.INVENTORY_PATH) throw new Error('INVENTORY_PATH is required.');
    const report = await accountInventory();
    const encrypted = encryptInventory(report, process.env.INVENTORY_PUBLIC_KEY);
    await mkdir(dirname(process.env.INVENTORY_PATH), {recursive:true,mode:0o700});
    await writeFile(process.env.INVENTORY_PATH, JSON.stringify(encrypted)+'\n', {mode:0o600,flag:'wx'});
    console.log(report.complete ? 'Encrypted account inventory completed.' : 'Account inventory incomplete; inspect the encrypted artifact for access gaps.');
    if (!report.complete) process.exitCode=1;
  } catch { console.error('Account inventory failed before a complete encrypted artifact could be prepared.'); process.exitCode=1; }
}
