// SPDX-License-Identifier: Apache-2.0
import { appendFile, realpath, writeFile } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { pathToFileURL } from 'node:url';

const TYPES = new Set(['code-scanning', 'dependabot']);
const CONTROLS = /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/gu;
const MAX_RESPONSE_BYTES = 1024 * 1024;

function object(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function field(value, limit = 1000) {
  return typeof value === 'string' ? value.replace(CONTROLS, ' ').slice(0, limit) : null;
}

function requiredField(value, name, limit = 1000) {
  const result = field(value, limit);
  if (!result?.trim()) throw new Error(`The GitHub alert is missing ${name}.`);
  return result;
}

function line(value) {
  return Number.isSafeInteger(value) && value > 0 ? value : null;
}

export function validateSelection(type, number) {
  if (!TYPES.has(type)) throw new Error('FINDING_TYPE must be code-scanning or dependabot.');
  const rawNumber = typeof number === 'number' ? String(number) : number;
  if (typeof rawNumber !== 'string' || !/^[1-9]\d*$/.test(rawNumber)) {
    throw new Error('ALERT_NUMBER must be a positive integer with no spaces or extra characters.');
  }
  const parsedNumber = Number(rawNumber);
  if (!Number.isSafeInteger(parsedNumber)) throw new Error('ALERT_NUMBER is too large.');
  return { type, number: parsedNumber };
}

function validateBranch(branch) {
  if (typeof branch !== 'string' || !branch || branch.length > 1024 || /[\s\u0000-\u001f\u007f-\u009f]/u.test(branch)) {
    throw new Error('DEFAULT_BRANCH must contain the repository default branch name.');
  }
  return branch;
}

function validateOpenAlert(alert, number) {
  if (!object(alert) || alert.number !== number) {
    throw new Error('GitHub returned an alert with an unexpected alert number.');
  }
  if (alert.state !== 'open') throw new Error('The selected security alert is not open.');
}

function openDefaultInstance(instance, defaultBranch) {
  return object(instance) && instance.ref === `refs/heads/${defaultBranch}` && instance.state === 'open';
}

// Project an explicit allowlist. API responses and their nested fields are untrusted data.
export function normalizeAlert(type, number, alert, defaultBranch) {
  const selection = validateSelection(type, number);
  validateOpenAlert(alert, selection.number);

  if (type === 'code-scanning') {
    const branch = validateBranch(defaultBranch);
    const instance = alert.most_recent_instance;
    if (!openDefaultInstance(instance, branch)) {
      throw new Error('The code-scanning alert must have an open instance on the default branch. Re-run scanning there and retry, or select an open default-branch alert.');
    }
    if (!object(alert.rule) || !object(alert.tool) || !object(instance.location)) {
      throw new Error('GitHub returned malformed code-scanning alert metadata.');
    }
    return {
      ...selection,
      data: {
        rule: {
          id: requiredField(alert.rule.id, 'rule ID', 300),
          description: field(alert.rule.description, 2000),
          severity: field(alert.rule.severity, 100),
          securitySeverity: field(alert.rule.security_severity_level, 100),
        },
        tool: {
          name: requiredField(alert.tool.name, 'tool name', 300),
          version: field(alert.tool.version, 100),
        },
        location: {
          path: requiredField(instance.location.path, 'source path', 2048),
          startLine: line(instance.location.start_line),
          endLine: line(instance.location.end_line),
          startColumn: line(instance.location.start_column),
          endColumn: line(instance.location.end_column),
        },
        message: field(instance.message?.text, 5000),
      },
    };
  }

  const advisory = alert.security_advisory;
  const dependency = alert.dependency;
  const vulnerability = alert.security_vulnerability;
  if (!object(advisory) || !object(dependency) || !object(dependency.package) || !object(vulnerability)) {
    throw new Error('GitHub returned malformed Dependabot alert metadata.');
  }
  return {
    ...selection,
    data: {
      advisory: {
        ghsaId: field(advisory.ghsa_id, 100),
        cveId: field(advisory.cve_id, 100),
        severity: field(advisory.severity, 100),
        summary: field(advisory.summary, 3000),
      },
      dependency: {
        package: requiredField(dependency.package.name, 'package name', 1000),
        ecosystem: field(dependency.package.ecosystem, 100),
        manifestPath: requiredField(dependency.manifest_path, 'manifest path', 2048),
        scope: field(dependency.scope, 100),
        vulnerableVersionRange: field(vulnerability.vulnerable_version_range, 1000),
        firstPatchedVersion: field(vulnerability.first_patched_version?.identifier, 300),
      },
    },
  };
}

function repositoryName(repository) {
  if (typeof repository !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9_.-]{1,100}$/.test(repository)) {
    throw new Error('GITHUB_REPOSITORY must be an owner/repository name.');
  }
  const repo = repository.split('/')[1];
  if (repo === '.' || repo === '..') throw new Error('GITHUB_REPOSITORY contains an invalid repository name.');
  return repository;
}

function githubUrls(apiUrl, repository, type, number) {
  let api;
  try {
    api = new URL(apiUrl || 'https://api.github.com');
  } catch {
    throw new Error('GITHUB_API_URL must be a valid HTTPS GitHub API URL.');
  }
  if (api.protocol !== 'https:' || api.username || api.password || api.search || api.hash || !['/', '/api/v3', '/api/v3/'].includes(api.pathname)) {
    throw new Error('GITHUB_API_URL must be an HTTPS GitHub API root without credentials, query, or fragment.');
  }
  const apiRoot = api.href.replace(/\/$/, '');
  const server = api.hostname === 'api.github.com' ? 'https://github.com' : api.origin;
  return {
    api: `${apiRoot}/repos/${repository}/${type}/alerts/${number}`,
    finding: `${server}/${repository}/security/${type}/${number}`,
  };
}

async function readAlert(response) {
  if (!response || !Number.isInteger(response.status)) throw new Error('GitHub returned an invalid API response.');
  if (response.status < 200 || response.status >= 300) {
    if (response.status === 401 || response.status === 403) {
      throw new Error(`GitHub API request failed (${response.status}). Check SECURITY_ALERTS_TOKEN access to the selected security alert type.`);
    }
    if (response.status === 404) throw new Error('GitHub API request failed (404). Check the alert number and token access.');
    throw new Error(`GitHub API request failed (${response.status}).`);
  }
  const declaredSize = Number(response.headers?.get?.('content-length'));
  if (declaredSize > MAX_RESPONSE_BYTES) throw new Error('GitHub API response exceeds the allowed size.');
  let text;
  try {
    // Fetch Response bodies are streams; enforce the bound before reading the whole body.
    if (response.body?.getReader) {
      const reader = response.body.getReader();
      const chunks = [];
      let size = 0;
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size > MAX_RESPONSE_BYTES) {
          await reader.cancel();
          throw new Error('too large');
        }
        chunks.push(value);
      }
      text = Buffer.concat(chunks).toString('utf8');
    } else {
      text = await response.text();
      if (Buffer.byteLength(text, 'utf8') > MAX_RESPONSE_BYTES) throw new Error('too large');
    }
    return JSON.parse(text);
  } catch {
    throw new Error('GitHub returned invalid, unreadable, or oversized alert JSON.');
  }
}

async function outputPath(env) {
  const path = env.ALERT_CONTEXT_PATH;
  if (typeof path !== 'string' || !isAbsolute(path) || /[\u0000-\u001f\u007f-\u009f]/u.test(path)) {
    throw new Error('ALERT_CONTEXT_PATH must be an absolute path outside the checkout.');
  }
  let parent;
  let workspace;
  try {
    parent = await realpath(dirname(path));
    workspace = await realpath(env.GITHUB_WORKSPACE || process.cwd());
  } catch {
    throw new Error('The context directory and repository checkout must already exist.');
  }
  const destination = join(parent, basename(path));
  const within = relative(workspace, destination);
  if (within === '' || (!within.startsWith(`..${sep}`) && within !== '..' && !isAbsolute(within))) {
    throw new Error('ALERT_CONTEXT_PATH must be outside the repository checkout.');
  }
  return destination;
}

export async function prepareAlert({ env = process.env, fetchImpl = globalThis.fetch } = {}) {
  const { type, number } = validateSelection(env.FINDING_TYPE, env.ALERT_NUMBER);
  const repository = repositoryName(env.GITHUB_REPOSITORY);
  validateBranch(env.DEFAULT_BRANCH);
  const urls = githubUrls(env.GITHUB_API_URL, repository, type, number);
  const contextPath = await outputPath(env);
  const token = env.SECURITY_ALERTS_TOKEN;
  if (typeof token !== 'string' || !token.trim() || /[\r\n]/u.test(token)) {
    throw new Error('SECURITY_ALERTS_TOKEN is required. Dependabot needs a token with Dependabot alerts read access.');
  }
  async function requestJson(url) {
    let response;
    try {
      response = await fetchImpl(url, {
        headers: {
          Accept: 'application/vnd.github+json',
          Authorization: `Bearer ${token}`,
          'X-GitHub-Api-Version': '2022-11-28',
        },
        redirect: 'error',
        signal: AbortSignal.timeout(30_000),
      });
    } catch {
      throw new Error('GitHub API request could not be completed. Check network access and API configuration.');
    }
    return readAlert(response);
  }
  let alert = await requestJson(urls.api);
  // Validate identity and global state before requesting any additional resources.
  validateOpenAlert(alert, number);
  if (type === 'code-scanning' && !openDefaultInstance(alert.most_recent_instance, env.DEFAULT_BRANCH)) {
    const ref = encodeURIComponent(`refs/heads/${env.DEFAULT_BRANCH}`);
    // Construct every URL locally; never follow instances_url or pagination links from API data.
    for (let page = 1; page <= 10; page++) {
      const instances = await requestJson(`${urls.api}/instances?ref=${ref}&per_page=100&page=${page}`);
      if (!Array.isArray(instances)) throw new Error('GitHub returned malformed code-scanning instances.');
      const instance = instances.find((candidate) => openDefaultInstance(candidate, env.DEFAULT_BRANCH));
      if (instance) {
        alert = { ...alert, most_recent_instance: instance };
        break;
      }
      if (instances.length < 100) break;
    }
  }
  const normalized = normalizeAlert(type, number, alert, env.DEFAULT_BRANCH);
  const context = { ...normalized, repository, url: urls.finding };
  // Never follow an existing file or symlink, and keep the selected alert private.
  await writeFile(contextPath, `${JSON.stringify(context, null, 2)}\n`, { flag: 'wx', mode: 0o600 });
  if (env.GITHUB_OUTPUT) {
    await appendFile(env.GITHUB_OUTPUT, `alert_context_path=${contextPath}\nfinding_url=${urls.finding}\n`);
  }
  return { contextPath, findingUrl: urls.finding };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    await prepareAlert();
    console.log('Prepared the selected security alert context.');
  } catch (error) {
    console.error(`Security alert context preparation failed: ${error.message}`);
    process.exitCode = 1;
  }
}
