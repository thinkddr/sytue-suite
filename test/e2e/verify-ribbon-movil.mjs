// La cinta (ribbon) de Sytue Docs desplegada en un teléfono: comprueba que lo que
// envuelve queda CUADRADO, que es lo que no estaba.
//
// El descuadre eran tres cosas, medidas en producción a 320 y 360 px:
//   1. los controles de un grupo que no caben envolvían CENTRADOS, así que la
//      segunda fila no alineaba con la primera;
//   2. el rótulo del grupo iba centrado bajo grupos de anchos muy dispares (72 px
//      «Portapapeles», 441 px «Fuente»), y salía hasta 163 px a la derecha del
//      primer icono al que nombra;
//   3. los separadores verticales caían al principio o al final de línea, donde no
//      separan dos grupos sino un grupo del borde.
//
// Aquí se mide (2) —el desfase entre el rótulo y su grupo— y se exige (3). Con el
// grupo alineado al principio, el desfase es el padding del grupo: 6 px.
//
// Uso: SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… SYTUE_AUDIT_TOTP=… bun run verify-ribbon-movil.mjs
//      SYTUE_BASE_DOMAIN=sytue.localhost para medir la suite local.
// Capturas en /tmp/ribbon/.
//
// ponytail: script de verificación, no forma parte de la suite de specs.
import { chromium, devices } from "playwright";
import { codigoTOTP } from "./tests/helpers.ts";

const DOM = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const ES_LOCAL = DOM.endsWith(".localhost");
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || (ES_LOCAL ? "admin@sytue.localhost" : ""),
  pass: process.env.SYTUE_AUDIT_PASS || (ES_LOCAL ? "sytue-dev-admin" : ""),
  totp: process.env.SYTUE_AUDIT_TOTP || undefined,
};
if (!CRED.email || !CRED.pass) {
  console.error("Faltan credenciales: exporta SYTUE_AUDIT_EMAIL y SYTUE_AUDIT_PASS.");
  process.exit(2);
}

/** Desfase tolerado entre el rótulo de un grupo y el borde del grupo: su padding. */
const PADDING_GRUPO = 8;
const PERFILES = ["Galaxy S9+", "Moto G4"]; // 320 y 360 px: el suelo y el ancho más común
// Los TRES tipos: cada uno trae sus propias pestañas —la hoja tiene «Datos» y su
// «Inicio» es otro componente (InicioSheets), la presentación otro (InicioSlides)—,
// así que medir solo el documento deja dos cintas enteras sin mirar.
const TIPOS = ["Documento", "Hoja de cálculo", "Presentación"];

/**
 * Login en el IdP. El retorno tras el segundo factor NO completa solo
 * (`/oauth2/authorize` acaba en ERR_ABORTED; reproducido también en Chrome a mano):
 * se vuelve a pedir el destino y ahí la sesión ya vale.
 */
async function entrar(page, destino) {
  await page.goto(destino, { waitUntil: "domcontentloaded", timeout: 45_000 });
  if (!/^id\./.test(new URL(page.url()).hostname)) return;
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  const totp = page.locator("input[name=code]");
  if (await totp.waitFor({ timeout: 20_000 }).then(() => true, () => false)) {
    await totp.fill(codigoTOTP(CRED.totp));
    await page.click("button[type=submit]");
    await page.waitForTimeout(3_000);
  }
  await page.goto(destino, { waitUntil: "domcontentloaded", timeout: 45_000 });
}

/** Geometría de la cinta desplegada. */
const medir = (page) =>
  page.evaluate(() => {
    const panel = document.getElementById("ribbon-tabpanel");
    if (!panel) return null;
    const pr = panel.getBoundingClientRect();
    const grupos = [...panel.querySelectorAll('[role="group"]')].map((g) => {
      const b = g.getBoundingClientRect();
      const rot = g.lastElementChild.getBoundingClientRect();
      return { rotulo: g.getAttribute("aria-label"), desfase: Math.round(rot.left - b.left) };
    });
    return {
      lineas: new Set(
        [...panel.querySelectorAll('[role="group"]')].map((g) =>
          Math.round(g.getBoundingClientRect().top),
        ),
      ).size,
      alto: Math.round(pr.height),
      pct: Math.round((pr.height / innerHeight) * 100),
      peor: grupos.sort((a, b) => b.desfase - a.desfase)[0],
      sepsVisibles: [...panel.children].filter(
        (c) => c.getAttribute("aria-hidden") === "true" && c.getBoundingClientRect().width > 0,
      ).length,
    };
  });

let fallos = 0;
for (const perfil of PERFILES) {
  const browser = await chromium.launch();
  // locale: la interfaz se mide en el idioma en que la ve el usuario (y los rótulos
  // de esta comprobación se buscan por su texto en castellano).
  const ctx = await browser.newContext({
    ...devices[perfil],
    locale: "es-ES",
    ignoreHTTPSErrors: true,
  });
  const page = await ctx.newPage();
  const inicio = `https://docus.${DOM}/docus/`;
  await entrar(page, inicio);

  const slug = perfil.replace(/\W+/g, "-").toLowerCase();
  const ancho = page.viewportSize().width;
  for (const tipo of TIPOS) {
    await page.goto(inicio, { waitUntil: "domcontentloaded" });
    // La lista la trae un fetch: sin esperarla, cualquier búsqueda da 0 y todo
    // parecería «no hay documentos de ese tipo».
    await page.locator('a[href*="edit?id="]').first().waitFor({ timeout: 30_000 });
    const enlace = page.locator('a[href*="edit?id="]', { hasText: tipo }).first();
    if (!(await enlace.count())) {
      console.log(`AUSENTE   ${perfil} (${ancho}px) sin ${tipo} en la lista: crea uno para medirlo.`);
      fallos++;
      continue;
    }
    // El enlace de la lista apuntaba a `<host>/edit?id=…`, sin el prefijo `/docus` de
    // la app, y daba el 404 de Traefik. Se corrige aquí para poder medir la cinta
    // también contra una versión desplegada que aún lo tenga.
    const href = new URL(await enlace.getAttribute("href"), page.url());
    if (!href.pathname.startsWith("/docus/")) href.pathname = `/docus${href.pathname}`;
    await page.goto(href.href, { waitUntil: "domcontentloaded" });
    const desplegar = page.locator('[aria-controls="ribbon-tabpanel"][aria-expanded]');
    await desplegar.waitFor({ timeout: 30_000 }).catch(() => {
      throw new Error(`el editor no llegó a montarse en ${page.url()} — ¿404 al abrir el documento?`);
    });
    await desplegar.click();
    await page.waitForTimeout(600);

    // Las pestañas se LEEN del DOM: cada tipo trae las suyas (§6.2).
    const tabs = await page.locator('[role="tab"]').evaluateAll((els) =>
      els.map((e) => e.id.replace("ribbon-tab-", "")),
    );
    for (const tab of tabs) {
      await page.locator(`#ribbon-tab-${tab}`).click();
      await page.waitForTimeout(300);
      const nombre = `${slug}-${tipo.split(" ")[0].toLowerCase()}-${tab}`;
      await page.screenshot({ path: `/tmp/ribbon/${nombre}.png` });
      const m = await medir(page);
      const mal = m.peor.desfase > PADDING_GRUPO || m.sepsVisibles > 0;
      if (mal) fallos++;
      console.log(
        `${mal ? "DESCUADRE" : "ok       "} ${perfil} (${ancho}px) ${tipo.padEnd(15)} ${tab.padEnd(13)} ` +
          `${m.lineas} líneas · ${m.alto}px (${m.pct}% de la pantalla) · ` +
          `rótulo peor colocado: «${m.peor.rotulo}» a ${m.peor.desfase}px de su grupo · ` +
          `${m.sepsVisibles} separadores`,
      );
    }
  }
  await browser.close();
}
console.log(fallos === 0 ? "\nLa cinta cuadra en todas las pestañas." : `\n${fallos} pestañas descuadradas.`);
process.exit(fallos === 0 ? 0 : 1);
