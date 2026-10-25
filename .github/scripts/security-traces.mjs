// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { execFileSync } from 'node:child_process';
import { writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

const text = value => typeof value === 'string' ? value.slice(0, 5000) : null;

/** Export analysis locations, not a token or executable instructions. */
export function goSecurityTraces({ env = process.env, api } = {}) {
  if (!/^[A-Za-z0-9_-]+\/[A-Za-z0-9_.-]+$/.test(env.GITHUB_REPOSITORY || '') ||
      ['.', '..'].includes(env.GITHUB_REPOSITORY.split('/')[1]) || !env.DEFAULT_BRANCH) throw new Error('Invalid repository or branch.');
  const root = `repos/${env.GITHUB_REPOSITORY}/code-scanning/analyses`;
  const request = api || ((endpoint, accept = 'application/vnd.github+json') => JSON.parse(execFileSync('gh', [
    'api', '--hostname', 'github.com', endpoint, '-H', `Accept: ${accept}`,
  ], { env, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 })));
  const analyses = request(`${root}?ref=${encodeURIComponent(`refs/heads/${env.DEFAULT_BRANCH}`)}&per_page=100`);
  if (!Array.isArray(analyses)) throw new Error('Invalid analyses response.');
  const analysis = analyses.find(item => item.category === '/language:go' && item.tool?.name === 'CodeQL');
  if (!Number.isSafeInteger(analysis?.id) || analysis.id < 1 || !/^[0-9a-f]{40,64}$/.test(analysis.commit_sha || '')) throw new Error('No valid Go analysis is available.');
  const sarif = request(`${root}/${analysis.id}`, 'application/sarif+json');
  if (!Array.isArray(sarif.runs)) throw new Error('Invalid SARIF response.');
  const results = [];
  for (const run of sarif.runs) {
    const location = value => {
      const physical = value?.physicalLocation;
      const artifact = physical?.artifactLocation;
      return {
        path: text(artifact?.uri ?? run.artifacts?.[artifact?.index]?.location?.uri),
        line: physical?.region?.startLine, column: physical?.region?.startColumn,
        endLine: physical?.region?.endLine, endColumn: physical?.region?.endColumn,
        message: text(value?.message?.text),
      };
    };
    for (const result of run.results || []) {
      results.push({
        rule: text(result.ruleId), message: text(result.message?.text),
        location: location(result.locations?.[0]),
        flows: (result.codeFlows || []).flatMap(flow => (flow.threadFlows || []).map(thread => (thread.locations || []).map(step => location(step.location)))),
      });
    }
  }
  return { analyzedCommit: analysis.commit_sha, analysisId: analysis.id, results };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (!process.env.TRACES_PATH) throw new Error('TRACES_PATH is required.');
    const result = goSecurityTraces();
    await writeFile(process.env.TRACES_PATH, JSON.stringify(result, null, 2) + '\n', { mode: 0o600 });
    console.log(`Exported ${result.results.length} Go analysis results.`);
  } catch (error) {
    console.error(`Security trace export failed: ${error.message}`);
    process.exitCode = 1;
  }
}
