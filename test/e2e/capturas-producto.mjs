// Capturas de producto para las fichas del portal (`/product/<key>`).
//
// POR QUÉ EXISTE. Las quince fichas describen pantallas que nadie ve: quien
// compara suites mira la imagen antes que la lista de funciones. Y una captura
// hecha a mano envejece sin que nadie se entere —el rediseño de la semana que
// viene la deja mintiendo— así que se regenera con un comando.
//
// EN TODOS LOS IDIOMAS. Cada ficha del portal existe en los cinco que habla la
// suite, y hasta ahora las cinco enseñaban la misma imagen en español: la mitad
// de la página traducida y la pantalla que la ilustra, no. Se dispara una pasada
// por idioma y cada captura sale como `<clave>.<idioma>.webp`.
//
// El CONTENIDO de la cuenta (asuntos, notas, nombres de fichero) no se traduce:
// es lo que hay dentro del buzón de la demostración y sigue en español. Lo que
// cambia es la interfaz, que es lo que la ficha viene a enseñar.
//
// CON QUÉ CUENTA SE CORRE, Y ESTO NO ES NEGOCIABLE: una cuenta de DEMOSTRACIÓN,
// con datos preparados para que se vean. Nunca la de una persona: lo que salga
// en la imagen se publica en internet, y el nombre de la barra superior o el
// asunto de un correo de verdad se quedan ahí para siempre.
//
// Uso:
//   SYTUE_SHOT_EMAIL=… SYTUE_SHOT_PASS=… bun run capturas-producto.mjs [key…]
//
// Variables:
//   SYTUE_BASE_DOMAIN  dominio de la instalación (por defecto sytue.eu)
//   SYTUE_SHOT_TENANT  host de organización (por defecto corporate); vacío ⇒ un
//                      subdominio por app, que es como responde una instalación
//                      sin organización propia
//   SYTUE_SHOT_OUT     dónde dejar los .webp (por defecto la carpeta del portal)
//   SYTUE_SHOT_LOCALES idiomas, separados por comas (por defecto, los cinco con
//                      captura propia: es, en, de, fr, it)
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { createHash } from "node:crypto";
import { activarOrganizacion, completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_SHOT_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const SALIDA =
  process.env.SYTUE_SHOT_OUT ||
  fileURLToPath(
    new URL("../../apps/frontend/apps/www/src/capturas/", import.meta.url),
  );

// `SYTUE_SHOT_*` manda, y `SYTUE_AUDIT_*` sirve de respaldo: son las que ya tiene
// cargadas quien audita producción (`auditar-prod.sh`, `~/.config/sytue/audit.env`),
// así que pedir un par nuevo para lo mismo obligaba a duplicar la credencial en un
// sitio más. Las específicas siguen primero para poder disparar con OTRA cuenta
// sin tocar el entorno.
const CRED = {
  email: process.env.SYTUE_SHOT_EMAIL || process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_SHOT_PASS || process.env.SYTUE_AUDIT_PASS || "",
  // La semilla del TOTP, si la cuenta lo tiene. Se pasa a `completarMFASiHaceFalta`:
  // por sí solo, él solo mira `SYTUE_AUDIT_TOTP`, no `SYTUE_SHOT_TOTP`.
  totp: process.env.SYTUE_SHOT_TOTP || process.env.SYTUE_AUDIT_TOTP || "",
};
if (!CRED.email || !CRED.pass) {
  console.error(
    "Faltan credenciales. SYTUE_SHOT_EMAIL/PASS o SYTUE_AUDIT_EMAIL/PASS.\n\n" +
      "Y DE UNA CUENTA CUYOS DATOS SE PUEDAN PUBLICAR: lo que salga en la imagen\n" +
      "acaba en el sitio público. Si la cuenta tiene correo o archivos de verdad,\n" +
      "revisa las capturas ANTES de commitearlas.",
  );
  process.exit(2);
}

// El tamaño de la ficha: `Captura.astro` reserva 1440×900 (16/10). Capturar en
// otra proporción no rompe nada —la imagen se encaja— pero se recorta, y lo que
// se pierde es justo el borde de la pantalla.
const ANCHO = 1440;
const ALTO = 900;

/**
 * Dónde vive cada producto. Las rutas SALEN del árbol de páginas y de `appPaths`
 * (packages/config/src/tokens.ts), no de la memoria: una inventada aquí no da
 * error, da el 404 de Astro, y la captura sale de una pantalla de error con
 * aspecto de producto roto.
 *
 * `segunda` es la captura que acompaña a la lista larga de funciones. Solo la
 * tienen los productos con OTRA pantalla que enseñar; donde no la hay se queda
 * sin ella, que es mejor que repetir la misma imagen dos veces.
 */
const PRODUCTOS = {
  one: { ruta: "/one", segunda: "/one/actividad" },
  mail: { ruta: "/mail", abrirPrimero: true },
  drive: { ruta: "/drive" },
  // La portada de cada producto enseña la LISTA de ficheros, no el editor: una ficha que
  // presume de editar documentos no puede ilustrarse con una parrilla de
  // tarjetas cerradas. `pinchar` abre el fichero de demostración que siembra
  // `sembrar-demo.mjs`, así la captura sale del editor con contenido real
  // dentro, que es donde viven las funciones que la ficha describe.
  docs: { ruta: "/docus/", pinchar: "Propuesta de servicio" },
  sheets: { ruta: "/calcus/", pinchar: "Presupuesto 2027" },
  slides: {
    ruta: "/diapos/",
    pinchar: "Resultados del trimestre",
  },
  // Sin `path` en la URL, `/certipdf/` es la portada que ofrece abrir un
  // fichero: la lista de PDFs recientes que siembra `sembrarPdf`. `pinchar`
  // entra en uno para enseñar el visor/editor, no la portada.
  pdf: { ruta: "/certipdf/", pinchar: "Contrato marco" },
  calendar: {
    ruta: "/calendar",
    // `/calendar/events/new` A SECAS ES LA MISMA PANTALLA: esa ruta monta la
    // misma isla y solo abre el compositor si la query trae el intent
    // `action=create` (el que compone el Hub al crear un evento desde un correo,
    // CONVENTIONS §11). Sin él, las dos capturas de la ficha salían idénticas y
    // el guion lo decía en cada tanda. Con él se ve lo que de verdad hay que
    // enseñar: el evento prerellenado con sus invitados.
    segunda:
      "/calendar/events/new?action=create" +
      "&title=Revisi%C3%B3n%20del%20plan%20trimestral" +
      "&description=Cerramos%20objetivos%20y%20repartimos%20las%20acciones%20del%20trimestre." +
      "&start=2026-09-15T10:00:00" +
      "&attendees=carla.benet@ejemplo.test,hector.lamas@ejemplo.test",
  },
  contacts: { ruta: "/contacts", abrirPrimero: true },
  // `pinchar` entra en el cuaderno de demostración antes de abrir su primera
  // nota: Notepads arranca en el último cuaderno usado, que en una cuenta real
  // es el de probar cosas. El nombre es el que siembra `sembrar-demo.mjs`.
  notes: { ruta: "/notes", pinchar: ["Operaciones", "Guardias de octubre"] },
  photos: { ruta: "/photos", segunda: "/photos/albums" },
  // Al canal de demostración, que es el que tiene una conversación de trabajo;
  // `/rooms` a secas abre el último visitado, que en una cuenta real puede ser
  // cualquiera. El nombre lo siembra `sembrar-demo.mjs`.
  rooms: { ruta: "/rooms", pinchar: "equipo" },
  // Sytue Cast: la portada con sesión (mis vídeos) y la portada pública.
  cast: { ruta: "/cast", segunda: "/cast/explore" },
  // Sytue Web: «Mis sitios» y el asistente de «Crear sitio».
  pages: { ruta: "/web", segunda: "/web/new" },
  // Sytue DevX NO se captura todavía: no está desplegado en la instalación que
  // se fotografía, y una ficha no puede enseñar una pantalla que aquí es un 404.
  // Se queda en la lista —con el aviso de siempre— para que el día que se
  // despliegue la tanda la recoja sola, sin que nadie tenga que acordarse.
  cloud: { ruta: "/cloud", segunda: "/cloud/usage", pendiente: true },
  // La Consola es `/admin`, la que administra UNA organización, y NO `platform`.
  //
  // `platform.<dominio>` es la misma imagen con otro host y otro client, pero es
  // la consola de QUIEN OPERA la plataforma: gestiona las organizaciones cliente,
  // sus altas y sus fusiones. No es producto que nadie compre, así que no tiene
  // ficha en el portal y no se captura. Por eso no aparece en esta lista, y no es
  // un olvido.
  //
  // Y la de una organización tampoco se dispara por defecto: la cuenta con la que se
  // hace la tanda vive en el tenant corporativo, y `/admin/users` es su plantilla
  // REAL —nombres y correos de personas—, que no se publica. Queda pendiente hasta
  // que haya una organización de demostración que administrar.
  console: { ruta: "/admin", segunda: "/admin/users", pendiente: true },
  // Sytue ID se enseña por «Mi cuenta», que desde 2026-09-11 es app Astro propia
  // y vive bajo el host de organización como las demás. Apuntaba al `/account`
  // del binario Go —que sigue en pie y lo seguirá hasta que la app tenga
  // paridad—, así que la ficha habría enseñado la pantalla VIEJA: el HTML sin
  // bundler, sin la Sytue Shell y sin el selector de aplicaciones, que es justo
  // lo que la ficha presume de tener.
  id: { ruta: "/account" },
  erp: { ruta: "/erp" },
};

const urlDe = (def, ruta) =>
  def.url ?? `https://${TENANT ? `${TENANT}.${DOMINIO}` : DOMINIO}${ruta}`;

/**
 * Entra si la pantalla que hay delante es el LOGIN del IdP.
 *
 * El host solo no basta: «Mi cuenta» vive en el propio IdP (`id.<dominio>/account`),
 * así que estar en su host no significa que haya que iniciar sesión, y buscar ahí
 * el formulario de entrada hacía fallar la captura de Sytue ID.
 */
async function asegurarSesion(page) {
  const u = new URL(page.url());
  if (u.hostname !== IDP || !u.pathname.includes("/login")) return;
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  await completarMFASiHaceFalta(page, CRED.totp || undefined).catch(() => {});
  await page.waitForURL((url) => !url.pathname.includes("/login"), {
    timeout: 90_000,
  });
  await activarOrganizacion(page);
}

/**
 * Espera a que la pantalla esté QUIETA antes de disparar.
 *
 * Sin esto las capturas salen a medio pintar: la shell hidrata sus islas
 * después del `load`, y una barra superior sin avatar o una lista con el
 * esqueleto puesto es exactamente lo que no se quiere publicar. `networkidle`
 * no basta —hay apps con conexiones vivas, y ahí no llega nunca—, así que el
 * fallo de la espera no es un fallo de la captura.
 */
async function asentar(page) {
  await page
    .waitForLoadState("networkidle", { timeout: 12_000 })
    .catch(() => {});
  await page.evaluate(() => document.fonts?.ready).catch(() => {});
  await page.waitForTimeout(1_500);
}

/**
 * Abre el primer elemento de la lista principal, si lo hay.
 *
 * Una bandeja de entrada cerrada enseña la mitad del producto: lo que hay que
 * ver es el mensaje abierto al lado de la lista. Va con tolerancia a propósito
 * —una cuenta recién sembrada puede no tener nada— y sin datos se queda con la
 * pantalla tal cual en vez de fallar.
 */
async function abrirPrimero(page) {
  // `[role=option]` PRIMERO: es lo que emite el ListItem del design system dentro
  // de un listbox, que es la lista de Mail y la de Notepads. Sin él, `abrirPrimero`
  // no encontraba nada y las dos fichas salían con el panel de lectura vacío
  // —«Selecciona una conversación»—, que es justo la mitad que había que enseñar.
  for (const sel of [
    '[role="option"]',
    'li[role="button"]',
    '[role="listitem"]',
    "li a",
    "tbody tr",
  ]) {
    const primero = page.locator(sel).first();
    if (await primero.isVisible().catch(() => false)) {
      await primero.click({ timeout: 5_000 }).catch(() => {});
      await asentar(page);
      return;
    }
  }
}

/**
 * Pincha, en orden, en el primer elemento que lleve cada texto.
 *
 * Sirve para llegar a la parte del producto que hay que enseñar cuando la app
 * abre por otra: Notepads arranca en el último cuaderno usado —en una cuenta
 * real, el de probar cosas—, así que hay que entrar en el de demostración y
 * luego abrir una nota.
 *
 * Por TEXTO y no por selector a propósito: el texto es lo que siembra
 * `sembrar-demo.mjs` y no cambia con un rediseño, mientras que la clase o el rol
 * del elemento sí. Va con tolerancia: si no está, la captura sale igual con lo
 * que haya, que es mejor que quedarse sin ficha.
 */
async function pinchar(page, textos) {
  for (const texto of [textos].flat()) {
    const objetivo = page.getByText(texto, { exact: false }).first();
    if (!(await objetivo.isVisible().catch(() => false))) continue;
    await objetivo.click({ timeout: 5_000 }).catch(() => {});
    await asentar(page);
  }
}

/**
 * LO QUE NO SE PUBLICA.
 *
 * Estas imágenes acaban en el sitio público, así que antes de disparar se
 * desenfoca lo que identifica a alguien: direcciones de correo, direcciones IP y
 * el `@usuario`.
 *
 * NO se tocan los dominios reservados para documentación (RFC 2606 y 6761:
 * `.example`, `.test`, `.invalid`, `.localhost`, `example.com/org/net`). Son
 * justamente los datos de demostración que la ficha viene a enseñar: desenfocar
 * `carla.benet@ejemplo.test` deja la pantalla de Contactos borrosa sin proteger
 * a nadie.
 *
 * Se hace con CSS y ANTES del disparo, no recortando el PNG después: un recorte
 * por coordenadas se desalinea en cuanto cambia el diseño, y entonces el dato
 * queda al aire sin que nadie se entere. Un `filter: blur()` sigue al elemento.
 *
 * Los campos de formulario se miran aparte: el correo de «Mi cuenta» vive en el
 * `value` de un input, que no es un nodo de texto y no lo ve el recorrido.
 */
async function taparDatos(page) {
  await page.addStyleTag({
    content:
      ".sytue-tapado{filter:blur(7px)!important;-webkit-filter:blur(7px)!important}",
  });
  return page.evaluate(() => {
    const CORREO = /[\w.!#$%&'*+/=?^`{|}~-]+@([\w-]+(?:\.[\w-]+)+)/g;
    const IP = /\b\d{1,3}(?:\.\d{1,3}){3}\b/;
    const DOCUMENTACION =
      /(?:\.(?:example|test|invalid|localhost)|(?:^|\.)example\.(?:com|org|net))$/i;
    // Un nombre de fichero como `sytue-mark@2400-2.png` NO es una dirección: la
    // extensión hace de falso dominio y el borrón acababa tapando una fila de la
    // actividad reciente que no tenía nada que tapar.
    const EXTENSION =
      /\.(?:png|jpe?g|gif|webp|svg|zip|pdf|md|txt|odt|ods|odp|docx?|xlsx?|pptx?|csv|eml|p7s|json|ya?ml)$/i;

    // El `@usuario` (ADR 0040) identifica la cuenta igual que su correo: con él la
    // encuentra cualquiera de otra organización. Solo el valor ENTERO y con punto,
    // para no tapar una mención como `@all` dentro de un mensaje.
    const USUARIO = /^@[\w-]+(?:\.[\w-]+)+$/;

    const sensible = (texto) => {
      if (!texto) return false;
      if (IP.test(texto) || USUARIO.test(texto.trim())) return true;
      for (const m of texto.matchAll(CORREO)) {
        if (!DOCUMENTACION.test(m[1]) && !EXTENSION.test(m[1])) return true;
      }
      return false;
    };

    let tapados = 0;
    const tapar = (el) => {
      if (!el || el.classList.contains("sytue-tapado")) return;
      el.classList.add("sytue-tapado");
      tapados++;
    };

    const paseo = document.createTreeWalker(
      document.body,
      NodeFilter.SHOW_TEXT,
    );
    const nodos = [];
    for (let n = paseo.nextNode(); n; n = paseo.nextNode()) nodos.push(n);
    for (const n of nodos) {
      if (sensible(n.nodeValue)) tapar(n.parentElement);
    }
    for (const campo of document.querySelectorAll("input, textarea")) {
      if (sensible(campo.value) || sensible(campo.placeholder)) tapar(campo);
    }
    return tapados;
  });
}

/**
 * PNG → WebP.
 *
 * Sin ImageMagick se conserva el PNG —la ficha lo sirve igual— PERO SE DICE. Se
 * saltaba en silencio, así que una tanda entera podía acabar en PNG sin que nadie
 * se enterara hasta ver el peso de la página que más visitas recibe: una captura
 * de 1440×900 pasa de ~1,5 MB a ~120 KB al convertirla.
 *
 * No hay respaldo en JavaScript a propósito: `sharp` no es dependencia de este
 * paquete —lo resolvía la caché global de quien lo probara, no el proyecto— y
 * declararlo sería añadir una dependencia nativa a un guion que se lanza a mano
 * tres veces al año. Instalar ImageMagick es más barato que eso.
 */
function aWebp(png, destino) {
  try {
    execFileSync(
      "magick",
      [png, "-quality", "82", "-define", "webp:method=6", destino],
      {
        stdio: "ignore",
      },
    );
    rmSync(png, { force: true });
    return destino;
  } catch {
    sinMagick = true;
    return png;
  }
}

/** Se avisa UNA vez al final, no por captura: quince líneas iguales son ruido. */
let sinMagick = false;

/**
 * Etiqueta BCP-47 completa para el contexto del navegador.
 *
 * No basta con la cookie: el `locale` del contexto es lo que ve
 * `Intl`/`toLocaleDateString` dentro de las islas, y sin él las fechas de la
 * pantalla alemana salían en español. Un idioma sin entrada aquí se usa tal cual,
 * que también es BCP-47 válido.
 */
const BCP47 = {
  es: "es-ES",
  en: "en-GB",
  de: "de-DE",
  fr: "fr-FR",
  it: "it-IT",
};

// LOS CINCO DE `BCP47`, y no todos los de `packages/i18n/src/locale.ts`: la suite
// habla veintiséis, pero las fichas solo traen captura propia en estos cinco y el
// resto cae a la del idioma por defecto (`[key].astro`). Leer la lista entera
// convertía una tanda en veintiséis pasadas para publicar cinco.
const idiomas = process.env.SYTUE_SHOT_LOCALES
  ? process.env.SYTUE_SHOT_LOCALES.split(",")
      .map((s) => s.trim())
      .filter(Boolean)
  : Object.keys(BCP47);

// Sin argumentos se saltan los `pendiente`: dispararlos solo produce errores
// esperados que tapan los de verdad. Se piden por nombre para probarlos.
const pedidos = process.argv.slice(2).filter((a) => !a.startsWith("-"));
const claves = pedidos.length
  ? pedidos
  : Object.keys(PRODUCTOS).filter((k) => !PRODUCTOS[k].pendiente);
const desconocidas = claves.filter((k) => !PRODUCTOS[k]);
if (desconocidas.length) {
  console.error(`Producto desconocido: ${desconocidas.join(", ")}`);
  process.exit(2);
}

mkdirSync(SALIDA, { recursive: true });
const navegador = await chromium.launch();

/**
 * Huella → nombre, para cazar la pantalla que no cambió.
 *
 * Con varios idiomas sirve además de comprobación del idioma: si la pantalla
 * alemana sale con la misma huella que la española es que el cambio no se
 * aplicó, y eso se ve en el aviso en vez de acabar publicado.
 */
const huellas = new Map();
let hechas = 0;

// UNA PASADA POR IDIOMA. Contexto nuevo en cada una y no solo cookie distinta:
// el `locale` del contexto es lo que ven `Intl` y `toLocaleDateString` dentro de
// las islas, así que reutilizando el de la primera las fechas de todas las
// pantallas salían en español. Cuesta un inicio de sesión por idioma.
for (const idioma of idiomas) {
  const contexto = await navegador.newContext({
    ignoreHTTPSErrors: true,
    viewport: { width: ANCHO, height: ALTO },
    // El doble de píxeles y luego se reduce: una captura a 1× en una pantalla
    // moderna se ve pastosa justo en el texto, que es lo que se venía a enseñar.
    deviceScaleFactor: 2,
    colorScheme: "light",
    locale: BCP47[idioma] ?? idioma,
    extraHTTPHeaders: {
      "Accept-Language": `${BCP47[idioma] ?? idioma},${idioma};q=0.9`,
    },
  });
  // LA COOKIE, Y NO SOLO LA CABECERA: `negotiateLocale` mira la cookie ANTES que
  // `Accept-Language`, así que una preferencia guardada en la cuenta pisaba la
  // cabecera y media suite salía en el idioma de siempre. Va en el dominio padre
  // para valer igual en el host de la organización y en el del IdP.
  await contexto.addCookies([
    { name: "sytue_locale", value: idioma, domain: `.${DOMINIO}`, path: "/" },
  ]);
  const page = await contexto.newPage();
  console.log(`\n— ${idioma} —`);

  for (const clave of claves) {
    const def = PRODUCTOS[clave];
    const tomas = [
      {
        nombre: clave,
        ruta: def.ruta,
        abrir: def.abrirPrimero,
        pinchar: def.pinchar,
      },
    ];
    if (def.segunda)
      tomas.push({ nombre: `${clave}-funciones`, ruta: def.segunda });

    for (const toma of tomas) {
      const url = urlDe(def, toma.ruta);
      try {
        const res = await page.goto(url, {
          waitUntil: "domcontentloaded",
          timeout: 60_000,
        });
        await asegurarSesion(page);
        await asentar(page);

        // NO SE GUARDA LO QUE NO SE HA PEDIDO. Las tres comprobaciones salen de una
        // tanda real (2026-09-11) en la que nada falló y tres capturas eran basura:
        // dos de un «404 page not found» —el panel de Cloud no está desplegado en el
        // piloto— y la de la Consola era la pantalla de Mi cuenta, porque la cuenta
        // con la que se disparó no tiene rol de administración y `/admin` la manda
        // allí. Publicar cualquiera de las tres habría sido peor que no tener ficha.
        const estado = res?.status() ?? 0;
        if (estado >= 400) {
          // El porqué cambia el arreglo: un 404 es un producto que no está en este
          // despliegue y un 403 es una cuenta a la que le falta un rol. Decir «no
          // se pudo» a secas manda a mirar el sitio equivocado.
          const pista = def.pendiente
            ? "pendiente a propósito: este producto todavía no se enseña"
            : estado === 403 || estado === 401
              ? "a la cuenta le falta el permiso de esta pantalla"
              : estado === 404
                ? "ese producto no está desplegado aquí"
                : "el servicio no la sirvió";
          console.log(
            `✗ ${toma.nombre.padEnd(18)} ${url}\n  HTTP ${estado}: ${pista}`,
          );
          continue;
        }
        // PROFUNDIZAR DENTRO DEL PRODUCTO NO ES IRSE DE ÉL.
        //
        // Esta guarda exigía la ruta EXACTA y rechazaba Rooms, que abre `/rooms` y
        // te deja en `/rooms/channels/<id>`: su primer canal. No es un fallo, es lo
        // que hace cualquier cliente de mensajería al abrirse —HTTP 200 y la app
        // entera pintada—, y el guion lo descartaba diciendo que faltaba un
        // permiso, que no tenía nada que ver.
        //
        // Lo que sí hay que cazar es acabar en OTRO producto, que es el caso de la
        // Consola: `/admin` manda a `/account` cuando la cuenta no administra, y
        // ahí el HTTP también es 200. Por eso la comprobación es el prefijo y no la
        // igualdad: `/rooms/...` sigue siendo Rooms, `/account` no es `/admin`.
        //
        // Y el segmento de organización tampoco es irse: si la app mete el slug
        // tras el suyo (`/<app>/<org>/…`), la ruta se compara también sin él.
        const destino = new URL(page.url());
        const pedida = new URL(url);
        const limpia = (s) => s.replace(/\/$/, "");
        const fin = limpia(destino.pathname);
        const ini = limpia(pedida.pathname);
        const sinOrg = TENANT
          ? limpia(fin.replace(new RegExp(`^/([^/]+)/${TENANT}(/|$)`), "/$1$2"))
          : fin;
        const dentro = (f) => f === ini || f.startsWith(`${ini}/`);
        if (!dentro(fin) && !dentro(sinOrg)) {
          console.log(
            `✗ ${toma.nombre.padEnd(18)} ${url}\n` +
              `  acabó en ${destino.pathname}, que es otro producto — ¿le falta un permiso a la cuenta?`,
          );
          continue;
        }

        if (toma.pinchar) await pinchar(page, toma.pinchar);
        if (toma.abrir) await abrirPrimero(page);
        // Lo último antes de disparar: después de abrir nada cambia en pantalla.
        const tapados = await taparDatos(page).catch(() => -1);
        const png = `/tmp/captura-${toma.nombre}.${idioma}.png`;
        await page.screenshot({ path: png });

        // Dos capturas idénticas significan que la segunda pantalla no llegó a
        // pintarse: se guardan igual —se ven bien— pero se avisa, porque publicar
        // la misma imagen dos veces en una ficha es lo que nadie mira y todos ven.
        const huella = createHash("sha256")
          .update(readFileSync(png))
          .digest("hex");
        if (huellas.has(huella)) {
          console.log(`  · ${toma.nombre}: idéntica a ${huellas.get(huella)}`);
        }
        huellas.set(huella, toma.nombre);
        // `<clave>.<idioma>.webp`: la ficha de producto se sirve en los cinco
        // idiomas y cada una enseña la suya.
        const salida = aWebp(png, `${SALIDA}${toma.nombre}.${idioma}.webp`);
        hechas++;
        console.log(`✓ ${toma.nombre.padEnd(18)} ${url}`);
        // Se dice SIEMPRE que se tapó algo: quien revisa la tanda tiene que saber
        // que esa imagen lleva un borrón, y por qué.
        if (tapados > 0) {
          console.log(
            `  · ${tapados} dato(s) desenfocado(s): correo, IP o @usuario de fuera de los dominios de ejemplo`,
          );
        } else if (tapados < 0) {
          console.log(
            "  · ⚠ no se pudo aplicar el desenfoque: revísala a ojo antes de publicarla",
          );
        }
        if (!existsSync(salida)) console.log(`  · sin guardar: ${salida}`);
      } catch (e) {
        console.log(
          `✗ ${toma.nombre.padEnd(18)} ${url}\n  ${e.message.split("\n")[0]}`,
        );
      }
    }
  }

  await contexto.close();
}

await navegador.close();
console.log(`\n${hechas} capturas en ${SALIDA} (${idiomas.join(", ")})`);
if (sinMagick) {
  console.log(
    "\n⚠ Sin ImageMagick: se quedan en PNG y pesan unas diez veces más.\n" +
      "  Instálalo (`apt install imagemagick`) y vuelve a lanzarlo, o conviértelas\n" +
      "  a mano: magick <f>.png -quality 82 -define webp:method=6 <f>.webp",
  );
}
