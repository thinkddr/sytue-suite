// Auditoría de vista móvil: detecta desbordamiento horizontal en cada app y
// perfil de dispositivo, y señala el elemento que lo provoca.
//
// Uso: bun run mobile-audit.mjs [app...]   (sin args = todas las que respondan)
// Salida: informe.json y capturas en un directorio privado indicado en consola.
//
// SUPERFICIE=escritorio mide las mismas apps en VENTANAS de escritorio en vez de
// en teléfonos. Es el
// mismo barrido con otra tabla de perfiles —ver PERFILES_ESCRITORIO—: la sesión,
// las rutas, las pestañas y los desplegables son idénticos, así que un guion
// aparte serían novecientas líneas duplicadas para diverger. Sale en
// Cada ejecución de escritorio también usa su propio directorio privado.
//
// POR DEFECTO VA CONTRA PRODUCCIÓN (sytue.eu): levantar la suite entera en local
// para auditarla ahoga la máquina, y prod es además lo que de verdad ve un
// cliente. Para apuntar a otro entorno: SYTUE_BASE_DOMAIN=sytue.localhost.
//
// Credenciales: NUNCA en el código. Vienen de SYTUE_AUDIT_EMAIL/PASS/TOTP (el
// usuario de pruebas, un employee sin privilegios), salvo contra .localhost,
// donde el admin de dev es público y está en el repo de todos modos.
//
// ponytail: script de verificación, no forma parte de la suite de specs.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium, devices } from 'playwright';
import { appsDesplegadas, desplegada } from './apps-desplegadas.mjs';
import { fileURLToPath } from 'node:url';
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { codigoTOTP, esCuentaDelIdPDesdeAdmin, porQueNoEntra } from './tests/helpers.ts';
import {
  DETECTOR,
  DETECTOR_DOCUMENTO,
  DETECTOR_FUGAS,
  DETECTOR_VERTICAL,
  unir,
  linea,
  lineaDocumento,
  lineaFugas,
  lineaVertical,
} from './detector-desbordamiento.mjs';
import { DETECTOR_MARGENES, lineaMargen } from './detector-margenes.mjs';

/**
 * QUÉ SE MIDE. Por defecto, desbordamiento horizontal. `AUDITAR=margenes` mide
 * en cambio los controles pegados al borde de su tarjeta o de la ventana.
 *
 * Es el mismo barrido —la sesión, las rutas, las pestañas y los desplegables son
 * idénticos— con otro medidor enchufado, por lo mismo que `SUPERFICIE=escritorio`
 * no es un guion aparte: serían novecientas líneas duplicadas para diverger.
 */
const MARGENES = process.env.AUDITAR === 'margenes';
const MEDIDOR = MARGENES ? DETECTOR_MARGENES : DETECTOR;
const LINEA = MARGENES ? lineaMargen : linea;

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const ES_LOCAL = DOMINIO.endsWith('.localhost');
const IDP = `id.${DOMINIO}`;

// El estado de sesión se guarda por entorno: mezclar el de prod con el de dev
// deja cookies del dominio equivocado y todo redirige al login sin decir por qué.
const STORAGE = fileURLToPath(
  new URL(ES_LOCAL ? '.auth/admin.json' : '.auth/prod-audit.json', import.meta.url),
);

const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || (ES_LOCAL ? 'admin@sytue.localhost' : ''),
  pass: process.env.SYTUE_AUDIT_PASS || (ES_LOCAL ? 'sytue-dev-admin' : ''),
  totp: process.env.SYTUE_AUDIT_TOTP || undefined, // undefined = el secreto de dev
};
if (!CRED.email || !CRED.pass) {
  console.error(
    `Faltan credenciales para ${DOMINIO}: exporta SYTUE_AUDIT_EMAIL y SYTUE_AUDIT_PASS ` +
      `(y SYTUE_AUDIT_TOTP si la cuenta tiene segundo factor).`,
  );
  process.exit(2);
}

/**
 * La sesión caduca antes de que termine el barrido: a mitad de camino todo
 * redirige al IdP y CUALQUIER app parece perfecta, porque la página de login no
 * desborda. Se vuelve a entrar en la misma pestaña y se sigue.
 *
 * `destino` es la URL que se quería medir. Estar en el IdP solo es un DESVÍO si
 * no era ahí adonde íbamos: las pantallas del propio Sytue ID se miden donde
 * están, y rellenar el formulario allí mediría la pantalla siguiente en vez de
 * la que se pidió. `/account` sin sesión SÍ desvía —a `/login/?return_to=…`—, y
 * ese caso es el de siempre: se entra y se vuelve.
 *
 * Y solo se envía el formulario de ACCESO, nunca otro: `/forgot-password` tiene
 * un campo `email` igual de rellenable y enviarlo dispararía un correo de
 * recuperación, o sea una MUTACIÓN en producción desde una auditoría de solo
 * lectura. En `/account` no hay formulario y esto reventaba con un timeout.
 */
/**
 * Segundos que el IdP pide esperar, o 0 si no nos está limitando.
 *
 * `loginRL` permite 10 accesos por minuto y al agotarse contesta 429 con
 * `Retry-After`. Desde el navegador eso se ve IGUAL que un código rechazado —se
 * vuelve al formulario—, así que sin mirar el status se culpa a la credencial de
 * lo que hace el bucket. Se anota por página, que es donde ocurre.
 */
const limitando = new WeakMap();
function vigilarLimite(page) {
  if (limitando.has(page)) return;
  limitando.set(page, 0);
  page.on('response', (res) => {
    if (res.status() !== 429) return;
    if (!/\/login\//.test(new URL(res.url()).pathname)) return;
    limitando.set(page, Number(res.headers()['retry-after'] ?? 0) || 30);
  });
}

/**
 * `page.goto` con los reintentos que merece cada fallo. Todas las navegaciones
 * del barrido pasan por aquí.
 *
 * Un reintento para cualquier error: con la suite entera en dev sobre una VM, un
 * dev server de Astro compilando bajo demanda tarda más que el timeout y eso se
 * leía como defecto de la app. Si falla dos veces, es real.
 *
 * DOS para `net::ERR_NETWORK_CHANGED`, porque ese NO ES LA APP: Chromium aborta
 * la navegación en curso cuando cambia una interfaz de red del anfitrión, y donde
 * corre este barrido —el runner del CI con sus contenedores, el piloto con k3s—
 * aparecen y desaparecen interfaces virtuales sin parar. El 14/09/2026 tumbó Fotos
 * y Mail, que cargaban bien cinco segundos después; y como el guion sale con 1
 * ante cualquier pantalla sin medir, la red del anfitrión pintaba la puerta de rojo.
 */
async function navegar(page, url) {
  for (let intento = 1; ; intento++) {
    try {
      const respuesta = await page.goto(url, {
        waitUntil: 'domcontentloaded',
        timeout: 45_000,
      });
      // El gateway puede no encontrar un pod durante una sustitución. Solo se
      // reintentan esos estados transitorios; el tercero se entrega al detector
      // HTTP, que mantiene el fallo si el servicio no se recupera.
      if ([502, 503, 504].includes(respuesta?.status()) && intento < 3) {
        await page.waitForTimeout(5_000);
        continue;
      }
      return respuesta;
    } catch (e) {
      const deLaRed = String(e).includes('ERR_NETWORK_CHANGED');
      if (intento >= (deLaRed ? 3 : 2)) throw e;
      await page.waitForTimeout(5_000);
    }
  }
}

async function asegurarSesion(page, destino = '') {
  vigilarLimite(page);
  // `/oauth2/authorize` es un PASO, no una pantalla: con sesión en el IdP redirige
  // solo de vuelta a la app. Con la doble ida y vuelta de OIDC entre el host del
  // producto y el del tenant, a los 3,5s de `domcontentloaded` a veces aún no ha
  // terminado, y leer la URL en ese instante tomaba el paso por una pantalla
  // desconocida y rompía el perfil entero (Docs en iPhone 12, 14/09/2026). Se
  // espera a que salga de ahí —a la app o al formulario— antes de decidir; si no
  // sale, lo de abajo sigue rompiendo con el mismo mensaje.
  //
  // Y si en 20 s no sale, se recarga —relanza el paso— hasta tres veces, esperando
  // lo que pida el IdP si está limitando. Medido en el run 2043 (2026-09-15): el
  // paso se quedó quieto en Pixel 5 sobre Calendar y rompió el perfil, con las
  // 671 visitas restantes entrando sin problema.
  const esPaso = (u) => /^\/oauth2\/authorize\/?$/.test(u.pathname);
  for (let intento = 1; esPaso(new URL(page.url())); intento++) {
    const salio = await page
      .waitForURL((u) => !esPaso(u), { timeout: 20_000 })
      .then(
        () => true,
        () => false,
      );
    if (salio || intento >= 3) break;
    const pedido = limitando.get(page) ?? 0;
    await page.waitForTimeout(pedido > 0 ? (pedido + 5) * 1000 : 5_000);
    await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {});
  }
  const aqui = new URL(page.url());
  if (aqui.hostname !== IDP) return false;
  const sinBarra = (p) => p.replace(/\/+$/, '');
  const d = destino ? new URL(destino) : null;
  if (d && d.hostname === IDP && sinBarra(d.pathname) === sinBarra(aqui.pathname)) return false;
  // La cuenta de auditoría es un employee y no puede entrar en la consola de
  // administración. El BFF de `admin` la lleva deliberadamente a la pantalla
  // de cuenta del IdP: esa ES la interfaz que puede ver esta cuenta, no un login
  // incompleto. Es una excepción de destino exacto; tratar cualquier `/account`
  // como válido escondería una redirección rota de otro producto.
  if (esCuentaDelIdPDesdeAdmin(destino, page.url(), DOMINIO)) return false;
  // Solo se envía el formulario de ACCESO, y se reconoce por el ÚNICO campo que
  // no comparte con los demás: la contraseña (`/forgot-password` tiene el mismo
  // `input[name=email]`, comprobado en producción). Si el IdP presenta otra cosa
  // —el paso del segundo factor, una recuperación—, ROMPE en vez de devolver
  // `false`: medir esa pantalla como si fuera la de la app sería un «ok» falso.
  if ((await page.locator('input[name=password]').count()) === 0)
    throw new Error(`el IdP muestra ${aqui.pathname}, que no es el formulario de acceso`);
  await page.fill('input[name=email]', CRED.email);
  await page.fill('input[name=password]', CRED.pass);
  await page.click('button[type=submit]');
  // EL SEGUNDO FACTOR SE DETECTA POR LA URL, no solo esperando al campo. El IdP
  // contesta al formulario con un 302 a `/login/?…&step_up=1`, y esperar 5 s a
  // que se pinte el input es una carrera que se pierde en cuanto el IdP tarda un
  // poco: el barrido se saltaba el relleno sin decir nada y luego medía TODAS las
  // pantallas sin sesión, contándolas como fallos del producto. Con `step_up=1`
  // sabemos que HAY que rellenar, así que esperar de más es correcto.
  const pideSegundoFactor = () => /[?&]step_up=1/.test(page.url());
  await page.waitForURL(pideSegundoFactor, { timeout: 15_000 }).catch(() => {});

  const totp = page.locator(
    'input[name=code], input[name=totp], input[autocomplete="one-time-code"]',
  );
  const hayCampo = await totp
    .first()
    .waitFor({ timeout: pideSegundoFactor() ? 30_000 : 5_000 })
    .then(
      () => true,
      () => false,
    );
  if (pideSegundoFactor() && !hayCampo) {
    throw new Error('el IdP pide segundo factor y su formulario no llegó a aparecer');
  }
  if (hayCampo) {
    await totp.first().fill(codigoTOTP(CRED.totp));
    await page.click('button[type=submit]');
    // Se ESPERA a que la URL deje de pedir el segundo factor, no un tiempo fijo:
    // un código CORRECTO deja `step_up=1` hasta que el IdP completa el redirect,
    // así que una espera corta declaraba rechazado lo que solo iba lento.
    const salio = await page
      .waitForURL((u) => !/[?&]step_up=1/.test(u.href), { timeout: 20_000 })
      .then(
        () => true,
        () => false,
      );
    if (!salio) {
      const pedido = limitando.get(page) ?? 0;
      if (pedido > 0) {
        throw new Error(
          `el IdP está limitando el acceso (429; 10/min): pide ${pedido} s. NO es la ` +
            'credencial — el bucket se agota con varios barridos seguidos.',
        );
      }
      // NO se nombra una causa que no se ha medido. Aquí solo consta que el
      // acceso no se completó, y eso lo produce cualquier cosa: el segundo
      // factor rechazado, sí, pero también que /oauth2/authorize devuelva al
      // formulario con la sesión ya elevada. Acusar a la semilla mandó dos veces
      // a rotar una credencial que era válida —huella 37349095, base32, reloj
      // correcto— mientras el fallo estaba en otro sitio. Un rechazo de código
      // sería 401 (`loginFail` escribe StatusUnauthorized), no un 302.
      //
      // Se vuelca lo OBSERVADO: la URL donde se quedó y el texto de la pantalla.
      const donde = page.url().slice(0, 140);
      const texto = (
        (await page
          .locator('body')
          .innerText()
          .catch(() => '')) || ''
      )
        .replace(/\s+/g, ' ')
        .slice(0, 160);
      throw new Error(
        `no se completó el acceso tras enviar el segundo factor.\n` +
          `  se quedó en: ${donde}\n` +
          `  pantalla:    «${texto}»\n` +
          `  Causas posibles, en orden de probabilidad: (1) /oauth2/authorize no ` +
          `reconoce la sesión recién elevada y devuelve al formulario; (2) la semilla ` +
          `de SYTUE_AUDIT_TOTP no es la del factor activo. Para distinguirlas, mirar ` +
          `si el IdP registró «requiresStepUp» en sus logs: si está, es (1).`,
      );
    }
  }
  // Fin del login = estar donde se iba. Y a veces se iba al PROPIO IdP: entrar
  // desde `/login/?return_to=/account` termina en `/account`, que sigue siendo
  // `id.<dominio>`, así que la condición «otro host» no se cumple nunca y esto
  // colgaba 60 s para dar la pantalla por caída. Se compara la RUTA exacta y no
  // «cualquier cosa que no sea /login», porque el segundo factor pasa por
  // `/login/mfa` y eso habría dado el login por terminado a mitad.
  const llegado =
    d && d.hostname === IDP
      ? (u) => sinBarra(u.pathname) === sinBarra(d.pathname)
      : (u) => u.hostname !== IDP;
  // CON ESPERA Y REINTENTO, porque el IdP limita el login a 10/min
  // (`loginLimiter` en services/id/internal/http/server.go: PerSecond 10/60,
  // Burst 10). Los 18 perfiles comparten la sesión guardada, pero cuando la
  // cookie caduca a mitad del barrido los 18 reintentan casi a la vez y el
  // bucket los tumba a todos: medido en el run 2015 del 2026-08-14, **79 FALLO
  // contra 50 ok**, todos con este mismo timeout, empezando en `iPhone SE` y
  // sin recuperarse ya nunca. Y esos fallos se imprimen igual que un fallo de
  // la app, así que una regresión de verdad quedaría enterrada entre ellos.
  //
  // El bucket se rellena a 10/min, así que esperar SÍ resuelve: no se sube el
  // límite del IdP —que está para lo que está— sino que se respeta.
  for (let intento = 1; ; intento++) {
    try {
      await page.waitForURL(llegado, { timeout: 60_000 });
      return true;
    } catch (e) {
      // Reintentar tiene sentido contra un limitador, que se rellena solo. NO lo
      // tiene contra una cuenta sin segundo factor: ahí los tres intentos son
      // tres minutos por perfil —dieciocho perfiles— para acabar en el mismo
      // sitio, y con el barrido entero contando pantallas vacías como fallos de
      // producto. Si la URL ya dice la causa, se corta aquí.
      const porque = porQueNoEntra(page.url());
      if (porque) throw new Error(`el login no llega a la app: ${porque}`);
      if (intento >= 3) throw e;
      // Se espera lo que PIDE el IdP (`Retry-After`) y no un número inventado:
      // con el bucket muy vacío, 30 s se quedan cortos y el reintento lo vuelve
      // a agotar. Si no nos está limitando, el escalón de siempre.
      const pedido = limitando.get(page) ?? 0;
      const espera = pedido > 0 ? (pedido + 5) * 1000 : intento * 30_000;
      console.log(
        pedido > 0
          ? `  · el IdP LIMITA el acceso (429, 10/min): pide ${pedido} s. Esperando y reintentando (${intento}/3).`
          : `  · el IdP no completó el acceso (intento ${intento}/3). Esperando ${espera / 1000} s y reintentando.`,
      );
      await page.waitForTimeout(espera);
      await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {});
      if (llegado(new URL(page.url()))) return true;
      await page.fill('input[name=email]', CRED.email).catch(() => {});
      await page.fill('input[name=password]', CRED.pass).catch(() => {});
      await page.click('button[type=submit]').catch(() => {});
    }
  }
}

/**
 * Rutas por app: la home y las superficies con más contenido. Las que dependen
 * de un dato (un documento, un canal, un proyecto) no se pueden escribir a mano:
 * `{ desde, enlace }` las descubre siguiendo el primer enlace que encaje.
 *
 * `opcional: true` marca la ruta que HOY no se puede resolver porque la cuenta
 * de auditoría no tiene ese dato. Sin la marca, el salto pondría la puerta
 * nocturna en rojo cada noche por un motivo conocido y dejaría de leerse; con
 * ella, el salto sigue saliendo en el informe pero no rompe, y CUALQUIER otro
 * salto —una ruta que antes resolvía y deja de hacerlo— sí rompe, que es
 * exactamente la señal que interesa.
 */
/**
 * GET de JSON a un BFF, hecho DESDE LA PÁGINA.
 *
 * Y no con `page.request.get`, que es lo que había: aquel usa el contexto de
 * peticiones de Playwright y no llevaba la cookie de sesión, así que los dos
 * resolvedores que preguntan por API —el PDF de Drive y los cuadernos de
 * Notes— se saltaban con un 401 mientras el navegador tenía sesión de sobra:
 * en la misma ejecución, `notes.sytue.eu/notes` se midió con el título «Sytue
 * Notes», o sea con sesión. Tres superficies sin medir por el transporte.
 *
 * `fetch` dentro de la página sale del origen correcto y con sus cookies, que
 * es exactamente como llama la aplicación a su propio BFF. Se devuelve el
 * estado además del cuerpo para poder distinguir «no autorizado» de «vacío»,
 * que es la diferencia entre un fallo del barrido y un hueco de la cuenta.
 */
async function pedirJson(page, url) {
  return page.evaluate(async (u) => {
    // EL ORIGEN LO PONE LA PÁGINA, no quien escribió la URL.
    //
    // Los resolvers componen `https://drive.<dominio>/drive/api/…` porque es el
    // nombre público del producto, pero al entrar con una cuenta de un tenant la
    // app redirige a `corporate.<dominio>/drive`: desde ahí ese fetch es
    // CROSS-ORIGIN y lo tumba CORS con «Failed to fetch», que no se parece a un
    // problema de origen — se parece a que el servicio está caído. Eso dejó seis
    // rutas sin medir (`/pdf` y `/?notebook=` en los tres tamaños de ventana) y
    // con ellas el barrido de escritorio en rojo, contando como «no aprobadas»
    // unas pantallas que nadie había llegado a mirar.
    //
    // Se conservan ruta y query; solo se cambia el origen por el de la página, que
    // es donde vive la cookie de sesión que este fetch necesita de todos modos.
    const destino = new URL(u, location.href);
    destino.protocol = location.protocol;
    destino.host = location.host;
    const r = await fetch(destino, {
      credentials: 'include',
      headers: { accept: 'application/json' },
    });
    let cuerpo = null;
    try {
      cuerpo = await r.json();
    } catch {
      /* un 401 del BFF puede venir sin cuerpo JSON */
    }
    return { ok: r.ok, status: r.status, cuerpo };
  }, url);
}

/** GET a la API de Sytue Fotos, pasando antes por su app para tener su sesión. */
async function pedirFotos(page, ruta) {
  await navegar(page, `https://photos.${DOMINIO}${BASES.photos}/`);
  await asegurarSesion(page);
  const r = await pedirJson(page, `https://photos.${DOMINIO}${BASES.photos}${ruta}`);
  if (!r.ok) throw new Error(`Sytue Fotos respondió ${r.status} a ${ruta}`);
  return r.cuerpo ?? {};
}

/** La foto más reciente de la cuenta, para las pantallas que se abren por `?id=`. */
async function primeraFoto(page) {
  const r = await pedirFotos(page, '/api/v1/timeline?limit=1');
  const id = (r.items ?? [])[0]?.id;
  if (!id) throw new Error('sin datos: la cuenta no tiene ninguna foto');
  return encodeURIComponent(id);
}

/**
 * La papelera de Docs SOLO si el despliegue ya la tiene.
 *
 * El barrido mide PRODUCCIÓN con el código de `main`, y entre que una pantalla se
 * fusiona y se despliega pasan horas o días: pedir `/trash` a una versión que no la
 * tiene es un 404 que pondría la puerta en rojo sin que nada esté roto. Se pregunta
 * a su BFF (un GET que no toca nada): si no existe, la ruta se salta como OPCIONAL y
 * lo dice; en cuanto el despliegue la trae, se mide y un fallo vuelve a romper.
 */
const PAPELERA_DOCS = {
  enlace: '/trash',
  // Se pregunta UNA vez por barrido, no en cada uno de los 18 perfiles.
  desplegada: undefined,
  resolver: async (page) => {
    if (PAPELERA_DOCS.desplegada === undefined) {
      await navegar(page, `https://docus.${DOMINIO}${BASES.docus}/`);
      await asegurarSesion(page);
      const r = await pedirJson(page, `https://docus.${DOMINIO}${BASES.docus}/api/trash`);
      PAPELERA_DOCS.desplegada = r.status !== 404;
      PAPELERA_DOCS.opcional = !PAPELERA_DOCS.desplegada;
    }
    if (!PAPELERA_DOCS.desplegada) {
      throw new Error('no desplegada todavía: el BFF no tiene /api/trash');
    }
    return '/trash';
  },
};

const APPS = {
  portal: [
    '/',
    '/actividad',
    '/bienvenida',
    '/buscar',
    '/descargas',
    '/dispositivos',
    '/notificaciones',
    '/datos',
  ],
  mail: ['/'],
  drive: ['/'],
  calendar: ['/', '/events/new'],
  contacts: ['/'],
  // El EDITOR (documento, hoja, presentación) es la superficie más ancha de la
  // suite. Estuvo SIN MEDIR mucho tiempo porque la cuenta de auditoría no tenía
  // documentos y crear uno era mutar producción — hasta que se autorizó
  // expresamente hacerlo con ESTA cuenta. Ahora lo siembra
  // `sembrar-auditoria.mjs` (un documento con título fijo, idempotente) y la ruta
  // deja de ser opcional: si no se resuelve, es un FALLO y no una limitación
  // conocida. Esa distinción es todo el valor de este cambio.
  // La ofimática, una app por producto desde el 2026-10-12 (ADR 0076): Docus lleva el
  // documento sembrado y el editor; Calcus y Diapos montan las mismas vistas.
  docus: [
    '/',
    // La papelera (2026-09-28): lista, restaurar y vaciar; su fila lleva dos botones.
    PAPELERA_DOCS,
    { desde: '/', enlace: '/edit?id=' },
  ],
  calcus: ['/', '/trash'],
  diapos: ['/', '/trash'],
  certipdf: [
    '/',
    {
      // Sytue PDF. No estaba en esta lista y ahí vivía el peor desbordamiento de
      // la suite: la barra medía 415px y sacaba la página de la pantalla en TODO
      // teléfono (+95px a 320, +55 a 360, +3 a 412).
      //
      // No hay ruta fija que escribir —se abre por RUTA de un fichero de Drive— ni
      // enlace que seguir: Drive lo abre con `window.open`, no con un `<a>`. Se le
      // pregunta a su buscador, que es un GET y no toca nada.
      enlace: '/pdf',
      resolver: async (page) => {
        // El BFF de Drive responde 401 si no se ha pasado antes por su app: la
        // cookie de sesión es suya, no de la suite.
        await navegar(page, `https://drive.${DOMINIO}${BASES.drive}`);
        await asegurarSesion(page);
        const r = await pedirJson(
          page,
          `https://drive.${DOMINIO}${BASES.drive}/api/search?q=.pdf&limit=50`,
        );
        if (!r.ok) throw new Error(`Drive respondió ${r.status} al buscar un PDF`);
        const items = r.cuerpo?.items ?? [];
        const ruta = items.find((i) => /\.pdf$/i.test(i.path ?? ''))?.path;
        if (!ruta) throw new Error('sin datos: ningún PDF en el Drive de la cuenta');
        return `/pdf?path=${encodeURIComponent(ruta)}`;
      },
    },
  ],
  // El canal NO necesita descubrirse: `rooms.<dominio>/` ya aterriza en
  // `/channels/<id>` por pushState, así que `/` mide la vista del canal. El
  // `{ desde, enlace }` que había aquí saltaba en los 18 perfiles buscando un
  // `a[href*="/channels/"]` que no existe, y no añadía ni una medida.
  //
  // `/share` es el destino del deep-link «compartir a Rooms» que componen Drive y
  // el Hub, y tampoco estaba: llevaba días devolviendo un 200 truncado de 21
  // bytes —pantalla en blanco— sin que ningún monitor lo viera. Aquí sale FALLO,
  // que es lo que tenía que haber pasado desde el principio.
  rooms: ['/', '/share'],
  // Sytue Cast: portada con sesión, portada pública y subida (§10 del encargo).
  cast: ['/', '/explore', '/upload'],
  // Sytue Web: «Mis sitios», el asistente y los ajustes de la organización (§14 del
  // encargo). El editor y la ficha de un sitio llevan un id y los mide el integrador.
  pages: ['/', '/new', '/settings'],
  // SYTUE FOTOS. La galería nació con una sola ruta aquí y desde entonces le han
  // salido cuatro más; medir solo la portada dejaba fuera justo las pantallas
  // nuevas, que son las que se rompen.
  //
  // Y aun así se escapaba lo que más se usa. El 2026-09-15 Daniel vio Fotos
  // desbordando en su teléfono con este barrido en verde: era el VISOR, que no
  // tiene ruta —se abre tocando una foto— y desbordaba 520px a 320. Ahora lo abre
  // `conDialogoAbierto`, y el editor, las parecidas, las cercanas y el álbum, que
  // no visitaba nadie, se resuelven por la API con la primera foto y el primer
  // álbum. Los siembra `sembrar-auditoria.mjs`: sin ellos es un FALLO, no una
  // limitación conocida. La página del enlace compartido sigue fuera: su
  // credencial es un token que este barrido no tiene.
  photos: [
    '/',
    '/favorites',
    '/shared',
    '/albums',
    '/duplicates',
    '/trash',
    ...['/edit?id=', '/similar?id=', '/nearby?id='].map((enlace) => ({
      enlace,
      resolver: async (page) => `${enlace}${await primeraFoto(page)}`,
    })),
    {
      enlace: '/album?id=',
      resolver: async (page) => {
        const r = await pedirFotos(page, '/api/v1/albums');
        const id = (r.items ?? [])[0]?.id;
        if (!id) throw new Error('sin datos: la cuenta no tiene ningún álbum');
        return `/album?id=${encodeURIComponent(id)}`;
      },
    },
  ],
  // SYTUE NOTES NO SE HABÍA MEDIDO NI UNA VEZ. Nació después del último barrido
  // y entró en producción sin pasar por aquí, que es exactamente cómo se colaron
  // las regresiones de agosto de 2026 en las otras diez.
  //
  // `/notebooks/{id}` NO se mide, y no es un olvido: esa página solo redirige a
  // `/?notebook=…` (lo dice su propio comentario), así que medirla sería medir un
  // 302. Lo que hay que mirar es la pantalla a la que lleva.
  notes: [
    '/',
    {
      // El cuaderno abierto y, dentro, una nota: el editor es la superficie más
      // ancha de esta app, igual que en Docs. Se resuelve por API y no siguiendo
      // un enlace porque la navegación es de la isla —query, no `<a href>`— y un
      // `{ desde, enlace }` se pasaría los 18 perfiles buscando un ancla que no
      // existe, como ya pasó con el canal de Rooms.
      enlace: '/?notebook=',
      resolver: async (page) => {
        await navegar(page, `https://notes.${DOMINIO}${BASES.notes}/`);
        await asegurarSesion(page);
        const r = await pedirJson(page, `https://notes.${DOMINIO}${BASES.notes}/api/notebooks`);
        if (!r.ok) throw new Error(`Notes respondió ${r.status} al pedir los cuadernos`);
        // `.notebooks`, no `.items`: cada BFF nombra su lista. Drive devuelve
        // `items` y Notes `notebooks`; darlo por hecho habría dicho «sin datos»
        // sobre una cuenta que sí tiene cuadernos, que es un falso negativo
        // silencioso —la app se quedaría sin medir y el barrido en verde—.
        const cuadernos = r.cuerpo?.notebooks ?? [];
        const cuaderno = cuadernos[0]?.id;
        if (!cuaderno) throw new Error('sin datos: la cuenta no tiene ningún cuaderno');
        // La nota es opcional DENTRO de esta ruta: un cuaderno vacío sigue siendo
        // una pantalla que medir, y exigir nota convertiría «cuaderno recién
        // creado» en un fallo del barrido.
        const rn = await pedirJson(
          page,
          `https://notes.${DOMINIO}${BASES.notes}/api/notebooks/${encodeURIComponent(cuaderno)}/notes`,
        );
        const nota = rn.ok ? (rn.cuerpo?.notes ?? [])[0]?.id : null;
        const q = new URLSearchParams({ notebook: cuaderno });
        if (nota) q.set('note', nota);
        return `/?${q.toString()}`;
      },
    },
  ],
  // Administración GLOBAL de la plataforma y de la organización. Solo la HOME:
  // con un employee, /users /groups /roles /audit /sso /apps devuelven 403 con
  // el cuerpo «No autorizado para este recurso.» (un párrafo suelto, un nodo),
  // sobre el que el detector no desborda jamás. Estaban en esta lista y contaban
  // como aprobadas: 30 de las 132 combinaciones del barrido de agosto de 2026
  // eran eso. Medido con la cuenta de auditoría el 11-08-2026.
  //   admin/     → /account, UI real.
  //   platform/  → /platform, la pantalla «no estás en la allowlist», UI real.
  // Las CONSOLAS —las pantallas más densas de la suite, con sus tablas— siguen
  // SIN MEDIR en móvil: hace falta una credencial de auditoría con rol de admin
  // de LECTURA. La comprobación de estado HTTP de más abajo impide que vuelvan a
  // colarse como aprobadas si alguien las devuelve a la lista sin la credencial.
  platform: ['/'],
  admin: ['/'],
  // El ERP y Bases NO ESTABAN, y tampoco en DIR_A_APP, así que ni siquiera
  // salían como SIN CUBRIR: el ERP estrenó armazón propio (riel, cajón y hoja) y
  // en producción no lo medía nadie (2026-10-05). Los dos son de una sola
  // página: lo demás —la ficha del ERP, una base abierta— depende de un dato.
  erp: ['/'],
  bases: ['/'],
  cloud: [
    '/',
    '/billing',
    '/usage',
    '/team',
    '/settings',
    '/tokens',
    '/templates',
    '/audit',
    // Mismo caso que el editor de Docs: sin proyecto en la organización de la
    // cuenta de auditoría no hay enlace que seguir.
    // Sigue opcional, y aquí el motivo NO es que falte permiso: crear un proyecto
    // de Sytue DevX levanta recursos que CUESTAN DINERO cada mes. Sembrar eso
    // para medir una pantalla sería pagar una factura por un barrido, así que se
    // queda sin medir a propósito y dicho.
    { desde: '/', enlace: '/projects/', opcional: true },
  ],
  // Sytue Git no estaba en esta lista: nunca se había medido, ni una vez. Solo
  // lo ANÓNIMO, que es lo que se puede: el servidor lleva REQUIRE_SIGNIN_VIEW=true
  // (deploy/sytue-git/bin/bootstrap-server.sh) y la cuenta de auditoría no tiene
  // identidad en Gitea, así que /explore y la UI de repositorio redirigen a
  // /user/login. La sesión de la suite no estorba: sus cookies son de otro host.
  // La UI CON SESIÓN (repositorio, código, incidencias, PR, acciones) queda SIN
  // MEDIR hasta que ops@ tenga cuenta. Y el sitio por donde tocaría arreglarlo
  // NO EXISTE: aquí ponía «el arreglo va por el CSS del tema propio
  // (theme-sytue-auto)» y ese tema no está en ninguna parte —`theme-sytue` no
  // aparece en el repositorio más que en esta línea, y deploy/sytue-git/ no tiene
  // un solo .css ni monta CUSTOM_PATH—. O sea que la salida escrita mandaba a un
  // fichero imaginario. Cuando haga falta, se CREA ese CSS y se monta en el
  // CUSTOM_PATH de Gitea (public/assets/css/theme-*.css); parchear Gitea, nunca.
  // `/user/sign_up` NO entra aquí a propósito: responde 200 a un anónimo en una
  // forja con REQUIRE_SIGNIN_VIEW=true y lo suyo es que deje de existir; medirla
  // sería atar esta puerta a que siga abierta.
  // La forja autoalojada (git.<dominio>) está RETIRADA desde el 2026-09-04: el
  // repositorio vive en GitHub desde el 2026-09-27, antes en git.gay (docs/operations.md#baja-scaleway). Sus tres rutas
  // caían al 404 del ingress y tumbaban el barrido cada noche.
  // SYTUE ID, que no se medía NUNCA y es la única pantalla que ve el 100% de los
  // usuarios: aquí se teclea la contraseña, el segundo factor y la passkey, y en
  // `/account` viven perfil, seguridad, dispositivos y accesos. No estaba porque
  // `id.` aparecía en este fichero solo como `IDP`, para detectar el redirect.
  //
  // La sirve Go con su propio CSS embebido (services/id/internal/http/login.go),
  // no el frontend Astro, así que tampoco existe para `paginasSinCubrir` —que
  // enumera `apps/frontend/apps/*/src/pages/**.astro`—: la puerta ni siquiera
  // sabía que le faltaba. Ahí seguía vivo el `min-height:100vh` del <body> que la
  // regla 4 del lint prohíbe en toda la suite.
  //
  // `/login` va primero a propósito: es la única ruta que miden los perfiles
  // `soloHome`. Y no hace falta excluir nada de la exploración interactiva —el
  // IdP no emite un solo `aria-haspopup`, `aria-expanded` ni `role="tab"`—, así
  // que aquí no se pulsa ningún botón: solo se mide.
  //
  // ponytail: el hueco de COBERTURA de lo servido por Go se cierra listando las
  // rutas, no enumerándolas. Las registra `s.mux.Handle` repartido en 40
  // ficheros y un enumerador por grep sería una segunda fuente de verdad que
  // diverge; si el IdP gana pantallas, se añaden aquí.
  id: ['/login', '/forgot-password', '/account'],
  // De www no basta la portada: el idioma va en la RUTA, así que `sytue.eu/`
  // sirve inglés por más `Accept-Language` que se mande, y el inglés es
  // justamente el idioma de palabras más cortas. Los titulares desbordaban el
  // documento entero en 23 combinaciones de las rutas localizadas y ninguna
  // estaba aquí. No hacen falta las 40 fichas: con el idioma de palabras más
  // largas (alemán) y el titular más largo de cada idioma se cubre el caso.
  www: [
    '/',
    '/es',
    '/de',
    '/fr',
    '/it',
    '/downloads',
    '/de/terms',
    '/de/privacy',
    '/de/product/docs',
    '/fr/product/rooms',
    '/it/product/drive',
  ],
};

/** El host no siempre es `<app>.<dominio>` (ver los routers del ingress). */
const HOSTS = { portal: `one.${DOMINIO}`, www: DOMINIO };

/**
 * El prefijo bajo el que se sirve cada app (`base` de Astro).
 *
 * SIN ESTO LA SONDA MIENTE, y mintió: las rutas de `APPS` son las del árbol de
 * páginas —`/pdf`, `/actividad`— pero en producción viven bajo el prefijo, así
 * que pedirlas en la raíz del host devuelve el 404 de Traefik. El barrido lo
 * apuntaba como fallo del producto («certipdf /pdf → 404») cuando la página estaba
 * perfectamente en `/certipdf/pdf`. Un hallazgo inventado por la propia sonda es de
 * lo más caro que hay: parece real y cuesta una investigación entera descartarlo.
 *
 * `www` y `platform` se sirven en la raíz de su host y por eso no están aquí.
 */
const BASES = {
  portal: '/one',
  mail: '/mail',
  drive: '/drive',
  calendar: '/calendar',
  contacts: '/contacts',
  docus: '/docus',
  calcus: '/calcus',
  diapos: '/diapos',
  certipdf: '/certipdf',
  rooms: '/rooms',
  cloud: '/cloud',
  admin: '/admin',
  notes: '/notes',
  photos: '/photos',
  cast: '/cast',
  pages: '/web',
  erp: '/erp',
  bases: '/bases',
};

/** La URL real de una ruta de esta app: el prefijo va delante. */
function urlDe(app, host, ruta) {
  const base = BASES[app] || '';
  if (!base) return `https://${host}${ruta}`;
  return `https://${host}${ruta === '/' ? base : base + ruta}`;
}

/**
 * Idioma del barrido. El alemán es el de palabras más largas —es el que rompió
 * los titulares de www— y en las apps con sesión no se medía nunca porque esto
 * estaba escrito a mano: `LOCALE=de-DE bun run mobile-audit.mjs portal mail`.
 */
const LOCALE = process.env.LOCALE || 'es-ES';

/**
 * Perfiles reales (Playwright). El motor es Chromium en todos porque en Android
 * lo es de hecho: Chrome, Samsung Internet, Edge y Opera son Blink.
 *
 * `soloHome` recorre únicamente la primera ruta de cada app: sirve para los
 * perfiles que solo añaden una anchura o una densidad, sin repetir las 25 rutas.
 */
const PERFILES = [
  // --- iOS ---
  ['iPhone SE', devices['iPhone SE']], // 320×568 — el suelo
  ['iPhone 12', devices['iPhone 12']], // 390×844 — el más común
  ['iPhone 14 Pro Max', devices['iPhone 14 Pro Max']], // 430×932
  ['iPhone 12 landscape', devices['iPhone 12 landscape']], // 844×390
  ['iPad Mini', devices['iPad Mini']], // 768×1024
  // --- Android: teléfonos ---
  ['Galaxy S9+', devices['Galaxy S9+']], // 320×658 dsf 4,5 — el más estrecho
  ['Moto G4', devices['Moto G4']], // 360×640 — el ancho MÁS común de Android
  ['Galaxy S24', devices['Galaxy S24'], { soloHome: true }], // 360×780
  ['Pixel 9', devices['Pixel 9'], { soloHome: true }], // 360×732
  ['Pixel 5', devices['Pixel 5']], // 393×727
  ['Pixel 7', devices['Pixel 7']], // 412×839
  ['Galaxy A55', devices['Galaxy A55'], { soloHome: true }], // 480×1040 — gama media
  ['LG Optimus L70', devices['LG Optimus L70'], { soloHome: true }], // 384×640 dsf 1,25
  // --- Android: plegables (los dos extremos del catálogo) ---
  ['Galaxy Z Flip 6 Cover', devices['Galaxy Z Flip 6 Cover']], // 360×298 — altura extrema
  ['Galaxy Z Fold 6', devices['Galaxy Z Fold 6'], { soloHome: true }], // 928×1004 abierto
  // --- Android: tablets y apaisado ---
  ['Galaxy Tab S9', devices['Galaxy Tab S9']], // 640×1024
  ['Galaxy Tab S4', devices['Galaxy Tab S4'], { soloHome: true }], // 712×1138
  ['Galaxy S24 landscape', devices['Galaxy S24 landscape']], // 780×360
];

/**
 * VENTANAS DE ESCRITORIO. `SUPERFICIE=escritorio` las usa en vez de los perfiles
 * de móvil; el resto del barrido (sesión, rutas, pestañas, desplegables) es el
 * mismo, que es justo por lo que esto no es un segundo guion.
 *
 * NO SON TAMAÑOS INVENTADOS: salieron del shell de escritorio de Wails (retirado
 * el 2026-09-23), que era lo que dejaba hacer con su ventana. 1280×800 era como
 * abría; 880×600 el MÍNIMO al que se podía encoger, y ahí no se había medido
 * nunca nada. Se siguen midiendo porque una ventana de navegador baja a eso
 * igual. El ancho mínimo cae ya en el lado «escritorio» de los puntos de ruptura
 * —la barra lateral sigue puesta— pero sin sitio para ella y el contenido a la
 * vez, que es el hueco exacto entre lo que cubre el barrido móvil y lo que se
 * despliega.
 *
 * Y 600px de ALTO es la mitad interesante: un diálogo de la suite mide más que
 * eso, y el móvil nunca lo destapa porque sus perfiles son altos y estrechos.
 */
const VENTANA_MINIMA = { width: 880, height: 600 };
const PERFILES_ESCRITORIO = [
  // El suelo: es donde aparece lo que nadie ha visto.
  ['ventana mínima 880×600', { viewport: VENTANA_MINIMA }],
  // Como abre la app recién instalada.
  ['ventana por defecto 1280×800', { viewport: { width: 1280, height: 800 } }],
  // Portátil de 13" a escala 125% (Windows): 1536×864 CSS. El alto realista.
  ['portátil 1536×864', { viewport: { width: 1536, height: 864 }, deviceScaleFactor: 1.25 }],
  // Y el otro extremo: que nada se estire hasta lo ilegible ni deje la columna
  // de contenido flotando en medio de dos metros de vacío.
  ['monitor 2560×1440', { viewport: { width: 2560, height: 1440 }, soloHome: true }],
];

/**
 * `SUPERFICIE=escritorio` cambia QUÉ ventanas se miden. La medida VERTICAL corre
 * en las dos superficies (ver donde se llama): durante un tiempo fue solo de
 * escritorio, con el argumento de que en móvil la página se desplaza y no hay techo.
 */
const ESCRITORIO = process.env.SUPERFICIE === 'escritorio';
const ELEGIDOS = ESCRITORIO
  ? PERFILES_ESCRITORIO.map(([n, p]) => [n, p, p.soloHome ? { soloHome: true } : undefined])
  : PERFILES;

/**
 * Recorre las PESTAÑAS de la vista y mide cada panel.
 *
 * Una lista de rutas no alcanza estas superficies: las pestañas son
 * `role="tab"`, no enlaces, así que no cambian la URL y no hay nada que listar.
 * En la administración de plataforma eso son cinco pantallas enteras —Uso,
 * Fusiones, Admins globales, Aprobaciones, Registro— que nunca se habían
 * medido, y era justo donde estaba el desbordamiento que reportó el usuario.
 *
 * Devuelve al final a la pestaña que estaba activa, para no alterar lo que mida
 * el resto del barrido.
 */
async function conPestanasRecorridas(page) {
  const n = await page.evaluate(() => {
    globalThis.__sytuePestanas = [...document.querySelectorAll('[role="tab"]')].filter((t) => {
      const r = t.getBoundingClientRect();
      return r.width > 8 && r.height > 8;
    });
    globalThis.__sytueActiva = globalThis.__sytuePestanas.findIndex(
      (t) => t.getAttribute('aria-selected') === 'true',
    );
    return globalThis.__sytuePestanas.length;
  });
  const hallazgos = [];
  const izq = [];
  // Tope de 8: cubre los paneles reales de la suite sin convertir cada ruta en
  // una sesión de clics. Si alguna vista tuviera más, se dice en el informe.
  for (let i = 0; i < Math.min(n, 8); i++) {
    await page.evaluate((k) => globalThis.__sytuePestanas[k]?.click(), i);
    await page.waitForTimeout(600); // el panel monta su contenido al activarse
    const r = await page.evaluate(MEDIDOR);
    if (!MARGENES) await page.evaluate(DETECTOR_FUGAS);
    unir(hallazgos, r.hallazgos);
    unir(izq, r.izq);
  }
  if (n > 0) {
    await page.evaluate(() => {
      const i = globalThis.__sytueActiva;
      if (i >= 0) globalThis.__sytuePestanas[i]?.click();
    });
    await page.waitForTimeout(300);
  }
  return { hallazgos, izq, pestanas: n };
}

/**
 * Estados INTERACTIVOS. El barrido solo medía la vista en reposo, y los dos
 * desbordamientos peores de la auditoría de agosto de 2026 vivían en un panel
 * que hay que ABRIR: el centro de notificaciones se salía 86px por la IZQUIERDA
 * en las diez apps, y el barrido lo declaró «ok» dieciocho veces.
 *
 * Abre uno a uno los desplegables visibles, de arriba abajo (el cromo primero),
 * y vuelve a medir. Solo abrir y cerrar con Esc: NO hay mutación —la lista de
 * notificaciones se carga al montar el componente, se abra o no, y «marcar como
 * leída» es otro botón—. Los filtros acotan a disparadores de verdad:
 * `aria-haspopup`/`aria-expanded` es contenido desplegable por definición (nunca
 * una acción), y nada dentro de un `form`, donde un botón puede enviar algo.
 */
async function conDesplegablesAbiertos(page) {
  const n = await page.evaluate(() => {
    globalThis.__sytueDisparadores = [
      ...document.querySelectorAll('button[aria-haspopup],button[aria-expanded]'),
    ]
      .filter((b) => {
        const r = b.getBoundingClientRect();
        return (
          r.width > 8 &&
          r.top >= 0 &&
          r.top < innerHeight &&
          b.getAttribute('aria-expanded') !== 'true' &&
          !b.closest('form')
        );
      })
      .sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top);
    return globalThis.__sytueDisparadores.length;
  });
  const hallazgos = [];
  const izq = [];
  for (let i = 0; i < Math.min(n, 6); i++) {
    await page.evaluate((k) => globalThis.__sytueDisparadores[k]?.click(), i);
    await page.waitForTimeout(400);
    const r = await page.evaluate(MEDIDOR);
    if (!MARGENES) await page.evaluate(DETECTOR_FUGAS);
    unir(hallazgos, r.hallazgos);
    unir(izq, r.izq);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(150);
  }
  return { hallazgos, izq };
}

/**
 * Lo que se abre en un DIÁLOGO desde el contenido y no tiene ruta propia: el visor
 * de Sytue Fotos, que es donde se pasa el rato mirando fotos y solo se abre tocando
 * una. Desbordaba 520px a 320 —«Cerrar» fuera de la pantalla— con este barrido en
 * verde (2026-09-15), porque aquí solo se abrían desplegables. Se abre el primer
 * disparador del `main` que declare `aria-haspopup="dialog"`, se mide y se cierra.
 */
async function conDialogoAbierto(page) {
  const disparador = page.locator('main [aria-haspopup="dialog"]').first();
  if (!(await disparador.count())) return { hallazgos: [], izq: [] };
  await disparador.click({ timeout: 5_000 }).catch(() => {});
  await page.waitForTimeout(1_500);
  const r = await page.evaluate(MEDIDOR);
  if (!MARGENES) await page.evaluate(DETECTOR_FUGAS);
  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  return r;
}

const pedidas = process.argv.slice(2);
const objetivo = Object.entries(APPS).filter(([a]) => !pedidas.length || pedidas.includes(a));

/* ---------------------------------------------------------------------------
 * COBERTURA: que la lista de arriba no se quede corta ella sola.
 *
 * `APPS` se mantenía a mano y cubría 20 de las 66 páginas `.astro` de la suite,
 * así que una pantalla nueva era invisible para esta puerta hasta que alguien se
 * acordara de editar la constante. No es hipotético: el peor desbordamiento de
 * la suite vivía en `/pdf` y una pantalla que devolvía un 200 en blanco en
 * `rooms/share`, y ninguna de las dos estaba listada — el barrido salía en verde
 * con las dos rotas debajo.
 *
 * Esto enumera el árbol de páginas y falla si aparece una que ni se visita ni
 * está declarada aquí abajo CON SU MOTIVO. `api/` y `auth/` quedan fuera: no son
 * pantallas.
 */
const DIR_A_APP = {
  portal: ['portal'],
  mail: ['mail'],
  drive: ['drive'],
  calendar: ['calendar'],
  contacts: ['contacts'],
  docus: ['docus'],
  calcus: ['calcus'],
  diapos: ['diapos'],
  certipdf: ['certipdf'],
  rooms: ['rooms'],
  'cloud-dashboard': ['cloud'],
  // Una sola app de Astro sirve los DOS hosts de administración.
  console: ['admin', 'platform'],
  www: ['www'],
  // Fotos y Notes no estaban, así que sus páginas nuevas nunca salían SIN CUBRIR:
  // el editor, las parecidas, las cercanas y el álbum de Fotos no los visitaba
  // nadie y el barrido seguía en verde (2026-09-15).
  photos: ['photos'],
  notes: ['notes'],
  erp: ['erp'],
  bases: ['bases'],
};

/** Página que hoy no se puede visitar, y por qué. Sin motivo escrito no entra. */
const NO_VISITABLES = [
  [
    /^(docus\/dev\/(cinta|superficies|editor|inicio|pdf|superposiciones)|bases\/dev\/(app|vistas|rejilla)|calendar\/dev\/servicios-vinculados|drive\/dev\/almacenamiento|mail\/dev\/correo-externo|rooms\/dev\/(conexiones|avisos-directos|idioma-sistema)|console\/dev\/conectar-app)$/,
    'bancos de prueba exclusivos de desarrollo: import.meta.env.DEV exige 404 en producción',
  ],
  [/\[/, 'ruta dinámica: depende de un dato, se alcanza con { desde, enlace } o { resolver }'],
  [
    /^console\//,
    'consola de administración: la cuenta de auditoría es un employee y estas pantallas ' +
      'devuelven 403 (hace falta una credencial con rol de admin de LECTURA)',
  ],
  [
    /^docus\/wopi\/editor$/,
    'editor WOPI: necesita una sesión de edición sobre un documento existente',
  ],
  [
    /^(calcus|diapos)\/edit$/,
    'el editor de hojas y presentaciones es el de Docus (@sytue/office, que se mide allí); ' +
      'la cuenta de auditoría solo tiene sembrado un documento',
  ],
  [/^www\/404$/, '404: no es una pantalla a la que se navegue'],
  [
    /^rooms\/pip$/,
    'ventana emergente de 420x320 de una llamada en curso (la abre window.open desde ' +
      'VideoRoom): sin canal redirige a la app y con canal ENTRARÍA en la sala',
  ],
  [
    /^cloud-dashboard\/confirm-alert$/,
    'confirmación de un destino mailto de las alertas: se llega por el enlace de un solo ' +
      'uso que recibe esa dirección (equivalente a auth/*); sin token solo pinta «no válido»',
  ],
  [
    /^rooms\/matrix\/(callback|external\/callback|native\/(callback|start))$/,
    'retorno e inicio del flujo SSO/OAuth de Matrix: completan el intercambio de sesión ' +
      'con parámetros de un solo uso y redirigen (equivalentes a auth/*)',
  ],
];

const RAIZ_APPS = fileURLToPath(new URL('../../apps/frontend/apps', import.meta.url));

/** Rutas declaradas para una app, sean literales o entradas por descubrir. */
const declaradas = (app) => (APPS[app] ?? []).map((e) => (typeof e === 'string' ? e : e.enlace));

function paginasSinCubrir(claves) {
  const faltan = [];
  for (const [dir, apps] of Object.entries(DIR_A_APP)) {
    if (!apps.some((a) => claves.includes(a))) continue;
    const base = `${RAIZ_APPS}/${dir}/src/pages`;
    if (!existsSync(base)) {
      // Que no se convierta en una comprobación muda si el árbol se mueve.
      faltan.push([dir, '(no existe)', 'no está el árbol de páginas donde se busca']);
      continue;
    }
    for (const f of readdirSync(base, { recursive: true })) {
      const p = String(f).replaceAll('\\', '/');
      if (!p.endsWith('.astro') || p.startsWith('api/') || p.startsWith('auth/')) continue;
      const id = `${dir}/${p.replace(/\.astro$/, '')}`;
      if (NO_VISITABLES.some(([re]) => re.test(id))) continue;
      const ruta =
        `/${p
          .replace(/\.astro$/, '')
          .replace(/^\[\.\.\.[^\]]+\]\/?/, '') // `[...locale]` es opcional en la URL
          .replace(/(^|\/)index$/, '')}`.replace(/\/$/, '') || '/';
      const cubierta = apps.some((a) =>
        declaradas(a).some(
          (d) => d === ruta || d.startsWith(`${ruta}?`) || (ruta !== '/' && d.endsWith(ruta)),
        ),
      );
      if (!cubierta) faltan.push([dir, ruta, `${p} no la visita nadie`]);
    }
  }
  return faltan;
}

const sinCubrir = paginasSinCubrir(objetivo.map(([a]) => a));
for (const [dir, ruta, motivo] of sinCubrir)
  console.log(`SIN CUBRIR ${dir.padEnd(16)} ${String(ruta).padEnd(28)} ${motivo}`);
if (sinCubrir.length)
  console.log(
    `${sinCubrir.length} páginas fuera de APPS: añádelas o declara el motivo en NO_VISITABLES.\n`,
  );

// Salidas SEPARADAS por superficie: los dos barridos corren en el mismo runner
// y con un solo destino el segundo borraría las capturas y el informe del
// primero — y el informe borrado es siempre el que hacía falta leer.
const SALIDA = createReportDirectory(ESCRITORIO ? 'escritorio' : 'mobile');
const INFORME = `${SALIDA}/informe.json`;
mkdirSync(SALIDA, { recursive: true });
const browser = await chromium.launch();
const informe = [];

// PERFILES="Moto G4,Galaxy S9+" acota el barrido a esos perfiles: para iterar
// sobre un arreglo concreto sin esperar a los dieciocho.
const PEDIDOS = (process.env.PERFILES || '')
  .split(',')
  .map((s) => s.trim())
  .filter(Boolean);

for (const [nombre, perfil, opts] of ELEGIDOS) {
  if (PEDIDOS.length && !PEDIDOS.includes(nombre)) continue;
  const ctx = await browser.newContext({
    ...perfil,
    // La primera vez contra un entorno no hay estado guardado: se arranca sin
    // sesión y `asegurarSesion` entra con las credenciales. Pasarle una ruta que
    // no existe hace que Playwright reviente antes de abrir el navegador.
    ...(existsSync(STORAGE) ? { storageState: STORAGE } : {}),
    ignoreHTTPSErrors: true,
    locale: LOCALE,
    extraHTTPHeaders: {
      'Accept-Language': `${LOCALE},${LOCALE.split('-')[0]};q=0.9`,
    },
  });
  const page = await ctx.newPage();
  // Qué productos levanta esta instalación (ver apps-desplegadas.mjs): lo que
  // el Hub diga que no está, se anota como SALTO CONOCIDO (opcional) y no como
  // fallo. Sin respuesta del Hub se mide todo, como siempre.
  //
  // SOLO SI YA HAY SESIÓN GUARDADA, igual que en el barrido de escritorio: sin
  // ella las dos llamadas contestan 401 y esto caía a «medir todo» antes de que
  // nadie hubiera entrado, que en un CI limpio es el caso NORMAL. El reintento
  // de abajo la consigue en cuanto el login ocurre dentro del bucle.
  let desplegadas = existsSync(STORAGE) ? await appsDesplegadas(page, DOMINIO) : null;
  for (const [app, todas] of objetivo) {
    const rutas = opts?.soloHome ? todas.slice(0, 1) : todas;
    const host = HOSTS[app] ?? `${app}.${DOMINIO}`;
    if (desplegadas === null) desplegadas = await appsDesplegadas(page, DOMINIO);
    if (!desplegada(app, desplegadas)) {
      console.log(
        `  ·       ${nombre.padEnd(20)} ${app}: no desplegada en esta instalación (según el Hub)`,
      );
      for (const entrada of rutas) {
        informe.push({
          app,
          ruta: typeof entrada === 'string' ? entrada : entrada.enlace,
          perfil: nombre,
          salto: 'no desplegada en esta instalación (según el Hub)',
          opcional: true,
        });
      }
      continue;
    }
    for (const entrada of rutas) {
      let ruta = entrada;
      try {
        if (typeof entrada === 'object' && entrada.resolver) {
          // La ruta la compone la propia entrada. Es la salida para lo que no se
          // alcanza siguiendo un enlace: Sytue PDF se abre con `window.open`, así
          // que no hay `<a href>` que mirar por más que se navegue.
          ruta = await entrada.resolver(page);
        } else if (typeof entrada === 'object') {
          await navegar(page, urlDe(app, host, entrada.desde));
          await page.waitForTimeout(3500);
          await asegurarSesion(page);
          const href = await page
            .locator(`a[href*="${entrada.enlace}"]`)
            .first()
            .getAttribute('href', { timeout: 10_000 });
          if (!href) throw new Error(`sin datos: ningún enlace a ${entrada.enlace}`);
          // El enlace YA lleva el prefijo (`/docus/<org>/edit?id=…`, ADR 0056) y
          // `urlDe` lo pone otra vez: sin quitarlo se pedía `/docus/docus/…`, un
          // 404. Se resuelve contra la página, así que vale igual con o sin
          // organización en la ruta y con enlaces relativos.
          const abs = new URL(href, page.url());
          const base = BASES[app] || '';
          ruta =
            (base && abs.pathname.startsWith(`${base}/`)
              ? abs.pathname.slice(base.length)
              : abs.pathname) + abs.search;
        }
      } catch (e) {
        console.log(
          `  SALTO   ${nombre.padEnd(20)} ${host}${entrada.enlace} ${String(e).split('\n')[0].slice(0, 60)}`,
        );
        // El salto TAMBIÉN va al informe: sin esto no entraba en el total, no
        // salía en el recuento y no afectaba al código de salida, así que las
        // tres vistas más profundas de la suite (editor de Docs, canal de Rooms,
        // proyecto de Cloud) llevaban 36 visitas sin auditarse y el resumen
        // decía «de 360 probadas» como si estuvieran dentro.
        informe.push({
          app,
          ruta: entrada.enlace,
          perfil: nombre,
          salto: String(e).split('\n')[0],
          opcional: entrada.opcional === true,
        });
        continue;
      }
      const url = urlDe(app, host, ruta);
      try {
        let resp = await navegar(page, url);
        await page.waitForTimeout(3500); // hidratación de las islas React
        if (await asegurarSesion(page, url)) {
          // Se guarda para los 17 contextos siguientes: sin esto cada perfil
          // repetiría el login y el IdP corta por ráfaga (5/min por cuenta),
          // así que el barrido moriría a mitad con la sesión «caducada».
          await ctx.storageState({ path: STORAGE });
          resp = await navegar(page, url);
          await page.waitForTimeout(3500);
        }
        // Sin interfaz no hay medida. Un 403 de la consola de administración
        // devuelve «No autorizado para este recurso.» —un párrafo suelto, UN
        // nodo— y un 404 devuelve cuatro líneas: sobre eso el detector no
        // desborda jamás y la combinación salía «ok». Eran 30 de las 132 del
        // barrido de agosto de 2026, el 23%, aprobadas sin haber pintado nada.
        // El estado HTTP es señal exacta y no una heurística sobre el texto: no
        // depende del idioma ni de cómo esté redactado el mensaje.
        const http = resp?.status() ?? 0;
        if (http >= 400) {
          console.log(
            `  SIN UI  ${nombre.padEnd(20)} ${url.padEnd(46)} HTTP ${http}: no hay interfaz que medir`,
          );
          informe.push({
            app,
            ruta,
            perfil: nombre,
            // EL MOTIVO SEGÚN EL ESTADO, y no uno solo para todos. Decir «la
            // cuenta de auditoría no llega a esta pantalla» ante un 404 es una
            // suposición, y manda a mirar los permisos cuando lo que pasa es que
            // la ruta no existe. Costó tiempo con `/one/bienvenida`, que era un
            // 404 de verdad —una página del repositorio a la que se llegaba por
            // un `returnTo` sin prefijo— y el informe lo describía como un hueco
            // de la cuenta.
            salto: `HTTP ${http}: ${
              http === 404
                ? 'esa ruta no existe en este despliegue'
                : 'la cuenta de auditoría no llega a esta pantalla'
            }`,
          });
          continue;
        }
        // Y tampoco hay medida si lo que hay delante NO es lo que se pidió. El
        // IdP responde a `/account` sin sesión con un 302 a `/login/?return_to=…`
        // que Playwright sigue solo, así que el estado final es 200 y la medida
        // de la pantalla de LOGIN se anotaría como si fuera la de `/account`: un
        // «ok» de una pantalla que no se ha visto, justo lo que esta puerta
        // existe para no hacer.
        //
        // Solo para `id`, y no como comprobación general, porque el resto de la
        // suite cambia la URL a propósito: `rooms/` aterriza en `/channels/<id>`
        // por pushState y ahí el desvío ES la pantalla correcta.
        // Y NO HAY MEDIDA SI LO QUE HAY DELANTE ES OTRA APLICACIÓN.
        //
        // `*.sytue.eu` es comodín y el ingress tiene un catch-all: cualquier host
        // que no esté enrutado —`git.sytue.eu`, `cloud.sytue.eu`, y también
        // `foo-bar-9z.sytue.eu`— acaba en `/calendar/auth/login`. Comprobado
        // contra producción. Así que el barrido pedía `git.sytue.eu/`, recibía un
        // 200 de Sytue Calendar y lo anotaba `ok`: una app entera aprobada sin
        // existir, y el informe lo delataba solo si alguien miraba el `titulo`,
        // que decía «Sytue Calendar».
        //
        // Se compara el PREFIJO DE APP y no la ruta entera, que es lo que impedía
        // generalizar la comprobación de `id` que hay debajo: la suite cambia la
        // URL a propósito —`rooms/` aterriza en `/rooms/channels/<id>`— y eso
        // sigue siendo la misma app. Solo salta cuando el prefijo pertenece a OTRA.
        const prefijo = new URL(page.url()).pathname.match(/^\/[a-z-]+/)?.[0] ?? '';
        const duenyo = Object.entries(BASES).find(([, b]) => b === prefijo)?.[0];
        if (duenyo && duenyo !== app) {
          console.log(
            `  SALTO   ${nombre.padEnd(20)} ${url.padEnd(46)} sirve ${duenyo}, no ${app}: el host no está enrutado`,
          );
          informe.push({
            app,
            ruta,
            perfil: nombre,
            salto: `el host sirve «${duenyo}» y no «${app}»: no existe esa superficie`,
          });
          continue;
        }
        if (app === 'id') {
          const aqui = new URL(page.url()).pathname.replace(/\/$/, '');
          if (aqui !== ruta.replace(/\/$/, '')) {
            console.log(
              `  SALTO   ${nombre.padEnd(20)} ${url.padEnd(46)} el IdP desvió a ${aqui}: no hay sesión para esta pantalla`,
            );
            informe.push({
              app,
              ruta,
              perfil: nombre,
              salto: `desvío a ${aqui}: la pantalla pedida no llegó a mostrarse`,
            });
            continue;
          }
        }
        // SIN DATOS tampoco es una medida. Cloud responde 200 en sus OCHO rutas
        // con el cromo entero y un aviso de «no tienes permiso para ver estos
        // datos» donde deberían ir los datos: 120 de las 299 combinaciones del
        // barrido eran eso —el 40%— y salían «ok», o sea aprobadas sin haber
        // pintado un dato. El estado HTTP no lo ve (es un 200) y mirar el TEXTO
        // depende del idioma y de la redacción, así que la señal la pone el
        // componente que lo dice: `data-sin-datos`.
        //
        // Cuenta como OPCIONAL y no rompe: el motivo es conocido —la cuenta de
        // auditoría no pertenece a ninguna organización de Cloud— y una puerta
        // permanentemente roja por algo sabido deja de leerse. Deja de mentir el
        // total, que es lo que se venía a arreglar; con una credencial de LECTURA
        // en una organización de Cloud estas 102 combinaciones se miden de verdad.
        //
        // Y se distingue el MOTIVO: «permiso» es el hueco conocido, pero un fallo
        // del control-plane pinta el mismo aviso, y eso no puede colarse como
        // hueco conocido — sería una caída de Cloud convertida en silencio.
        const sinDatos = await page.evaluate(
          () => document.querySelector('[data-sin-datos]')?.getAttribute('data-sin-datos') ?? null,
        );
        if (sinDatos) {
          const porPermiso = sinDatos === 'permiso';
          console.log(
            `SIN DATOS ${nombre.padEnd(20)} ${url.padEnd(46)} ` +
              (porPermiso
                ? 'la cuenta no puede ver los datos de esta pantalla'
                : 'la pantalla NO pudo cargar sus datos (fallo, no permiso)'),
          );
          informe.push({
            app,
            ruta,
            perfil: nombre,
            salto: porPermiso
              ? 'sin datos: la pantalla avisa de que la cuenta no puede verlos'
              : 'sin datos: la pantalla avisa de que no pudo cargarlos',
            opcional: porPermiso,
          });
          continue;
        }
        const r = await page.evaluate(MEDIDOR);
        const abiertos = await conDesplegablesAbiertos(page);
        unir(r.hallazgos, abiertos.hallazgos);
        unir(r.izq, abiertos.izq);
        const pestanas = await conPestanasRecorridas(page);
        unir(r.hallazgos, pestanas.hallazgos);
        unir(r.izq, pestanas.izq);
        const dialogo = await conDialogoAbierto(page);
        unir(r.hallazgos, dialogo.hallazgos);
        unir(r.izq, dialogo.izq);
        // Lo vertical: algo pulsable por debajo de la ventana a lo que no se llega
        // desplazando nada. Se mide con los desplegables ya recorridos por si acaso,
        // pero en reposo basta —lo que se busca es un panel más alto que la ventana,
        // no un menú abierto—.
        //
        // TAMBIÉN EN MÓVIL. Era solo de escritorio porque «en móvil la página se
        // desplaza y no hay techo», y eso deja de ser verdad en cuanto algo recorta:
        // en la v3.11.1 la hoja de la forma «Marco» recortaba la portada de Docs, y en
        // un iPhone 12 no se llegaba a la lista de documentos —«Recientes» empezaba en
        // y=950 con el documento acabado en 664—. Escritorio lo cantó (−170 px a
        // 1280×800); el barrido de móvil, que es donde no se veía NI UN documento,
        // salió en verde. No en la pasada de márgenes, que juzga otra cosa.
        const v = MARGENES ? null : await page.evaluate(DETECTOR_VERTICAL);
        // El documento contra su armazón, en las dos superficies: en una pantalla de
        // alto fijo no desplaza, y en una de página el marco llega a su final. No en
        // la pasada de márgenes, que juzga otra cosa.
        const doc = MARGENES ? null : await page.evaluate(DETECTOR_DOCUMENTO);
        // Y la condición de la que sale, que no espera a que haya filas bastantes.
        const fugas = MARGENES ? [] : await page.evaluate(DETECTOR_FUGAS);
        const grave =
          r.hallazgos.length > 0 ||
          r.izq.length > 0 ||
          (v?.fuera.length ?? 0) > 0 ||
          Boolean(doc?.falla) ||
          fugas.length > 0;
        const slug = `${app}${ruta.replace(/\W+/g, '-')}--${nombre.replace(/\s+/g, '-')}`;
        // SHOTS=1 captura también lo que pasa: sin desbordamiento la vista puede
        // seguir siendo mala (todo apilado, cabecera de tres pisos), y eso no lo
        // dice ninguna medida.
        if (grave || process.env.SHOTS)
          await page.screenshot({
            path: `${SALIDA}/${slug}.png`,
            fullPage: false,
          });
        informe.push({
          app,
          ruta,
          perfil: nombre,
          ...r,
          vertical: v,
          documento: doc,
          fugas,
          grave,
        });
        console.log(
          `${grave ? (MARGENES ? 'APRETADO' : 'DESBORDA') : '   ok   '} ${nombre.padEnd(24)} ${url.padEnd(46)}` +
            ` ${LINEA(r)}${v ? lineaVertical(v) : ''}${lineaDocumento(doc)}${lineaFugas(fugas)}`,
        );
      } catch (e) {
        console.log(
          `  FALLO  ${nombre.padEnd(20)} ${url} ${String(e).split('\n')[0].slice(0, 80)}`,
        );
        informe.push({
          app,
          ruta,
          perfil: nombre,
          fallo: String(e).split('\n')[0],
        });
      }
    }
  }
  await ctx.close();
}
await browser.close();
writeReport(INFORME, JSON.stringify(informe, null, 2));
const graves = informe.filter((r) => r.grave);
const caidas = informe.filter((r) => r.fallo);
const saltadas = informe.filter((r) => r.salto && !r.opcional);
const conocidas = informe.filter((r) => r.salto && r.opcional);
const medidas = informe.filter((r) => r.grave !== undefined);
const izquierda = graves.filter((r) => r.izq?.length);
console.log(
  `\n${graves.length} combinaciones desbordadas de ${medidas.length} MEDIDAS` +
    ` (${informe.length} visitadas)` +
    `${izquierda.length ? ` (${izquierda.length} por la IZQUIERDA: recortado e inalcanzable)` : ''}.`,
);
if (caidas.length)
  console.log(
    `${caidas.length} no se pudieron medir (app caída o lenta): NO cuentan como aprobadas.`,
  );
if (saltadas.length)
  console.log(
    `${saltadas.length} rutas no se pudieron medir (sin interfaz o sin resolver): ` +
      `TAMPOCO cuentan como aprobadas.`,
  );
if (conocidas.length)
  console.log(
    `${conocidas.length} visitas SIN MEDIR por un motivo conocido (la cuenta de auditoría no ` +
      `tiene documentos, ni proyecto, ni permiso sobre los datos de Cloud): no cuentan como ` +
      `aprobadas, y dejan de contar como medidas.`,
  );
if (sinCubrir.length)
  console.log(
    `${sinCubrir.length} páginas de la suite NO las visita esta puerta (ver «SIN CUBRIR» arriba).`,
  );
// Código de salida, para poder usarlo como PUERTA y no solo como informe: un
// script que siempre sale con 0 no impide que la regresión entre.
//
// Cuenta lo NO medido, no solo lo desbordado. Antes solo miraba `graves`, así
// que un barrido contra un dominio inexistente —donde no se cargó una sola
// página— salía con 0 y ponía la puerta nocturna en verde. Lo único que no
// rompe es la ruta marcada `opcional` (ver APPS), que ya se dice a gritos aquí
// arriba y en cada línea SALTO.
//
// Y una pantalla que esta lista no visita cuenta igual que una que desborda: el
// agujero no se ve en ninguna medida, solo en que no la hay.
process.exit(graves.length || caidas.length || saltadas.length || sinCubrir.length ? 1 : 0);
