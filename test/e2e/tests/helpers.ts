// Helpers compartidos por los specs e2e de Sytue One.
import { createHmac } from "node:crypto";
import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Secreto TOTP del usuario admin de dev. DEBE coincidir con seedTOTPSecretB32 en
 * services/id/cmd/id/seed.go (SOLO DEV). Con él calculamos el segundo factor sin
 * depender de una app autenticadora ni de librerías externas.
 */
export const TOTP_SECRET_DEV = "JBSWY3DPEHPK3PXP";

/** Decodifica base32 (RFC 4648, alfabeto A-Z2-7, sin padding) a bytes. */
function base32Decode(s: string): Buffer {
  const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  const limpio = s.toUpperCase().replace(/=+$/, "").replace(/\s+/g, "");
  let bits = 0;
  let valor = 0;
  const out: number[] = [];
  for (const ch of limpio) {
    const idx = A.indexOf(ch);
    // El mensaje NOMBRA lo que se esperaba, porque este es el punto donde muere
    // la auditoría entera y el carácter suelto no basta para saber qué hacer.
    // El caso real (2026-08-16): en `SYTUE_AUDIT_TOTP` se guardó un código de
    // seis dígitos en vez de la semilla. `otplib` no se quejaba —descarta lo que
    // no reconoce— y generaba un código VÁLIDO EN FORMA pero incorrecto, así que
    // el único síntoma era que Sytue ID lo rechazaba y el login se quedaba en
    // `/login/mfa`. Un fallo mudo cuesta una investigación; este cuesta una línea.
    if (idx < 0)
      throw new Error(
        `base32 inválido: «${ch}». El secreto TOTP es la SEMILLA (A-Z y 2-7, sin ` +
          `0/1/8/9), la que se enseña una vez junto al QR — no un código de 6 dígitos.`,
      );
    valor = (valor << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      out.push((valor >>> bits) & 0xff);
      valor &= (1 << bits) - 1; // descarta los bits ya emitidos (evita overflow int32)
    }
  }
  return Buffer.from(out);
}

/**
 * Código TOTP (RFC 6238: HOTP/HMAC-SHA1, 6 dígitos, paso 30 s) de un secreto
 * base32. Mismo algoritmo que services/id/internal/totp (que valida con ±1 paso
 * de holgura). Sin dependencias: solo node:crypto.
 */
export function codigoTOTP(
  secretBase32: string = TOTP_SECRET_DEV,
  at: number = Date.now(),
): string {
  const counter = Math.floor(at / 1000 / 30);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const hmac = createHmac("sha1", base32Decode(secretBase32))
    .update(buf)
    .digest();
  const offset = hmac[hmac.length - 1] & 0x0f;
  const bin =
    ((hmac[offset] & 0x7f) << 24) |
    (hmac[offset + 1] << 16) |
    (hmac[offset + 2] << 8) |
    hmac[offset + 3];
  return String(bin % 1_000_000).padStart(6, "0");
}

/**
 * Si el IdP muestra el segundo factor (step-up MFA: input[name=code]), lo
 * completa y envía. Es no-op si no hay reto MFA (usuario sin MFA o login ya
 * resuelto): espera acotada al campo y sale.
 *
 * La semilla es la de `SYTUE_AUDIT_TOTP` si está definida —la cuenta de
 * auditoría de `~/.config/sytue/audit.env`, contra producción— y si no, la del
 * admin de dev. Antes era SIEMPRE la de dev: las sondas contra producción que
 * pasaban por aquí tecleaban un código que no podía valer.
 *
 * El formulario MFA lo renderiza el IdP (Go, server-side) casi al instante tras
 * el POST de credenciales, así que una espera corta basta para distinguir «hay
 * MFA» de «no hay»; el coste en el caso sin-MFA es esa espera, una sola vez.
 */
export async function completarMFASiHaceFalta(
  page: Page,
  semilla: string = process.env.SYTUE_AUDIT_TOTP || TOTP_SECRET_DEV,
): Promise<void> {
  const campo = page.locator("input[name=code]");
  const hayMFA = await campo.waitFor({ state: "visible", timeout: 6_000 }).then(
    () => true,
    () => false,
  );
  if (!hayMFA) return;
  await campo.fill(codigoTOTP(semilla));
  await page.click("button[type=submit]");
}

/**
 * Deja la sesión en la organización `slug` (por omisión, `SYTUE_AUDIT_ORG`), y es
 * no-op sin ella o si ya es la activa. Se llama justo DESPUÉS de entrar en una app.
 *
 * POR QUÉ HACE FALTA. Al entrar, Sytue ID abre la sesión en la PRIMERA organización
 * de la cuenta (`orgTrasLogin`), y el host o el path de la URL no la eligen (ADR
 * 0056: la org del path no autoriza). La cuenta de auditoría es de `corporate` desde
 * mucho antes que la de pruebas, así que sin esto las sondas miden `corporate`
 * aunque la cuenta administre otra. Es el mismo cambio que hace el menú de cuenta
 * de la shell: `PUT /auth/organizations` y rehacer el SSO de esta app, porque su
 * token lleva el `tenant` anterior firmado dentro. Las demás apps ya autorizan con
 * la organización nueva: el IdP la cambia en TODAS las sesiones de la cuenta.
 */
export async function activarOrganizacion(
  page: Page,
  slug: string = process.env.SYTUE_AUDIT_ORG ?? "",
): Promise<void> {
  if (!slug) return;
  // `fetch` relativo: el `basePathScript` de la app le pone su prefijo.
  const r = await page.evaluate(async (slug: string) => {
    const l = await fetch("/auth/organizations");
    if (!l.ok) return `GET ${l.status}`;
    const { organizations = [] } = (await l.json()) as {
      organizations?: { id: string; slug: string; active: boolean }[];
    };
    const org = organizations.find((o) => o.slug === slug);
    if (!org) return "no-miembro";
    if (org.active) return "ya";
    const p = await fetch("/auth/organizations", {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ org_id: org.id }),
    });
    return p.ok ? "cambiada" : `PUT ${p.status}`;
  }, slug);
  if (r === "ya") return;
  if (r === "no-miembro")
    throw new Error(`la cuenta no es miembro activo de «${slug}» (SYTUE_AUDIT_ORG)`);
  // 402/403 aquí suele ser una organización sin plazas o suspendida.
  if (r !== "cambiada") throw new Error(`no se pudo activar «${slug}»: ${r}`);
  const aqui = new URL(page.url());
  const base = await page.evaluate(
    () => (window as { __sytueBase?: string }).__sytueBase ?? "",
  );
  await page.goto(
    `${aqui.origin}${base.replace(/\/+$/, "")}/auth/login?returnTo=${encodeURIComponent(aqui.pathname + aqui.search)}`,
  );
  await page.waitForURL((u) => u.origin === aqui.origin && !u.pathname.includes("/auth/"), {
    timeout: 60_000,
  });
}

/**
 * Por qué el login se quedó donde se quedó, cuando la URL basta para saberlo.
 *
 * Devuelve el diagnóstico o `null` si esa URL no dice nada concluyente. Es
 * DEDUCCIÓN DE LO MEDIDO, no una conjetura: el IdP solo manda a
 * `/account?mfa=required` desde un sitio (`desviarAStepUp`, cuando la política
 * de plataforma exige segundo factor y la cuenta no lo tiene enrolado), así que
 * ver esa URL es ver la causa, no inferirla.
 *
 * POR QUÉ ESTÁ AQUÍ Y NO EN CADA SONDA. Ese caso exacto costó TRES rondas de
 * diagnóstico y dos rotaciones de una credencial que estaba bien: sin sesión,
 * las pantallas salen vacías y se cuentan como fallos de producto, así que el
 * barrido entero miente sin decir por qué. Cada sonda tenía su propio mensaje de
 * «no salí del IdP» y ninguna miraba esto; con la comprobación en un solo sitio,
 * arreglarlo una vez lo arregla en todas.
 */
export function porQueNoEntra(href: string): string | null {
  let u: URL;
  try {
    u = new URL(href);
  } catch {
    return null;
  }
  if (
    u.pathname.replace(/\/+$/, "") === "/account" &&
    u.searchParams.get("mfa") === "required"
  ) {
    return (
      "la cuenta NO tiene segundo factor enrolado y la política de plataforma lo exige.\n" +
      "  No es la semilla, ni el reloj, ni el navegador: el IdP abrió la sesión con\n" +
      '  amr:["pwd"], y al entrar a una app la verja de plataforma pide un factor que\n' +
      "  la cuenta no tiene, así que devuelve al alta. Se arregla dándolo de alta y\n" +
      "  CONFIRMÁNDOLO (sin el paso de confirmación no queda activo):\n" +
      "      test/e2e/rotar-totp-auditoria.sh"
    );
  }
  return null;
}

/**
 * La portada de Admin envía al employee a su cuenta del IdP. Ese destino
 * concreto no es un formulario de login ni una pantalla administrativa medida.
 * Un alta de MFA obligatoria sigue siendo un fallo de acceso.
 */
export function esCuentaDelIdPDesdeAdmin(
  destino: string,
  actual: string,
  dominio: string,
): boolean {
  if (!destino) return false;
  const d = new URL(destino);
  const a = new URL(actual);
  const sinBarra = (p: string): string => p.replace(/\/+$/, "");
  return (
    d.hostname === `admin.${dominio}` && sinBarra(d.pathname) === "/admin" &&
    a.hostname === `id.${dominio}` && sinBarra(a.pathname) === "/account" &&
    porQueNoEntra(actual) === null
  );
}

/** Las 9 apps de la suite (clave → URL canónica). */
export const APPS = {
  one: "https://one.sytue.localhost/",
  mail: "https://mail.sytue.localhost/",
  drive: "https://drive.sytue.localhost/",
  docs: "https://docus.sytue.localhost/docus/",
  calendar: "https://calendar.sytue.localhost/",
  contacts: "https://contacts.sytue.localhost/",
  rooms: "https://rooms.sytue.localhost/",
  notes: "https://notes.sytue.localhost/",
  photos: "https://photos.sytue.localhost/",
  cloud: "https://cloud.sytue.localhost/",
  console: "https://admin.sytue.localhost/",
} as const;

export type AppKey = keyof typeof APPS;

/**
 * Navega a una app y espera a que la red repose. El networkidle es «suave»:
 * si el dev-server sigue chispeando (HMR de Vite), no rompe el test.
 */
export async function gotoApp(page: Page, app: AppKey): Promise<void> {
  await page.goto(APPS[app], { waitUntil: "domcontentloaded" });
  await page
    .waitForLoadState("networkidle", { timeout: 15_000 })
    .catch(() => {});
}

/** ¿Estamos en el formulario de login del IdP? (predicado para waitForURL). */
export const enLoginDelIdP = (url: URL): boolean =>
  url.hostname === "id.sytue.localhost" && url.pathname.startsWith("/login");

/** Abre el portal sin sesión y espera el formulario del IdP listo para rellenar. */
export async function irAlFormularioDelIdP(page: Page): Promise<void> {
  await page.goto(APPS.one, { waitUntil: "domcontentloaded" });
  // Esperar a estar en el IdP antes de rellenar: si algún salto de la cadena
  // fuera client-side, fill() competiría con una navegación en curso.
  await page.waitForURL(enLoginDelIdP, { waitUntil: "domcontentloaded" });
}

/**
 * Rellena y envía el formulario de login del IdP, resistente al lockout suave.
 *
 * El IdP limita /login/submit POR CUENTA (token bucket de 5, recarga 1 cada
 * 12 s — services/id/internal/http/server.go) y consume cupo también en los
 * envíos CORRECTOS; además hay un límite por IP (10/min). Como todas las
 * ejecuciones e2e concurrentes comparten la cuenta admin y la IP, un login
 * válido puede recibir la página 401 «Email o contraseña incorrectos». En ese
 * caso se espera una recarga del bucket y se reintenta entrando de nuevo por
 * el portal (el enlace «Reintentar» del IdP pierde el return_to del flujo
 * OIDC, así que no sirve para reanudarlo).
 *
 * El llamante debe estar ya en el formulario (ver irAlFormularioDelIdP).
 */
export async function loginEnIdP(
  page: Page,
  email: string,
  pass: string,
): Promise<void> {
  const INTENTOS = 4;
  // Periodo de recarga del limitador por cuenta (60 s / 5 tokens) + margen.
  // No es un sleep arbitrario: por debajo de 12 s el reintento nace muerto.
  const RECARGA_MS = 13_000;

  for (let intento = 1; intento <= INTENTOS; intento++) {
    if (intento > 1) {
      await page.waitForTimeout(RECARGA_MS);
      await page.goto(APPS.one, { waitUntil: "domcontentloaded" });
      // Si un intento anterior llegó a crear la sesión SSO (p. ej. callback
      // lento que terminó en segundo plano), el authorize entra directo.
      if (new URL(page.url()).hostname === "one.sytue.localhost") return;
      await page.waitForURL(enLoginDelIdP, { waitUntil: "domcontentloaded" });
    }
    await page.fill("input[name=email]", email);
    await page.fill("input[name=password]", pass);
    await page.click("button[type=submit]");
    // Segundo factor: si la cuenta tiene MFA (p. ej. el admin de dev, que es
    // admin y exige step-up), el IdP intercala el formulario TOTP antes de
    // volver a la app. No-op para cuentas sin MFA.
    await completarMFASiHaceFalta(page);
    // Éxito: la cadena OIDC abandona el IdP. Rechazo: página 401 del propio
    // IdP (lockout o credenciales). Ambas esperas se cancelan con .catch para
    // no dejar promesas rechazadas sueltas cuando gana la otra rama.
    const resultado = await Promise.race([
      page
        .waitForURL((url) => url.hostname !== "id.sytue.localhost", {
          timeout: 90_000,
          waitUntil: "domcontentloaded",
        })
        .then(() => "ok" as const)
        .catch(() => "agotado" as const),
      page
        .getByRole("link", { name: "Reintentar" })
        .waitFor({ timeout: 90_000 })
        .then(() => "rechazo" as const)
        .catch(() => "agotado" as const),
    ]);
    if (resultado === "ok") return;
  }
  throw new Error(
    `Login en el IdP rechazado tras ${INTENTOS} intentos (¿lockout por cuenta persistente?)`,
  );
}

/** Ruido conocido del dev-server de Vite/Astro que no cuenta como error real. */
const VITE_DEV_NOISE = [
  /@fs\//, // módulos servidos vía /@fs/…
  /\.vite\/deps/, // deps pre-empaquetadas en caliente
  /ERR_ABORTED/, // peticiones abortadas por HMR
];

/**
 * Empieza a coleccionar errores de consola de la página (llámala ANTES de
 * navegar) y devuelve la aserción a ejecutar al final del test.
 *
 *   const sinErrores = expectNoConsoleErrors(page);
 *   await gotoApp(page, 'mail');
 *   sinErrores();
 */
export function expectNoConsoleErrors(page: Page): () => void {
  const errores: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const texto = msg.text();
    if (VITE_DEV_NOISE.some((re) => re.test(texto))) return;
    errores.push(texto.slice(0, 300));
  });
  return () => expect(errores, "errores de consola inesperados").toEqual([]);
}

/**
 * Sidebar de la app (el <nav> de marca de @sytue/ui). Su nombre accesible sale del
 * catálogo compartido, así que cambia con el idioma: la suite sirve en inglés por
 * defecto y estos specs se escribieron cuando la interfaz solo hablaba español.
 * Vive aquí y no copiado en cada spec porque cuando esa etiqueta se tradujo rompió
 * cinco de golpe.
 */
export function sidebarNav(page: Page): Locator {
  return page.getByRole("navigation", {
    name: /Navegaci[oó]n principal|Main navigation/,
  });
}
