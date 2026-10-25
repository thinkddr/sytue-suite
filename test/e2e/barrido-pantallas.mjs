// Barrido funcional pantalla por pantalla contra producción.
//
// QUÉ MIDE, Y POR QUÉ NO LO MEDÍA NADIE. `mobile-audit.mjs` solo mira si la
// página desborda a lo ancho: una pantalla que carga rota, con la consola llena
// de errores y media API devolviendo 500, la aprueba igual. Esto mira lo otro:
//
//   · errores de consola del navegador (excepciones, recursos que no cargan),
//   · peticiones de red fallidas (≥400) con su ruta,
//   · que la pantalla tenga contenido de verdad y no un esqueleto vacío.
//
// Uso: bun run barrido-pantallas.mjs [app...]
// Salida: informe.json en el directorio privado por ejecución indicado en consola.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { existsSync } from "node:fs";
import { activarOrganizacion, codigoTOTP, esCuentaDelIdPDesdeAdmin, porQueNoEntra } from "./tests/helpers.ts";
import { appsDesplegadas, desplegada } from "./apps-desplegadas.mjs";

const INFORME = `${createReportDirectory('barrido')}/informe.json`;

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const IDP = `id.${DOMINIO}`;
const STORAGE = fileURLToPath(new URL(".auth/prod-audit.json", import.meta.url));

const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_AUDIT_PASS || "",
  totp: process.env.SYTUE_AUDIT_TOTP || "",
};

// Rutas por app. SALEN DEL ÁRBOL DE PÁGINAS, no de la memoria: la primera
// versión de esta lista llevaba un `/projects` inventado en Cloud que devolvía
// 404 y se contaba como hallazgo del producto. Si aquí aparece una ruta que no
// existe, el barrido se inventa fallos; si falta una que sí, los esconde.
//
// EL `base` NO ES OPCIONAL, y por faltar aquí pasó justo lo que el párrafo de
// arriba advierte. Cada app se construye con su prefijo (`/certipdf`, `/one`…), así
// que `certipdf.sytue.eu/pdf` es la raíz del host —donde el ingress no enruta nada—
// y devuelve el 404 de Traefik. El barrido lo apuntaba como fallo de Sytue Docs
// cuando la página estaba perfectamente en `/certipdf/pdf`: un hallazgo inventado
// por la sonda, del tipo más caro de descartar porque parece real.
//
// Las rutas se siguen escribiendo SIN prefijo —son las del árbol de páginas— y
// `urlDe()` lo compone. Así esta lista se puede copiar de `src/pages/` tal cual.
const APPS = {
  portal: {
    host: "one",
    base: "/one",
    rutas: ["/", "/actividad", "/bienvenida", "/buscar", "/descargas", "/dispositivos", "/notificaciones"],
  },
  mail: { host: "mail", base: "/mail", rutas: ["/"] },
  drive: { host: "drive", base: "/drive", rutas: ["/"] },
  calendar: { host: "calendar", base: "/calendar", rutas: ["/", "/events/new"] },
  contacts: { host: "contacts", base: "/contacts", rutas: ["/"] },
  // La ofimática, una app por producto (ADR 0076).
  docs: { host: "docus", base: "/docus", rutas: ["/", "/trash"] },
  sheets: { host: "calcus", base: "/calcus", rutas: ["/", "/trash"] },
  slides: { host: "diapos", base: "/diapos", rutas: ["/", "/trash"] },
  pdf: { host: "certipdf", base: "/certipdf", rutas: ["/", "/pdf"] },
  rooms: { host: "rooms", base: "/rooms", rutas: ["/", "/share"] },
  // SYTUE ERP: una sola página (el módulo va en `?m=`), pero es la que carga el
  // catálogo entero y la organización; si el servicio no contesta, es aquí
  // donde se ve.
  erp: { host: "erp", base: "/erp", rutas: ["/"] },
  // SYTUE WEB: se salta sola mientras la instalación no la levante (`PAGES_ENABLED`
  // en el piloto; `OPCIONALES` de apps-desplegadas.mjs).
  pages: { host: "web", base: "/web", rutas: ["/", "/new", "/settings"] },
  // SYTUE NOTES: nació después del último barrido y entró en producción sin
  // pasar por aquí ni una vez.
  //
  // Las dos rutas con identificador van con uno que NO existe a propósito, misma
  // doctrina que las de proyecto de Cloud unas líneas más abajo: no se comprueba
  // lo que pintan —para eso está mobile-audit, que siembra un cuaderno de
  // verdad—, sino LO QUE IMPORTA aquí, que ante un cuaderno ausente respondan
  // (404, redirección, un vacío explicado) en vez de reventar en el servidor.
  // Esa es exactamente la clase de fallo que tenía `rooms/share`: una excepción
  // a mitad del render que sale como 200 y ningún monitor ve.
  //
  // `/p/` es el enlace público, la ÚNICA superficie de la suite que se sirve sin
  // sesión: si esa revienta, revienta para cualquiera que reciba el enlace.
  notes: {
    host: "notes",
    base: "/notes",
    rutas: ["/", "/notebooks/no-existe-auditoria", "/p/no-existe-auditoria"],
  },
  cloud: {
    host: "cloud",
    base: "/cloud",
    rutas: [
      "/",
      "/audit",
      "/billing",
      "/settings",
      "/team",
      "/templates",
      "/tokens",
      "/usage",
      // Las 14 de un PROYECTO, con un slug que NO existe a propósito.
      //
      // Con la cuenta de auditoría no hay ningún proyecto —crear uno devuelve
      // 403— así que estas rutas llevaban sin medirse desde siempre, y «no se
      // pueden medir» se estaba leyendo como «están bien». Con un slug
      // inventado no se comprueba lo que pintan, pero sí LO QUE IMPORTA aquí:
      // que ante un proyecto ausente respondan —404, redirección, un vacío
      // explicado— en vez de reventar en el servidor. Es exactamente el fallo
      // que tenía `rooms/share`: una excepción a mitad del render que sale como
      // 200 con el cuerpo «Internal server error», invisible para todo monitor.
      "/projects/no-existe-auditoria/",
      "/projects/no-existe-auditoria/analytics",
      "/projects/no-existe-auditoria/apps",
      "/projects/no-existe-auditoria/database",
      "/projects/no-existe-auditoria/deployments",
      "/projects/no-existe-auditoria/domains",
      "/projects/no-existe-auditoria/env",
      "/projects/no-existe-auditoria/functions",
      "/projects/no-existe-auditoria/logs",
      "/projects/no-existe-auditoria/monitoring",
      "/projects/no-existe-auditoria/settings",
      "/projects/no-existe-auditoria/storage",
      "/projects/no-existe-auditoria/terminal",
    ],
  },
  // La consola de plataforma es DONDE SE CONFIGURA la suite: 17 pantallas, y el
  // barrido entraba solo por la portada. Justo las de configuración —la clase de
  // pantalla que más depende de que una API conteste— quedaban sin mirar.
  // Salen del árbol de páginas (apps/frontend/apps/console/src/pages), no de la
  // memoria: una ruta inventada aquí se cuenta como fallo del producto.
  admin: {
    host: "admin",
    base: "/admin",
    rutas: [
      "/",
      "/access",
      "/account",
      "/apps",
      "/audit",
      "/compliance",
      "/devices",
      "/editor-de-documentos",
      "/governance",
      "/groups",
      "/migrations",
      // `/platform` NO: esa página se sirve solo en platform.<dominio> (la misma app
      // de Astro atiende los dos hosts) y bajo /admin responde 404 a propósito.
      "/privileged-access",
      "/roles",
      "/sensitivity-labels",
      "/sso",
      "/users",
    ],
  },
};

/**
 * La URL real de una ruta: el prefijo de la app va delante.
 *
 * `"/"` da el prefijo pelado (`/docus`, no `/docus/`) porque así es como responde
 * el Ingress sin redirección de por medio; una barra de más se come un salto.
 */
function urlDe(def, ruta) {
  const base = def.base || "";
  return `${def.origin || `https://${def.host}.${DOMINIO}`}${ruta === "/" ? base || "/" : base + ruta}`;
}

/**
 * Reentra si la sesión caducó a mitad del barrido.
 *
 * CON ESPERA Y REINTENTO, por lo mismo que `mobile-audit.mjs`: el IdP limita el
 * login a 10/min (`loginLimiter`, PerSecond 10/60, Burst 10). Cuando la cookie
 * caduca a mitad, cada pantalla que sigue intenta entrar y el bucket las tumba
 * a todas — medido el 2026-08-14 barriendo Cloud: las 21 rutas salieron con
 * `waitForURL: Timeout` y ninguna medía la app, sino el acceso.
 *
 * Esto NO es cosmético: sin el reintento, un tope de accesos se lee como
 * «Cloud está roto», que es precisamente el falso positivo que este barrido
 * existe para no producir.
 */
async function asegurarSesion(page, destino) {
  if (new URL(page.url()).hostname !== IDP) return false;
  if (esCuentaDelIdPDesdeAdmin(destino, page.url(), DOMINIO)) return false;
  for (let intento = 1; ; intento++) {
    try {
      await page.fill("input[name=email]", CRED.email);
      await page.fill("input[name=password]", CRED.pass);
      await page.click("button[type=submit]");
      const totp = page.locator(
        'input[name=code], input[name=totp], input[autocomplete="one-time-code"]',
      );
      const pideSegundo = await totp
        .first()
        .waitFor({ timeout: 5_000 })
        .then(() => true, () => false);
      if (pideSegundo) {
        if (!CRED.totp) {
          throw new Error("la cuenta pide segundo factor y falta SYTUE_AUDIT_TOTP");
        }
        await totp.first().fill(codigoTOTP(CRED.totp));
        await page.click("button[type=submit]");
      }
      await page.waitForURL((u) => u.hostname !== IDP, { timeout: 60_000 });
      await activarOrganizacion(page);
      return true;
    } catch (e) {
      // Si la URL ya dice la causa, no se reintenta: esperar noventa segundos
      // para acabar en el mismo sitio solo retrasa el diagnóstico.
      const porque = porQueNoEntra(page.url());
      if (porque) throw new Error(`el login no llega a la app: ${porque}`);
      if (intento >= 3) throw e;
      const espera = intento * 30_000;
      console.log(
        `  · el IdP no completó el acceso (intento ${intento}/3); probablemente el ` +
          `tope de 10 accesos/min. Esperando ${espera / 1000} s.`,
      );
      await page.waitForTimeout(espera);
      await page.reload({ waitUntil: "domcontentloaded" }).catch(() => {});
      if (new URL(page.url()).hostname !== IDP) return true;
    }
  }
}

// Ruido conocido que NO es un hallazgo. Lista corta y explícita: filtrar por
// patrones vagos acaba escondiendo lo que se venía a buscar.
const RUIDO = [
  /favicon\.ico/,
  /Download the .* DevTools/,
  /\[vite\]/,
];
const esRuido = (t) => RUIDO.some((r) => r.test(t));

// La cuenta de auditoría es un employee deliberadamente sin esos permisos. Estas
// llamadas se hacen para decidir qué controles ocultar, no porque la pantalla necesite
// el dato para cargar. Un 403 suyo no es una caída: la respuesta sigue vigilada aquí,
// en vez de silenciar todos los 403 y convertir una regresión de autorización en verde.
const DENEGACIONES_ESPERADAS = new Set([
  "/drive/api/storage-external",
  "/erp/api/linea_extracto/estadisticas",
  "/erp/api/obligaciones",
]);

const resultados = [];
const apps = process.argv.slice(2).length
  ? process.argv.slice(2)
  : Object.keys(APPS);

const navegador = await chromium.launch();
const contexto = await navegador.newContext({
  storageState: existsSync(STORAGE) ? STORAGE : undefined,
  locale: "es-ES",
});
const page = await contexto.newPage();

// Qué productos levanta esta instalación (ver apps-desplegadas.mjs).
//
// SOLO SE PREGUNTA SI YA HAY SESIÓN GUARDADA. Sin ella, las dos llamadas
// contestan 401 y esto devolvía `null` —«medir todo»— antes de que nadie
// hubiera iniciado sesión: en un CI limpio, donde `.auth/prod-audit.json` no
// existe, era el caso NORMAL, no el raro. El resultado del 2026-09-11: las
// veinticinco rutas de Sytue DevX medidas en un piloto que no lo despliega, y
// el barrido en rojo por «31 rutas no se pudieron medir».
//
// El login ocurre dentro del bucle, la primera vez que una página redirige al
// IDP, así que a partir de ahí sí se puede preguntar: el reintento de abajo lo
// hace en cuanto hay con qué.
let desplegadas = existsSync(STORAGE) ? await appsDesplegadas(page, DOMINIO) : null;

for (const app of apps) {
  const def = APPS[app] ? { ...APPS[app] } : null;
  if (!def) {
    console.log(`  ? ${app}: no está en la lista`);
    continue;
  }
  // Se reintenta mientras no se sepa: la sesión aparece dentro del bucle, así
  // que la primera app se mide a ciegas y las siguientes ya no.
  if (desplegadas === null) desplegadas = await appsDesplegadas(page, DOMINIO);
  if (!desplegada(app, desplegadas)) {
    console.log(`  ·  ${app.padEnd(9)} no desplegada en esta instalación (según el Hub): no se mide`);
    continue;
  }
  for (const ruta of def.rutas) {
    const url = urlDe(def, ruta);
    const consola = [];
    const red = [];
    let sinPermiso = false;
    // Las rutas «no-existe-auditoria» son SONDAS: piden algo que no existe para
    // ver la pantalla de 404 del producto. Ese 404 —y la línea de consola con la
    // que el navegador lo anuncia— es la respuesta correcta, no un hallazgo.
    const sonda = ruta.includes("no-existe-auditoria");
    const rutaPedida = new URL(url).pathname;
    const onConsole = (m) => {
      if (m.type() !== "error" || esRuido(m.text())) return;
      if (sonda && /status of 404/.test(m.text())) return;
      // La respuesta conserva la URL y decide si el 403 es esperado. Chromium solo
      // escribe aquí el mensaje genérico, sin ella; duplicarlo no añade señal.
      if (/Failed to load resource: the server responded with a status of 403/.test(m.text())) return;
      consola.push(m.text());
    };
    const onPageError = (e) => consola.push(`uncaught: ${e.message}`);
    const onResponse = (r) => {
      if (r.status() < 400 || esRuido(r.url())) return;
      const camino = new URL(r.url()).pathname;
      if (sonda && r.status() === 404 && camino === rutaPedida) return;
      if (r.status() === 403 && app === "admin") {
        sinPermiso = true;
        return;
      }
      if (r.status() === 403 && DENEGACIONES_ESPERADAS.has(camino)) return;
      red.push(`${r.status()} ${r.request().method()} ${camino}`);
    };
    page.on("console", onConsole);
    page.on("pageerror", onPageError);
    page.on("response", onResponse);

    let error = null;
    let texto = 0;
    let titulo = "";
    let urlFinal = "";
    try {
      // Una sonda de ausencia verifica la respuesta HTTP. Un 404 de texto plano
      // puede abrir una descarga en Chromium: no es un fallo del servidor ni una UI.
      const ausencia = sonda ? await contexto.request.get(url) : null;
      if (ausencia?.status() === 404 && new URL(ausencia.url()).pathname === rutaPedida) {
        titulo = "404 esperado (sonda de ausencia)";
        texto = (await ausencia.text()).trim().length;
        urlFinal = rutaPedida;
      } else {
        await page.goto(url, { waitUntil: "domcontentloaded", timeout: 45_000 });
        if (await asegurarSesion(page, url)) {
          await page.goto(url, { waitUntil: "domcontentloaded", timeout: 45_000 });
        }
        // Un `networkidle` estricto no llega nunca en apps con SSE o websockets
        // abiertos —Rooms y el hub los tienen—, así que se espera un margen fijo.
        await page.waitForTimeout(2_500);
        titulo = (await page.title()) || "";
        texto = ((await page.locator("body").innerText()) || "").trim().length;
        // ¿Acabamos DONDE pedimos? Las pantallas de la consola exigen rol
        // admin/owner y la cuenta de auditoría es employee: sin esto, dieciséis
        // pantallas perfectamente sanas se contarían como fallo del producto —el
        // «hallazgo inventado» contra el que ya avisa la cabecera de este fichero—.
        // Se compara la RUTA, no la URL entera: los parámetros no cambian de pantalla.
        urlFinal = new URL(page.url()).pathname;
        // La portada descubre el namespace de la organización (ADR 0056). Seguir
        // pidiendo rutas sin él marcaba pantallas medidas como «sin permiso» y no
        // reconocía el 404 esperado de las sondas tras su redirección canónica.
        const final = new URL(page.url());
        if (ruta === "/" && app !== "admin" && final.pathname.startsWith(`${def.base}/`) &&
            final.pathname.slice(def.base.length + 1).replace(/\/$/, "").indexOf("/") === -1) {
          def.origin = final.origin;
          def.base = final.pathname.replace(/\/$/, "");
        }
      }
    } catch (e) {
      error = e.message.split("\n")[0];
    }

    page.off("console", onConsole);
    page.off("pageerror", onPageError);
    page.off("response", onResponse);

    // Redirigidos fuera de la ruta pedida = la sesión no tiene permiso para esa
    // pantalla. No es un fallo del producto ni un aprobado: es que no se ha medido.
    const pedida = new URL(url).pathname;
    const desviada = Boolean(urlFinal) && urlFinal !== pedida && !urlFinal.startsWith(pedida);
    const r = { app, ruta, url, titulo, texto, error, consola, red, desviada, sinPermiso, urlFinal };
    resultados.push(r);
    const sinAcceso = desviada || sinPermiso;
    const ausenciaEsperada = titulo === "404 esperado (sonda de ausencia)";
    const mal = !sinAcceso && (error || consola.length || red.length || (!ausenciaEsperada && texto < 40));
    console.log(
      `  ${sinAcceso ? "–" : mal ? "✗" : "ok"}  ${app.padEnd(9)} ${ruta.padEnd(14)} ` +
        (sinAcceso
          ? `sin permiso para esta sesión (acabó en ${urlFinal})`
          : `texto=${String(texto).padEnd(6)} consola=${consola.length} red=${red.length}` +
            (error ? ` ERROR: ${error}` : "")),
    );
    if (desviada) continue;
    for (const c of consola.slice(0, 3)) console.log(`         consola: ${c.slice(0, 150)}`);
    for (const n of red.slice(0, 5)) console.log(`         red:     ${n}`);
  }
}

await contexto.storageState({ path: STORAGE });
await navegador.close();
writeReport(INFORME, JSON.stringify(resultados, null, 2));

const conFallo = resultados.filter(
  (r) => !r.desviada && !r.sinPermiso && (r.error || r.consola.length || r.red.length || (r.titulo !== "404 esperado (sonda de ausencia)" && r.texto < 40)),
);
const sinPermiso = resultados.filter((r) => r.desviada || r.sinPermiso).length;
console.log(
  `\n${conFallo.length} pantallas con algo que mirar, de ${resultados.length - sinPermiso} medidas` +
    (sinPermiso ? ` (${sinPermiso} sin permiso para esta sesión, no medidas).` : "."),
);

// SALE EN ROJO SI HAY ALGO. Un barrido nocturno que siempre termina en verde no
// lo lee nadie a las dos semanas: la señal tiene que ser el color del job, no un
// número dentro de un log de 300 líneas. El umbral es cero a propósito — cuando
// aparezca ruido tolerable, va a la lista RUIDO con su motivo escrito, no a un
// «si son menos de N, pasa».
//
// Y si NO se visitó nada, también es rojo: un barrido de cero pantallas es un
// fallo de acceso disfrazado de éxito.
if (!resultados.length) {
  console.error("\n✗ no se visitó ninguna pantalla: revisa credenciales y red.");
  process.exit(1);
}
if (conFallo.length) process.exit(1);
