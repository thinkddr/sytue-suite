// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { compile, ConversionError } from "./parser.ts";
import type { Node } from "./ir.ts";

let directory: string;
beforeEach(async () => {
  directory = await mkdtemp(join(tmpdir(), "astro-fyne-parser-"));
});
afterEach(async () => {
  await rm(directory, { recursive: true, force: true });
});

async function source(path: string, text: string): Promise<string> {
  const file = join(directory, path);
  await mkdir(dirname(file), { recursive: true });
  await writeFile(file, text);
  return file;
}

function elements(nodes: Node[]): Extract<Node, { kind: "element" }>[] {
  return nodes.flatMap((node) => {
    if (node.kind === "element") return [node, ...elements(node.children)];
    if (node.kind === "component" || node.kind === "each")
      return elements(node.children);
    if (node.kind === "conditional")
      return [...elements(node.yes), ...elements(node.no)];
    return [];
  });
}

test("Astro imports a Preact component, preserving props, state, events and list branches", async () => {
  const entry = await source(
    "Page.astro",
    `---
import { Form as ContactForm } from './Form.js';
const title = 'Contactos';
const people = [{ name: 'Ana', active: true }, { name: 'Luis', active: false }];
---
<main id="page"><h1>{title}</h1><ContactForm client:load people={people} /></main>`,
  );
  await source(
    "Form.tsx",
    `import { useState as state } from 'preact/hooks';
export function Form({ people, onSave }) {
  const [name, setName] = state('');
  return <form id="form">
    <label>Nombre<input value={name} onInput={event => setName(event.currentTarget.value)} /></label>
    <button disabled={!name} onClick={() => onSave(name)}>Guardar</button>
    <ul>{people.map((person, index) => person.active ? <li id={\`person-\${index}\`}>{person.name}</li> : null)}</ul>
  </form>;
}`,
  );
  const program = await compile(entry);
  expect(program.sources.map((s) => s.path)).toEqual([
    "Form.tsx",
    "Page.astro",
  ]);
  expect(program.actions).toEqual(["onSave"]);
  const page = program.components.find((c) => c.name === program.entry)!;
  expect(elements(page.body).map((n) => n.tag)).toEqual(["main", "h1"]);
  const form = program.components.find((c) => c.name.startsWith("Form_"))!;
  expect(form.props).toEqual(["people", "onSave"]);
  expect(form.states).toEqual([
    {
      name: "name",
      setter: "setName",
      initial: { kind: "literal", value: "" },
    },
  ]);
  const input = elements(form.body).find((n) => n.tag === "input")!;
  expect(input.events.onInput!.parameter).toBe("event");
  expect(input.events.onInput!.steps[0]!.kind).toBe("set");
  expect(input.events.onInput!.steps[0]!.name).toBe("name");
  const button = elements(form.body).find((n) => n.tag === "button")!;
  expect(button.events.onClick!.steps).toEqual([
    { kind: "call", name: "onSave", args: [{ kind: "name", name: "name" }] },
  ]);
  const ul = elements(form.body).find((n) => n.tag === "ul")!;
  expect(ul.children[0]!.kind).toBe("each");
  if (ul.children[0]!.kind === "each") {
    expect(ul.children[0]!.index).toBe("index");
    expect(ul.children[0]!.children[0]!.kind).toBe("conditional");
  }
});

test("Astro expressions with JSX map and conditionals remain declarative", async () => {
  const entry = await source(
    "Page.astro",
    `---
const items = ['Uno', 'Dos'];
const visible = true;
---
<section>{visible ? <ul>{items.map((item) => <li>{item}</li>)}</ul> : <p>Vacío</p>}</section>`,
  );
  const program = await compile(entry);
  const section = elements(program.components[0]!.body)[0]!;
  expect(section.children[0]!.kind).toBe("conditional");
  const list = elements(section.children).find((n) => n.tag === "ul")!;
  expect(list.children[0]!.kind).toBe("each");
  expect(elements(list.children)[0]!.children[0]!).toEqual({
    kind: "text",
    value: { kind: "name", name: "item" },
  });
});

test("setter updater reads the state and sequential handler steps are preserved", async () => {
  const entry = await source(
    "Counter.tsx",
    `import { useState } from 'preact/hooks';
export default ({ onCount }) => {
  const [count, setCount] = useState(0);
  const add = () => { setCount(current => current + 1); onCount(count); };
  return <button onClick={add}>{count}</button>;
};`,
  );
  const program = await compile(entry);
  const button = elements(program.components[0]!.body)[0]!;
  expect(button.events.onClick!.steps).toEqual([
    {
      kind: "set",
      name: "count",
      updater: true,
      args: [
        {
          kind: "binary",
          op: "+",
          left: { kind: "current", name: "count" },
          right: { kind: "literal", value: 1 },
        },
      ],
    },
    { kind: "call", name: "onCount", args: [{ kind: "name", name: "count" }] },
  ]);
});

test("object props and updater batches retain the distinction from closure values", async () => {
  const entry = await source(
    "Counter.tsx",
    `import { useState } from 'preact/hooks';
export function Counter(props) {
  const [count, setCount] = useState(0);
  return <button onClick={() => {
    setCount(count + 1); setCount(previous => previous + 1); setCount(count + 2);
  }}>{props.title}{count}</button>;
}`,
  );
  const program = await compile(entry);
  const counter = program.components[0]!;
  expect(counter.propsObject).toBe("props");
  expect(counter.props).toEqual([]);
  const steps = elements(counter.body)[0]!.events.onClick!.steps;
  expect(steps.map((step) => step.updater === true)).toEqual([
    false,
    true,
    false,
  ]);
  expect(steps.map((step) => step.args[0]!)).toEqual([
    {
      kind: "binary",
      op: "+",
      left: { kind: "name", name: "count" },
      right: { kind: "literal", value: 1 },
    },
    {
      kind: "binary",
      op: "+",
      left: { kind: "current", name: "count" },
      right: { kind: "literal", value: 1 },
    },
    {
      kind: "binary",
      op: "+",
      left: { kind: "name", name: "count" },
      right: { kind: "literal", value: 2 },
    },
  ]);
});

test("updater parameters remain distinct from captured names of this and other states", async () => {
  const entry = await source(
    "Page.tsx",
    `import {useState} from 'preact/hooks';
export function Page() {
  const [a,setA] = useState(0); const [b,setB] = useState(1);
  return <button onClick={() => { setB(2); setA(current => current + b); setA(current => current + a); }} />;
}`,
  );
  const compiled = await compile(entry);
  const steps = elements(compiled.components[0]!.body)[0]!.events.onClick!
    .steps;
  expect(steps[1]!.args[0]).toEqual({
    kind: "binary",
    op: "+",
    left: { kind: "current", name: "a" },
    right: { kind: "name", name: "b" },
  });
  expect(steps[2]!.args[0]).toEqual({
    kind: "binary",
    op: "+",
    left: { kind: "current", name: "a" },
    right: { kind: "name", name: "a" },
  });
});

test("an updater alias shadows an outer state only inside that updater", async () => {
  const entry = await source(
    "Page.tsx",
    `import {useState} from 'preact/hooks';
export function Page() {
  const [prev,setPrev] = useState(2); const [count,setCount] = useState(0);
  return <button onClick={() => { setPrev(9); setCount(prev => prev + 1); setCount(current => current + prev); }} />;
}`,
  );
  const compiled = await compile(entry);
  const steps = elements(compiled.components[0]!.body)[0]!.events.onClick!
    .steps;
  expect(steps[1]!.args[0]).toEqual({
    kind: "binary",
    op: "+",
    left: { kind: "current", name: "count" },
    right: { kind: "literal", value: 1 },
  });
  expect(steps[2]!.args[0]).toEqual({
    kind: "binary",
    op: "+",
    left: { kind: "current", name: "count" },
    right: { kind: "name", name: "prev" },
  });
});

test("handlers reject unreachable steps after return with their source location", async () => {
  const entry = await source(
    "Page.tsx",
    `import {useState} from 'preact/hooks';
export function Page() {
  const [count,setCount] = useState(0);
  return <button onClick={() => { return setCount(1); setCount(2); }} />;
}`,
  );
  await expect(compile(entry)).rejects.toThrow(
    "Código después de return en un handler",
  );
});

test("updater parameters with defaults or rest require an explicit adapter", async () => {
  for (const parameter of ["current = 1", "...current"]) {
    const entry = await source(
      "Page.tsx",
      `import {useState} from 'preact/hooks';
export function Page() { const [count,setCount] = useState(0); return <button onClick={() => setCount((${parameter}) => current + 1)} />; }`,
    );
    await expect(compile(entry)).rejects.toThrow(
      "El updater del setter debe ser valor",
    );
  }
});

test("inline and imported CSS require measurement and all stylesheet hashes are kept", async () => {
  const entry = await source(
    "Page.astro",
    `---
import './base.css';
const { title } = Astro.props;
---
<h1>{title}</h1><style>h1 { color: rebeccapurple }</style>`,
  );
  await source("base.css", `@import './spacing.css'; h1 { padding: 10.5px }`);
  await source("spacing.css", `h1 { margin: 0 }`);
  const program = await compile(entry);
  expect(program.hasStyles).toBe(true);
  expect(program.sources.map((item) => item.path)).toEqual([
    "Page.astro",
    "base.css",
    "spacing.css",
  ]);
  expect(elements(program.components[0]!.body).map((node) => node.tag)).toEqual(
    ["h1"],
  );
});

test("default imports and named aliases resolve to the same canonical component", async () => {
  const entry = await source(
    "Page.tsx",
    `import Primary from './Button';
import { Action as Secondary } from './Button';
export default function Page() { return <main><Primary title="A" /><Secondary title="B" /></main>; }`,
  );
  await source(
    "Button.tsx",
    `export function Action({ title }) { return <button>{title}</button>; }
export default Action;`,
  );
  const program = await compile(entry);
  expect(program.components).toHaveLength(2);
  const page = program.components.find((c) => c.name === program.entry)!;
  const children = elements(page.body)[0]!.children;
  expect(children[0]!.kind).toBe("component");
  expect(children[1]!.kind).toBe("component");
  if (children[0]!.kind === "component" && children[1]!.kind === "component") {
    expect(children[0]!.name).toBe(children[1]!.name);
    expect(children[0]!.id).not.toBe(children[1]!.id);
  }
});

test("state initializes after its seed constant and before a derived constant", async () => {
  const entry = await source(
    "Counter.tsx",
    `import { useState } from 'preact/hooks';
const seed = 7;
export function Counter() {
  const [count, setCount] = useState(seed);
  const doubled = count * 2;
  return <button onClick={() => setCount(count + 1)}>{doubled}</button>;
}`,
  );
  const counter = (await compile(entry)).components[0]!;
  expect(counter.initializers).toEqual([
    { kind: "constant", name: "seed" },
    { kind: "state", name: "count" },
    { kind: "constant", name: "doubled" },
  ]);
  expect(counter.states[0]!.initial).toEqual({ kind: "name", name: "seed" });
});

test("undefined remains distinct from null and overflowing numeric literals fail", async () => {
  const entry = await source(
    "Page.tsx",
    `const absent = undefined; const empty = null;
export function Page() { return <p>{absent === empty}</p>; }`,
  );
  const program = await compile(entry);
  expect(
    program.components[0]!.constants.map((constant) => constant.value),
  ).toEqual([{ kind: "undefined" }, { kind: "literal", value: null }]);
  await expect(
    compile(
      await source(
        "Overflow.tsx",
        `const value = 1e400;
export function Overflow() { return <p>{value}</p>; }`,
      ),
    ),
  ).rejects.toThrow("debe ser finito");
});

test("keyed component identity is rejected instead of degrading to index identity", async () => {
  const entry = await source(
    "Page.tsx",
    `function Counter() { return <span>0</span>; }
export function Page({ items }) { return <main>{items.map(item => <Counter key={item.id} />)}</main>; }`,
  );
  await expect(compile(entry)).rejects.toThrow("key requiere identidad");
});

test("loose equality requires a native adapter instead of an unsupported runtime operator", async () => {
  for (const operator of ["==", "!="]) {
    const entry = await source(
      "Page.tsx",
      `export function Page({ count }) { return <p>{count ${operator} '0'}</p>; }`,
    );
    await expect(compile(entry)).rejects.toThrow(
      `Operador no soportado: ${operator}`,
    );
  }
});

test("package adapters are optional and unavailable external components fail explicitly", async () => {
  const entry = await source(
    "Page.tsx",
    `import { Button as Action, Card } from '@example/design';
export function Page({ onSave }) { return <Card><Action onClick={() => onSave()}>Guardar</Action></Card>; }`,
  );
  await expect(compile(entry)).rejects.toThrow(
    "Componente externo sin adaptador",
  );
  const program = await compile(entry, undefined, {
    adapters: { "@example/design": { Button: "Button", Card: "Card" } },
  });
  const card = program.components[0]!.body[0]!;
  expect(card.kind).toBe("component");
  if (card.kind === "component") {
    expect(card.name).toBe("$ui.Card");
    expect(card.children[0]!.kind).toBe("element");
    if (card.children[0]!.kind === "element") {
      expect(card.children[0]!.attrs["data-native-component"]).toEqual({
        kind: "literal",
        value: "Button",
      });
      expect(card.children[0]!.events.onClick!.steps[0]!.name).toBe("onSave");
    }
  }
});

test("source hashes and IDs are deterministic, and source changes alter the hash", async () => {
  const entry = await source(
    "Page.tsx",
    `export function Page() { return <main><p>Hola</p></main>; }`,
  );
  const first = await compile(entry);
  expect(await compile(entry)).toEqual(first);
  await writeFile(
    entry,
    (await readFile(entry, "utf8")).replace("Hola", "Adiós"),
  );
  const second = await compile(entry);
  expect(second.entry).toBe(first.entry);
  expect(second.sources[0]!.hash).not.toBe(first.sources[0]!.hash);
});

test("SSR, unknown hooks, DOM, network and unknown callbacks are never skipped", async () => {
  const cases: [string, string, string][] = [
    [
      "Page.astro",
      `---\nconst data = await fetch('/api/private');\n---\n<p>{data}</p>`,
      "no convertible",
    ],
    [
      "Page.tsx",
      `import { useEffect } from 'preact/hooks';\nexport function Page() { useEffect(() => {}, []); return <p>Hola</p>; }`,
      "otros efectos",
    ],
    [
      "Page.tsx",
      `export function Page() { return <p>{document.title}</p>; }`,
      "Binding desconocido: document",
    ],
    [
      "Page.tsx",
      `export function Page() { const data = fetch('/api'); return <p>{data}</p>; }`,
      "no convertible",
    ],
    [
      "Page.tsx",
      `export function Page() { return <button onClick={() => save()}>Guardar</button>; }`,
      "no declarada en props",
    ],
    [
      "Page.tsx",
      `export function Page() { return <div dangerouslySetInnerHTML={{__html: '<p>x</p>'}} />; }`,
      "depende del DOM",
    ],
    [
      "Page.astro",
      `---\nconst title = Astro.locals.title;\n---\n<p>{title}</p>`,
      "depende de SSR",
    ],
  ];
  for (const [path, text, message] of cases) {
    await expect(compile(await source(path, text))).rejects.toThrow(message);
  }
});

test("spread, duplicate IDs, complex setters and recursive components fail with locations", async () => {
  const cases = [
    `export function Page({ extra }) { return <div {...extra} />; }`,
    `export function Page() { return <main><p id="same" /><p id="same" /></main>; }`,
    `import { useState } from 'preact/hooks'; export function Page() {
      const [value, setValue] = useState(0);
      return <button onClick={() => setValue(current => { return current + 1; })} />;
    }`,
    `export function Page() { return <Page />; }`,
  ];
  for (const text of cases) {
    try {
      await compile(await source("Page.tsx", text));
      throw new Error("La conversión aceptó una construcción no soportada.");
    } catch (error) {
      expect(error).toBeInstanceOf(ConversionError);
      expect((error as Error).message).toMatch(/^Page\.tsx:\d+:\d+: /);
    }
  }
});

test("styles and attributes reach the emitter instead of being discarded", async () => {
  const entry = await source(
    "Page.tsx",
    `export function Page({ label }) {
    return <div id="box" className="grid gap-4" style={{ padding: 10.5 }} aria-label={label}>
      <span>Hola </span>{label}
    </div>;
  }`,
  );
  const program = await compile(entry);
  const box = elements(program.components[0]!.body)[0]!;
  expect(box.id).toBe("box");
  expect(box.attrs.className).toEqual({ kind: "literal", value: "grid gap-4" });
  expect(box.attrs.style).toEqual({
    kind: "object",
    entries: { padding: { kind: "literal", value: 10.5 } },
  });
  expect(box.attrs["aria-label"]).toEqual({ kind: "name", name: "label" });
  expect(elements(box.children)[0]!.children[0]!).toEqual({
    kind: "text",
    value: { kind: "literal", value: "Hola " },
  });
});

test("JSX decodes semicolon-terminated HTML references without decoding JavaScript strings", async () => {
  const entry = await source(
    "Page.tsx",
    `export function Page() {
    return <p id="entity&amp;node" aria-label="&quot;A&quot; &#x1F642; &copy;">A &amp; B &#169; &#x1F642; &NotEqualTilde; &#128; &amp withoutsemicolon {"&amp; &#169;"}</p>;
  }`,
  );
  const compiled = await compile(entry);
  const paragraph = elements(compiled.components[0]!.body)[0]!;
  expect(paragraph.id).toBe("entity&node");
  expect(paragraph.attrs["aria-label"]).toEqual({
    kind: "literal",
    value: '"A" 🙂 ©',
  });
  expect(paragraph.children).toEqual([
    {
      kind: "text",
      value: {
        kind: "literal",
        value: "A & B © 🙂 &NotEqualTilde; \u0080 &amp withoutsemicolon ",
      },
    },
    { kind: "text", value: { kind: "literal", value: "&amp; &#169;" } },
  ]);
});

test("Astro applies HTML text and attribute rules but preserves expression strings", async () => {
  const entry = await source(
    "Page.astro",
    `<p id="entity&amp;node" aria-label="&quot;A&quot; &copy &#x1F642;">A &amp; B &copy withoutsemicolon &#169; &#x1F642; &NotEqualTilde; &#128; {"&amp; &#169;"}</p>`,
  );
  const compiled = await compile(entry);
  const paragraph = elements(compiled.components[0]!.body)[0]!;
  expect(paragraph.id).toBe("entity&node");
  expect(paragraph.attrs["aria-label"]).toEqual({
    kind: "literal",
    value: '"A" © 🙂',
  });
  expect(paragraph.children).toEqual([
    {
      kind: "text",
      value: { kind: "literal", value: "A & B © withoutsemicolon © 🙂 ≂̸ € " },
    },
    { kind: "text", value: { kind: "literal", value: "&amp; &#169;" } },
  ]);
});

test("Astro attributes preserve ambiguous semicolonless references unlike text", async () => {
  const entry = await source(
    "Page.astro",
    `<p aria-label="&copycat &amp=tag &copy" />`,
  );
  const compiled = await compile(entry);
  expect(
    elements(compiled.components[0]!.body)[0]!.attrs["aria-label"],
  ).toEqual({ kind: "literal", value: "&copycat &amp=tag ©" });
});

test("JSX expression attributes keep literal references exactly as JavaScript supplied them", async () => {
  const entry = await source(
    "Page.tsx",
    `export function Page() { return <p aria-label={"&amp;"}>&amp;amp; &unknown;</p>; }`,
  );
  const compiled = await compile(entry);
  const paragraph = elements(compiled.components[0]!.body)[0]!;
  expect(paragraph.attrs["aria-label"]).toEqual({
    kind: "literal",
    value: "&amp;",
  });
  expect(paragraph.children).toEqual([
    { kind: "text", value: { kind: "literal", value: "&amp; &unknown;" } },
  ]);
});

test("shadowed conversion builtins fail instead of using the global builtin", async () => {
  for (const name of ["String", "Number", "Boolean"]) {
    for (const input of [
      `export function Page({${name}}) { return <p>{${name}(1)}</p>; }`,
      `export function Page() { const ${name} = 1; return <p>{${name}(1)}</p>; }`,
      `export function Page() { const ${name} = () => 'local'; return <p>{${name}(1)}</p>; }`,
      `const title = ${name}(1); const ${name} = 'later'; export function Page() { return <p>{title}</p>; }`,
      `export function Page() { const title = ${name}(1); const ${name} = 'later'; return <p>{title}</p>; }`,
      `function ${name}(value) { return <p>{value}</p>; } export function Page() { return <p>{${name}(1)}</p>; }`,
      `import {${name}} from './host'; export function Page() { return <p>{${name}(1)}</p>; }`,
      `export function Page({items}) { return <main>{items.map(${name} => <p>{${name}(1)}</p>)}</main>; }`,
    ]) {
      const entry = await source("Page.tsx", input);
      await expect(compile(entry)).rejects.toThrow(`Builtin ${name} sombreado`);
    }
  }
});

test("only declared callback props authorize host action calls", async () => {
  for (const input of [
    `export function Page() { const save = 'not a callback'; return <button onClick={() => save()} />; }`,
    `export function Page() { const save = 'not a callback'; return <button onClick={save} />; }`,
    `import {useState} from 'preact/hooks'; export function Page() { const [save,setSave] = useState(0); return <button onClick={() => save()} />; }`,
    `export function Page({save,items}) { return <main>{items.map(save => <button onClick={() => save()} />)}</main>; }`,
    `export function Page({save}) { return <input onInput={save => save()} />; }`,
    `import {useState} from 'preact/hooks'; export function Page({items}) { const [count,setCount]=useState(0); return <main>{items.map(setCount=><button onClick={()=>setCount(1)} />)}</main>; }`,
    `export function Page({items,save}) { const handle=()=>save(); return <main>{items.map(handle=><button onClick={handle} />)}</main>; }`,
  ]) {
    await expect(compile(await source("Page.tsx", input))).rejects.toThrow(
      "no declarad",
    );
  }
});

test("an event parameter cannot silently turn into a state setter", async () => {
  const entry = await source(
    "Page.tsx",
    `import {useState} from 'preact/hooks'; export function Page() { const [count,setCount]=useState(0); return <input onInput={setCount=>setCount(1)} />; }`,
  );
  await expect(compile(entry)).rejects.toThrow("sombrea un setter");
});

test("module calls to translation require a native initialization lifecycle", async () => {
  for (const input of [
    `const title = t('module'); export function Page() { return <p>{title}</p>; }`,
    `const data = {title:t('module')}; export function Page() { return <p>{data.title}</p>; }`,
    `const title = String(t('module')); export function Page() { return <p>{title}</p>; }`,
    `const title = false && t('module'); export function Page() { return <p>{title}</p>; }`,
  ]) {
    await expect(compile(await source("Page.tsx", input))).rejects.toThrow(
      "constante de módulo con llamadas al host",
    );
  }
});

test("module constants cannot read a component parameter as an ambient module binding", async () => {
  const entry = await source(
    "Page.tsx",
    `const title = label; export function Page({label}) { return <p>{title}</p>; }`,
  );
  await expect(compile(entry)).rejects.toThrow("Binding desconocido: label");
});

test("component translation cannot be shadowed by constants, hooks or updater parameters", async () => {
  for (const input of [
    `export function Page() { const t = 'local'; return <p>{t('key')}</p>; }`,
    `export function Page() { const t = () => 'local'; return <p>{t('key')}</p>; }`,
    `export function Page() { const title = t('key'); const t = 'later'; return <p>{title}</p>; }`,
    `import {useState} from 'preact/hooks'; export function Page() { const [count,setCount]=useState(0); return <button onClick={()=>setCount(t=>t('key'))} />; }`,
    `export function Page({t,items}) { return <main>{items.map(t=><p>{t('key')}</p>)}</main>; }`,
  ]) {
    await expect(compile(await source("Page.tsx", input))).rejects.toThrow(
      "t sombreado",
    );
  }
});

test("global pure builtins, declared callbacks and component-local translated constants remain supported", async () => {
  const entry = await source(
    "Page.tsx",
    `const suffix=String(Number('2'));
export function Page({save,t}) { const title=t('title'); const visible=Boolean(1); return <main><p>{title}{suffix}{String(visible)}</p><button onClick={()=>save(title)} /><button onClick={save} /><button onClick={()=>t('clicked')} /></main>; }`,
  );
  const compiled = await compile(entry);
  expect(compiled.actions).toEqual(["save", "t"]);
  const buttons = elements(compiled.components[0]!.body).filter(
    (node) => node.tag === "button",
  );
  expect(buttons[0]!.events.onClick!.steps[0]!.name).toBe("save");
  expect(buttons[1]!.events.onClick!.steps[0]!.name).toBe("save");
  expect(buttons[2]!.events.onClick!.steps[0]!.name).toBe("t");
});

test("named handlers reject list shadows until lexical captures are qualified", async () => {
  for (const input of [
    `import {useState} from 'preact/hooks'; export function Page() { const [count,setCount]=useState(0); const items=[10]; const add=()=>setCount(count+1); return <main>{items.map(count=><button onClick={add} />)}</main>; }`,
    `export function Page({save}) { const count=0; const items=[10]; const add=()=>save(count+1); return <main>{items.map((item,count)=><button onClick={add} />)}</main>; }`,
    `export function Page({save,count}) { const items=[10]; const add=()=>save(count+1); return <main>{items.map(count=><section>{items.map(item=><button onClick={add} />)}</section>)}</main>; }`,
    `const count=0; export function Page({save}) { const items=[10]; const add=()=>save(count+1); return <main>{items.map(count=><button onClick={add} />)}</main>; }`,
  ]) {
    await expect(compile(await source("Page.tsx", input))).rejects.toThrow(
      "handler nombrado add se usa bajo map con bindings externos sombreados (count)",
    );
  }
});

test("named handlers remain supported outside shadows and inline handlers preserve list bindings", async () => {
  const entry = await source(
    "Page.tsx",
    `import {useState} from 'preact/hooks'; export function Page() { const [count,setCount]=useState(0); const items=[10]; const add=()=>setCount(count+1); return <main><button onClick={add} />{items.map(item=><button onClick={add} />)}{items.map(count=><button onClick={()=>setCount(count+1)} />)}</main>; }`,
  );
  const compiled = await compile(entry);
  const buttons = elements(compiled.components[0]!.body).filter(
    (node) => node.tag === "button",
  );
  expect(buttons).toHaveLength(3);
  for (const button of buttons)
    expect(button.events.onClick!.steps[0]).toEqual({
      kind: "set",
      name: "count",
      args: [
        {
          kind: "binary",
          op: "+",
          left: { kind: "name", name: "count" },
          right: { kind: "literal", value: 1 },
        },
      ],
    });
});

test("lexical undefined bindings never become an absent child or ambient undefined", async () => {
  for (const input of [
    `export function Page({undefined}) { return <p>{undefined}</p>; }`,
    `export function Page() { const undefined='visible'; return <p>{String(undefined)}</p>; }`,
    `const undefined=7; export function Page() { return <p>{undefined}</p>; }`,
    `import {undefined} from './host'; export function Page() { return <p>{undefined}</p>; }`,
    `export function Page(undefined) { return <p>{undefined.value}</p>; }`,
    `export function Page({items}) { return <main>{items.map(undefined=><p>{undefined}</p>)}</main>; }`,
    `import {useState} from 'preact/hooks'; export function Page() { const [value,setValue]=useState(''); return <input onInput={undefined=>setValue(undefined.currentTarget.value)} />; }`,
    `import {useState} from 'preact/hooks'; export function Page() { const [count,setCount]=useState(0); return <button onClick={()=>setCount(undefined=>undefined+1)} />; }`,
    `export function Page() { const value=String(undefined); const undefined=7; return <p>{value}</p>; }`,
  ]) {
    await expect(compile(await source("Page.tsx", input))).rejects.toThrow(
      "undefined sombreado por un binding léxico",
    );
  }
  const compiled = await compile(
    await source(
      "Page.tsx",
      `export function Page() { return <main>{undefined}<p>{String(undefined)}</p></main>; }`,
    ),
  );
  const main = elements(compiled.components[0]!.body)[0]!;
  expect(main.children).toHaveLength(1);
  expect(elements(main.children)[0]!.children).toEqual([
    {
      kind: "text",
      value: { kind: "call", name: "String", args: [{ kind: "undefined" }] },
    },
  ]);
});

test("ordinary component nested children require an explicit native contract", async () => {
  for (const body of [
    `<Box><button>Visible</button></Box>`,
    `<Box children="provided"><p>Visible</p></Box>`,
  ]) {
    const entry = await source(
      "Page.tsx",
      `function Box({children}) { return <section>{children}</section>; } export function Page() { return ${body}; }`,
    );
    await expect(compile(entry)).rejects.toThrow(
      "Hijos de componentes requieren un contrato nativo explícito",
    );
  }
  await source(
    "Box.tsx",
    `export function Box({children}) { return <section>{children}</section>; }`,
  );
  const astro = await source(
    "Page.astro",
    `---\nimport {Box} from './Box.tsx';\n---\n<Box><p>Visible</p></Box>`,
  );
  await expect(compile(astro)).rejects.toThrow(
    "Hijos de componentes requieren un contrato nativo explícito",
  );
  const compiled = await compile(
    await source(
      "Page.tsx",
      `function Box({children}) { return <section>{children}</section>; } export function Page() { return <main><Box /><Box children="provided" /></main>; }`,
    ),
  );
  const page = compiled.components.find(
    (component) => component.name === compiled.entry,
  )!;
  const components = elements(page.body)[0]!.children;
  expect(components).toHaveLength(2);
  expect(components[1]).toMatchObject({
    kind: "component",
    props: { children: { kind: "literal", value: "provided" } },
    children: [],
  });
});

test("async list callbacks fail rather than render promised children synchronously", async () => {
  for (const callback of [
    `async item=><li>{item}</li>`,
    `async (item,index)=>{ return <li>{item}</li>; }`,
  ]) {
    const entry = await source(
      "Page.tsx",
      `export function Page({items}) { return <ul>{items.map(${callback})}</ul>; }`,
    );
    await expect(compile(entry)).rejects.toThrow("map async devuelve promesas");
  }
  const compiled = await compile(
    await source(
      "Page.tsx",
      `export function Page({items}) { return <ul>{items.map(item=><li>{item}</li>)}</ul>; }`,
    ),
  );
  expect(elements(compiled.components[0]!.body)[0]!.children[0]!.kind).toBe(
    "each",
  );
});
