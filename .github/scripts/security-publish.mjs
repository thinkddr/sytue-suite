// SPDX-License-Identifier: Apache-2.0
import { execFile } from 'node:child_process';
import { constants } from 'node:fs';
import { appendFile, lstat, mkdir, mkdtemp, open, realpath, rm, writeFile } from 'node:fs/promises';
import { isAbsolute, join, relative, resolve, sep } from 'node:path';
import { promisify } from 'node:util';
import { pathToFileURL } from 'node:url';
import { validateSelection } from './security-alert.mjs';

const executeFile = promisify(execFile);
const MAX_BYTES = 2 * 1024 * 1024;
const SUBMITTED = '<!-- codex-security-submitted -->';
const controls = /[\u0000-\u001f\u007f-\u009f]/u;
const object = (value) => value !== null && typeof value === 'object' && !Array.isArray(value);
const escapePattern = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
class PublishError extends Error {}
const fail = (message) => { throw new PublishError(message); };
const lines = (text) => text.split(/\r?\n/u);
const normalize = (text) => text.replace(/\r\n/gu, '\n').trim();

function positive(value, name) {
  if (typeof value !== 'string' || !/^[1-9][0-9]*$/u.test(value) || !Number.isSafeInteger(Number(value))) fail(`${name} must be a positive integer.`);
  return value;
}

function absolute(value, name) {
  if (typeof value !== 'string' || !isAbsolute(value) || controls.test(value)) fail(`${name} must be an absolute path.`);
  return value;
}

function configuration(env) {
  const repository = env.GITHUB_REPOSITORY;
  if (typeof repository !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9_.-]{1,100}$/u.test(repository) || ['.', '..'].includes(repository.split('/')[1])) fail('GITHUB_REPOSITORY must be an owner/repository name.');
  const { type, number } = validateSelection(env.FINDING_TYPE, env.ALERT_NUMBER);
  if (typeof env.GH_TOKEN !== 'string' || !env.GH_TOKEN.trim() || /\s/u.test(env.GH_TOKEN)) fail('GH_TOKEN must contain a valid token.');
  const defaultBranch = env.DEFAULT_BRANCH;
  if (typeof defaultBranch !== 'string' || !defaultBranch || defaultBranch.length > 1024 || /[\s\u0000-\u001f\u007f-\u009f~^:?*\[\\]/u.test(defaultBranch) || defaultBranch.startsWith('-') || defaultBranch.startsWith('/') || defaultBranch.endsWith('/') || defaultBranch.endsWith('.') || defaultBranch.includes('..') || defaultBranch.includes('@{') || defaultBranch.includes('//') || defaultBranch.split('/').some((part) => part.startsWith('.') || part.endsWith('.lock'))) fail('DEFAULT_BRANCH must be a valid branch name.');
  const runId = positive(env.GITHUB_RUN_ID, 'GITHUB_RUN_ID');
  const attempt = positive(env.GITHUB_RUN_ATTEMPT, 'GITHUB_RUN_ATTEMPT');
  const requestPath = `docs/security-findings/${type}-${number}.md`;
  if (env.REQUEST_PATH !== requestPath) fail('REQUEST_PATH does not match the selected finding.');
  const title = `security: request Codex fix for ${type} alert #${number}`;
  if (env.PR_TITLE !== title) fail('PR_TITLE does not match the selected finding.');
  const existing = env.EXISTING_PR_NUMBER ? positive(env.EXISTING_PR_NUMBER, 'EXISTING_PR_NUMBER') : null;
  for (const name of ['GITHUB_OUTPUT', 'GITHUB_STEP_SUMMARY']) if (env[name]) absolute(env[name], name);
  return {
    repository, owner: repository.split('/')[0], repo: repository.split('/')[1], type, number,
    defaultBranch, runId, attempt, requestPath, title, existing,
    directory: absolute(env.SECURITY_REQUEST_DIR, 'SECURITY_REQUEST_DIR'),
    marker: `<!-- codex-security-finding: ${repository} ${type} ${number} -->`,
    branch: `codex/security/${type}-${number}-${runId}-${attempt}`,
  };
}

async function readPrivate(path) {
  const handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.size > MAX_BYTES) fail('A request artifact is missing or oversized.');
    const value = await handle.readFile('utf8');
    if (Buffer.byteLength(value, 'utf8') > MAX_BYTES || value.includes('\u0000')) fail('A request artifact is oversized or invalid.');
    return value;
  } finally { await handle.close(); }
}

function parseJson(value, stage) {
  try { return JSON.parse(value); }
  catch { fail(`GitHub returned invalid JSON while ${stage}.`); }
}

function validatePr(pr, config) {
  const branch = new RegExp(`^codex/security/${escapePattern(config.type)}-${config.number}-[1-9][0-9]*-[1-9][0-9]*$`, 'u');
  if (!object(pr) || pr.state !== 'OPEN' || pr.isCrossRepository !== false || pr.baseRefName !== config.defaultBranch || typeof pr.headRefName !== 'string' || !branch.test(pr.headRefName) || !object(pr.headRepositoryOwner) || typeof pr.headRepositoryOwner.login !== 'string' || pr.headRepositoryOwner.login.toLowerCase() !== config.owner.toLowerCase() || !object(pr.headRepository) || typeof pr.headRepository.name !== 'string' || pr.headRepository.name.toLowerCase() !== config.repo.toLowerCase() || typeof pr.body !== 'string' || Buffer.byteLength(pr.body, 'utf8') > MAX_BYTES || !lines(pr.body).includes(config.marker)) fail('The request PR identity, repository, branch or finding marker does not match.');
  if (pr.headRepository.nameWithOwner !== undefined && String(pr.headRepository.nameWithOwner).toLowerCase() !== config.repository.toLowerCase()) fail('The request PR repository does not match.');
  return pr;
}

function validateUrl(value, config, number, comment = false) {
  const pattern = new RegExp(`^https://github\\.com/${escapePattern(config.repository)}/pull/(${number || '[1-9][0-9]*'})${comment ? '#issuecomment-[1-9][0-9]*' : ''}$`, 'u');
  const match = value.trim().match(pattern);
  if (!match || !Number.isSafeInteger(Number(match[1]))) fail('GitHub returned an unexpected pull request URL.');
  return { number: match[1], url: `https://github.com/${config.repository}/pull/${match[1]}` };
}

async function defaultRunCommand(command, args, options) {
  const result = await executeFile(command, args, { ...options, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  return result.stdout;
}

export async function publishSecurityRequest({ env = process.env, runCommand = defaultRunCommand } = {}) {
  const config = configuration(env);
  const workspace = await realpath(env.GITHUB_WORKSPACE || process.cwd());
  if ((await lstat(config.directory)).isSymbolicLink()) fail('The request directory must not be a symlink.');
  const directory = await realpath(config.directory);
  const child = relative(workspace, directory);
  if (!(child === '..' || child.startsWith(`..${sep}`) || isAbsolute(child)) || controls.test(directory)) fail('The request directory must be outside the repository checkout.');
  const [requestBody, prBody, codexComment] = await Promise.all(['request.md', 'pr-body.md', 'codex-comment.md'].map((name) => readPrivate(join(directory, name))));
  if (!lines(prBody).includes(config.marker) || !codexComment.startsWith('@codex ') || !normalize(codexComment)) fail('The generated request artifacts do not match the selected finding.');
  const command = async (stage, binary, args) => {
    let value;
    try { value = await runCommand(binary, args, { cwd: workspace, env: { ...process.env, ...env, GH_PROMPT_DISABLED: '1' } }); }
    catch { fail(`Unable to ${stage}. Check GitHub permissions and retry; the request remains resumable.`); }
    const output = typeof value === 'string' ? value : value?.stdout;
    if (typeof output !== 'string' || Buffer.byteLength(output, 'utf8') > 16 * 1024 * 1024) fail(`Unexpected command output while trying to ${stage}.`);
    return output;
  };
  const view = async (number) => validatePr(parseJson(await command('read the request PR', 'gh', ['pr', 'view', number, '--repo', config.repository, '--json', 'headRefName,headRepositoryOwner,headRepository,isCrossRepository,baseRefName,state,body']), 'reading the request PR'), config);
  let number = config.existing;
  let url;
  if (!number) {
    // Each parent must be a real directory in this checkout; never follow a tracked symlink.
    let parent = workspace;
    for (const part of ['docs', 'security-findings']) {
      parent = join(parent, part);
      await mkdir(parent, { recursive: true });
      const info = await lstat(parent);
      if (info.isSymbolicLink() || !info.isDirectory()) fail('The request destination must contain only real directories.');
    }
    await writeFile(join(workspace, config.requestPath), requestBody, { flag: 'wx', mode: 0o600 });
    await command('stage the request file', 'git', ['add', '--', config.requestPath]);
    await command('commit the request file', 'git', ['-c', 'core.hooksPath=/dev/null', '-c', 'user.name=codex-security[bot]', '-c', 'user.email=codex-security[bot]@users.noreply.github.com', 'commit', '-m', config.title, '--only', '--', config.requestPath]);
    await command('configure GitHub push authentication', 'gh', ['auth', 'setup-git']);
    await command('push the request branch', 'git', ['-c', 'core.hooksPath=/dev/null', 'push', 'origin', `HEAD:refs/heads/${config.branch}`]);
    const created = validateUrl(await command('create the request PR', 'gh', ['pr', 'create', '--repo', config.repository, '--draft', '--base', config.defaultBranch, '--head', config.branch, '--title', config.title, '--body-file', join(directory, 'pr-body.md')]), config);
    ({ number, url } = created);
  } else url = `https://github.com/${config.repository}/pull/${number}`;
  let pr = await view(number);
  let reusedComment = false;
  if (!lines(pr.body).includes(SUBMITTED)) {
    const login = (await command('identify the connected GitHub account', 'gh', ['api', 'user', '--jq', '.login'])).trim();
    if (!/^[A-Za-z0-9][A-Za-z0-9-]{0,38}$/u.test(login)) fail('GitHub returned an invalid connected account.');
    const pages = parseJson(await command('read existing Codex mentions', 'gh', ['api', '--paginate', '--slurp', `repos/${config.repository}/issues/${number}/comments?per_page=100`]), 'reading existing Codex mentions');
    if (!Array.isArray(pages) || !pages.every((page) => Array.isArray(page) && page.every((item) => object(item) && typeof item.body === 'string' && object(item.user) && typeof item.user.login === 'string'))) fail('GitHub returned invalid request comments.');
    reusedComment = pages.flat().some((item) => item.user.login.toLowerCase() === login.toLowerCase() && normalize(item.body) === normalize(codexComment));
    if (!reusedComment) validateUrl(await command('send the Codex mention', 'gh', ['pr', 'comment', number, '--repo', config.repository, '--body-file', join(directory, 'codex-comment.md')]), config, number, true);
    // Re-read immediately before editing so human changes since dispatch are retained.
    pr = await view(number);
    if (!lines(pr.body).includes(SUBMITTED)) {
      const temporary = await mkdtemp(join(directory, 'publish-'));
      try {
        const bodyFile = join(temporary, 'pr-body.md');
        await writeFile(bodyFile, `${pr.body.replace(/\s*$/u, '')}\n\n${SUBMITTED}\n`, { flag: 'wx', mode: 0o600 });
        await command('mark the confirmed Codex handoff', 'gh', ['pr', 'edit', number, '--repo', config.repository, '--body-file', bodyFile]);
      } finally { await rm(temporary, { recursive: true, force: true }); }
    }
  }
  if (env.GITHUB_OUTPUT) await appendFile(env.GITHUB_OUTPUT, `pr_url=${url}\npr_number=${number}\n`);
  if (env.GITHUB_STEP_SUMMARY) await appendFile(env.GITHUB_STEP_SUMMARY, `Encargo de seguridad: [PR #${number}](${url}). Mención @codex confirmada con la cuenta del token; comprueba la reacción del conector en el PR.\n`);
  return { pr_url: url, pr_number: number, reusedComment };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { await publishSecurityRequest(); console.log('Published the native Codex security finding handoff.'); }
  catch (error) { console.error(`Security request publishing failed: ${error instanceof PublishError ? error.message : 'Unable to validate configuration or access request files.'}`); process.exitCode = 1; }
}
