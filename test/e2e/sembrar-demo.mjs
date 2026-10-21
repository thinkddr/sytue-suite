// Contenido de DEMOSTRACIÓN para las capturas de producto del portal.
//
// POR QUÉ EXISTE. `capturas-producto.mjs` fotografía lo que haya en la cuenta, y
// media suite salía vacía o con restos de probar: un cuaderno «test» con una
// nota «prueba de nota», nueve «PDF sin título», un canal con «holalala». Eso no
// lo arregla una captura mejor hecha; hace falta que dentro haya algo que
// enseñar. Este guion lo pone.
//
// TODO LO QUE SIEMBRA ES INVENTADO Y PUBLICABLE: nombres de `ejemplo.test`
// —dominio reservado para documentación, RFC 6761, que no es de nadie— y textos
// de oficina sin un solo dato de una persona real. Lo que salga en la imagen
// acaba en internet.
//
// ES IDEMPOTENTE POR NOMBRE: cada sección mira si su cuaderno / canal / documento
// ya está antes de crearlo, así que lanzarlo dos veces no duplica nada. No BORRA
// nunca: lo que ya hubiera en la cuenta se queda donde está.
//
// Uso:
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… bun run sembrar-demo.mjs [sección…]
//
// Secciones: notes, docs, rooms, photos, albumes, pdf. Sin argumentos, todas.
//
// No confundir con `tests/seed-demo.spec.ts`, que siembra el entorno de
// desarrollo (`*.sytue.localhost`) con DOS cuentas y sirve a los tests de
// extremo a extremo. Este apunta a una instalación desplegada, con la cuenta que
// se le dé, y solo existe para que las fichas del portal enseñen un producto
// usado en vez de uno recién instalado.
import { chromium } from "playwright";
import { activarOrganizacion, completarMFASiHaceFalta } from "./tests/helpers.ts";

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || "sytue.eu";
const TENANT = process.env.SYTUE_SHOT_TENANT ?? "corporate";
const IDP = `id.${DOMINIO}`;
const BASE = `https://${TENANT ? `${TENANT}.${DOMINIO}` : DOMINIO}`;

const CRED = {
  email: process.env.SYTUE_SHOT_EMAIL || process.env.SYTUE_AUDIT_EMAIL || "",
  pass: process.env.SYTUE_SHOT_PASS || process.env.SYTUE_AUDIT_PASS || "",
  // La semilla del TOTP, si la cuenta lo tiene, como en `capturas-producto.mjs`:
  // `completarMFASiHaceFalta` por sí solo solo mira `SYTUE_AUDIT_TOTP`.
  totp: process.env.SYTUE_SHOT_TOTP || process.env.SYTUE_AUDIT_TOTP || "",
};
if (!CRED.email || !CRED.pass) {
  console.error(
    "Faltan credenciales: SYTUE_SHOT_EMAIL/PASS o SYTUE_AUDIT_EMAIL/PASS.",
  );
  process.exit(2);
}

// --- Lo que se siembra -------------------------------------------------------

const CUADERNO = "Operaciones";

/** Notas del cuaderno. El formato nativo de Notepads es HTML. */
const NOTAS = [
  {
    title: "Guardias de octubre",
    content:
      "<h2>Reparto</h2><ul><li>Semana 1 — Carla</li><li>Semana 2 — Héctor</li>" +
      "<li>Semana 3 — Iñaki</li><li>Semana 4 — Marta</li></ul>" +
      "<p>Los cambios se avisan con una semana y se apuntan aquí, no por mensaje suelto.</p>",
  },
  {
    title: "Runbook: restaurar una copia",
    content:
      "<h2>Antes de tocar nada</h2><ol><li>Avisar en el canal de operaciones.</li>" +
      "<li>Comprobar la fecha de la última copia buena.</li>" +
      "<li>Restaurar sobre el entorno de pruebas primero.</li></ol>" +
      "<p>Tiempo objetivo de recuperación: <strong>4 horas</strong>.</p>",
  },
  {
    title: "Preguntas frecuentes del alta de clientes",
    content:
      "<p>Las tres que salen siempre:</p><ul>" +
      "<li><strong>¿Se conserva el correo antiguo?</strong> Sí, la migración lo trae entero.</li>" +
      "<li><strong>¿Cuánto tarda?</strong> Entre dos y cinco días según el volumen.</li>" +
      "<li><strong>¿Hace falta parar?</strong> No: el buzón sigue recibiendo durante la copia.</li></ul>",
  },
  {
    title: "Ideas para la próxima retrospectiva",
    content:
      "<ul><li>Revisar por qué las incidencias de acceso se repiten los viernes.</li>" +
      "<li>Turnar quien lleva la reunión.</li>" +
      "<li>Medir cuánto tarda de verdad un alta, de punta a punta.</li></ul>",
  },
  {
    title: "Plantilla de acta",
    content:
      "<h2>Asistentes</h2><p>—</p><h2>Decisiones</h2><ul><li>—</li></ul>" +
      "<h2>Acciones</h2><table><tr><th>Qué</th><th>Quién</th><th>Cuándo</th></tr>" +
      "<tr><td>—</td><td>—</td><td>—</td></tr></table>",
  },
];

/**
 * Clave del registro CRDT donde vive el contenido, según el tipo de documento
 * (`apps/frontend/packages/office/src/editor/types.ts`, `CONTENT_KEY`).
 */
const CONTENT_KEY = {
  document: "doc:html",
  spreadsheet: "doc:sheet",
  presentation: "doc:slides",
};

/** Documentos de Docus, Calcus y Diapos. Solo el título: es lo que ve la lista. */
// El `content` SOLO va en los tres que abre `capturas-producto.mjs` (uno por
// tipo, por su título): son los únicos que una captura enseña abiertos, y un
// documento en blanco con la barra de herramientas puesta no demuestra nada.
// Los demás se quedan con solo el título, que es lo que ve la lista.
const DOCUMENTOS = [
  {
    kind: "document",
    title: "Propuesta de servicio — Nordika",
    content:
      "<h1>Propuesta de servicio — Nordika</h1>" +
      "<p>Migración del correo y el almacenamiento de la organización a Sytue One, con continuidad del dominio propio.</p>" +
      "<h2>Alcance</h2><ul><li>40 buzones con su historial completo</li>" +
      "<li>Migración de ficheros compartidos a Sytue Drive</li>" +
      "<li>Calendario y contactos del equipo</li></ul>" +
      "<h2>Calendario</h2><p>Ventana de migración: <strong>dos fines de semana</strong>, sin corte del servicio entre semana.</p>",
  },
  { kind: "document", title: "Manual de incorporación" },
  { kind: "document", title: "Política de uso aceptable" },
  { kind: "document", title: "Acta del comité de producto" },
  {
    kind: "spreadsheet",
    title: "Presupuesto 2027",
    content: JSON.stringify({
      active: 0,
      sheets: [
        {
          name: "Presupuesto",
          cells: {
            A1: "Partida",
            B1: "Importe",
            A2: "Licencias",
            B2: "12400",
            A3: "Infraestructura",
            B3: "8200",
            A4: "Formación",
            B4: "1500",
            A5: "Total",
            B5: "22100",
          },
        },
      ],
    }),
  },
  { kind: "spreadsheet", title: "Inventario de equipos" },
  { kind: "spreadsheet", title: "Horas por proyecto" },
  {
    kind: "presentation",
    title: "Resultados del trimestre",
    content: JSON.stringify({
      slides: [
        {
          id: "s1",
          boxes: [
            {
              id: "b1",
              x: 10,
              y: 10,
              w: 80,
              h: 20,
              kind: "text",
              html: "<h1>Resultados del trimestre</h1>",
            },
            {
              id: "b2",
              x: 10,
              y: 40,
              w: 80,
              h: 40,
              kind: "text",
              html: "<p>Altas: +18 % respecto al trimestre anterior</p><p>Soporte: tiempo de respuesta medio bajo dos horas</p>",
            },
          ],
        },
      ],
    }),
  },
  { kind: "presentation", title: "Formación: seguridad del correo" },
  { kind: "presentation", title: "Presentación comercial 2027" },
];

const CANAL = { name: "equipo", topic: "Guardias, despliegues e incidencias" };

/** La conversación del canal. Sale toda de la misma cuenta: es la que hay. */
const MENSAJES = [
  "Buenos días. Ventana de mantenimiento esta noche de 23:00 a 00:30, solo afecta al entorno de pruebas.",
  "Subida la nota con el reparto de guardias de octubre al cuaderno de Operaciones.",
  "El alta de Lusitana ha terminado sin incidencias: buzones, dominio verificado y calendario del equipo.",
  "Recordatorio: la formación de seguridad del correo es el miércoles a las 10:00, dura 45 minutos.",
  "La restauración de prueba del martes salió en 2 h 10 min, por debajo del objetivo de 4 h.",
  "Cerrada la incidencia de accesos del viernes. Era una política que pedía segundo factor en la red de invitados; queda documentado en el runbook.",
];

// --- Fontanería --------------------------------------------------------------

const navegador = await chromium.launch();
const contexto = await navegador.newContext({
  ignoreHTTPSErrors: true,
  locale: "es-ES",
});
const page = await contexto.newPage();

/** Entra si la pantalla que hay delante es la del IdP. */
async function asegurarSesion() {
  if (new URL(page.url()).hostname !== IDP) return;
  await page.fill("input[name=email]", CRED.email);
  await page.fill("input[name=password]", CRED.pass);
  await page.click("button[type=submit]");
  await completarMFASiHaceFalta(page, CRED.totp || undefined).catch(() => {});
  await page.waitForURL((u) => u.hostname !== IDP, { timeout: 90_000 });
  await activarOrganizacion(page);
}

/**
 * Abre la app y deja la página lista para llamar a SU BFF.
 *
 * Hay que pasar por la app antes de llamar a su API: cada BFF tiene su propia
 * cookie de sesión y el `fetch` va desde la página, que es quien la lleva. Ir
 * directo a `/notes/api/...` sin haber abierto Notepads da un 401.
 */
async function abrir(app) {
  await page.goto(`${BASE}/${app}`, {
    waitUntil: "domcontentloaded",
    timeout: 60_000,
  });
  await asegurarSesion();
  await page
    .waitForLoadState("networkidle", { timeout: 15_000 })
    .catch(() => {});
}

/** fetch desde la página, con su sesión. Devuelve {ok, status, datos}. */
function pedir(ruta, opciones = {}) {
  return page.evaluate(
    async ({ ruta, opciones }) => {
      const res = await fetch(ruta, {
        method: opciones.method ?? "GET",
        credentials: "include",
        ...(opciones.body
          ? {
              headers: { "content-type": "application/json" },
              body: JSON.stringify(opciones.body),
            }
          : {}),
      });
      const texto = await res.text();
      let datos = null;
      try {
        datos = JSON.parse(texto);
      } catch {
        datos = texto.slice(0, 200);
      }
      return { ok: res.ok, status: res.status, datos };
    },
    { ruta, opciones },
  );
}

const hecho = [];
const fallado = [];
const ok = (que) => hecho.push(que) && console.log(`✓ ${que}`);
const mal = (que, detalle) =>
  fallado.push(que) && console.log(`✗ ${que}: ${detalle}`);

// --- Notepads ----------------------------------------------------------------

async function sembrarNotes() {
  await abrir("notes");
  const lista = await pedir("/notes/api/notebooks");
  if (!lista.ok)
    return mal("Notepads", `no se pudo leer la lista (HTTP ${lista.status})`);

  const cuadernos = lista.datos?.notebooks ?? lista.datos ?? [];
  let cuaderno = cuadernos.find?.((n) => n.title === CUADERNO);
  if (!cuaderno) {
    const creado = await pedir("/notes/api/notebooks", {
      method: "POST",
      body: { title: CUADERNO, emoji: "🛠️" },
    });
    if (!creado.ok)
      return mal(
        "Notepads",
        `no se pudo crear el cuaderno (HTTP ${creado.status})`,
      );
    cuaderno = creado.datos?.notebook ?? creado.datos;
    ok(`Notepads: cuaderno «${CUADERNO}»`);
  }

  const dentro = await pedir(`/notes/api/notebooks/${cuaderno.id}/notes`);
  const yaHay = new Set((dentro.datos?.notes ?? []).map((n) => n.title));
  for (const nota of NOTAS) {
    if (yaHay.has(nota.title)) continue;
    const r = await pedir(`/notes/api/notebooks/${cuaderno.id}/notes`, {
      method: "POST",
      body: nota,
    });
    if (r.ok) ok(`Notepads: «${nota.title}»`);
    else mal(`Notepads: «${nota.title}»`, `HTTP ${r.status}`);
  }
}

// --- Docus / Calcus / Diapos -------------------------------------------------

async function sembrarDocs() {
  // El BFF de la ofimática es el mismo en las cuatro apps (`@sytue/office`): desde
  // Docus se siembran también las hojas y las presentaciones.
  await abrir("docus");
  const lista = await pedir("/docus/api/documents");
  const existentes = lista.datos?.documents ?? lista.datos?.items ?? [];
  const porTitulo = new Map(existentes.map((d) => [d.title, d]));

  for (const doc of DOCUMENTOS) {
    let creado = porTitulo.get(doc.title);
    if (!creado) {
      const r = await pedir("/docus/api/create", {
        method: "POST",
        body: { kind: doc.kind, title: doc.title },
      });
      if (!r.ok) {
        mal(`Docs: «${doc.title}»`, `HTTP ${r.status}`);
        continue;
      }
      creado = r.datos?.document ?? r.datos;
      ok(`Docs: ${doc.kind} «${doc.title}»`);
    }
    if (!doc.content) continue;

    // El contenido va aparte, como una operación CRDT (`doc:html`/`doc:sheet`/
    // `doc:slides` según el tipo): `POST /api/create` solo pone el título, y un
    // documento sin contenido es justo lo que una captura del editor no puede
    // enseñar.
    const clave = CONTENT_KEY[doc.kind];
    const estado = await pedir(
      `/docus/api/state?id=${encodeURIComponent(creado.id)}`,
    );
    if (estado.datos?.state?.registers?.[clave]) continue; // ya tiene contenido, no se pisa
    const r2 = await pedir(
      `/docus/api/ops?id=${encodeURIComponent(creado.id)}`,
      {
        method: "POST",
        body: {
          ops: [
            {
              kind: "set",
              dot: { a: "sembrar-demo", c: 1 },
              key: clave,
              value: doc.content,
              lamport: 1,
            },
          ],
        },
      },
    );
    if (r2.ok) ok(`Docs: contenido de «${doc.title}»`);
    else mal(`Docs: contenido de «${doc.title}»`, `HTTP ${r2.status}`);
  }
}

// --- Rooms -------------------------------------------------------------------

async function sembrarRooms() {
  await abrir("rooms");
  const lista = await pedir("/rooms/api/channels");
  if (!lista.ok)
    return mal("Rooms", `no se pudo leer la lista (HTTP ${lista.status})`);

  const canales = lista.datos?.channels ?? lista.datos ?? [];
  let canal = canales.find?.((c) => c.name === CANAL.name);
  if (!canal) {
    const creado = await pedir("/rooms/api/channels", {
      method: "POST",
      body: { ...CANAL, kind: "public" },
    });
    if (!creado.ok)
      return mal("Rooms", `no se pudo crear el canal (HTTP ${creado.status})`);
    canal = creado.datos?.channel ?? creado.datos;
    ok(`Rooms: canal #${CANAL.name}`);
  }

  // Si el canal ya tiene conversación, no se añade otra encima.
  const previos = await pedir(`/rooms/api/messages?channelId=${canal.id}`);
  const cuantos = (previos.datos?.messages ?? []).length;
  if (cuantos >= MENSAJES.length) {
    console.log(
      `  · Rooms: #${CANAL.name} ya tiene ${cuantos} mensajes, no se toca`,
    );
    return;
  }
  for (const texto of MENSAJES) {
    const r = await pedir("/rooms/api/send", {
      method: "POST",
      body: { channelId: canal.id, body: texto },
    });
    if (!r.ok) mal("Rooms: mensaje", `HTTP ${r.status}`);
  }
  ok(`Rooms: ${MENSAJES.length} mensajes en #${CANAL.name}`);
}

// --- Fotos -------------------------------------------------------------------

/**
 * DE DÓNDE SALEN LAS FOTOS.
 *
 * De Wikimedia Commons, y SOLO las que la propia API declara **CC0 o dominio
 * público**: se comprueba licencia por licencia y se descarta lo que no lo sea.
 * Eso importa porque estas imágenes acaban en una página pública del portal, y
 * una foto con licencia que pide atribución la pediría también ahí.
 *
 * Se piden ya redimensionadas (`iiurlwidth`), que es lo que hace falta para una
 * galería y evita subir originales de veinte megas al Drive de la cuenta.
 */
const LICENCIA_LIBRE = /^(cc0|public domain|pd)/i;

/**
 * Los temas se buscan por separado y no en una sola consulta: pidiendo «paisaje»
 * salen doce fotos de bosque verde, y una galería de demostración que enseña
 * doce veces lo mismo parece un error de carga, no un carrete.
 */
const TEMAS = [
  "landscape OR mountain",
  "coast OR beach OR sea",
  "city OR street OR architecture",
  "flower OR garden OR tree",
  "food OR market OR coffee",
  "bird OR animal OR wildlife",
];

/**
 * Nombres de CÁMARA, no descriptivos.
 *
 * Poner «Amanecer en la sierra» a lo que Commons devolviera dejaba el nombre
 * peleado con la foto —una gaviota llamada «Higuera del patio»—, y eso se ve en
 * cuanto alguien abre una. Un carrete de verdad viene con el nombre que pone la
 * cámara; así nunca miente.
 */
const NOMBRES_FOTO = Array.from(
  { length: 24 },
  (_, i) => `IMG_${String(2041 + i * 7).padStart(4, "0")}`,
);

async function buscarPorTema(tema, cuantas) {
  const url =
    "https://commons.wikimedia.org/w/api.php?action=query&generator=search" +
    "&gsrsearch=" +
    encodeURIComponent(`incategory:CC-Zero (${tema})`) +
    `&gsrnamespace=6&gsrlimit=${cuantas * 4}` +
    "&prop=imageinfo&iiprop=url%7Cextmetadata%7Csize&iiurlwidth=1600&format=json";
  const res = await fetch(url, {
    headers: { "user-agent": "sytue-demo-seed/1.0" },
  });
  if (!res.ok) throw new Error(`Commons respondió ${res.status}`);
  const paginas = Object.values(((await res.json()).query ?? {}).pages ?? {});
  const salen = [];
  for (const p of paginas) {
    const ii = p.imageinfo?.[0];
    if (!ii?.thumburl) continue;
    const lic = ii.extmetadata?.LicenseShortName?.value ?? "";
    // Lo que no diga CC0 o dominio público NO se usa, aunque la categoría lo
    // prometa: la categoría la pone cualquiera, la licencia la declara el
    // fichero.
    if (!LICENCIA_LIBRE.test(lic)) continue;
    // Las reproducciones de cuadros de museo son de dominio público pero no
    // parecen el carrete de nadie.
    if (/\bMET\b|painting|Museum/i.test(p.title)) continue;
    if ((ii.width ?? 0) < 1200) continue;
    salen.push({ titulo: p.title, url: ii.thumburl, licencia: lic });
    if (salen.length >= cuantas) break;
  }
  return salen;
}

async function sembrarPhotos() {
  await abrir("photos");
  const yaHay = await pedir("/photos/api/v1/assets");
  const cuantas = (yaHay.datos?.items ?? yaHay.datos?.assets ?? []).length;
  if (cuantas >= NOMBRES_FOTO.length) {
    console.log(`  · Fotos: la galería ya tiene ${cuantas} fotos, no se toca`);
    return;
  }

  const porTema = Math.ceil(NOMBRES_FOTO.length / TEMAS.length);
  const fotos = [];
  const yaPuesta = new Set();
  for (const tema of TEMAS) {
    for (const f of await buscarPorTema(tema, porTema)) {
      if (yaPuesta.has(f.url)) continue;
      yaPuesta.add(f.url);
      fotos.push(f);
    }
  }
  if (fotos.length < 4)
    return mal(
      "Fotos",
      `solo ${fotos.length} imágenes libres, no merece la pena`,
    );

  for (const [i, foto] of fotos.entries()) {
    const bin = await fetch(foto.url, {
      headers: { "user-agent": "sytue-demo-seed/1.0" },
    });
    if (!bin.ok) {
      mal(`Fotos: ${foto.titulo}`, `descarga HTTP ${bin.status}`);
      continue;
    }
    const b64 = Buffer.from(await bin.arrayBuffer()).toString("base64");
    const nombre = `${NOMBRES_FOTO[i] ?? `Foto ${i + 1}`}.jpg`;
    const r = await page.evaluate(
      async ({ b64, nombre }) => {
        const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
        const subida = await fetch(
          `/photos/api/drive-subir?nombre=${encodeURIComponent(nombre)}`,
          {
            method: "POST",
            credentials: "include",
            headers: { "content-type": "image/jpeg" },
            body: bytes,
          },
        );
        if (!subida.ok)
          return { ok: false, paso: "drive", status: subida.status };
        const { id } = await subida.json();
        const alta = await fetch("/photos/api/v1/assets", {
          method: "POST",
          credentials: "include",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ driveFileId: id }),
        });
        return { ok: alta.ok, paso: "indexar", status: alta.status };
      },
      { b64, nombre },
    );
    if (r.ok) ok(`Fotos: «${nombre}» (${foto.licencia})`);
    else mal(`Fotos: «${nombre}»`, `${r.paso} HTTP ${r.status}`);
  }
}

/**
 * Álbumes con las fotos ya subidas, repartidas en orden.
 *
 * La pestaña de álbumes es la SEGUNDA captura de la ficha de Fotos, y vacía dice
 * «todavía no hay álbumes»: la peor cara posible de una función que existe.
 */
const ALBUMES = [
  "Viaje de verano",
  "La ciudad",
  "Del huerto",
  "Sin clasificar",
];

async function sembrarAlbumes() {
  await abrir("photos");
  const hay = await pedir("/photos/api/v1/albums");
  if (!hay.ok)
    return mal("Álbumes", `no se pudo leer la lista (HTTP ${hay.status})`);
  const existentes = new Set((hay.datos?.items ?? []).map((a) => a.title));

  const fotos = await pedir("/photos/api/v1/timeline");
  const ids = (fotos.datos?.items ?? []).map((f) => f.id);
  if (ids.length < ALBUMES.length)
    return mal("Álbumes", "no hay fotos suficientes");

  // Se reparten en orden y sin repetir: un álbum que enseña la misma foto que el
  // de al lado se nota en la portada.
  const porAlbum = Math.floor(ids.length / ALBUMES.length);
  for (const [i, titulo] of ALBUMES.entries()) {
    if (existentes.has(titulo)) continue;
    const creado = await pedir("/photos/api/v1/albums", {
      method: "POST",
      body: { title: titulo },
    });
    if (!creado.ok) {
      mal(`Álbumes: «${titulo}»`, `HTTP ${creado.status}`);
      continue;
    }
    const album = creado.datos?.album ?? creado.datos;
    const suyas = ids.slice(i * porAlbum, (i + 1) * porAlbum);
    const r = await pedir(`/photos/api/v1/albums/${album.id}/assets`, {
      method: "POST",
      body: { ids: suyas },
    });
    if (r.ok) ok(`Álbumes: «${titulo}» con ${suyas.length} fotos`);
    else mal(`Álbumes: «${titulo}»`, `añadir fotos HTTP ${r.status}`);
  }
}

// --- CertiPDF ---------------------------------------------------------------

/** Lo que se ve en la portada de CertiPDF es el NOMBRE, la fecha y el tamaño. */
const PDFS = [
  "Contrato marco - Nordika (firmado).pdf",
  "Anexo de tratamiento de datos.pdf",
  "Certificado de entrega 2026-0912.pdf",
  "Politica de uso aceptable.pdf",
  "Presupuesto aprobado del trimestre.pdf",
  "Manual de usuario - Sytue Drive.pdf",
  "Acta del comite de producto.pdf",
  "Plan de continuidad de negocio.pdf",
];

/**
 * Un PDF de una página con un título dentro.
 *
 * Se arma a mano —son treinta líneas de sintaxis PDF— en vez de traer una
 * librería o llamar a ImageMagick: la portada de CertiPDF enseña el nombre, la
 * fecha y el tamaño, nunca el contenido, así que lo único que hace falta es que
 * el fichero sea un PDF de verdad y se pueda abrir.
 *
 * Texto en Helvetica con WinAnsiEncoding y contenido en latin1, que es lo que
 * entiende esa fuente base sin incrustar nada.
 */
function pdfDeUnaPagina(titulo) {
  const texto = titulo
    .replace(/\.pdf$/i, "")
    .replace(/[^\x20-\xff]/g, "-")
    .replace(/([\\()])/g, "\\$1");
  const flujo = `BT /F1 22 Tf 72 742 Td (${texto}) Tj ET\nBT /F1 12 Tf 72 706 Td (Documento de demostracion. Sytue CertiPDF.) Tj ET`;
  const objetos = [
    "<</Type/Catalog/Pages 2 0 R>>",
    "<</Type/Pages/Kids[3 0 R]/Count 1>>",
    "<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]" +
      "/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>",
    "<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>",
    `<</Length ${Buffer.byteLength(flujo, "latin1")}>>\nstream\n${flujo}\nendstream`,
  ];

  let salida = "%PDF-1.4\n";
  const posiciones = [];
  objetos.forEach((cuerpo, i) => {
    posiciones.push(Buffer.byteLength(salida, "latin1"));
    salida += `${i + 1} 0 obj\n${cuerpo}\nendobj\n`;
  });
  const inicioXref = Buffer.byteLength(salida, "latin1");
  salida += `xref\n0 ${objetos.length + 1}\n0000000000 65535 f \n`;
  for (const pos of posiciones)
    salida += `${String(pos).padStart(10, "0")} 00000 n \n`;
  salida += `trailer\n<</Size ${objetos.length + 1}/Root 1 0 R>>\nstartxref\n${inicioXref}\n%%EOF\n`;
  return Buffer.from(salida, "latin1");
}

async function sembrarPdf() {
  await abrir("certipdf");
  const yaHay = await pedir("/certipdf/api/pdf-recientes");
  const nombres = new Set((yaHay.datos?.items ?? []).map((f) => f.name));
  for (const nombre of PDFS) {
    if (nombres.has(nombre)) continue;
    const b64 = pdfDeUnaPagina(nombre).toString("base64");
    const r = await page.evaluate(
      async ({ b64, nombre }) => {
        const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
        const res = await fetch(
          `/certipdf/api/pdf-abrir?name=${encodeURIComponent(nombre)}`,
          {
            method: "POST",
            credentials: "include",
            headers: { "content-type": "application/pdf" },
            body: bytes,
          },
        );
        return { ok: res.ok, status: res.status };
      },
      { b64, nombre },
    );
    if (r.ok) ok(`CertiPDF: «${nombre}»`);
    else mal(`CertiPDF: «${nombre}»`, `HTTP ${r.status}`);
  }
}

// --- Lanzadera ---------------------------------------------------------------

const SECCIONES = {
  notes: sembrarNotes,
  docs: sembrarDocs,
  rooms: sembrarRooms,
  photos: sembrarPhotos,
  albumes: sembrarAlbumes,
  pdf: sembrarPdf,
};
const pedidas = process.argv.slice(2).filter((a) => !a.startsWith("-"));
const claves = pedidas.length ? pedidas : Object.keys(SECCIONES);
const desconocidas = claves.filter((k) => !SECCIONES[k]);
if (desconocidas.length) {
  console.error(`Sección desconocida: ${desconocidas.join(", ")}`);
  process.exit(2);
}

for (const clave of claves) {
  try {
    await SECCIONES[clave]();
  } catch (e) {
    mal(clave, e.message.split("\n")[0]);
  }
}

await navegador.close();
console.log(`\n${hecho.length} cosas sembradas, ${fallado.length} fallos`);
process.exit(fallado.length ? 1 : 0);
