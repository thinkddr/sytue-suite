// SPDX-License-Identifier: Apache-2.0
// LA BARRA ÚNICA DEL TELÉFONO en las diecinueve superficies, con sus marcos reales y el
// CSS que genera Tailwind, sin servidor (banco: packages/shell/test/barra-unica-banco.tsx).
//
// Por superficie, ancho (320, 390, 767), tema (claro, oscuro) y forma (Marco, Carril),
// en tres estados —en reposo, con el cajón abierto y con la suite desplegada— afirma:
//   · una sola barra arriba (ni la de 44 de antes ni otra cabecera de app encima);
//   · nada desborda (`scrollWidth <= clientWidth`);
//   · NINGUNA FRANJA CLARA: los píxeles entre el borde de la barra y el contenido, los
//     de la zona segura de iOS (simulada a 47 px) y el fondo que asoma al estirar la
//     página son del cromo o de la hoja, nunca una banda clara de otra cosa. Es el
//     precedente de AppShell.tsx: la franja blanca de 44 px entre la topbar y el cajón;
//   · el selector de apps de la suite pinta su marca (no un cuadrado vacío);
//   · el nombre de marca de la barra cabe entero.
// Y guarda las PNG de 390 en BARRA_UNICA_OUT para el mosaico.
//
//   S=… ; tailwindcss -i apps/frontend/packages/shell/test/barra-unica-banco.css -o $S/b.css
//   BARRA_UNICA_JS=$S/b.js bun run test/e2e/barra-unica.mjs --compilar
//   BARRA_UNICA_CSS=$S/b.css BARRA_UNICA_JS=$S/b.js BARRA_UNICA_OUT=$S/out node test/e2e/barra-unica.mjs
//
// VISTO EN ROJO: con una banda blanca de 14 px sobre la hoja (oscuro: «banda clara entre
// la barra y el contenido»), y sin la regla del lienzo de «Carril» en appearance.css
// («el fondo de la página asoma claro al estirarla»).
//
// APPS, ANCHOS, TEMAS y FORMAS acotan la pasada (listas separadas por comas).
// Playwright sale de test/e2e/node_modules en el CI y, en local, del global
// (NODE_PATH=/opt/node22/lib/node_modules): no es una dependencia nueva.
import { createRequire } from "node:module";
import { mkdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

if (process.argv.includes("--compilar")) {
  const { compilarBanco } = await import("./lib/compilar-banco.mjs");
  await compilarBanco(
    fileURLToPath(
      new URL(
        "../../apps/frontend/packages/shell/test/barra-unica-banco.tsx",
        import.meta.url,
      ),
    ),
    process.env.BARRA_UNICA_JS || "/tmp/barra-unica.js",
  );
  process.exit(0);
}

const req = createRequire(import.meta.url);
let chromium;
try {
  ({ chromium } = req("playwright"));
} catch {
  ({ chromium } = createRequire("/opt/node22/lib/node_modules/")("playwright"));
}
const css = readFileSync(
  process.env.BARRA_UNICA_CSS || "/tmp/barra-unica.css",
  "utf8",
);
const js = readFileSync(
  process.env.BARRA_UNICA_JS || "/tmp/barra-unica.js",
  "utf8",
);
const out = process.env.BARRA_UNICA_OUT || "/tmp/barra-unica";
mkdirSync(out, { recursive: true });
const lista = (v, d) => (process.env[v] || d).split(",").filter(Boolean);
const APPS = lista(
  "APPS",
  "calendar,mail,contacts,rooms,drive,console,cloud,notes,bases,pages,cast,photos,account,portal,docs,sheets,slides,pdf,erp",
);
const ANCHOS = lista("ANCHOS", "320,390,767").map(Number);
const TEMAS = lista("TEMAS", "light,dark");
const FORMAS = lista("FORMAS", "marco,carril");
const SEGURA = 47; // la zona segura superior de un iPhone con isla

let fallos = 0;
function exigir(condicion, mensaje) {
  if (condicion) return;
  fallos++;
  console.error(`✗ ${mensaje}`);
}

const correos = [1, 2, 3].map((i) => ({
  id: `e${i}`,
  threadId: `t${i}`,
  subject: `Asunto ${i}`,
  from: [{ name: `Persona ${i}`, email: `p${i}@x.eu` }],
  to: [],
  cc: [],
  preview: "Vista previa",
  receivedAt: "2026-11-06T10:00:00Z",
  keywords: { $seen: true },
  hasAttachment: false,
  threadCount: 1,
  mailboxId: "in",
}));
// Un JWT de juguete, sin firma: la app solo lee de él la organización. Se arma por
// partes para que en el fuente no quede un literal con cara de secreto: gitleaks lo
// levantaba aquí y en la exportación a sytue-suite (run 42387388102).
const tokenDeJuguete = [
  "x",
  btoa(JSON.stringify({ tenant: "acme" })).replace(/=+$/, ""),
  "x",
].join(".");

const API = {
  "/auth/token": {
    accessToken: tokenDeJuguete,
    expiresIn: 3600,
  },
  "/api/calendars": {
    calendars: [
      {
        id: "c1",
        url: "/c/",
        displayName: "Personal",
        color: "#3d8fd6",
        isDefault: true,
      },
    ],
  },
  "/api/events": { events: [] },
  "/api/invitations": { invitations: [] },
  "/api/reminders": { reminders: [], next_cursor: "" },
  "/api/shared": { calendars: [] },
  "/api/mailboxes": {
    mailboxes: [
      {
        id: "in",
        name: "Inbox",
        role: "inbox",
        totalEmails: 3,
        unreadEmails: 0,
      },
    ],
  },
  "/api/emails": { emails: correos, nextCursor: "" },
  "/api/labels": { labels: [] },
  "/api/aliases": { aliases: [], primary: "ana@sytue.test" },
  "/api/shared-mailboxes": { sharedMailboxes: [] },
  "/api/signature": { signature: "" },
  "/api/filters": { filters: [] },
  "/api/external-mail/summary": { items: [] },
  "/api/quota": {},
};

const browser = await chromium.launch();

async function abrir({ app, w, tema, forma, l = "es" }) {
  const movil = w < 768;
  const ctx = await browser.newContext({
    viewport: { width: w, height: 700 },
    deviceScaleFactor: 2,
    hasTouch: movil,
    isMobile: movil,
    locale: l,
    timezoneId: "Europe/Madrid",
  });
  const page = await ctx.newPage();
  await page.clock.setFixedTime(new Date("2026-11-06T19:36:00+01:00"));
  await page.addInitScript(
    ([t, f]) => {
      try {
        localStorage.setItem("sytue-theme", t);
        localStorage.setItem("sytue-forma", f);
      } catch {
        // sin almacenamiento, lo de por defecto
      }
    },
    [tema, forma],
  );
  const errores = [];
  page.on("pageerror", (e) => errores.push(e.message));
  const base = "http://barra-unica.sytue.test/";
  await page.route("**/*", (route) => {
    const u = new URL(route.request().url());
    if (u.origin + u.pathname === base) {
      return route.fulfill({
        contentType: "text/html",
        body: `<!doctype html><html lang="${l}" class="h-full" data-theme="${tema}" ${forma === "carril" ? 'data-forma="carril"' : ""} data-color="app" data-app="${app}"><head><meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"></head><body class="bg-bg text-text h-full antialiased"></body></html>`,
      });
    }
    const p = u.pathname;
    const json = (o) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(o),
      });
    if (p in API) return json(API[p]);
    if (p.includes("unread_count")) return json({ unread: 0 });
    if (p.endsWith("/external-mail/summary")) return json({ items: [] });
    return json({});
  });
  await page.goto(`${base}?l=${l}&app=${app}`);
  await page.addStyleTag({ content: css });
  await page.addScriptTag({ content: js, type: "module" });
  await page.waitForSelector('header[role="banner"]', { timeout: 15000 });
  await page.waitForTimeout(900);
  return { page, ctx, errores };
}

/**
 * Las franjas, sobre los PÍXELES de la captura. Bajo la barra, en tres columnas (los dos
 * bordes y el centro), la franja de 16 px que la toca tiene que ser del color de la
 * barra o del contenido que sigue debajo: una banda CLARA que no es ni lo uno ni lo
 * otro —la barra de 44 blanca entre dos cromos, el fondo de la página entre la barra y
 * la hoja— es una franja. Arriba, la zona segura es la barra. Y el lienzo de la página,
 * que es lo que asoma al estirarla (sobrescroll, tirar para refrescar), no es claro.
 */
async function franjas(page, estado) {
  const png = (await page.screenshot()).toString("base64");
  return page.evaluate(
    async ({ png, estado }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${png}`;
      await img.decode();
      const lienzo = document.createElement("canvas");
      lienzo.width = img.width;
      lienzo.height = img.height;
      const c2 = lienzo.getContext("2d");
      c2.drawImage(img, 0, 0);
      const k = img.width / innerWidth;
      const px = (x, y) => [
        ...c2.getImageData(Math.round(x * k), Math.round(y * k), 1, 1).data,
      ];
      const dist = (a, b) =>
        Math.max(...[0, 1, 2].map((i) => Math.abs(a[i] - b[i])));
      const luz = (a) => (0.2126 * a[0] + 0.7152 * a[1] + 0.0722 * a[2]) / 255;
      const header = document.querySelector('header[role="banner"]');
      const abajo = header.getBoundingClientRect().bottom;
      // Los fondos propios de lo que hay en ese punto DENTRO del contenido (la hoja, su
      // panel): el margen de la hoja alrededor de una tarjeta es la hoja, no una franja.
      const fondoHoja = (x, y) => {
        const fondos = [];
        for (let e = document.elementFromPoint(x, y); e; e = e.parentElement) {
          if (e === document.body || e === document.documentElement) break;
          if (!e.closest('main, .sytue-sheet, [role="dialog"], aside, nav'))
            continue;
          const m = getComputedStyle(e)
            .backgroundColor.match(/[\d.]+/g)
            ?.map(Number);
          if (m && (m[3] ?? 1) > 0.5) fondos.push(m);
        }
        // Dentro del contenido (en «Carril» no hay hoja y el contenido va sobre el
        // fondo de la página), el fondo de la página es el del contenido.
        const e0 = document.elementFromPoint(x, y);
        if (e0?.closest("main, .sytue-sheet")) {
          const m = getComputedStyle(document.body)
            .backgroundColor.match(/[\d.]+/g)
            ?.map(Number);
          if (m) fondos.push(m);
        }
        return fondos;
      };
      const malas = [];
      for (const x of [2, Math.round(innerWidth / 2), innerWidth - 3]) {
        const barra = px(x, abajo - 3);
        const debajo = [28, 36, 44].map((dy) => px(x, abajo + dy));
        for (const dy of [1, 5, 10, 15]) {
          const p = px(x, abajo + dy);
          if (luz(p) < 0.72) continue; // una franja OSCURA no es lo que se busca aquí
          if (dist(p, barra) <= 14) continue; // es la barra (o su degradado)
          if (debajo.some((d) => dist(p, d) <= 14)) continue; // es lo que sigue: el contenido
          if (fondoHoja(x, abajo + dy).some((d) => dist(p, d) <= 14)) continue; // la hoja
          malas.push(
            `${estado}: banda clara en (${x},${Math.round(abajo + dy)}) rgb(${p.slice(0, 3)}) entre la barra rgb(${barra.slice(0, 3)}) y el contenido rgb(${debajo[1].slice(0, 3)})`,
          );
          break;
        }
      }
      const seg = parseFloat(getComputedStyle(header).paddingTop);
      if (seg > 0) {
        const e = document.elementFromPoint(Math.round(innerWidth / 2), 2);
        if (!e?.closest('header[role="banner"]'))
          malas.push(`${estado}: la zona segura no es la barra`);
      }
      if (innerWidth < 768) {
        const cs = getComputedStyle(document.documentElement);
        const m = cs.backgroundColor.match(/[\d.]+/g)?.map(Number) ?? [
          0, 0, 0, 0,
        ];
        const lienzoClaro =
          !cs.backgroundImage.includes("gradient") &&
          (m[3] ?? 1) > 0.5 &&
          luz(m) > 0.72;
        const cuerpo = getComputedStyle(document.body);
        const mb = cuerpo.backgroundColor.match(/[\d.]+/g)?.map(Number) ?? [
          0, 0, 0, 0,
        ];
        const sinLienzo =
          cs.backgroundColor === "rgba(0, 0, 0, 0)" &&
          !cs.backgroundImage.includes("gradient");
        const cuerpoClaro =
          sinLienzo &&
          !cuerpo.backgroundImage.includes("gradient") &&
          luz(mb) > 0.72;
        if (lienzoClaro || cuerpoClaro)
          malas.push(
            `${estado}: el fondo de la página asoma claro al estirarla`,
          );
      }
      return malas;
    },
    { png, estado },
  );
}

for (const forma of FORMAS) {
  for (const tema of TEMAS) {
    for (const app of APPS) {
      for (const w of ANCHOS) {
        const { page, ctx, errores } = await abrir({ app, w, tema, forma });
        const nombre = `${app}-${w}-${tema === "light" ? "claro" : "oscuro"}-${forma}`;
        // La zona segura de un iPhone: la barra la rellena por arriba.
        await page.addStyleTag({
          content: `:root{--sytue-safe-top:${SEGURA}px}`,
        });
        await page.waitForTimeout(100);
        const m = await page.evaluate(() => ({
          ancho: document.documentElement.scrollWidth,
          barras: [
            ...document.querySelectorAll(
              'header[role="banner"], .sytue-mobilebar',
            ),
          ].filter(
            (e) =>
              getComputedStyle(e).display !== "none" &&
              e.getBoundingClientRect().height > 0,
          ).length,
        }));
        exigir(m.ancho <= w, `${nombre}: desborda (${m.ancho})`);
        // El nombre de marca de la barra cabe ENTERO (a 320, «CertiPDF», «Contacts»,
        // «Notepads»…): sin «…».
        const titulo = await page.evaluate(() => {
          const el = document.querySelector(
            'header[role="banner"] [data-marca-app] + div > span',
          );
          if (!el || getComputedStyle(el).display === "none") return null;
          return {
            texto: el.textContent,
            cabe: el.scrollWidth <= el.clientWidth,
          };
        });
        exigir(
          w >= 768 || !titulo || titulo.cabe,
          `${nombre}: «${titulo?.texto}» no cabe en la barra`,
        );
        exigir(w >= 768 || m.barras === 1, `${nombre}: ${m.barras} barras`);
        for (const f of await franjas(page, `${nombre} reposo`))
          exigir(false, f);
        if (w === 390) {
          await page.screenshot({
            path: join(out, `${nombre}.png`),
            clip: { x: 0, y: 0, width: w, height: 240 },
          });
        }
        // El cajón.
        const menu = page
          .locator('header[role="banner"] button[aria-controls]')
          .first();
        if (w < 768 && (await menu.count()) && (await menu.isVisible())) {
          await menu.click();
          await page.waitForTimeout(350);
          for (const f of await franjas(page, `${nombre} cajón`))
            exigir(false, f);
          if (w === 390)
            await page.screenshot({
              path: join(out, `${nombre}-cajon.png`),
              clip: { x: 0, y: 0, width: w, height: 360 },
            });
          await menu.click().catch(() => {});
          await page.keyboard.press("Escape");
          await page.waitForTimeout(250);
        }
        // La suite desplegada.
        const entrada = page
          .locator('header[role="banner"] button[aria-label^="Sytue"]')
          .first();
        if (w < 768 && (await entrada.isVisible().catch(() => false))) {
          await entrada.click();
          await page.waitForTimeout(250);
          for (const f of await franjas(page, `${nombre} suite`))
            exigir(false, f);
          const s = await page.evaluate(() => {
            const el = document.querySelector(
              'header[role="banner"] .sytue-esquina-marca',
            );
            const svg = el?.querySelector("svg, img");
            const r = svg?.getBoundingClientRect();
            return {
              marca: Boolean(r && r.width > 10 && r.width < 200),
              texto: el?.innerText ?? "",
            };
          });
          exigir(
            s.marca && !s.texto.includes("data:"),
            `${nombre}: el selector de apps sin su marca`,
          );
          const ancho = await page.evaluate(
            () => document.documentElement.scrollWidth,
          );
          exigir(ancho <= w, `${nombre} suite: desborda (${ancho})`);
          if (w === 390)
            await page.screenshot({
              path: join(out, `${nombre}-suite.png`),
              clip: { x: 0, y: 0, width: w, height: 160 },
            });
        }
        // Los errores de datos de las apps sin todas sus respuestas simuladas no son de
        // la barra: se dicen, sin contar como fallo.
        if (errores.length && w === 390)
          console.log(`  (aviso) ${nombre}: ${errores[0]}`);
        await ctx.close();
      }
    }
  }
}
await browser.close();
if (fallos) {
  console.error(`barra-unica: ${fallos} comprobación(es) fallida(s)`);
  process.exit(1);
}
console.log(`barra-unica: todo en orden; capturas en ${out}`);
