/**
 * Los productos de Sytue que se publican como app MÓVIL independiente.
 *
 * Cada uno es su propia app en App Store y Play Store: su identificador de paquete, su
 * nombre, su icono y su esquema de deep link. Lo que comparten es este proyecto Capacitor
 * —igual que las de escritorio comparten apps/nucleo—, porque diez proyectos
 * serían diez package.json, diez node_modules y diez sitios donde repetir cada arreglo.
 *
 * Se elige con SYTUE_APP:
 *
 *   SYTUE_APP=mail npx cap sync android    # produce Sytue Mail
 *   npx cap sync android                   # sin variable: la suite (Sytue One)
 *
 * Rooms no está: vive en apps/mobile/rooms, con push nativo, badge de no leídos y
 * su propio canal nativo, que el resto no necesita.
 *
 * La tabla debe cuadrar con el `esquema` de cada apps/fyne/cmd/<app>/FyneApp.toml y
 * con PRODUCT_SCHEMES de @sytue/shell: son el mismo contrato visto desde tres sitios.
 */

export interface Product {
  /** Clave del producto, y valor de SYTUE_APP. */
  key: string;
  /** Identificador de paquete: el que ve la tienda. Único por app. */
  appId: string;
  /** Nombre visible bajo el icono. */
  appName: string;
  /** Subdominio dentro del apex de la suite: «mail» → https://mail.sytue.eu. */
  host: string;
  /**
   * Ruta inicial. Es lo que distingue a Sheets de Slides: ambas viven en docs.* y
   * arrancan filtradas por tipo (la web lee ?kind=).
   */
  path?: string;
  /** Esquema de deep link PROPIO. Dos apps no pueden compartirlo. */
  scheme: string;
  /** SVG de marca del que salen icono y splash (packages/ui/src/assets/icons). */
  icon: string;
}

export const PRODUCTS: Product[] = [
  {
    key: "one",
    appId: "eu.sytue.one",
    appName: "Sytue One",
    host: "one",
    scheme: "sytue",
    icon: "one",
  },
  {
    key: "mail",
    appId: "eu.sytue.mail",
    appName: "Sytue Mail",
    host: "mail",
    scheme: "sytue-mail",
    icon: "mail",
  },
  {
    key: "drive",
    appId: "eu.sytue.drive",
    appName: "Sytue Drive",
    host: "drive",
    scheme: "sytue-drive",
    icon: "drive",
  },
  {
    key: "docs",
    appId: "eu.sytue.docs",
    appName: "Sytue Docus",
    host: "docus",
    path: "/docus/",
    scheme: "sytue-docs",
    icon: "docs",
  },
  {
    key: "sheets",
    appId: "eu.sytue.sheets",
    appName: "Sytue Calcus",
    host: "calcus",
    path: "/calcus/",
    scheme: "sytue-sheets",
    icon: "sheets",
  },
  {
    key: "slides",
    appId: "eu.sytue.slides",
    appName: "Sytue Diapos",
    host: "diapos",
    path: "/diapos/",
    scheme: "sytue-slides",
    icon: "slides",
  },
  {
    key: "pdf",
    appId: "eu.sytue.pdf",
    appName: "Sytue certiDocs",
    host: "certipdf",
    path: "/certipdf/",
    scheme: "sytue-pdf",
    icon: "pdf",
  },
  {
    key: "calendar",
    appId: "eu.sytue.calendar",
    appName: "Sytue Calendar",
    host: "calendar",
    scheme: "sytue-calendar",
    icon: "calendar",
  },
  {
    key: "contacts",
    appId: "eu.sytue.contacts",
    appName: "Sytue Contacts",
    host: "contacts",
    scheme: "sytue-contacts",
    icon: "contacts",
  },
  {
    key: "notes",
    appId: "eu.sytue.notes",
    appName: "Sytue Notepads",
    host: "notes",
    scheme: "sytue-notes",
    icon: "notes",
  },
  {
    key: "photos",
    appId: "eu.sytue.photos",
    appName: "Sytue Photos",
    host: "photos",
    scheme: "sytue-photos",
    icon: "photos",
  },
  {
    key: "erp",
    appId: "eu.sytue.erp",
    appName: "Sytue ERP",
    host: "erp",
    scheme: "sytue-erp",
    icon: "erp",
  },
  {
    key: "cloud",
    appId: "eu.sytue.cloud",
    appName: "Sytue Cloud",
    host: "cloud",
    scheme: "sytue-cloud",
    icon: "cloud",
  },
  {
    key: "admin",
    appId: "eu.sytue.admin",
    appName: "Sytue Admin",
    host: "admin",
    scheme: "sytue-admin",
    icon: "console",
  },
  {
    key: "cast",
    appId: "eu.sytue.cast",
    appName: "Sytue Cast",
    host: "cast",
    scheme: "sytue-cast",
    icon: "cast",
  },
  {
    key: "bases",
    appId: "eu.sytue.bases",
    appName: "Sytue Bases",
    host: "bases",
    scheme: "sytue-bases",
    icon: "bases",
  },
];

/**
 * Hosts que pueden permanecer dentro del WebView autenticado. Es una lista
 * cerrada: los sitios de usuarios bajo *.apps/preview.sytue.eu se abren fuera
 * y nunca comparten la confianza del contenedor nativo.
 */
const FIRST_PARTY_LABELS = [
  "",
  "www",
  "one",
  "portal",
  "admin",
  "console",
  "id",
  "hub",
  "mail",
  "drive",
  "docs",
  "calendar",
  "contacts",
  "notes",
  "rooms",
  "cloud",
  "integrations",
  "mta-sts",
] as const;

export function suiteApex(suiteURL: string): "sytue.eu" | "sytue.localhost" {
  let parsed: URL;
  try {
    parsed = new URL(suiteURL);
  } catch {
    return "sytue.eu";
  }
  const host = parsed.hostname.toLowerCase();
  if (host === "sytue.localhost" || host.endsWith(".sytue.localhost")) {
    return "sytue.localhost";
  }
  return "sytue.eu";
}

export function trustedSuiteHosts(
  apex: "sytue.eu" | "sytue.localhost",
): string[] {
  return FIRST_PARTY_LABELS.map((label) =>
    label === "" ? apex : `${label}.${apex}`,
  );
}

/**
 * activeProduct resuelve el producto que se está construyendo. Una clave desconocida
 * FALLA en vez de caer a la suite: construir en silencio la app equivocada y subirla a la
 * tienda es mucho peor que un error aquí.
 */
export function activeProduct(): Product {
  const key = (process.env.SYTUE_APP ?? "one").trim().toLowerCase();
  const p = PRODUCTS.find((x) => x.key === key);
  if (!p) {
    throw new Error(
      `SYTUE_APP="${key}" no es un producto conocido. Válidos: ${PRODUCTS.map((x) => x.key).join(", ")}`,
    );
  }
  return p;
}

/**
 * productURL compone la URL que carga el webview. El apex se DERIVA de SYTUE_URL
 * (`one.sytue.localhost` → `sytue.localhost`) en vez de fijarse, para que el mismo
 * proyecto sirva en producción y en el entorno local sin ramificar.
 */
export function productURL(p: Product, suiteURL: string): string {
  let base: URL;
  try {
    base = new URL(suiteURL);
  } catch {
    base = new URL("https://one.sytue.eu");
  }
  const apex = suiteApex(base.toString());
  const baseIsTrusted = trustedSuiteHosts(apex).includes(
    base.hostname.toLowerCase(),
  );
  const secureScheme =
    base.protocol === "https:" ||
    (base.protocol === "http:" && apex === "sytue.localhost");
  if (!baseIsTrusted || !secureScheme || base.username || base.password) {
    base = new URL("https://one.sytue.eu");
  }
  const effectiveApex = suiteApex(base.toString());
  const out = new URL(
    `${base.protocol}//${p.host}.${effectiveApex}${base.port ? `:${base.port}` : ""}`,
  );
  if (p.path) {
    const rel = new URL(p.path, out);
    out.pathname = rel.pathname;
    out.search = rel.search;
  }
  return out.toString();
}
