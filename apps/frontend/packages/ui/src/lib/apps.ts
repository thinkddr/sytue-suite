/**
 * Catalogo de apps de la suite para el AppGrid/Launcher (app switcher de 9+
 * puntos). Orden canonico segun branding/index.json (appSwitcher.order).
 * Las URLs y colores provienen de @sytue/config (tokens). docs/design.md §9.
 */
import { appDomains, appUrl, prefijoDeApp, productColor, type ProductKey } from '@sytue/config';
import { productIcons, productIconsSm } from '../assets/icons.js';

export interface SuiteApp {
  key: ProductKey;
  /** Nombre visible. Es marca —«Sytue Mail»—, así que NO se traduce. */
  name: string;
  href: string;
  iconUrl: string;
  /** Icono en tamaño óptico pequeño (15-20 px): trazo 2.1 y detalle podado. */
  iconUrlSm: string;
  color: string;
  /** Si la app está desactivada (p. ej. sin permisos o plan insuficiente). */
  disabled?: boolean;
  /** Descripción opcional de la app o estado traducido. */
  description?: string;
}

/**
 * A qué entrada pertenece un producto (nomenclatura canónica v4).
 *
 * `hub` es la ENTRADA COMÚN (lanzador, búsqueda, notificaciones, presencia) y
 * `workhub` la suite OFIMÁTICA. Son cosas distintas y conviene no mezclarlas: el
 * error de llamar «Docs» a las dos es justo lo que esta tabla deshace.
 */
export type SuiteV4 = 'hub' | 'workhub' | 'autonomo';

/**
 * Disponibilidad REAL de un producto. No es decoración: `www` no puede prometer
 * lo que no hay (encargo v4 §14, «no inventar estados»), y esta es la única
 * fuente que decide qué se presenta como disponible.
 */
export type Disponibilidad = 'disponible' | 'piloto' | 'beta' | 'desarrollo' | 'no-disponible';

/**
 * Una entrada del catálogo canónico. Es la FUENTE DE VERDAD de la que beben el
 * conmutador, el lanzador del Hub, el sitio público y los instaladores
 * (encargo v4 §14: «un catálogo reutilizable como fuente de verdad»).
 */
export interface EntradaCatalogo {
  /** Clave TÉCNICA: subdominio, client_id de OIDC, chart, columna. NO es el nombre. */
  key: ProductKey;
  /** Nombre visible v4. Es marca, así que NO se traduce. */
  name: string;
  suite: SuiteV4;
  /**
   * Puerta del producto dentro de su host, cuando no es la raíz de su prefijo. Con
   * ruta y no la raíz: el ingress solo redirige la raíz DESNUDA al prefijo
   * (`^https?://host/?$`), así que una ruta propia tiene que ir ya con el prefijo, y
   * el host de la organización lo conserva al redirigir (medido el 2026-09-09).
   */
  ruta?: string;
  /** Despliegue que lo sostiene, si es el de otro producto (el servicio de Docs). */
  despliegue?: ProductKey;
  disponibilidad: Disponibilidad;
  /** Si se publica como app NATIVA: tiene su `apps/fyne/cmd/<app>` (lo ata un test). */
  nativa: boolean;
}

/**
 * EL CATÁLOGO. Nomenclatura canónica v4 (encargo del 2026-09-21) y el porqué de
 * lo que NO se renombró:
 *
 * - El portal se llama **Sytue Hub**. Conserva la clave `one` y la ruta `/one`:
 *   cambiar la clave arrastraría client_id de OIDC, ingress y enlaces guardados,
 *   y lo que se quería cambiar era el nombre.
 * - **Sytue WorkHub** es la suite ofimática (Docus, Calcus, Diapos, CertiPDF,
 *   Photos y Notepads). No tiene clave propia porque no es un despliegue: es la
 *   agrupación con la que se presenta. Hub ≠ WorkHub.
 * - `pdf` se llama **Sytue CertiPDF** (antes «CertiPDF»). La clave sigue siendo
 *   `pdf`: es la del token de color, del esquema nativo y de la columna del tipo; su
 *   host es `certipdf.*` desde el 2026-10-12 (ADR 0076), como `docus.*`, `calcus.*` y
 *   `diapos.*` para `docs`, `sheets` y `slides`.
 * - `cloud` se llama **Sytue DevX** (antes «Sytue DevX»). La clave sigue siendo
 *   `cloud`: es el subdominio, el client_id `sytue-cloud` y tres charts.
 * - `console` se llama **Sytue Admin** desde el 2026-09-09; su host ya es
 *   `admin.*` y `console.*` se retiró el 2026-09-11.
 *
 * La tabla de migración completa, con condición de retirada y prueba de cada
 * alias, está en `docs/architecture.md` §«Migración de nombres v4».
 */
const CATALOGO_ORDENADO: readonly EntradaCatalogo[] = [
  { key: 'one', name: 'Sytue Hub', suite: 'hub', disponibilidad: 'disponible', nativa: true },
  {
    key: 'mail',
    name: 'Sytue Mail',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'drive',
    name: 'Sytue Drive',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'calendar',
    name: 'Sytue Calendar',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'contacts',
    name: 'Sytue Contacts',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  // La ofimática: una app, un host y un prefijo por producto desde el 2026-10-12 (ADR
  // 0076), así que ya no hay `ruta`. El `despliegue` de Calcus, Diapos y CertiPDF sigue
  // siendo `docs` porque es el SERVICIO de documentos que los sostiene a los cuatro: si
  // la instalación no lo levanta, ninguno funciona.
  {
    key: 'docs',
    name: 'Sytue Docus',
    suite: 'workhub',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'sheets',
    name: 'Sytue Calcus',
    suite: 'workhub',
    despliegue: 'docs',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'slides',
    name: 'Sytue Diapos',
    suite: 'workhub',
    despliegue: 'docs',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'pdf',
    name: 'Sytue CertiPDF',
    suite: 'workhub',
    despliegue: 'docs',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'notes',
    name: 'Sytue Notepads',
    suite: 'workhub',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'photos',
    name: 'Sytue Photos',
    suite: 'workhub',
    disponibilidad: 'disponible',
    nativa: true,
  },
  {
    key: 'rooms',
    name: 'Sytue Rooms',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  // Sytue Cast (vídeo bajo demanda y directo, ADR 0061): servicio, web y app de Fyne
  // (`apps/fyne/cmd/cast`, en el paquete de instaladores `one`).
  {
    key: 'cast',
    name: 'Sytue Cast',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    nativa: true,
  },
  // En el conmutador y el Hub el prefijo «Sytue » se recorta para mostrar solo
  // «ERP»; en el pie público y el catálogo se muestra el nombre completo.
  { key: 'erp', name: 'Sytue ERP', suite: 'autonomo', disponibilidad: 'disponible', nativa: true },
  // Sytue Web (webs y tiendas de la organización, ADR 0062) entra en `desarrollo` y
  // solo web (decisión 9 del dueño): el integrador lo sube a `disponible` cuando el
  // servicio, la web y el servidor de sitios estén desplegados. Solo organizaciones
  // (`PRODUCTOS_SOLO_ORGANIZACION` de @sytue/config).
  {
    key: 'pages',
    name: 'Sytue Web',
    suite: 'autonomo',
    // La puerta es «Mis sitios», también sin org durante SSR.
    ruta: '',
    disponibilidad: 'desarrollo',
    nativa: false,
  },
  // Sytue Bases (tablas relacionales colaborativas, ADR 0068): `disponible` y encendida
  // por defecto desde el 2026-10-04 (decisión del dueño). Solo organizaciones
  // (`PRODUCTOS_SOLO_ORGANIZACION` de @sytue/config). Con app nativa desde el 2026-10-05
  // (`apps/fyne/cmd/bases`), que llega a la API por `bases.<dominio>/api/v1`.
  {
    key: 'bases',
    name: 'Sytue Bases',
    suite: 'autonomo',
    // La puerta es «Mis bases», también sin org durante SSR.
    ruta: '',
    disponibilidad: 'disponible',
    nativa: true,
  },
  // DevX se retiró del sitio público el 2026-09-15 «mientras dure la fase
  // piloto» (commit 8907c2f9f) y el umbrella lo instala solo con CLOUD_ENABLED:
  // presentarlo como disponible sería prometer lo que no hay.
  {
    key: 'cloud',
    name: 'Sytue DevX',
    suite: 'autonomo',
    disponibilidad: 'desarrollo',
    // Su app de escritorio era de Wails y se retiró con él (2026-09-23).
    nativa: false,
  },
  {
    key: 'console',
    name: 'Sytue Admin',
    suite: 'autonomo',
    disponibilidad: 'disponible',
    // Como DevX: la app de Wails se retiró y en Fyne no hay `cmd/` de Admin.
    nativa: false,
  },
  // Sytue ID no se instala: es el IdP, y se usa desde el navegador.
  { key: 'id', name: 'Sytue ID', suite: 'autonomo', disponibilidad: 'disponible', nativa: false },
];

/** El catálogo indexado por clave técnica. */
export const CATALOGO: ReadonlyMap<ProductKey, EntradaCatalogo> = new Map(
  CATALOGO_ORDENADO.map((e) => [e.key, e]),
);

/** Productos de Sytue WorkHub, en orden de catálogo. */
export const WORKHUB: readonly ProductKey[] = CATALOGO_ORDENADO.filter(
  (e) => e.suite === 'workhub',
).map((e) => e.key);

/** Orden canonico del app switcher (branding/index.json).
 *  Chat y Meet se unificaron en Rooms: 'rooms' es su UNICA tesela de presentacion.
 *  'chat'/'meet' siguen en los tokens para resolver el `source` del Hub via
 *  appForKey (el conector Hub aun emite source 'chat'), pero NO aparecen en el
 *  switcher, el Hub ni el sitio publico.
 *
 *  HUB, IGUAL: su superficie —actividad, notificaciones y búsqueda unificada— vive
 *  DENTRO de Sytue Hub, así que ofrecerlo como tesela aparte mandaba a la persona a
 *  un sitio que ya está donde acaba de entrar. Se resuelve por SOURCE_ALIAS. */
export const APP_SWITCHER_ORDER: readonly ProductKey[] = CATALOGO_ORDENADO.map((e) => e.key);

/**
 * Qué despliegue sostiene cada producto. El Hub enumera lo LEVANTADO por clave de
 * despliegue (`docs`, `rooms`…); los productos que viven dentro de otro se
 * presentan si ese otro está vivo.
 */
export const DESPLIEGUE: Partial<Record<ProductKey, ProductKey>> = {
  ...Object.fromEntries(
    CATALOGO_ORDENADO.filter((e) => e.despliegue).map((e) => [e.key, e.despliegue]),
  ),
  // Fuera del switcher, así que no tienen fila en el catálogo: Chat y Meet son Rooms.
  chat: 'rooms',
  meet: 'rooms',
};

/**
 * El href de la tesela de una app, con o sin organización (ADR 0056).
 *
 * Sin org —SSR, contexto sin sesión, o `orgActual` sin nada que leer en la URL
 * actual— sale la forma de siempre: el dominio pelado para las apps sin `ruta`
 * (Traefik redirige la raíz al prefijo) y dominio+prefijo para las que la
 * llevan. Org es opcional y null-safe a propósito: el catálogo también se
 * consume en SSR y fuera de sesión, donde no hay org que componer.
 *
 * Con org TODO sale canónico vía `appUrl`, que compone dominio+prefijo+org:
 * también las teselas sin `ruta`, que apuntando al dominio pelado pagaban el
 * redirect de Traefik a la raíz del prefijo MÁS el 302 del middleware del
 * destino. Y en un host SIN prefijo (`id`: lo sirve un binario Go con sus
 * rutas en la raíz) la org no se cuelga —no hay nada tras lo que escopiarla— y
 * queda como está: `prefijoDeApp` lo dice sin duplicar la tabla de prefijos.
 */
export const hrefDe = (key: ProductKey, org?: string | null): string => {
  const ruta = CATALOGO.get(key)?.ruta;
  if (!org || prefijoDeApp(key) === null) {
    return ruta === undefined ? appDomains[key] : appUrl(key, ruta);
  }
  return appUrl(key, ruta ?? '', org);
};

const teselaDe = (e: EntradaCatalogo): SuiteApp => ({
  key: e.key,
  name: e.name,
  href: hrefDe(e.key),
  iconUrl: productIcons[e.key],
  iconUrlSm: productIconsSm[e.key],
  color: productColor[e.key],
});

export const suiteApps: SuiteApp[] = CATALOGO_ORDENADO.map(teselaDe);

/**
 * Deja solo las aplicaciones que ESTA instalación levanta.
 *
 * La lista la da el Hub (`GET /api/v1/apps`), que la lee de su configuración: lo
 * sabe el despliegue, no el navegador. VACÍA o ausente = «no se sabe», y
 * entonces se presenta la suite entera —que es como se ha comportado siempre—:
 * es preferible ofrecer algo que no está a esconder media suite porque una
 * variable no llegó.
 *
 * `one` no se filtra nunca: es el propio Sytue Hub, la puerta de vuelta a casa
 * desde cualquier aplicación, y quedarse sin ella es peor que cualquier tesela
 * de más.
 */
export function appsPresentables(apps: SuiteApp[], desplegadas?: readonly string[]): SuiteApp[] {
  if (!desplegadas || desplegadas.length === 0) return apps;
  const vivas = new Set(desplegadas.map((k) => k.toLowerCase()));
  return apps.filter((a) => a.key === 'one' || vivas.has(DESPLIEGUE[a.key] ?? a.key));
}

/**
 * Índice por clave, del catálogo COMPLETO y no del presentado.
 *
 * Si se construyera con `suiteApps`, apagar DevX dejaría a `appForKey('cloud')`
 * devolviendo undefined, y con él se quedarían sin icono ni nombre una
 * notificación del Hub o un resultado de búsqueda etiquetados así, que pueden
 * llegar de todas formas —de otra organización, o de antes de apagarlo—. Una
 * cosa es no OFRECER DevX y otra no saber nombrarlo.
 */
const byKey = new Map<ProductKey, SuiteApp>(CATALOGO_ORDENADO.map((e) => [e.key, teselaDe(e)]));

/** Alias de origen -> producto de presentacion: Chat y Meet se unificaron en Rooms
 *  y la superficie del Hub vive dentro de la entrada comun, asi que un `source`
 *  'chat'/'meet' se muestra bajo Rooms y uno 'hub' bajo Sytue Hub. Sin esto, un
 *  resultado o una notificacion etiquetada 'hub' se quedaria sin icono ni nombre. */
const SOURCE_ALIAS: Record<string, ProductKey> = { chat: 'rooms', meet: 'rooms', hub: 'one' };

/** Resuelve una clave de producto u origen del Hub al SuiteApp de presentacion
 *  (aplicando el alias Chat/Meet -> Rooms). undefined si no es de la suite. */
export function appForKey(key: string): SuiteApp | undefined {
  return byKey.get((SOURCE_ALIAS[key] ?? key) as ProductKey);
}

/**
 * ¿Puede esta persona entrar en Sytue Admin?
 *
 * NO ES UNA COMPROBACIÓN DE SEGURIDAD y conviene decirlo antes de que alguien se
 * apoye en ella: el gate de verdad está en el servidor —`apps/console` exige
 * owner/admin, y el layout rechaza a quien no abra ninguna sección—. Esto solo
 * decide si se PINTA la tesela, porque ofrecer una puerta que va a dar un 403 es
 * mala interfaz, no un agujero.
 *
 * LOS ADMINISTRADORES DELEGADOS CUENTAN, y son la razón de que esto no sea un
 * `roles.includes('admin')` de una línea: quien tiene permisos granulares no es
 * `admin` ni `owner`, pero entra y trabaja. Esconderle la tesela lo dejaría sin
 * puerta a lo único que puede administrar.
 */
export function puedeAdministrar(
  roles: readonly string[] | undefined,
  perms: readonly string[] | undefined,
): boolean {
  const r = roles ?? [];
  return r.includes('owner') || r.includes('admin') || (perms?.length ?? 0) > 0;
}

/** Clave del producto Sytue Admin en el catálogo. */
export const CLAVE_ADMIN = 'console' as const;
