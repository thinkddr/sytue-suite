// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { emitGo, sourceHash } from "./emit.ts";
import type { Measurements } from "./emit.ts";
import { compile } from "./parser.ts";

let directory: string;
beforeEach(async () => {
  directory = await mkdtemp(join(tmpdir(), "astro-fyne-emitter-"));
});
afterEach(async () => {
  await rm(directory, { recursive: true, force: true });
});

async function program(source: string) {
  const entry = join(directory, "Page.tsx");
  await writeFile(entry, source);
  return compile(entry);
}

const options = { name: "Page", packageName: "generated" };

test("HTML input and change events keep distinct immediate and commit callbacks", async () => {
  const source =
    await program(`export function Page({onEdit,onCommit}) { return <input
    onInput={event=>onEdit(event.currentTarget.value)}
    onChange={event=>onCommit(event.currentTarget.value)} />; }`);
  const go = emitGo(source, options);
  expect(go).toContain("OnChange: func(value string)");
  expect(go).toContain("OnCommit: func(value string)");
  expect(go).toContain("view.SetAutoRefreshEvents(false)");
});

test("invalid capture metadata or wrong style types never produce invalid Go", async () => {
  const source = await program(
    `export function Page() { return <div id="panel" />; }`,
  );
  const valid: Measurements = {
    schema: 1,
    sourceHash: sourceHash(source),
    state: "default",
    viewport: { width: 100, height: 30, scale: 1 },
    nodes: { panel: { measured: true, width: 100, height: 30 } },
  };
  const invalid: unknown[] = [
    null,
    { ...valid, viewport: { width: -1, height: 30, scale: 1 } },
    { ...valid, viewport: { width: 100, height: 30, scale: 3 } },
    { ...valid, state: "" },
    { ...valid, nodes: [] },
    { ...valid, tokens: { "--color-primary": 5 } },
    { ...valid, nodes: { panel: { measured: true, width: "100" } } },
    { ...valid, nodes: { panel: { measured: true, color: 5 } } },
    { ...valid, nodes: { panel: { measured: true, width: 1e100 } } },
    { ...valid, nodes: { panel: { measured: true, radius: -1 } } },
  ];
  for (const measurements of invalid)
    expect(() =>
      emitGo(source, {
        ...options,
        measurements: measurements as Measurements,
      }),
    ).toThrow();
});

test("missing scalar props keep undefined semantics in emitted Go", async () => {
  const source = await program(`export function Page({ missing }) {
    return <p>{String(missing)}{missing === null}{missing === undefined}</p>;
  }`);
  const go = emitGo(source, options);
  expect(go).toContain('webui.String(webui.Get(scope, "missing"))');
  expect(go).toContain('webui.Binary("===", webui.Get(scope, "missing"), nil)');
  expect(go).toContain(
    'webui.Binary("===", webui.Get(scope, "missing"), webui.Undefined)',
  );
});

test("the generated constructor reports invalid native trees before returning a widget", async () => {
  const source = await program(
    `export function Page() { return <p>Hola</p>; }`,
  );
  const go = emitGo(source, options);
  const construction = go.indexOf("view = webui.NewView(");
  const errorCheck = go.indexOf(
    "if err := view.Error(); err != nil { return nil, err }",
    construction,
  );
  const success = go.indexOf(
    "return &PageWidget{View:view}, nil",
    construction,
  );
  expect(construction).toBeGreaterThan(-1);
  expect(errorCheck).toBeGreaterThan(construction);
  expect(success).toBeGreaterThan(errorCheck);
});

test("display flex defaults to row without overriding an explicit column", async () => {
  const row = await program(
    `export function Page() { return <main className="flex"><p>A</p><p>B</p></main>; }`,
  );
  expect(emitGo(row, options)).toContain('Direction: "row"');
  const column = await program(
    `export function Page() { return <main className="flex-col flex"><p>A</p><p>B</p></main>; }`,
  );
  expect(emitGo(column, options)).toContain('Direction: "column"');
  const inline = await program(
    `export function Page() { return <main style={{display:'flex'}}><p>A</p><p>B</p></main>; }`,
  );
  expect(emitGo(inline, options)).toContain('Direction: "row"');
});

test("symbolic Tailwind colors require browser capture instead of invalid Go styles", async () => {
  for (const className of [
    "border",
    "bg-primary",
    "text-text-muted",
    "border-border-strong",
  ]) {
    const source = await program(
      `export function Page() { return <div id="panel" className="${className}" />; }`,
    );
    expect(() => emitGo(source, options)).toThrow(
      "capturar su color calculado",
    );
    const measurements: Measurements = {
      schema: 1,
      sourceHash: sourceHash(source),
      state: "default",
      viewport: { width: 100, height: 30, scale: 1 },
      nodes: {
        panel: {
          measured: true,
          x: 0,
          y: 0,
          width: 100,
          height: 30,
          background: "#123456",
          opacity: 1,
        },
      },
    };
    const go = emitGo(source, { ...options, measurements });
    expect(go).toContain('Background: "#123456"');
    expect(go).not.toContain('Background: "primary"');
    expect(go).not.toContain('Color: "text-muted"');
  }
});

test("unsupported CSS numeric units never become a string in a Go numeric field", async () => {
  const source = await program(
    `export function Page() { return <div style={{width:'100%'}} />; }`,
  );
  expect(() => emitGo(source, options)).toThrow(
    "width necesita px/rem o captura",
  );
});

test("entry constructors namespace shared component helpers", async () => {
  const source = await program(`function Shared() { return <p>Compartido</p>; }
export function Page() { return <main><Shared /><Shared /></main>; }`);
  const first = emitGo(source, { ...options, name: "First" });
  const second = emitGo(source, { ...options, name: "Second" });
  const helper = source.components.find((component) =>
    component.name.startsWith("Shared_"),
  )!.name;
  expect(first).toContain(`func buildFirst_${helper}(`);
  expect(first).not.toContain(`func buildSecond_${helper}(`);
  expect(second).toContain(`func buildSecond_${helper}(`);
});

test("lexical shadows fail explicitly rather than overwrite props or choose a module constant", async () => {
  for (const source of [
    `const label = 'module'; export function Page({label}) { return <p>{label}</p>; }`,
    `const value = 1; export function Page() { const value = 2; return <p>{value}</p>; }`,
    `import {useState} from 'preact/hooks'; const count = 10; export function Page() { const [count,setCount] = useState(0); return <p>{count}</p>; }`,
    `const props = 'module'; export function Page(props) { return <p>{props.label}</p>; }`,
  ]) {
    const compiled = await program(source);
    expect(() => emitGo(compiled, options)).toThrow("colisión lexical");
  }
});

test("a user prop named __astroProps is not overwritten by an internal helper", async () => {
  const compiled = await program(
    `export function Page({__astroProps}) { return <p>{__astroProps}</p>; }`,
  );
  const go = emitGo(compiled, options);
  expect(go).toContain('webui.ChildText(webui.Get(scope, "__astroProps"))');
  expect(go).not.toContain('scope["__astroProps"] = props');
});

test("only the updater parameter reads pending state while closed-over states keep event values", async () => {
  const compiled = await program(`import {useState} from 'preact/hooks';
export function Page() {
  const [a,setA] = useState(0); const [b,setB] = useState(1);
  return <button onClick={() => { setB(2); setA(current => current + b); setA(current => current + a); }} />;
}`);
  const go = emitGo(compiled, options);
  expect(go).toContain(
    'webui.Binary("+", webui.Get(pending, "a"), webui.Get(eventScope, "b"))',
  );
  expect(go).toContain(
    'webui.Binary("+", webui.Get(pending, "a"), webui.Get(eventScope, "a"))',
  );
  expect(go).not.toContain('webui.Get(pending, "b")');
});

test("an event parameter shadowing the state cannot replace the updater current value", async () => {
  const compiled = await program(`import {useState} from 'preact/hooks';
export function Page() { const [count,setCount] = useState(0); return <input onInput={count => setCount(current => current + 1)} />; }`);
  const go = emitGo(compiled, options);
  expect(go).toContain('eventScope["count"] = webui.Scope');
  expect(go).toContain("pending := cloneScope(scope)");
  expect(go).not.toContain("pending := cloneScope(eventScope)");
  expect(go).toContain('webui.Get(pending, "count")');
});

test("mapped handlers update the component state while loop names keep lexical values", async () => {
  const compiled = await program(`import {useState} from 'preact/hooks';
export function Page() { const [count,setCount] = useState(0); const items=[10,20]; return <main>{items.map(count => <button onClick={() => setCount(current => current + count)} />)}<p>{count}</p></main>; }`);
  const go = emitGo(compiled, options);
  const key = `componentPrefix + "/${compiled.entry}/count"`;
  expect(go).toContain("componentPrefix := prefix");
  expect(go).toContain(`pending["count"] = state[${key}]`);
  expect(go).toContain(`state[${key}] = pending["count"]`);
  expect(go).toContain(
    'webui.Binary("+", webui.Get(pending, "count"), webui.Get(eventScope, "count"))',
  );
});

test("action-only handlers do not force a source render without a state update", async () => {
  const compiled = await program(
    `export function Page({observe}) { return <button onClick={() => observe()}>Observe</button>; }`,
  );
  const go = emitGo(compiled, options);
  const tap = go.slice(go.indexOf("OnTap: func()"));
  expect(tap).toContain('actions["observe"]()');
  expect(tap).not.toContain("refresh()");
});

test("HTML forms require native submission behavior rather than a silent container", async () => {
  for (const body of [
    `<form><input /><button>Submit</button></form>`,
    `<form><button type="button">Click</button></form>`,
  ]) {
    const compiled = await program(
      `export function Page() { return ${body}; }`,
    );
    expect(() => emitGo(compiled, options)).toThrow(
      "semántica submit requiere binding nativo",
    );
  }
  const supported = await program(
    `export function Page({save}) { return <section><button type="button" onClick={()=>save()}>Save</button></section>; }`,
  );
  expect(emitGo(supported, options)).toContain('actions["save"]()');
});
