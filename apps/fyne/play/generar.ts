/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * Genera los textos de la ficha de Google Play de cada app (`<app>/<idioma>/*.txt`) a
 * partir del catálogo de www, que ya está en los 26 idiomas: las fichas dicen lo mismo
 * que la web y no hay una segunda traducción que mantener. Lo que la app del TELÉFONO no
 * hace (atajos de teclado, arrastrar, el cliente de escritorio, cámara, «en tu navegador»)
 * se deja fuera clave a clave, abajo. `bun apps/fyne/play/generar.ts` desde la raíz.
 *
 * Y, con las MISMAS fichas, los textos de la App Store (`../appstore/<app>/<locale>/*.txt`):
 * un generador y no dos, para que las dos tiendas no puedan decir cosas distintas.
 */
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const AQUI = dirname(new URL(import.meta.url).pathname);
const WWW = join(AQUI, "../../frontend/apps/www/src/i18n/messages");

// Idioma de la suite → código de Play. `ga` y `mt` no están entre los de Play Console.
const IDIOMAS: Record<string, string> = {
  bg: "bg",
  cs: "cs-CZ",
  da: "da-DK",
  de: "de-DE",
  el: "el-GR",
  en: "en-US",
  es: "es-ES",
  et: "et",
  fi: "fi-FI",
  fr: "fr-FR",
  hr: "hr",
  hu: "hu-HU",
  is: "is-IS",
  it: "it-IT",
  lt: "lt",
  lv: "lv",
  nb: "no-NO",
  nl: "nl-NL",
  pl: "pl-PL",
  pt: "pt-PT",
  ro: "ro",
  sk: "sk",
  sl: "sl",
  sv: "sv-SE",
};

// Idioma de la suite → locale de App Store Connect. Diecinueve de los veintiséis: la App
// Store no tiene búlgaro, estonio, irlandés, islandés, letón, lituano ni maltés («App Store
// localizations», developer.apple.com/help/app-store-connect/reference/app-store-localizations;
// el esloveno, `sl-SI`, entró el 2026-03-31). Los códigos son los de la API, no los de Play.
const APPLE: Record<string, string> = {
  cs: "cs",
  da: "da",
  de: "de-DE",
  el: "el",
  en: "en-US",
  es: "es-ES",
  fi: "fi",
  fr: "fr-FR",
  hr: "hr",
  hu: "hu",
  it: "it",
  nb: "no",
  nl: "nl-NL",
  pl: "pl",
  pt: "pt-PT",
  ro: "ro",
  sk: "sk",
  sl: "sl-SI",
  sv: "sv",
};
const APPSTORE = join(AQUI, "../appstore");
const NUCLEO = join(AQUI, "../../nucleo/i18n/locales");

// Palabras clave de la App Store que no se traducen: los estándares y formatos que el
// catálogo de www nombra en cada producto. Ninguna marca ajena (App Review 2.3.7).
const SIGLAS: Record<string, string[]> = {
  hub: ["suite", "SSO"],
  mail: ["email", "JMAP", "PGP", "S/MIME"],
  drive: ["WebDAV", "S3", "cloud"],
  docus: ["DOCX", "ODT", "OpenDocument"],
  calcus: ["XLSX", "ODS", "CSV"],
  diapos: ["PPTX", "ODP", "slides"],
  certipdf: ["PDF", "signature", "annotate"],
  calendar: ["CalDAV", "ICS"],
  contacts: ["CardDAV", "CSV", "vCard"],
  notes: ["notes", "HTML", "PDF"],
  photos: ["EXIF", "album"],
  rooms: ["chat", "video", "meeting"],
  cast: ["video", "live", "streaming"],
  erp: ["ERP", "invoicing", "accounting"],
  bases: ["database", "CSV", "JSON"],
};

// `corta`: candidatas por orden; vale la primera que quepa en los 80 de Play en ESE idioma.
type Ficha = {
  producto: string;
  corta: string[];
  entrada?: string;
  claves: string[];
};
const pf = (p: string, n: number, fuera: number[] = []) =>
  Array.from({ length: n }, (_, i) => i + 1)
    .filter((i) => !fuera.includes(i))
    .map((i) => `product.${p}.f${i}`);
const pp = (p: string, fuera: number[] = []) =>
  [1, 2, 3, 4]
    .filter((i) => !fuera.includes(i))
    .map((i) => `product.${p}.p${i}`);

// La ficha de cada app: claves del catálogo. `entrada` abre la descripción larga (por
// defecto, la primera de tagline/blurb que no sea ya la corta); `claves`, los puntos.
const FICHAS: Record<string, Ficha> = {
  hub: {
    producto: "one",
    corta: ["product.one.blurb", "product.one.tagline"],
    claves: [...pp("one", [2]), ...pf("one", 8, [4])],
  },
  mail: {
    producto: "mail",
    corta: ["product.mail.tagline"],
    claves: [...pp("mail"), ...pf("mail", 12, [12])],
  },
  drive: {
    producto: "drive",
    corta: ["product.drive.tagline"],
    claves: [...pp("drive"), ...pf("drive", 10, [2, 9])],
  },
  docus: {
    producto: "docs",
    corta: ["product.docs.p1", "product.docs.standard"],
    entrada: "product.docs.standard",
    claves: [...pp("docs", [1, 2, 3]), ...pf("docs", 11)],
  },
  calcus: {
    producto: "sheets",
    corta: ["product.sheets.blurb", "product.sheets.tagline"],
    entrada: "product.sheets.tagline",
    claves: [...pp("sheets"), ...pf("sheets", 10)],
  },
  diapos: {
    producto: "slides",
    corta: ["product.slides.blurb", "product.slides.tagline"],
    entrada: "product.slides.tagline",
    claves: [...pp("slides"), ...pf("slides", 8)],
  },
  // «en tu navegador» (tagline) y la lectura en voz alta (f10) son de la web.
  certipdf: {
    producto: "pdf",
    corta: ["product.pdf.blurb", "product.pdf.p1"],
    claves: [...pp("pdf"), ...pf("pdf", 10, [10])],
  },
  calendar: {
    producto: "calendar",
    corta: ["product.calendar.tagline"],
    claves: [...pp("calendar"), ...pf("calendar", 11)],
  },
  contacts: {
    producto: "contacts",
    corta: ["product.contacts.tagline"],
    claves: [...pp("contacts"), ...pf("contacts", 10)],
  },
  // p3 y f6: el cifrado ocurre «en tu navegador»; en la app es en la app, y la clave no lo dice.
  notes: {
    producto: "notes",
    corta: ["product.notes.tagline"],
    claves: [...pp("notes", [3]), ...pf("notes", 12, [6])],
  },
  photos: {
    producto: "photos",
    corta: [
      "product.photos.tagline",
      "product.photos.blurb",
      "product.photos.p4",
    ],
    claves: [...pp("photos"), ...pf("photos", 12, [3, 5])],
  },
  // Sin Ctrl+G (f13). El adaptador de contenido cifrado no implementa búsqueda
  // (p4/f11), programados (f5) ni comandos remotos/votos de tarjetas (f6). f7
  // promete pantalla y grabación en todos los destinos: iOS no tiene ReplayKit
  // y una llamada cifrada no admite la grabación del servidor. p2 cubre la llamada.
  rooms: {
    producto: "rooms",
    corta: ["product.rooms.tagline", "product.rooms.blurb", "product.rooms.p1"],
    claves: [...pp("rooms", [4]), ...pf("rooms", 13, [5, 6, 7, 11, 13])],
  },
  // En el teléfono se ve la versión WebM de 720p, no el HLS adaptativo que elige el
  // navegador (f2), y el reproductor incrustable con oEmbed es de la web (f9; ADR 0061:
  // «la única excepción es el incrustado»).
  cast: {
    producto: "cast",
    corta: [
      "product.cast.tagline",
      "product.cast.blurb",
      "product.cast.p1",
      "product.cast.p2",
    ],
    claves: [...pp("cast"), ...pf("cast", 11, [2, 9])],
  },
  erp: {
    producto: "erp",
    corta: ["product.erp.tagline", "product.erp.blurb"],
    entrada: "product.erp.blurb",
    claves: [...pp("erp"), ...pf("erp", 10)],
  },
  // f3 habla del teclado y del lector de pantalla de la web; f5, de importar un CSV, y
  // f6, del registro de cambios: la app exporta pero no importa, y no enseña el registro.
  bases: {
    producto: "bases",
    corta: [
      "product.bases.blurb",
      "product.bases.tagline",
      "product.bases.p1",
      "product.bases.p3",
      "product.bases.p2",
    ],
    entrada: "product.bases.tagline",
    claves: [...pp("bases"), ...pf("bases", 6, [3, 5, 6])],
  },
};

// Cierre común a todas: lo que es verdad de la suite entera (inventario de lo que sale a
// terceros): cifrado en tránsito, infraestructura Sytue en la UE y ubicación de
// los servicios externos según su proveedor.
const CIERRE = "product.mail.p4";

const nombre = (app: string) =>
  /^\s*Name\s*=\s*"([^"]+)"/m.exec(
    readFileSync(join(AQUI, `../cmd/${app}/FyneApp.toml`), "utf8"),
  )![1];

for (const [suite, play] of Object.entries(IDIOMAS)) {
  const mod = await import(join(WWW, `${suite}.ts`));
  const cat = Object.values(mod)[0] as Record<string, string>;
  const t = (k: string) => {
    const v = cat[k];
    if (!v) throw new Error(`${suite}: falta ${k}`);
    return v.trim();
  };
  const nucleo: Record<string, string> = JSON.parse(
    readFileSync(join(NUCLEO, `${suite}.json`), "utf8"),
  );
  for (const [app, f] of Object.entries(FICHAS)) {
    const dir = join(AQUI, app, play);
    mkdirSync(dir, { recursive: true });
    const corta = f.corta.map(t).find((v) => [...v].length <= 80);
    if (!corta)
      throw new Error(`${suite}/${app}: ninguna descripción corta cabe en 80`);
    const entrada = f.entrada
      ? t(f.entrada)
      : [`product.${f.producto}.tagline`, `product.${f.producto}.blurb`]
          .map(t)
          .find((v) => v !== corta)!;
    const larga = [
      entrada,
      "",
      ...f.claves.map((k) => `• ${t(k)}`),
      "",
      t(CIERRE),
    ].join("\n");
    writeFileSync(join(dir, "title.txt"), nombre(app) + "\n");
    writeFileSync(join(dir, "short_description.txt"), corta + "\n");
    writeFileSync(join(dir, "full_description.txt"), larga + "\n");

    // ---- App Store: los mismos textos, con sus nombres y sus límites.
    const apple = APPLE[suite];
    if (!apple) continue;
    const dirA = join(APPSTORE, app, apple);
    mkdirSync(dirA, { recursive: true });
    const breve = nucleo[`app.${f.producto}.description`]?.trim() ?? "";
    // Subtítulo (≤ 30): lo que el Hub pone bajo el nombre de la app, o la corta de Play
    // si cabe. Es opcional: en el idioma en que nada cabe no se escribe, antes que recortar.
    const sub = [breve, corta].find((v) => v && [...v].length <= 30);
    const fsub = join(dirA, "subtitle.txt");
    if (sub) writeFileSync(fsub, sub.replace(/\.$/, "") + "\n");
    else rmSync(fsub, { force: true });
    // Palabras clave (≤ 100, con sus comas): las siglas del producto y las palabras de su
    // descripción breve en ESE idioma, sin repetir las del nombre (Apple ya indexa el nombre).
    const delNombre = nombre(app).toLowerCase().split(/\s+/);
    const claves: string[] = [];
    for (const c of [...SIGLAS[app], ...breve.split(/[^\p{L}\p{N}]+/u)]) {
      const yaEsta = claves.some((x) => x.toLowerCase() === c.toLowerCase());
      if ([...c].length < 4 && !SIGLAS[app].includes(c)) continue;
      if (yaEsta || delNombre.includes(c.toLowerCase())) continue;
      if ([...[...claves, c].join(",")].length > 100) break;
      claves.push(c);
    }
    writeFileSync(join(dirA, "name.txt"), nombre(app) + "\n");
    writeFileSync(join(dirA, "promotional_text.txt"), corta + "\n");
    writeFileSync(join(dirA, "description.txt"), larga + "\n");
    writeFileSync(join(dirA, "keywords.txt"), claves.join(",") + "\n");
  }
}

// ------------------------------------------------------------ Seguridad de los datos
//
// El CSV de «Seguridad de los datos», en el formato de exportación de Play Console. Sale
// del inventario de lo que sale a terceros (medido el 2026-09-14): nada se VENDE ni se
// COMPARTE —Scaleway, Mollie y Sweego son encargados que tratan por nuestra cuenta, que
// Play no cuenta como «compartir»—, no hay SDK de terceros ni analítica, los avisos de
// Android llegan por el SSE del Hub (sin Firebase: ni identificador de dispositivo) y todo
// va cifrado en tránsito (TLS 1.3). Lo «recogido» es lo que la app manda a NUESTROS
// servidores para funcionar.

// Tipos de datos del formulario: pregunta → respuestas posibles.
const TIPOS: Record<string, string[]> = {
  PSL_DATA_TYPES_PERSONAL: [
    "PSL_NAME",
    "PSL_EMAIL",
    "PSL_USER_ACCOUNT",
    "PSL_ADDRESS",
    "PSL_PHONE",
    "PSL_RACE_ETHNICITY",
    "PSL_POLITICAL_RELIGIOUS",
    "PSL_SEXUAL_ORIENTATION_GENDER_IDENTITY",
    "PSL_OTHER_PERSONAL",
  ],
  PSL_DATA_TYPES_FINANCIAL: [
    "PSL_CREDIT_DEBIT_BANK_ACCOUNT_NUMBER",
    "PSL_PURCHASE_HISTORY",
    "PSL_CREDIT_SCORE",
    "PSL_OTHER",
  ],
  PSL_DATA_TYPES_LOCATION: ["PSL_APPROX_LOCATION", "PSL_PRECISE_LOCATION"],
  PSL_DATA_TYPES_SEARCH_AND_BROWSING: ["PSL_WEB_BROWSING_HISTORY"],
  PSL_DATA_TYPES_EMAIL_AND_TEXT: [
    "PSL_EMAILS",
    "PSL_SMS_CALL_LOG",
    "PSL_OTHER_MESSAGES",
  ],
  PSL_DATA_TYPES_PHOTOS_AND_VIDEOS: ["PSL_PHOTOS", "PSL_VIDEOS"],
  PSL_DATA_TYPES_AUDIO: ["PSL_AUDIO", "PSL_MUSIC", "PSL_OTHER_AUDIO"],
  PSL_DATA_TYPES_HEALTH_AND_FITNESS: ["PSL_HEALTH", "PSL_FITNESS"],
  PSL_DATA_TYPES_CONTACTS: ["PSL_CONTACTS"],
  PSL_DATA_TYPES_CALENDAR: ["PSL_CALENDAR"],
  PSL_DATA_TYPES_APP_PERFORMANCE: [
    "PSL_CRASH_LOGS",
    "PSL_PERFORMANCE_DIAGNOSTICS",
    "PSL_OTHER_PERFORMANCE",
  ],
  PSL_DATA_TYPES_FILES_AND_DOCS: ["PSL_FILES_AND_DOCS"],
  PSL_DATA_TYPES_APP_ACTIVITY: [
    "PSL_USER_INTERACTION",
    "PSL_IN_APP_SEARCH_HISTORY",
    "PSL_APPS_ON_DEVICE",
    "PSL_USER_GENERATED_CONTENT",
    "PSL_OTHER_APP_ACTIVITY",
  ],
  PSL_DATA_TYPES_IDENTIFIERS: ["PSL_DEVICE_ID"],
};
const PROPOSITOS = [
  "PSL_APP_FUNCTIONALITY",
  "PSL_ANALYTICS",
  "PSL_DEVELOPER_COMMUNICATIONS",
  "PSL_FRAUD_PREVENTION_SECURITY",
  "PSL_ADVERTISING",
  "PSL_PERSONALIZATION",
  "PSL_ACCOUNT_MANAGEMENT",
];

// Un dato recogido: si solo se trata al vuelo (efímero), si el usuario puede no darlo
// (opcional) y para qué.
type Dato = { efimero?: boolean; opcional?: boolean; para: string[] };
const FUNCION: Dato = { para: ["PSL_APP_FUNCTIONALITY"] };
// La cuenta de Sytue ID, en todas: sin ella no hay app.
const CUENTA: Record<string, Dato> = {
  PSL_NAME: { para: ["PSL_APP_FUNCTIONALITY", "PSL_ACCOUNT_MANAGEMENT"] },
  PSL_EMAIL: { para: ["PSL_APP_FUNCTIONALITY", "PSL_ACCOUNT_MANAGEMENT"] },
  PSL_USER_ACCOUNT: {
    para: ["PSL_APP_FUNCTIONALITY", "PSL_ACCOUNT_MANAGEMENT"],
  },
  // La búsqueda de cada app pregunta al servidor y no se guarda.
  PSL_IN_APP_SEARCH_HISTORY: { efimero: true, para: ["PSL_APP_FUNCTIONALITY"] },
};
// Lo propio de cada app, además de la cuenta.
const DATOS: Record<string, Record<string, Dato>> = {
  // El alta guiada del Hub pide el móvil y lo verifica por SMS (Sweego, encargado).
  hub: {
    PSL_PHONE: {
      opcional: true,
      para: ["PSL_ACCOUNT_MANAGEMENT", "PSL_FRAUD_PREVENTION_SECURITY"],
    },
  },
  mail: {
    PSL_EMAILS: FUNCION,
    PSL_FILES_AND_DOCS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
  },
  drive: {
    PSL_FILES_AND_DOCS: FUNCION,
    PSL_PHOTOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_VIDEOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
  },
  docus: { PSL_FILES_AND_DOCS: FUNCION },
  calcus: { PSL_FILES_AND_DOCS: FUNCION },
  diapos: { PSL_FILES_AND_DOCS: FUNCION },
  certipdf: { PSL_FILES_AND_DOCS: FUNCION },
  calendar: { PSL_CALENDAR: FUNCION },
  contacts: { PSL_CONTACTS: FUNCION },
  notes: {
    PSL_USER_GENERATED_CONTENT: FUNCION,
    PSL_FILES_AND_DOCS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
  },
  photos: { PSL_PHOTOS: FUNCION },
  // La voz y la cámara (o la pantalla) de una llamada van al SFU. No son efímeras: desde
  // el teléfono también se graba la reunión (`RecordMeeting`), y la grabación se guarda.
  rooms: {
    PSL_OTHER_MESSAGES: FUNCION,
    PSL_AUDIO: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_VIDEOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_PHOTOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_FILES_AND_DOCS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
  },
  // Los vídeos que se suben o se emiten (la grabación del directo se queda como vídeo), con
  // su audio; el póster propio, si se sube; el título y la descripción; y el chat y los
  // comentarios. Todo opcional: se puede ver sin subir, emitir ni escribir nada. La cámara
  // y el micrófono solo se abren al emitir.
  cast: {
    PSL_VIDEOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_AUDIO: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_PHOTOS: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    PSL_USER_GENERATED_CONTENT: {
      opcional: true,
      para: ["PSL_APP_FUNCTIONALITY"],
    },
    PSL_OTHER_MESSAGES: { opcional: true, para: ["PSL_APP_FUNCTIONALITY"] },
    // Quién está viendo un directo: `live_viewers` lo guarda mientras dura (el recuento y
    // el pico que ve quien emite), así que ni es efímero ni se puede evitar si se mira. La
    // vista de un vídeo se cuenta con un hash con sal de (persona o IP, vídeo, hora) en
    // memoria (services/cast/internal/service/vistas.go). La clave de emisión la crea el
    // servidor: no es un dato que se recoja del usuario.
    PSL_USER_INTERACTION: FUNCION,
  },
  // Los libros, facturas y movimientos de la organización.
  erp: { PSL_OTHER: FUNCION },
  // Lo que la organización escribe en sus tablas. Los adjuntos no se suben desde la app.
  bases: { PSL_USER_GENERATED_CONTENT: FUNCION },
};

// Métodos de alta y respuestas de borrado, con las etiquetas de la plantilla de Play.
const ALTAS: Record<string, string> = {
  PSL_ACM_USER_ID_PASSWORD: "Username and password",
  PSL_ACM_USER_ID_OTHER_AUTH: "Username and other authentication",
  PSL_ACM_USER_ID_PASSWORD_OTHER_AUTH:
    "Username, password, and other authentication",
  PSL_ACM_OAUTH: "OAuth",
  PSL_ACM_OTHER: "Other",
  PSL_ACM_NONE: "My app does not allow users to create an account",
};
const BORRADO: Record<string, string> = {
  DATA_DELETION_YES: "Yes",
  DATA_DELETION_NO: "No",
  DATA_DELETION_NO_AUTO_DELETED:
    "No, but user data is automatically deleted within 90 days",
};
const BORRAR_CUENTA = "https://id.sytue.eu/account/delete";
const BORRAR_DATOS = "https://sytue.eu/privacidad";

// Tipo de dato de Play → «Categoría: tipo» de la App Store (App Privacy Details). Apple
// no tiene «gestión de la cuenta» como finalidad: autenticar es «App Functionality».
const APPLE_PRIVACIDAD: Record<string, string> = {
  PSL_NAME: "Contact Info: Name",
  PSL_EMAIL: "Contact Info: Email Address",
  PSL_PHONE: "Contact Info: Phone Number",
  PSL_USER_ACCOUNT: "Identifiers: User ID",
  PSL_EMAILS: "User Content: Emails or Text Messages",
  PSL_OTHER_MESSAGES: "User Content: Emails or Text Messages",
  PSL_PHOTOS: "User Content: Photos or Videos",
  PSL_VIDEOS: "User Content: Photos or Videos",
  PSL_AUDIO: "User Content: Audio Data",
  PSL_FILES_AND_DOCS: "User Content: Other User Content",
  PSL_USER_GENERATED_CONTENT: "User Content: Other User Content",
  PSL_CALENDAR: "User Content: Other User Content",
  PSL_CONTACTS: "Contacts: Contacts",
  PSL_USER_INTERACTION: "Usage Data: Product Interaction",
  PSL_OTHER: "Financial Info: Other Financial Info",
};

const csv = (celdas: string[]) =>
  celdas
    .map((c) => (/[",\n]/.test(c) ? `"${c.replaceAll('"', '""')}"` : c))
    .join(",");
for (const app of Object.keys(FICHAS)) {
  const datos = { ...CUENTA, ...DATOS[app] };
  const filas = [
    [
      "Question ID (machine readable)",
      "Response ID (machine readable)",
      "Response value",
      "Answer requirement",
      "Human-friendly question label",
    ],
    [
      "PSL_DATA_COLLECTION_COLLECTS_PERSONAL_DATA",
      "",
      "true",
      "REQUIRED",
      "Does your app collect or share any of the required user data types?",
    ],
    [
      "PSL_DATA_COLLECTION_ENCRYPTED_IN_TRANSIT",
      "",
      "true",
      "MAYBE_REQUIRED",
      "Is all of the user data collected by your app encrypted in transit?",
    ],
  ];
  // Cómo se crea la cuenta: en Sytue ID, con usuario y contraseña (la verificación del móvil
  // por SMS y las invitaciones de la organización no son otro método de alta). Sin esta
  // pregunta la API rechaza el CSV entero («Response missing for
  // PSL_SUPPORTED_ACCOUNT_CREATION_METHODS»).
  for (const [r, etiqueta] of Object.entries(ALTAS))
    filas.push([
      "PSL_SUPPORTED_ACCOUNT_CREATION_METHODS",
      r,
      r === "PSL_ACM_USER_ID_PASSWORD" ? "true" : "",
      "MULTIPLE_CHOICE",
      `Which of the following methods of account creation does your app support? Select all that apply / ${etiqueta}`,
    ]);
  // Si se puede crear cuenta, Play exige la web para pedir que se borre sin abrir la app.
  filas.push([
    "PSL_ACCOUNT_DELETION_URL",
    "",
    BORRAR_CUENTA,
    "MAYBE_REQUIRED",
    "Add a link that users can use to request that their account and associated data is deleted",
  ]);
  // Y la de borrar datos SIN borrar la cuenta: la política de privacidad dice cómo ejercer la
  // supresión (RGPD, art. 17). Sin esta fila la API contesta «Response missing for
  // PSL_DATA_DELETION_URL» (Notes, 26/09).
  filas.push([
    "PSL_DATA_DELETION_URL",
    "",
    BORRAR_DATOS,
    "MAYBE_REQUIRED",
    "Add a link that users can use to request that some or all of their data is deleted, without requiring them to delete their account",
  ]);
  // Sustituye a PSL_DATA_COLLECTION_USER_REQUEST_DELETE, que ya no está en la plantilla.
  for (const [r, etiqueta] of Object.entries(BORRADO))
    filas.push([
      "PSL_SUPPORT_DATA_DELETION_BY_USER",
      r,
      r === "DATA_DELETION_YES" ? "true" : "",
      "SINGLE_CHOICE",
      `Do you provide a way for users to request that their data is deleted? / ${etiqueta}`,
    ]);
  for (const [pregunta, respuestas] of Object.entries(TIPOS))
    for (const r of respuestas)
      filas.push([
        pregunta,
        r,
        r in datos ? "true" : "",
        "MULTIPLE_CHOICE",
        `${pregunta} / ${r}`,
      ]);
  for (const [r, d] of Object.entries(datos)) {
    const q = `PSL_DATA_USAGE_RESPONSES:${r}`;
    filas.push([
      `${q}:PSL_DATA_USAGE_COLLECTION_AND_SHARING`,
      "PSL_DATA_USAGE_ONLY_COLLECTED",
      "true",
      "MULTIPLE_CHOICE",
      `${r}: collected`,
    ]);
    filas.push([
      `${q}:PSL_DATA_USAGE_COLLECTION_AND_SHARING`,
      "PSL_DATA_USAGE_ONLY_SHARED",
      "",
      "MULTIPLE_CHOICE",
      `${r}: shared`,
    ]);
    filas.push([
      `${q}:PSL_DATA_USAGE_EPHEMERAL`,
      "",
      d.efimero ? "true" : "false",
      "SINGLE_CHOICE",
      `${r}: processed ephemerally`,
    ]);
    filas.push([
      `${q}:DATA_USAGE_USER_CONTROL`,
      d.opcional
        ? "PSL_DATA_USAGE_USER_CONTROL_OPTIONAL"
        : "PSL_DATA_USAGE_USER_CONTROL_REQUIRED",
      "true",
      "SINGLE_CHOICE",
      `${r}: required or optional`,
    ]);
    for (const p of PROPOSITOS)
      filas.push([
        `${q}:DATA_USAGE_COLLECTION_PURPOSE`,
        p,
        d.para.includes(p) ? "true" : "",
        "MULTIPLE_CHOICE",
        `${r}: collection purpose`,
      ]);
  }
  writeFileSync(
    join(AQUI, app, "seguridad-datos.csv"),
    filas.map(csv).join("\n") + "\n",
  );

  // Las etiquetas de privacidad de la App Store («App Privacy»), de los MISMOS datos: lo
  // declarado a Apple no puede decir otra cosa que lo declarado a Google. La API de App
  // Store Connect no las admite: este fichero es lo que se marca a mano en la web.
  const etiquetas = new Map<string, string[]>();
  for (const [r, d] of Object.entries(datos)) {
    // Para Apple «recoger» es guardar más de lo que dura atender la petición: lo efímero
    // (la búsqueda) no se declara.
    if (d.efimero) continue;
    const tipo = APPLE_PRIVACIDAD[r];
    if (!tipo) throw new Error(`${app}: ${r} no tiene tipo de dato de Apple`);
    etiquetas.set(tipo, [...(etiquetas.get(tipo) ?? []), r]);
  }
  writeFileSync(
    join(APPSTORE, app, "privacidad.txt"),
    [
      "¿Recoge datos esta app? SÍ",
      "¿Se usan para seguimiento (tracking)? NO, ninguno",
      "",
      "Tipo de dato | Finalidad | Vinculado a la identidad | Seguimiento | Sale de (Play)",
      ...[...etiquetas].map(
        ([tipo, de]) =>
          `${tipo} | App Functionality | Sí | No | ${de.join(", ")}`,
      ),
    ].join("\n") + "\n",
  );
}
