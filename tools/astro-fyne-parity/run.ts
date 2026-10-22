// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { strict as assert } from 'node:assert';
import {
  mkdir,
  readFile,
  writeFile,
  readdir,
  copyFile,
} from 'node:fs/promises';
import { join, resolve, basename, dirname } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';

const root = resolve(import.meta.dir, '../..');
const compiler = resolve(process.argv[2] ?? 'parity/compiler');
const out = join(root, 'artifacts/astro-conversion-parity');
const generated = join(compiler, 'native/.sytue-parity-probe');
const productProbe = join(root, 'apps/fyne/cmd/.astro-conversion-probe');
for (const dir of [out, generated, productProbe, join(out, 'wrappers')])
  await mkdir(dir, { recursive: true });
const { bundleJavascript } = await import(
  pathToFileURL(join(compiler, 'src/javascript.ts')).href
);
const { generateJavascript } = await import(
  pathToFileURL(join(compiler, 'src/javascript-cli.ts')).href
);
const { chromium } = await import(
  pathToFileURL(Bun.resolveSync('playwright', compiler)).href
);
async function command(argv: string[], cwd = root) {
  const child = Bun.spawn(argv, { cwd, stdout: 'pipe', stderr: 'pipe' });
  const [stdout, stderr, status] = await Promise.all([
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
    child.exited,
  ]);
  assert.equal(status, 0, `${argv.join(' ')} failed:\n${stderr}`);
  return stdout;
}
const sha = (text: string) => createHash('sha256').update(text).digest('hex');
// Measure dependency advisories alongside conversion diagnostics. The upstream
// braces advisory currently has no patched release; keep its failure visible
// without losing the independent source/browser/native conversion evidence.
const audit = Bun.spawn(['bun', 'audit', '--audit-level=high'], {
  cwd: join(root, 'apps/frontend'), stdout: 'pipe', stderr: 'pipe',
});
const [auditOutput, auditError, auditStatus] = await Promise.all([
  new Response(audit.stdout).text(), new Response(audit.stderr).text(), audit.exited,
]);
const dependencyAudit = {
  passed: auditStatus === 0,
  exitCode: auditStatus,
  output: auditOutput + auditError,
};
await writeFile(join(out, 'dependency-audit.txt'), dependencyAudit.output);
if (!dependencyAudit.passed)
  console.log('::warning::Upstream dependency audit failed; retain its output in the measured report.');
// Supply the real generated CSS to conversion as well as to browser references.
// The source components stay unmodified; CSS/font adapters are measured explicitly.
await command(
  ['bunx', '--no-install', '@tailwindcss/cli', '-i', 'packages/ui/src/styles.css',
    '-o', join(out, 'styles.css'), '--minify'],
  join(root, 'apps/frontend'),
);
const cases: any[] = [];
for (const variant of ['primary', 'secondary', 'ghost', 'danger'])
  for (const size of ['xs', 'sm', 'md', 'lg'])
    cases.push({
      file: 'Button',
      export: 'Button',
      props: { variant, size, children: 'Paridad' },
      interactive: 'button',
    });
cases.push({
  file: 'Button',
  export: 'Button',
  props: { disabled: true, children: 'Paridad' },
  interactive: 'button',
});
cases.push({
  file: 'Button',
  export: 'Button',
  props: { loading: true, children: 'Paridad' },
  interactive: 'button',
});
for (const item of [
  { file: 'Badge', export: 'Badge', props: { children: 'Paridad' } },
  { file: 'Card', export: 'Card', props: { children: 'Paridad' } },
  {
    file: 'Form',
    export: 'Input',
    props: { type: 'text' },
    interactive: 'input',
  },
  { file: 'Form', export: 'Textarea', props: {}, interactive: 'input' },
  {
    file: 'Form',
    export: 'Switch',
    props: { label: 'Paridad', checked: true },
  },
  {
    file: 'EmptyState',
    export: 'EmptyState',
    props: { title: 'Estado vacío', description: 'Sin elementos' },
  },
  { file: 'Avatar', export: 'Avatar', props: { name: 'Ana', size: 'md' } },
  { file: 'Icon', export: 'Icon', props: { name: 'plus', size: 20 } },
])
  cases.push(item);
const results: any[] = [];
const constructors: string[] = [];
const imports: string[] = [];
const references: string[] = [];
for (const [index, item] of cases.entries()) {
  item.id = `Case${String(index).padStart(3, '0')}`;
  item.source = `apps/frontend/packages/ui/src/components/${item.file}.tsx`;
  item.sourceHash = sha(await readFile(join(root, item.source), 'utf8'));
  const wrapper = join(out, 'wrappers', `${item.id}.tsx`);
  const handlers =
    item.interactive === 'button'
      ? 'onClick={()=>setCount(value=>value+1)}'
      : item.interactive === 'input'
        ? 'value={value} onInput={(event:any)=>setValue(event.currentTarget.value)}'
        : '';
  const provider = join(root, 'apps/frontend/packages/i18n/src/preact.tsx');
  await writeFile(
    wrapper,
    `import {h} from "preact";import {useState} from "preact/hooks";
import {I18nProvider} from ${JSON.stringify(provider)};
import {${item.export} as Original} from ${JSON.stringify(join(root, item.source))};
export default function Probe(){const[count,setCount]=useState(0);const[value,setValue]=useState("Astro");return <I18nProvider locale="es"><div id="fixture"><Original {...${JSON.stringify(item.props)}} id="parity-control" ${handlers}/><p id="parity-count">{count}</p><p id="parity-value">{value}</p></div></I18nProvider>}`,
  );
  imports.push(`import ${item.id} from ${JSON.stringify(wrapper)};`);
  references.push(`${JSON.stringify(item.id)}:${item.id}`);
  const result: any = {
    id: item.id,
    source: item.source,
    sourceHash: item.sourceHash,
    export: item.export,
    props: item.props,
    roundtripVerified: false,
    pixelParityVerified: false,
  };
  try {
    const archive = await bundleJavascript(wrapper, 'default', {}, [], [join(out, 'styles.css')]);
    const path = join(out, `${item.id}.json`);
    await writeFile(path, JSON.stringify(archive));
    await generateJavascript({
      program: path,
      out: generated,
      name: item.id,
      packageName: 'main',
      direction: 'native',
    });
    result.generatedGo = true;
    result.codeHash = archive.codeHash;
    constructors.push(
      `{${JSON.stringify(item.id)},func()(*js.Widget,error){return New${item.id}(nil,nil)}}`,
    );
  } catch (error) {
    result.generatedGo = false;
    result.phase = 'generation';
    result.diagnostic = String(error);
  }
  results.push(result);
}
// Compile and mount the actual generated Go, not a JS-only approximation.
console.log(`Generated Go for ${constructors.length}/${cases.length} original UI scenarios`);
await writeFile(
  join(generated, 'main.go'),
  `package main
import("encoding/json";"fmt";"fyne.io/fyne/v2/test";js "github.com/thinkddr/astro-fyne/native/javascript")
func main(){app:=test.NewApp();defer app.Quit();results:=[]map[string]any{};for _,item:=range []struct{id string;new func()(*js.Widget,error)}{${constructors.join(',')}}{view,err:=item.new();result:=map[string]any{"id":item.id,"mounted":err==nil};if err!=nil{result["diagnostic"]=err.Error()}else{archive,e:=view.ExportJavascript();if e!=nil{result["diagnostic"]=e.Error()}else{result["archive"]=archive};view.Close()};results=append(results,result)};data,_:=json.Marshal(results);fmt.Println(string(data))}`,
);
await command(['gofmt', '-w', generated], compiler);
const native = JSON.parse(
  await command(
    ['go', 'run', './.sytue-parity-probe'],
    join(compiler, 'native'),
  ),
);
await writeFile(
  join(out, 'generated-go.json'),
  JSON.stringify(native, null, 2),
);
for (const result of results) {
  const actual = native.find((item: any) => item.id === result.id);
  if (actual) {
    result.mounted = actual.mounted;
    result.phase = 'native-mount';
    if (actual.diagnostic) result.diagnostic = actual.diagnostic;
  }
}
// Derive the page corpus from the actual tree, including dynamic/dev routes.
const pages: string[] = [];
for await (const source of new Bun.Glob(
  'apps/frontend/apps/*/src/pages/**/*.astro',
).scan({ cwd: root, onlyFiles: true }))
  pages.push(source);
for (const source of pages.sort()) {
  const file = Bun.file(join(root, source));
  if (!(await file.exists())) continue;
  const result: any = {
    id: `page-${source}`,
    source,
    sourceHash: sha(await file.text()),
    roundtripVerified: false,
    pixelParityVerified: false,
  };
  try {
    const archive = await bundleJavascript(join(root, source));
    result.bundled = true;
    result.codeHash = archive.codeHash;
    result.phase = 'page-needs-props-and-native-mount';
  } catch (error) {
    result.bundled = false;
    result.phase = 'generation';
    result.diagnostic = String(error);
  }
  results.push(result);
}
await writeFile(
  join(out, 'conversion-attempts.json'),
  JSON.stringify({ diagnosticOnly: true, cases: results }, null, 2) + '\n',
);
console.log(`Recorded conversion attempts for ${pages.length} real Astro pages`);
// Serve the same original design-system CSS and fonts to browser references.
let css = await readFile(join(out, 'styles.css'), 'utf8');
const assets = new Map<string, string>();
for (const family of ['figtree', 'inter']) {
  const dir = join(
    dirname(Bun.resolveSync(
      `@fontsource-variable/${family}`,
      join(root, 'apps/frontend/packages/ui'),
    )),
    'files',
  );
  for (const file of await readdir(dir))
    assets.set(basename(file), join(dir, file));
}
css = css.replace(/url\(([^)]+)\)/g, (whole, raw) => {
  const value = raw.replace(/^["']|["']$/g, '');
  const name = basename(value);
  return assets.has(name) ? `url("/fonts/${name}")` : whole;
});
const reference = join(out, 'reference.tsx');
await writeFile(
  reference,
  `import {h,render} from "preact";${imports.join('\n')}
const cases={${references.join(',')}};window.selectCase=(id)=>{render(null,document.getElementById("probe"));render(h(cases[id],{}),document.getElementById("probe"))};`,
);
const build = await Bun.build({
  entrypoints: [reference],
  target: 'browser',
  format: 'esm',
  jsx: { runtime: 'automatic', importSource: 'preact' },
  plugins: [
    {
      name: 'single-preact',
      setup(builder) {
        builder.onResolve({ filter: /^preact(?:\/.*)?$/ }, (args) => ({
          path: Bun.resolveSync(args.path, compiler),
        }));
      },
    },
  ],
});
assert.ok(build.success, build.logs.map((log) => log.message).join('\n'));
const script = await build.outputs[0]!.text();
const server = Bun.serve({
  hostname: '127.0.0.1',
  port: 0,
  fetch(request) {
    const path = new URL(request.url).pathname;
    if (path === '/reference.js')
      return new Response(script, {
        headers: { 'Content-Type': 'text/javascript' },
      });
    if (path === '/styles.css')
      return new Response(css, { headers: { 'Content-Type': 'text/css' } });
    if (path.startsWith('/fonts/')) {
      const file = assets.get(path.slice(7));
      return file
        ? new Response(Bun.file(file))
        : new Response('Missing font', { status: 404 });
    }
    return new Response(
      '<html lang="es" data-theme="light"><head><link rel="stylesheet" href="/styles.css"></head><body><div id="probe"></div><script type="module" src="/reference.js"></script></body></html>',
      { headers: { 'Content-Type': 'text/html' } },
    );
  },
});
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({
    viewport: { width: 400, height: 240 },
    deviceScaleFactor: 2,
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(server.url.href);
  await page.waitForFunction(
    () => typeof (window as any).selectCase === 'function',
  );
  for (const item of cases) {
    await page.evaluate((id) => (window as any).selectCase(id), item.id);
    await page.evaluate(() => document.fonts.ready);
    const loadedFonts = await page.evaluate(() =>
      Array.from(document.fonts)
        .filter((face) => face.status === 'loaded')
        .map((face) => face.family),
    );
    assert.ok(
      loadedFonts.some((family) => family.includes('Figtree')),
      `${item.id} did not load the original Figtree font: ${loadedFonts}`,
    );
    await page.evaluate(
      () =>
        new Promise((done) =>
          requestAnimationFrame(() => requestAnimationFrame(done)),
        ),
    );
    assert.deepEqual(errors, [], `${item.id} original source failed`);
    const snapshot = () =>
      page.evaluate(() => ({
        count: document.getElementById('parity-count')!.textContent,
        value: document.getElementById('parity-value')!.textContent,
      }));
    const before = await snapshot();
    if (item.interactive === 'button')
      await page
        .locator('#parity-control')
        .evaluate((element: any) => element.click());
    if (item.interactive === 'input')
      await page.locator('#parity-control').fill('Fyne');
    await page.evaluate(
      () =>
        new Promise((done) =>
          requestAnimationFrame(() => requestAnimationFrame(done)),
        ),
    );
    const after = await snapshot();
    if (item.interactive === 'button')
      assert.equal(
        after.count,
        item.props.disabled || item.props.loading ? '0' : '1',
      );
    if (item.interactive === 'input') assert.equal(after.value, 'Fyne');
    const font = await page
      .locator('#fixture')
      .evaluate((element) => getComputedStyle(element).fontFamily);
    await page
      .locator('#fixture')
      .screenshot({ path: join(out, `${item.id}-original.png`) });
    const result = results.find((result) => result.id === item.id);
    result.originalBrowser = {
      passed: true,
      before,
      after,
      font,
      loadedFonts,
      scale: 2,
      styled: true,
    };
    console.log(`${item.id}: original browser state, events and Figtree passed`);
  }
} finally {
  await browser.close();
  server.stop(true);
}
// The original native control retains the product fork, theme and callbacks.
await copyFile(
  join(import.meta.dir, 'native.go'),
  join(productProbe, 'main.go'),
);
const product = join(root, 'apps/fyne');
const modfile = join(out, 'product-probe.mod');
const originalMod = await readFile(join(product, 'go.mod'), 'utf8');
await writeFile(
  modfile,
  originalMod.replace(
    /(=>\s+)(\.\.?\/[^\s]+)/g,
    (_, prefix, path) => prefix + resolve(product, path),
  ),
);
await copyFile(join(product, 'go.sum'), join(out, 'product-probe.sum'));
await command(
  [
    'go',
    'mod',
    'edit',
    `-modfile=${modfile}`,
    '-require=github.com/thinkddr/astro-fyne/native@v0.0.0',
    `-replace=github.com/thinkddr/astro-fyne/native=${join(compiler, 'native')}`,
  ],
  product,
);
const originalNative = JSON.parse(
  await command(
    [
      'go',
      'run',
      '-mod=mod',
      `-modfile=${modfile}`,
      '-tags',
      'ci,paridad_astro_fyne',
      './cmd/.astro-conversion-probe',
      out,
    ],
    product,
  ),
);
const adapterTests = await command(
  [
    'go', 'test', '-mod=mod', `-modfile=${modfile}`, '-tags', 'ci',
    '-p=2', '-race', '-v', './webui/sytue',
  ],
  product,
);
await writeFile(join(out, 'product-adapter-tests.txt'), adapterTests);
for (const actual of originalNative.converted) {
  const result = results.find((item) => item.id === actual.id);
  assert.ok(result, `Unexpected product archive: ${actual.id}`);
  result.productNative = actual;
}
delete originalNative.converted;
results.push({
  id: 'native-button',
  ...originalNative,
  phase: 'native-scene-export',
  roundtripVerified: false,
});
const report = {
  schema: 1,
  diagnosticOnly: true,
  universalParityVerified: false,
  pixelPerfectVerified: false,
  productCommit: (await command(['git', 'rev-parse', 'HEAD'])).trim(),
  compilerCommit: (
    await command(['git', 'rev-parse', 'HEAD'], compiler)
  ).trim(),
  tolerance: { channel: 0, pixels: 0 },
  dependencyAudit,
  cases: results,
  summary: {
    cases: results.length,
    originalBrowserPassed: cases.length,
    generatedGoMounted: native.filter((item: any) => item.mounted).length,
    productNativeMounted: results.filter((item) => item.productNative?.mounted).length,
    productAdapterTestsPassed: true,
    dependencyAuditPassed: dependencyAudit.passed,
    completeRoundtrips: 0,
    pixelComparisons: 0,
    blocked: results.filter((result) => result.diagnostic).length,
  },
  note: 'Original browser/native references were exercised. Unsupported conversions remain blocked; reference captures are not converted-image comparisons. No complete parity is claimed.',
};
await writeFile(
  join(out, 'report.json'),
  JSON.stringify(report, null, 2) + '\n',
);
// Private Actions logs retain the measured JSON even when account-wide
// artifact storage is exhausted. Do not log embedded source or font bytes.
const loggedReport = new TextEncoder().encode(
  JSON.stringify(report.summary) + '\nASTRO_FYNE_PARITY_REPORT_BEGIN\n' +
  JSON.stringify(report, null, 2) + '\nASTRO_FYNE_PARITY_REPORT_END\n',
);
// Await bounded writes: a large console.log can leave a CI pipe buffer
// undrained when Bun exits, truncating the report around 64 KiB.
for (let offset = 0; offset < loggedReport.length;) {
  const written = await Bun.write(Bun.stdout, loggedReport.subarray(offset, offset + 4096));
  assert.ok(written > 0, 'Unable to preserve the private measured report');
  offset += written;
}
console.log(
  '::warning::Complete conversion parity is not verified. See the private report.json and original reference captures.',
);
if (process.env.GITHUB_STEP_SUMMARY)
  await writeFile(
    process.env.GITHUB_STEP_SUMMARY,
    `Conversion parity is **not complete**.\n\n${results.length} real source cases; ${cases.length} original-browser scenarios passed; ${report.summary.generatedGoMounted} generated native widgets mounted; ${report.summary.blocked} conversion diagnostics; **0 verified roundtrips and 0 converted-image comparisons**.\n\nPrivate evidence: report.json. Pixel tolerance remains zero.\n`,
  );
