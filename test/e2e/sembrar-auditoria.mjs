// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Deja a la cuenta de auditoría con el contenido MÍNIMO que el barrido necesita
// para alcanzar las pantallas donde vive el trabajo real.
//
// EL PUNTO CIEGO QUE CIERRA. El barrido móvil recorre las páginas de aterrizaje,
// pero las rutas que dependen de un dato —`docus.sytue.eu/docus/edit?id=…`, el canal, el
// fichero— no se podían resolver porque la cuenta no tenía nada. Se contaban como
// «saltadas», que NO es aprobado, y eran justo las pantallas del editor: donde se
// pasa el día quien usa la suite, y las últimas que se han medido nunca.
//
// ESCRIBE EN PRODUCCIÓN, y eso está autorizado expresamente para ESTA cuenta
// (`ops@sytue.com`) hasta el 2026-11-13. Fuera de esa autorización, este guion no
// se ejecuta: no es una prueba de solo lectura y no debe parecerlo.
//
// ES IDEMPOTENTE, que es la parte que importa. Si el documento ya existe no crea
// otro: un sembrado que acumula deja el buzón de pruebas con cuatrocientos
// documentos «Auditoría» en un mes, y entonces alguien lo desactiva.
//
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… bun run sembrar-auditoria.mjs
//
// En la nube: `mobile.yml` → `workflow_dispatch` con
// `reproductor=sembrar-auditoria.mjs`. Desde el portátil NO (ver la regla de la
// casa: lo que decide es dónde corre el navegador).
import { chromium } from 'playwright';
import { activarOrganizacion, codigoTOTP, porQueNoEntra } from './tests/helpers.ts';

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
// El `base` de Astro con el que se sirve Sytue Docus (el documento sembrado es de
// texto) y el de CertiPDF (ADR 0076). Sin él, la raíz del host es donde el ingress no
// enruta nada y el sembrado no llegaba ni a la portada: pedía el 404 de Traefik y
// esperaba una sesión que nunca iba a aparecer.
const BASE_DOCS = '/docus';
const BASE_PDF = '/certipdf';
const BASE_NOTES = '/notes';
const IDP = `id.${DOMINIO}`;
/**
 * El host por el que se entra, y NO es el subdominio de cada aplicación.
 *
 * El BFF compone su `redirect_uri` en `<organizacion>.<dominio>` —el client
 * `sytue-docus` lo tiene registrado como `corporate.sytue.eu/docus/auth/callback`—
 * así que entrar por `docs.<dominio>` abre un flujo que EMPIEZA en un host y
 * TERMINA en otro. El propio BFF lo documenta: la cookie de estado lleva el
 * prefijo `__Host-`, es host-only, y en el callback no existe; entonces «solo se
 * reinicia el login». Ese reinicio es lo que dejaba al sembrado dando vueltas
 * después de un segundo factor que el IdP había aceptado sin problema.
 *
 * Entrando por el host de organización el flujo es de un solo host de principio
 * a fin, que es como lo vive quien usa la suite: `corporate.sytue.eu/docus`.
 */
const HOST_ENTRADA = `${process.env.SYTUE_ORG_HOST || 'corporate'}.${DOMINIO}`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || '',
  pass: process.env.SYTUE_AUDIT_PASS || '',
  totp: process.env.SYTUE_AUDIT_TOTP || '',
};
if (!CRED.email || !CRED.pass) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.');
  process.exit(2);
}

/**
 * Títulos FIJOS y reconocibles. Que se llamen siempre igual es lo que hace el
 * sembrado idempotente y, además, lo que permite a una persona ver de un vistazo
 * que ese contenido es de la auditoría y no de alguien.
 */
const TITULO_DOC = 'Auditoría · no borrar';
/**
 * EL TÍTULO NO SALE COMO SE PIDE. El documento nace con su `.odt` en la unidad y
 * Drive desambigua el nombre si ya existe: pedir «Auditoría · no borrar» con un
 * fichero de ese nombre en `Sytue Docs/` da «Auditoría · no borrar (2)». Y basta
 * con que el original se borre en Docs —su `.odt` se queda en la unidad— para que
 * el título exacto no vuelva a existir: buscándolo tal cual, cada noche se creaba
 * otro (el 2026-09-28 había 44 en Drive). Por eso se reconoce también el sufijo.
 */
const ES_DE_AUDITORIA = (titulo, base) =>
  titulo.startsWith(base) && /^( \(\d+\))?$/.test(titulo.slice(base.length));
// Mismo título fijo y misma idempotencia para lo demás que hay que sembrar: sin
// esto el barrido acumularía un cuaderno y un PDF por noche hasta que alguien lo
// apagara, que es lo que ya se razonó para el documento.
const TITULO_CUADERNO = 'Auditoría · no borrar';
const NOMBRE_PDF = 'Auditoria-no-borrar.pdf';
const TITULO_ALBUM = 'Auditoría · no borrar';

const browser = await chromium.launch();
const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();

// EL LIMITADOR DEL IdP HABLA POR CABECERA, no por la pantalla. `loginRL` permite
// 10 accesos por minuto y, al agotarse, contesta 429 con `Retry-After`. Desde el
// navegador eso se ve igual que un código rechazado —vuelves al formulario—, así
// que sin mirar el status se acusa a la credencial de lo que hace el bucket. Y
// pasó: un día entero persiguiendo una semilla que era correcta.
let ultimo429 = 0;

/**
 * Bitácora de lo que el SERVIDOR NO PUEDE VER.
 *
 * El 2026-09-07 el sembrado falló durante horas «tras enviar el segundo factor»
 * y la auditoría del IdP demostró que el servidor hacía su trabajo: cada código
 * enviado daba `auth.mfa.success` y abría una sesión con amr=[pwd,mfa], y
 * `/oauth2/authorize` no desvió a step-up ni una sola vez —los 36 retos del día
 * llevaban el `target` vacío, o sea que salían del paso de la CONTRASEÑA, no de
 * authorize—. Aun así el navegador se quedaba en la pantalla del código.
 *
 * Con el servidor descartado, lo que queda está entre el 302 y la página
 * siguiente: que la redirección no se siga, que se siga y vuelva al login, o que
 * la cookie de sesión no se guarde. Ninguna de las tres deja rastro en el
 * servidor, así que se registran aquí y solo se vuelcan si algo falla.
 */
const redirecciones = [];
page.on('response', (res) => {
  const st = res.status();
  const url = new URL(res.url());
  if (st === 429 && /\/login\//.test(url.pathname)) {
    ultimo429 = Number(res.headers()['retry-after'] ?? 0) || 30;
  }
  // Solo el IdP y solo lo que redirige o falla: el resto es ruido (imágenes,
  // CSS) y enterraría lo que importa.
  // TODO documento, de CUALQUIER host de la suite. Antes solo se miraban los 3xx
  // del IdP, y el reinicio del login ocurre en el host de organización: la cadena
  // se cortaba justo donde empezaba lo interesante.
  if (!url.hostname.endsWith(DOMINIO)) return;
  if (res.request().resourceType() !== 'document') return;
  redirecciones.push(`${st} ${res.request().method()} ${url.hostname}${url.pathname}${url.search.slice(0, 50)}` +
    (res.headers()['location'] ? ` → ${res.headers()['location'].slice(0, 80)}` : ''));
});

// PETICIONES QUE NI LLEGAN A HACERSE. El 302 del segundo factor apunta a
// /oauth2/authorize y el navegador no lo pide: no hay respuesta que registrar, así
// que sin esto la cadena simplemente se corta y no dice por qué. `requestfailed`
// es el único sitio donde eso deja rastro.
const fallidas = [];
page.on('requestfailed', (req) => {
  if (!req.url().includes(DOMINIO)) return;
  fallidas.push(`${req.method()} ${req.url().slice(0, 100)} — ${req.failure()?.errorText ?? '?'}`);
});

/** Lo observado, para el mensaje de error. Sin valores de cookie: son sesiones. */
async function rastro() {
  const cookies = await page.context().cookies().catch(() => []);
  const nombres = cookies
    .filter((c) => c.domain.includes(DOMINIO))
    .map((c) => `${c.name}@${c.domain}${c.path === '/' ? '' : c.path}`)
    .join(', ');
  return (
    `  cadena:      ${redirecciones.slice(-14).join('\n               ') || '(ninguna)'}\n` +
    `  cookies:     ${nombres || '(ninguna del dominio)'}\n` +
    `  fallidas:    ${fallidas.slice(-5).join('\n               ') || '(ninguna)'}`
  );
}

async function entrar() {
  await page.goto(`https://${HOST_ENTRADA}${BASE_DOCS}`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);
  if (new URL(page.url()).hostname !== IDP) return;
  await page.fill('input[name=email]', CRED.email);
  await page.fill('input[name=password]', CRED.pass);
  await page.click('button[type=submit]');
  // EL SEGUNDO FACTOR SE DETECTA POR LA URL, no solo esperando a que aparezca un
  // campo. El IdP contesta al formulario con un 302 a `/login/?…&step_up=1`, y
  // esperar 5 s a que se pinte el input era una carrera que se perdía en cuanto
  // el IdP tardaba un poco: el guion se saltaba el relleno sin decir nada y
  // moría 60 s después con «el login no sale del IdP», enseñando justo la
  // pantalla que no había rellenado. Con `step_up=1` sabemos que HAY que
  // rellenar, así que esperar de más es correcto y no responder es un fallo.
  const pideSegundoFactor = () => /[?&]step_up=1/.test(page.url());
  await page.waitForURL(pideSegundoFactor, { timeout: 15_000 }).catch(() => {});

  const totp = page.locator('input[name=code], input[name=totp], input[autocomplete="one-time-code"]');
  const hayCampo = await totp
    .first()
    .waitFor({ timeout: pideSegundoFactor() ? 30_000 : 5_000 })
    .then(() => true, () => false);

  if (pideSegundoFactor() && !hayCampo) {
    throw new Error('el IdP pide segundo factor y su formulario no llegó a aparecer');
  }
  if (hayCampo) {
    if (!CRED.totp) throw new Error('la cuenta pide segundo factor y no hay TOTP');
    // Se deja constancia del PASO con el que se calcula, no del código. Es lo
    // único que hace falta para saber si un rechazo viene del reloj —comparar el
    // paso con el de otra máquina— y no expone nada: el paso es público, sale de
    // dividir la hora entre 30.
    const paso = Math.floor(Date.now() / 1000 / 30);
    console.log(`· TOTP: semilla de ${CRED.totp.length} caracteres, paso ${paso}`);
    await totp.first().fill(codigoTOTP(CRED.totp));
    await page.click('button[type=submit]');
    // Se ESPERA a que la URL deje de pedir el segundo factor, no un tiempo fijo.
    // Con un `waitForTimeout` corto esto era otra carrera: un código CORRECTO
    // deja la URL en `step_up=1` hasta que el IdP completa el redirect al flujo
    // OAuth, así que 1,5 s de espera declaraban rechazado lo que solo iba lento.
    // Si de verdad se rechaza, el IdP se queda ahí y el plazo se agota.
    const salio = await page
      .waitForURL((u) => !/[?&]step_up=1/.test(u.href), { timeout: 20_000 })
      .then(() => true, () => false);
    if (!salio) {
      // NO SE DEPENDE DE QUE EL NAVEGADOR SIGA ESE 302. Medido el 2026-09-07: el
      // IdP acepta el código, crea la sesión elevada y contesta 302 a
      // /oauth2/authorize, y el navegador no llega a pedirlo —la cadena registra
      // el POST y ninguna navegación detrás—. La sesión ya está abierta y su
      // cookie guardada, así que el destino se alcanza yendo a él directamente.
      //
      // Es un rodeo consciente, no un arreglo: lo que falla sigue fallando y por
      // eso se registra igual más abajo si esto tampoco sale. Pero un barrido de
      // interfaz no puede quedarse bloqueado por el último salto de un login que
      // el servidor ya dio por bueno.
      const destino = new URL(page.url()).searchParams.get('return_to');
      if (destino) {
        await page.goto(new URL(destino, `https://${IDP}`).href, {
          waitUntil: 'domcontentloaded',
          timeout: 45_000,
        }).catch(() => {});
        await page.waitForTimeout(2000);
      }
    }
    if (!salio && /[?&]step_up=1/.test(page.url())) {
      if (ultimo429 > 0) {
        throw new Error(
          `el IdP está limitando el acceso (429; 10/min). Reintentar en ${ultimo429} s. ` +
            'NO es la credencial: el bucket se agota con varios barridos seguidos.',
        );
      }
      // NO se nombra una causa que no se ha medido: esto lo produce cualquier
      // fallo que no sea un 429, y acusar a la semilla mandó dos veces a rotar
      // una credencial válida mientras el fallo estaba en /oauth2/authorize. Un
      // código rechazado sería 401 (`loginFail` escribe StatusUnauthorized), no
      // un 302. Se vuelca lo observado y se deja la discriminación al log del IdP.
      // La URL ENTERA, no 140 caracteres: el `step_up=1` va detrás de un
      // `return_to` largo, así que el corte escondía justo el dato que decía si
      // el IdP seguía pidiendo el segundo factor. Se perdió una tarde por esto.
      const donde = page.url();
      const texto = ((await page.locator('body').innerText().catch(() => '')) || '')
        .replace(/\s+/g, ' ')
        .slice(0, 160);
      throw new Error(
        'no se completó el acceso tras enviar el segundo factor.\n' +
          `  se quedó en: ${donde}\n` +
          `  pantalla:    «${texto}»\n` +
          `${await rastro()}\n` +
          '  LAS DOS CAUSAS OBVIAS ESTÁN DESCARTADAS con datos del 2026-09-07: la ' +
          'semilla es correcta (cada envío dio auth.mfa.success y abrió sesión con ' +
          'amr=[pwd,mfa]) y /oauth2/authorize NO desvió a step-up (los 36 retos del ' +
          'día llevan target vacío, o sea que salen del paso de la contraseña). ' +
          'Mira la cadena y las cookies de arriba: el fallo está entre el 302 y la ' +
          'página siguiente, no en el servidor.',
      );
    }
  }
  // Un `Timeout` a secas aquí no dice NADA: no distingue una contraseña caducada
  // de un TOTP rechazado, de un paso nuevo en el login o de que el IdP no cargue.
  // Y es el punto donde muere toda la auditoría, así que el mensaje tiene que
  // llevar la pantalla en la que se quedó.
  try {
    await page.waitForURL((u) => u.hostname !== IDP, { timeout: 60_000 });
    await activarOrganizacion(page);
  } catch {
    // Antes del volcado, lo que la URL ya dice con certeza. Un minuto de espera
    // para acabar enseñando una pantalla que hay que interpretar es peor que
    // nombrar la causa cuando se puede probar.
    const porque = porQueNoEntra(page.url());
    if (porque) throw new Error(`el login no llega a la app: ${porque}`);
    const texto = ((await page.locator('body').innerText().catch(() => '')) || '')
      .replace(/\s+/g, ' ')
      .slice(0, 300);
    throw new Error(`el login no sale del IdP. URL=${page.url()} · pantalla: «${texto}»`);
  }
  await page.waitForTimeout(2000);
}

/**
 * Un documento de texto con el título fijo, creado solo si no lo hay.
 *
 * Va por el BFF de la propia app y no por la interfaz: pulsar «Nuevo documento» y
 * renombrarlo son cinco pasos que se rompen con cualquier cambio de la pantalla, y
 * lo que este guion tiene que garantizar es el DATO, no el camino para crearlo.
 */
async function sembrarDocumento() {
  // BAJO EL HOST DE ORGANIZACIÓN TODO VA CON PREFIJO, incluidas estas dos rutas.
  // Iban a `/api/…` a pelo, que funcionaba entrando por `docs.<dominio>` —ahí la
  // app está en la raíz— y da 404 aquí, donde vive en `/docs`. Y encima el login
  // aterriza en `corporate.<dominio>/`, la raíz, así que ni siquiera la página
  // actual salvaba el path relativo. Las de Notes, Drive y PDF ya lo llevaban.
  await page.goto(`https://${HOST_ENTRADA}${BASE_DOCS}`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  // Con `q` y no la primera página: la búsqueda la hace el servicio, así que no
  // depende de cuántos documentos tenga la cuenta.
  const existente = await page.evaluate(async ([titulo, base]) => {
    const res = await fetch(`${base}/api/documents?limit=200&q=${encodeURIComponent(titulo)}`, {
      headers: { accept: 'application/json' },
    });
    if (!res.ok) return { error: res.status };
    return { docs: (await res.json()).documents ?? [] };
  }, [TITULO_DOC, BASE_DOCS]);

  if (existente.error) throw new Error(`listar documentos: HTTP ${existente.error}`);
  // El MÁS ANTIGUO, para que sea siempre el mismo aunque quede algún duplicado.
  const ya = existente.docs
    .filter((d) => ES_DE_AUDITORIA(d.title ?? '', TITULO_DOC))
    .sort((a, b) => String(a.createdAt).localeCompare(String(b.createdAt)))[0];
  if (ya) {
    console.log(`ya estaba: documento ${ya.id} («${ya.title}»)`);
    return ya.id;
  }

  const creado = await page.evaluate(async ([titulo, base]) => {
    const res = await fetch(`${base}/api/create`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ kind: 'document', title: titulo }),
    });
    return { estado: res.status, texto: (await res.text()).slice(0, 300) };
  }, [TITULO_DOC, BASE_DOCS]);
  if (creado.estado >= 300) throw new Error(`crear documento: HTTP ${creado.estado} ${creado.texto}`);
  const nuevo = JSON.parse(creado.texto);
  const id = nuevo?.document?.id ?? nuevo?.id ?? '';
  console.log(`creado: documento ${id} («${nuevo?.document?.title ?? nuevo?.title ?? TITULO_DOC}»)`);
  return id;
}

/**
 * Un cuaderno de Sytue Notepads con una nota dentro.
 *
 * SIN ESTO EL BARRIDO NO MIDE NOTES. Su ruta de cuaderno se resuelve preguntando
 * a la API por el primero que haya, y una cuenta recién hecha no tiene ninguno:
 * la auditoría diría «sin datos» y la app entera —la más nueva de la suite—
 * seguiría sin medirse en móvil, que es como llegó a producción.
 *
 * La NOTA no es un extra: el cuaderno vacío es una lista y el editor es la
 * superficie ancha, la que desborda.
 */
async function sembrarCuaderno() {
  await page.goto(`https://${HOST_ENTRADA}${BASE_NOTES}/`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  const cuaderno = await page.evaluate(async (titulo) => {
    const lista = await fetch('/notes/api/notebooks', { headers: { accept: 'application/json' } });
    if (!lista.ok) return { error: `listar cuadernos: HTTP ${lista.status}` };
    // `.notebooks`: cada BFF nombra su lista (Drive usa `items`).
    const ya = ((await lista.json()).notebooks ?? []).find((c) => c.title === titulo);
    if (ya) return { id: ya.id, nuevo: false };
    const res = await fetch('/notes/api/notebooks', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ title: titulo }),
    });
    if (!res.ok) return { error: `crear cuaderno: HTTP ${res.status} ${(await res.text()).slice(0, 200)}` };
    return { id: (await res.json())?.id ?? '', nuevo: true };
  }, TITULO_CUADERNO);
  if (cuaderno.error) throw new Error(cuaderno.error);
  if (!cuaderno.id) throw new Error('no se obtuvo el id del cuaderno');
  console.log(`${cuaderno.nuevo ? 'creado' : 'ya estaba'}: cuaderno ${cuaderno.id}`);

  const nota = await page.evaluate(async ([id, titulo]) => {
    const lista = await fetch(`/notes/api/notebooks/${encodeURIComponent(id)}/notes`, {
      headers: { accept: 'application/json' },
    });
    if (lista.ok && ((await lista.json()).notes ?? []).length > 0) return { nuevo: false };
    const res = await fetch(`/notes/api/notebooks/${encodeURIComponent(id)}/notes`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ title: titulo }),
    });
    if (!res.ok) return { error: `crear nota: HTTP ${res.status} ${(await res.text()).slice(0, 200)}` };
    return { nuevo: true };
  }, [cuaderno.id, TITULO_CUADERNO]);
  if (nota.error) throw new Error(nota.error);
  console.log(`${nota.nuevo ? 'creada' : 'ya estaba'}: nota en el cuaderno`);
  return cuaderno.id;
}

/**
 * Un PDF en el Drive de la cuenta.
 *
 * La ruta de Sytue PDF ya estaba en la auditoría, pero colgando de que la cuenta
 * tuviera un PDF POR CASUALIDAD: su resolutor busca `.pdf` en Drive y, si no hay,
 * el barrido dice «sin datos» y el visor —donde vivió el peor desbordamiento de
 * la suite, 415 px de barra— se queda sin medir. Estar cubierto y depender de la
 * suerte no es estar cubierto.
 *
 * El fichero es el PDF mínimo válido que ya sabe escribir la propia aplicación
 * (`packages/office/src/pdf/blanco.ts`): cuatro objetos y una tabla xref. Se sube por el
 * mismo endpoint que usa el botón «crear PDF en blanco», así que si esto funciona,
 * ese botón también.
 */
async function sembrarPdf() {
  // LA BÚSQUEDA VA EN EL ORIGEN DE DRIVE y la subida en el de Docs, y no es un
  // rodeo: cada BFF sirve sus rutas bajo SU host, así que un `/drive/api/...`
  // relativo desde la página de Docs pide a `docs.<dominio>` algo que ahí no
  // existe. Además la cookie de sesión de Drive es suya, así que hay que pasar
  // por su app antes de preguntarle nada.
  await page.goto(`https://${HOST_ENTRADA}/drive`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);
  const rutas = await page.evaluate(async (nombre) => {
    const res = await fetch(`/drive/api/search?q=${encodeURIComponent(nombre)}&limit=20`, {
      headers: { accept: 'application/json' },
    });
    if (!res.ok) return [];
    return ((await res.json()).items ?? []).map((i) => i.path ?? '');
  }, NOMBRE_PDF.replace(/\.pdf$/, ''));
  // Mismo sufijo « (N)» que el documento: Drive desambigua también aquí.
  const base = NOMBRE_PDF.replace(/\.pdf$/, '');
  const ya = rutas.some((r) => /\.pdf$/.test(r) && ES_DE_AUDITORIA(r.split('/').pop().replace(/\.pdf$/, ''), base));
  if (ya) {
    console.log(`ya estaba: PDF «${NOMBRE_PDF}»`);
    return;
  }

  await page.goto(`https://${HOST_ENTRADA}${BASE_PDF}`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1000);

  const r = await page.evaluate(async (nombre) => {
    // PDF 1.4 de una página A4 vacía, con los desplazamientos de la xref
    // calculados al vuelo: escritos a mano se descuadran al tocar una letra.
    const objetos = [
      '<< /Type /Catalog /Pages 2 0 R >>',
      '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Resources << >> >>',
    ];
    let cuerpo = '%PDF-1.4\n';
    const offsets = [];
    objetos.forEach((obj, i) => {
      offsets.push(cuerpo.length);
      cuerpo += `${i + 1} 0 obj\n${obj}\nendobj\n`;
    });
    const inicio = cuerpo.length;
    cuerpo +=
      `xref\n0 ${objetos.length + 1}\n0000000000 65535 f \n` +
      offsets.map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('') +
      `trailer\n<< /Size ${objetos.length + 1} /Root 1 0 R >>\nstartxref\n${inicio}\n%%EOF\n`;
    const bytes = new Uint8Array(cuerpo.length);
    for (let i = 0; i < cuerpo.length; i++) bytes[i] = cuerpo.charCodeAt(i) & 0xff;

    const res = await fetch(`/certipdf/api/pdf-abrir?name=${encodeURIComponent(nombre)}`, {
      method: 'POST',
      headers: { 'content-type': 'application/octet-stream' },
      body: new Blob([bytes]),
    });
    if (!res.ok) return { error: `subir PDF: HTTP ${res.status} ${(await res.text()).slice(0, 200)}` };
    return { ruta: (await res.json())?.path ?? '' };
  }, NOMBRE_PDF);
  if (r.error) throw new Error(r.error);
  console.log(`creado: PDF «${NOMBRE_PDF}»${r.ruta ? ` en ${r.ruta}` : ''}`);
}

/**
 * Fotos y un álbum en Sytue Fotos.
 *
 * El barrido abre el visor tocando una foto y mide el editor, las parecidas, las
 * cercanas y el álbum por `?id=`: sin fotos no hay nada de eso que medir, y justo
 * ahí desbordaba el visor (+520px a 320, 2026-09-15). Hoy la cuenta tiene fotos;
 * esto es para que el barrido no dependa de que las siga teniendo.
 *
 * Las imágenes las pinta un <canvas> del propio navegador —ni ficheros en el
 * repositorio ni ImageMagick en el runner— y son degradados con un círculo: nada
 * que parezca una persona, porque las capturas del barrido se publican. Las
 * proporciones son las que tensan la maquetación: panorámica, retrato y cuadrada.
 * Se suben por el mismo <input type=file> que usa la galería.
 */
async function sembrarFotos() {
  await page.goto(`https://${HOST_ENTRADA}/photos/`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2000);
  const api = (ruta, metodo = 'GET', cuerpo) =>
    page.evaluate(
      async ([r, m, c]) => {
        const res = await fetch(`/photos${r}`, {
          method: m,
          headers: { accept: 'application/json', ...(c ? { 'content-type': 'application/json' } : {}) },
          ...(c ? { body: JSON.stringify(c) } : {}),
        });
        return { ok: res.ok, status: res.status, cuerpo: await res.json().catch(() => null) };
      },
      [ruta, metodo, cuerpo],
    );
  const fotos = async () => {
    const r = await api('/api/v1/timeline?limit=50');
    if (!r.ok) throw new Error(`Fotos: línea de tiempo HTTP ${r.status}`);
    return r.cuerpo?.items ?? [];
  };

  let lista = await fotos();
  if (lista.length > 0) {
    console.log(`ya estaban: ${lista.length} fotos`);
  } else {
    const formas = [
      ['panoramica', 2400, 600],
      ['retrato', 900, 1600],
      ['cuadrada', 1200, 1200],
    ];
    const ficheros = [];
    for (const [i, [nombre, ancho, alto]] of formas.entries()) {
      const base64 = await page.evaluate(
        ([w, h, tono]) => {
          const lienzo = document.createElement('canvas');
          lienzo.width = w;
          lienzo.height = h;
          const g = lienzo.getContext('2d');
          const degradado = g.createLinearGradient(0, 0, w, h);
          degradado.addColorStop(0, `hsl(${tono} 55% 32%)`);
          degradado.addColorStop(1, `hsl(${(tono + 110) % 360} 65% 72%)`);
          g.fillStyle = degradado;
          g.fillRect(0, 0, w, h);
          g.fillStyle = 'rgba(255, 255, 255, 0.35)';
          g.beginPath();
          g.arc(w * 0.7, h * 0.35, Math.min(w, h) * 0.2, 0, Math.PI * 2);
          g.fill();
          return lienzo.toDataURL('image/jpeg', 0.85).split(',')[1];
        },
        [ancho, alto, (i * 97 + 140) % 360],
      );
      ficheros.push({
        name: `auditoria-no-borrar-${nombre}.jpg`,
        mimeType: 'image/jpeg',
        buffer: Buffer.from(base64, 'base64'),
      });
    }
    await page.locator('input[type=file]').first().setInputFiles(ficheros);
    for (let i = 0; i < 40 && lista.length < ficheros.length; i++) {
      await page.waitForTimeout(3000);
      lista = await fotos();
    }
    if (lista.length < ficheros.length) {
      throw new Error(`Fotos: solo aparecieron ${lista.length} de ${ficheros.length} fotos subidas`);
    }
    console.log(`creadas: ${lista.length} fotos sintéticas`);
  }

  const albumes = await api('/api/v1/albums');
  if (!albumes.ok) throw new Error(`Fotos: álbumes HTTP ${albumes.status}`);
  if ((albumes.cuerpo?.items ?? []).some((a) => a.title === TITULO_ALBUM)) {
    console.log(`ya estaba: álbum «${TITULO_ALBUM}»`);
    return;
  }
  const album = await api('/api/v1/albums', 'POST', { title: TITULO_ALBUM });
  if (!album.ok || !album.cuerpo?.id) throw new Error(`Fotos: crear álbum HTTP ${album.status}`);
  const ids = lista.slice(0, 6).map((f) => f.id);
  const dentro = await api(`/api/v1/albums/${encodeURIComponent(album.cuerpo.id)}/assets`, 'POST', { ids });
  if (!dentro.ok) throw new Error(`Fotos: añadir al álbum HTTP ${dentro.status}`);
  console.log(`creado: álbum «${TITULO_ALBUM}» con ${ids.length} fotos`);
}

try {
  await entrar();
  console.log('sesión en', page.url());
  const docId = await sembrarDocumento();
  if (!docId) throw new Error('no se obtuvo el id del documento');

  // Se COMPRUEBA que la ruta que estaba ciega ya se alcanza. Sembrar y no
  // verificarlo dejaría el mismo agujero con una capa de confianza encima.
  await page.goto(`https://${HOST_ENTRADA}${BASE_DOCS}/edit?id=${encodeURIComponent(docId)}`, {
    waitUntil: 'domcontentloaded',
  });
  await page.waitForTimeout(3000);
  // Se pide la ruta SIN organización y se acepta la canónica CON ella: desde el
  // ADR 0056 el middleware redirige `/docus/edit` a `/docus/<org>/edit` (302 local),
  // y así vale igual para una versión desplegada con o sin ese cambio. Exigir
  // `/docus/edit` exacto es lo que falló el 2026-09-28 con el editor abierto.
  const alcanzada = new RegExp(`^${BASE_DOCS}(/[^/]+)?/edit$`).test(new URL(page.url()).pathname);
  const cuerpo = (await page.evaluate(() => document.body.innerText)).length;
  console.log(`/edit?id=… alcanzada: ${alcanzada} · caracteres en la página: ${cuerpo}`);
  if (!alcanzada || cuerpo < 50) {
    throw new Error('la ruta del editor sigue sin resolverse con el documento sembrado');
  }
  // Y lo que faltaba por sembrar. Van DESPUÉS de comprobar el editor: si el
  // login o Docs fallan, el mensaje debe seguir señalando eso y no un error de
  // Notes tres pantallas más adelante.
  await sembrarCuaderno();
  await sembrarPdf();
  await sembrarFotos();
  console.log('OK: la auditoría ya puede medir el editor de Docs, Sytue Notepads, Sytue PDF y Sytue Fotos en móvil.');
} catch (e) {
  console.error('FALLÓ EL SEMBRADO:', e.message);
  process.exitCode = 1;
} finally {
  await browser.close();
}
