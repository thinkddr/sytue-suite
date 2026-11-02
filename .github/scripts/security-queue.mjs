// SPDX-License-Identifier: Apache-2.0
import { appendFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { validateSelection } from './security-alert.mjs';

const PAGE_SIZE = 100;
const MAX_PAGES = 20;
const MAX_RESPONSE_BYTES = 2 * 1024 * 1024;
const MAX_OPEN_REQUESTS = 2;
const PRIORITY = new Map([['critical', 0], ['high', 1], ['medium', 2], ['low', 3]]);
const object = (value) => value !== null && typeof value === 'object' && !Array.isArray(value);
const key = (type, number) => `${type}:${number}`;
const escapePattern = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

function configuration(env) {
  const repository = env.GITHUB_REPOSITORY;
  if (typeof repository !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9_.-]{1,100}$/.test(repository) || ['.', '..'].includes(repository.split('/')[1])) {
    throw new Error('GITHUB_REPOSITORY must be an owner/repository name.');
  }
  if (typeof env.DEFAULT_BRANCH !== 'string' || !env.DEFAULT_BRANCH || env.DEFAULT_BRANCH.length > 1024 || /[\s\u0000-\u001f\u007f-\u009f]/u.test(env.DEFAULT_BRANCH)) {
    throw new Error('DEFAULT_BRANCH must contain the repository default branch name.');
  }
  let api;
  try { api = new URL(env.GITHUB_API_URL || 'https://api.github.com'); }
  catch { throw new Error('GITHUB_API_URL must be a valid HTTPS GitHub API root.'); }
  if (api.protocol !== 'https:' || api.username || api.password || api.search || api.hash || !['/', '/api/v3', '/api/v3/'].includes(api.pathname)) {
    throw new Error('GITHUB_API_URL must be an HTTPS GitHub API root without credentials, query, or fragment.');
  }
  const hasManualInput = env.FINDING_TYPE !== undefined && env.FINDING_TYPE !== '' || env.ALERT_NUMBER !== undefined && env.ALERT_NUMBER !== '';
  let manual;
  if (env.GITHUB_EVENT_NAME === 'workflow_dispatch') manual = validateSelection(env.FINDING_TYPE, env.ALERT_NUMBER);
  else if (hasManualInput) throw new Error('Explicit alert selection is only allowed for workflow_dispatch.');
  const tokens = { prs: token(env.GH_TOKEN, 'GH_TOKEN') };
  if (!manual) {
    tokens.code = token(env.CODE_SCANNING_TOKEN, 'CODE_SCANNING_TOKEN');
    tokens.dependabot = token(env.SECURITY_ALERTS_TOKEN, 'SECURITY_ALERTS_TOKEN');
  }
  return {
    repository, defaultBranch: env.DEFAULT_BRANCH, manual, tokens,
    apiRoot: `${api.href.replace(/\/$/, '')}/repos/${repository}`,
    server: api.hostname === 'api.github.com' ? 'https://github.com' : api.origin,
  };
}

function token(value, name) {
  if (typeof value !== 'string' || !value.trim() || /[\r\n]/u.test(value)) throw new Error(`${name} is required and must contain a valid token.`);
  return value;
}

async function responseJson(response) {
  if (!response || !Number.isInteger(response.status) || response.redirected === true) throw new Error('GitHub returned an invalid or redirected API response.');
  if (response.status < 200 || response.status >= 300) throw new Error(`GitHub API request failed (${response.status}). Check security alert and pull request permissions.`);
  if (Number(response.headers?.get?.('content-length')) > MAX_RESPONSE_BYTES) throw new Error('GitHub API response exceeds the allowed size.');
  try {
    let text;
    if (response.body?.getReader) {
      const reader = response.body.getReader();
      const chunks = [];
      let size = 0;
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size > MAX_RESPONSE_BYTES) { await reader.cancel(); throw new Error('too large'); }
        chunks.push(value);
      }
      text = Buffer.concat(chunks).toString('utf8');
    } else {
      text = await response.text();
      if (Buffer.byteLength(text, 'utf8') > MAX_RESPONSE_BYTES) throw new Error('too large');
    }
    const value = JSON.parse(text);
    if (!Array.isArray(value) || value.length > PAGE_SIZE) throw new Error('invalid list');
    return value;
  } catch { throw new Error('GitHub returned malformed, unreadable, or oversized list JSON.'); }
}

function validNumber(value) {
  if (!Number.isSafeInteger(value) || value < 1) throw new Error('GitHub returned an invalid security alert or pull request number.');
  return value;
}

function validString(value, limit) {
  return typeof value === 'string' && value.length <= limit && !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/u.test(value);
}

function dependabotCursor(response, endpoint) {
  if (typeof response.headers?.get !== 'function') throw new Error('GitHub returned missing pagination headers.');
  const header = response.headers.get('link');
  if (header === null || header === '') return null;
  if (!validString(header, 8192) || /[\r\n]/u.test(header)) throw new Error('GitHub returned invalid pagination metadata.');
  let cursor = null;
  for (const entry of header.split(/,(?=\s*<)/u)) {
    const match = entry.match(/^\s*<([^<>]+)>\s*;\s*rel="([a-z]+)"\s*$/u);
    if (!match) throw new Error('GitHub returned invalid pagination metadata.');
    if (match[2] !== 'next') continue;
    if (cursor !== null) throw new Error('GitHub returned ambiguous pagination metadata.');
    let target;
    try { target = new URL(match[1]); }
    catch { throw new Error('GitHub returned an invalid pagination URL.'); }
    const expected = new URL(endpoint);
    if (target.origin !== expected.origin || target.pathname !== expected.pathname || target.username || target.password || target.hash || target.searchParams.getAll('after').length !== 1) {
      throw new Error('GitHub returned an unexpected pagination URL.');
    }
    cursor = target.searchParams.get('after');
    if (typeof cursor !== 'string' || !cursor || cursor.length > 2048 || /[\s\u0000-\u001f\u007f-\u009f]/u.test(cursor)) throw new Error('GitHub returned an invalid pagination cursor.');
  }
  return cursor;
}

function pullRequestAssignments(prs, { repository, server }) {
  const historical = new Set();
  const open = new Set();
  const pending = new Map();
  let openRequests = 0;
  const marker = new RegExp(`^<!-- codex-security-finding: ${escapePattern(repository)} (code-scanning|dependabot) ([1-9]\\d*) -->$`, 'gm');
  const alertUrl = new RegExp(`(?:^|[\\s(<])${escapePattern(server)}/${escapePattern(repository)}/security/(code-scanning|dependabot)/([1-9]\\d*)(?=$|[\\s)>.,])`, 'g');
  for (const pr of prs) {
    if (!object(pr) || !['open', 'closed'].includes(pr.state) || !object(pr.head) || !validString(pr.head.ref, 1024) || !pr.head.ref || /\s/u.test(pr.head.ref) || !validString(pr.title, 1024) || !(pr.body === null || validString(pr.body, 65536)) || !(pr.head.repo === null || object(pr.head.repo) && validString(pr.head.repo.full_name, 140))) {
      throw new Error('GitHub returned malformed pull request metadata.');
    }
    validNumber(pr.number);
    const sameRepository = pr.head.repo?.full_name?.toLowerCase() === repository.toLowerCase();
    const branch = sameRepository && pr.head.ref.match(/^codex\/security\/(code-scanning|dependabot)-([1-9]\d*)-.+$/u);
    const resumableBranch = branch && /^codex\/security\/(?:code-scanning|dependabot)-[1-9]\d*-[1-9]\d*-[1-9]\d*$/u.test(pr.head.ref);
    const assigned = new Set();
    if (branch) assigned.add(key(branch[1], validNumber(Number(branch[2]))));
    const marked = new Set();
    if (sameRepository) for (const match of (pr.body || '').matchAll(marker)) {
      const finding = key(match[1], validNumber(Number(match[2])));
      assigned.add(finding);
      marked.add(finding);
    }
    for (const finding of assigned) {
      historical.add(finding);
      if (pr.state === 'open') open.add(finding);
    }
    if (pr.state !== 'open') continue;
    if (sameRepository && pr.head.ref.startsWith('codex/security/')) openRequests++;
    if (assigned.size > 1) throw new Error('A security request pull request has conflicting finding identifiers.');
    if (resumableBranch && assigned.size === 1 && marked.size === 1 && !/^<!-- codex-security-submitted -->$/m.test(pr.body || '')) {
      const [finding] = assigned;
      pending.set(finding, Math.min(pending.get(finding) || pr.number, pr.number));
    }
    // Only repository-controlled branches can reserve work; public fork PRs
    // must not suppress remediation by adding alert URLs or range titles.
    if (!sameRepository) continue;
    // Existing human or agent fixes may predate this workflow's marker and branch naming.
    for (const match of (pr.body || '').matchAll(alertUrl)) open.add(key(match[1], validNumber(Number(match[2]))));
    for (const match of pr.title.matchAll(/\bcode[ -]scanning alerts? #([1-9]\d*)(?: to #([1-9]\d*))?\b/gi)) {
      const start = validNumber(Number(match[1]));
      const end = match[2] ? validNumber(Number(match[2])) : start;
      if (end < start || end - start >= 1000) throw new Error('An open pull request contains an unsupported security alert range.');
      for (let number = start; number <= end; number++) open.add(key('code-scanning', number));
    }
  }
  return { historical, open, pending, openRequests };
}

function candidate(type, alert) {
  if (!object(alert) || typeof alert.state !== 'string') throw new Error('GitHub returned malformed security alert metadata.');
  const number = validNumber(alert.number);
  const states = type === 'code-scanning' ? ['open', 'dismissed', 'fixed'] : ['open', 'dismissed', 'fixed', 'auto_dismissed'];
  if (!states.includes(alert.state)) throw new Error('GitHub returned an invalid security alert state.');
  let severity;
  if (type === 'code-scanning') {
    if (!object(alert.rule)) throw new Error('GitHub returned malformed code-scanning alert metadata.');
    severity = alert.rule.security_severity_level;
    if (severity === null || severity === undefined) {
      const fallback = { error: 'high', warning: 'medium', note: 'low', none: 'low' };
      severity = fallback[alert.rule.severity];
    }
  } else {
    if (!object(alert.security_advisory)) throw new Error('GitHub returned malformed Dependabot alert metadata.');
    severity = alert.security_advisory.severity;
  }
  if (!PRIORITY.has(severity)) throw new Error('GitHub returned an invalid security alert severity.');
  return { type, number, severity, state: alert.state };
}

/** Select at most one finding. Never writes secrets or follows response-controlled URLs. */
export async function selectSecurityFinding({ env = process.env, fetchImpl = globalThis.fetch } = {}) {
  const config = configuration(env);
  async function list(endpoint, query, authorization) {
    const values = [];
    const seen = new Set();
    const cursors = new Set();
    const cursorPagination = endpoint === 'dependabot/alerts';
    let after;
    for (let page = 1; page <= MAX_PAGES; page++) {
      const params = new URLSearchParams({ ...query, per_page: String(PAGE_SIZE) });
      if (cursorPagination) { if (after) params.set('after', after); }
      else params.set('page', String(page));
      const url = `${config.apiRoot}/${endpoint}?${params}`;
      let response;
      try {
        response = await fetchImpl(url, {
          headers: { Accept: 'application/vnd.github+json', Authorization: `Bearer ${authorization}`, 'X-GitHub-Api-Version': '2022-11-28' },
          redirect: 'error', signal: AbortSignal.timeout(30_000),
        });
      } catch { throw new Error('GitHub API request could not be completed. Check network access and API configuration.'); }
      const items = await responseJson(response);
      const nextCursor = cursorPagination ? dependabotCursor(response, `${config.apiRoot}/${endpoint}`) : null;
      for (const item of items) {
        if (!object(item)) throw new Error('GitHub returned malformed list metadata.');
        const number = validNumber(item.number);
        if (seen.has(number)) throw new Error('GitHub returned duplicate list items; retry after the repository state stabilizes.');
        seen.add(number);
        values.push(item);
      }
      if (cursorPagination) {
        if (!nextCursor) return values;
        if (cursors.has(nextCursor)) throw new Error('GitHub returned a repeated pagination cursor.');
        cursors.add(nextCursor);
        after = nextCursor;
      } else if (items.length < PAGE_SIZE) return values;
    }
    throw new Error('GitHub pagination limit reached; refusing to select from incomplete repository history.');
  }
  const prs = await list('pulls', { state: 'all', sort: 'created', direction: 'asc' }, config.tokens.prs);
  const { historical, open, pending, openRequests } = pullRequestAssignments(prs, config);
  let result;
  if (config.manual) {
    const finding = key(config.manual.type, config.manual.number);
    if (pending.has(finding)) result = { hasFinding: true, ...config.manual, existingPrNumber: pending.get(finding), reason: 'resume-pending' };
    else if (openRequests >= MAX_OPEN_REQUESTS) result = { hasFinding: false, reason: 'capacity-reached' };
    else result = open.has(finding)
      ? { hasFinding: false, reason: 'manual-already-open' }
      : { hasFinding: true, ...config.manual, reason: 'selected' };
  } else {
    // Sequential calls make API failures deterministic and keep tokens scoped to one endpoint.
    const code = await list('code-scanning/alerts', { state: 'open', ref: `refs/heads/${config.defaultBranch}`, sort: 'created', direction: 'asc' }, config.tokens.code);
    const dependabot = await list('dependabot/alerts', { state: 'open', sort: 'created', direction: 'asc' }, config.tokens.dependabot);
    const candidates = [
      ...code.map((alert) => candidate('code-scanning', alert)),
      ...dependabot.map((alert) => candidate('dependabot', alert)),
    ].filter((alert) => alert.state === 'open');
    const eligible = candidates.filter((alert) => !historical.has(key(alert.type, alert.number)) && !open.has(key(alert.type, alert.number)));
    const compare = (a, b) => PRIORITY.get(a.severity) - PRIORITY.get(b.severity) || a.number - b.number || a.type.localeCompare(b.type);
    eligible.sort(compare);
    const resumable = candidates.filter((alert) => pending.has(key(alert.type, alert.number))).sort(compare);
    if (resumable.length) {
      const selected = resumable[0];
      result = { hasFinding: true, type: selected.type, number: selected.number, existingPrNumber: pending.get(key(selected.type, selected.number)), reason: 'resume-pending' };
    } else if (openRequests >= MAX_OPEN_REQUESTS) result = { hasFinding: false, reason: 'capacity-reached' };
    else result = eligible.length
      ? { hasFinding: true, type: eligible[0].type, number: eligible[0].number, reason: 'selected' }
      : { hasFinding: false, reason: candidates.length ? 'already-assigned' : 'no-open-alerts' };
  }
  if (env.GITHUB_OUTPUT) {
    const outputs = { has_finding: String(result.hasFinding), finding_type: result.type || '', alert_number: result.number || '', existing_pr_number: result.existingPrNumber || '', reason: result.reason };
    await appendFile(env.GITHUB_OUTPUT, Object.entries(outputs).map(([name, value]) => `${name}=${value}\n`).join(''));
  }
  return result;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { const result = await selectSecurityFinding(); console.log(`Security queue checked: ${result.reason}.`); }
  catch (error) { console.error(`Security queue selection failed: ${error.message}`); process.exitCode = 1; }
}
