/*
 * Proveedor de datos de Sytue Analytics para el panel de Cloud (SOLO servidor).
 *
 * Habla con la API de lectura de `services/analytics` (:8095) con el access token
 * de la sesión, igual que lib/cloud.ts con el control-plane. NUNCA expone el token
 * al cliente. docs/CONVENTIONS.md §8, §14.
 *
 * Lo que este fichero NO hace, y es lo importante: no existe un endpoint que
 * devuelva un visitante, porque no hay nada que devolver. Los agregados no
 * contienen identificadores (ADR 0026). Aquí solo hay recuentos.
 *
 * `visitorsAreDailySum` viaja hasta la UI a propósito: la API lo marca en cada
 * respuesta porque «visitantes» es la SUMA DE ÚNICOS DIARIOS y no un único de
 * rango deduplicado —deduplicar entre días exigiría correlacionar identificadores
 * que ya no existen, la sal de ayer se destruyó—. Enseñarlo sin decirlo sería
 * inflar la cifra.
 */
import { HttpClient, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { serverEnv } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);

/** Sitio medido: un hostname del cliente. */
export interface AnalyticsSite {
  id: string;
  host: string;
  eventAllowlist: string[];
  createdAt: string;
}

export interface AnalyticsTotals {
  pageviews: number;
  visitors: number;
  visits: number;
  bounces: number;
  events: number;
  days: number;
}

export interface AnalyticsPoint {
  /** ISO del comienzo del intervalo (día u hora). */
  at: string;
  pageviews: number;
  visitors: number;
}

export interface AnalyticsRow {
  value: string;
  pageviews: number;
  visitors: number;
}

/** Todo lo que la pantalla necesita, en una sola carga del servidor. */
export interface AnalyticsView {
  /** `false` = no hay servicio de analítica cableado (dev sin ANALYTICS_API_URL). */
  live: boolean;
  /** Ausente = el host de este proyecto aún no se mide. */
  site?: AnalyticsSite;
  totals?: AnalyticsTotals;
  points: AnalyticsPoint[];
  topPaths: AnalyticsRow[];
  topReferrers: AnalyticsRow[];
  /**
   * Acciones medidas SIN `pageview`: clic saliente, descarga, `mailto:`, `tel:` y
   * los eventos que el sitio haya declarado. El rollup ya las materializaba en la
   * dimension `event` y nada las pedia, asi que Sytue contaba las descargas de
   * todos sus clientes y no se las ensenaba a ninguno.
   */
  topEvents: AnalyticsRow[];
  /**
   * Desglose por país, navegador, SO, dispositivo, clic saliente, descarga y
   * tres campañas UTM (source/medium/campaign). El rollup ya las agrega TODAS
   * —14 dimensiones, `Dimensions` en store/read.go, con `GET /dimensions` para
   * descubrirlas— y este panel solo pedía `path`, `referrer` y `event`, así que
   * un proyecto no podía ver de qué país venía su tráfico ni el rendimiento de
   * una campaña, aunque el dato ya estaba calculado y guardado.
   *
   * ponytail: quedan fuera `utm_content`/`utm_term` (los dos parámetros UTM
   * menos usados) y `ref` (alias corto de campaña, redundante con
   * `utm_campaign` para casi todo el mundo) — añadir sus dos tarjetas cuando
   * alguien los pida de verdad, el patrón es el mismo que los nueve de aquí.
   */
  topCountries: AnalyticsRow[];
  topBrowsers: AnalyticsRow[];
  topOs: AnalyticsRow[];
  topDevices: AnalyticsRow[];
  topOutbound: AnalyticsRow[];
  topDownloads: AnalyticsRow[];
  topUtmSource: AnalyticsRow[];
  topUtmMedium: AnalyticsRow[];
  topUtmCampaign: AnalyticsRow[];
  /** Ver la cabecera del fichero: obliga a etiquetar la cifra de visitantes. */
  visitorsAreDailySum: boolean;
}

/**
 * Hostnames que un proyecto puede reclamar para medir.
 *
 * Pura y aparte del endpoint porque es la comprobación de propiedad, y esa es la
 * que separa «medir mi web» de «reclamar la de otro»: el host es ÚNICO GLOBAL en
 * Analytics, así que quien lo registra primero deja al dueño sin poder medirlo.
 *
 * Devuelve `null` —y no un conjunto vacío— cuando alguna de las dos cargas no
 * viene del control-plane vivo: ambas degradan al seed de demostración, que trae
 * dominios de ejemplo, y aceptarlos convertiría una caída del control-plane en un
 * permiso.
 */
export function ownedHostsFrom(
  project: { live: boolean; data: { productionUrl?: string } } | undefined,
  domains: { live: boolean; data: { host?: string; verified?: boolean }[] },
): Set<string> | null {
  if (!project?.live || !domains.live) return null;
  const hosts = new Set<string>();
  if (project.data.productionUrl) {
    try {
      hosts.add(new URL(project.data.productionUrl).hostname.toLowerCase());
    } catch {
      /* una URL ilegible del control-plane no autoriza nada */
    }
  }
  for (const d of domains.data) {
    // Solo los verificados: un dominio «pendiente» es una intención, no una
    // propiedad demostrada, y medirlo sería exactamente el secuestro de host.
    if (d.verified && d.host) hosts.add(d.host.toLowerCase());
  }
  return hosts;
}

/** URL base de la API de lectura de Analytics. Vacía = la función está apagada. */
export function analyticsBaseUrl(): string {
  return env.SYTUE_ANALYTICS_API_URL ?? '';
}

function http(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${analyticsBaseUrl().replace(/\/$/, '')}/api/v1`,
    auth: bearer(session.tokens.accessToken),
  });
}

/** Rango [desde, hasta] en fechas ISO, con `hasta` = hoy. */
function range(days: number): { from: string; to: string } {
  const to = new Date();
  const from = new Date(to.getTime() - (days - 1) * 86_400_000);
  return { from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10) };
}

/**
 * Sitio que mide un hostname concreto, si existe.
 *
 * Se resuelve listando y filtrando porque la API no tiene búsqueda por host: son
 * los sitios de UNA organización, así que la lista es corta por construcción.
 */
async function findSite(session: SytueSession, host: string): Promise<AnalyticsSite | undefined> {
  const res = await http(session).json<{ sites?: RawSite[] }>({ path: '/sites' });
  return (res.sites ?? []).map(mapSite).find((s) => s.host === host.toLowerCase());
}

interface RawSite {
  id: string;
  host: string;
  event_allowlist?: string[] | null;
  created_at: string;
}

function mapSite(s: RawSite): AnalyticsSite {
  return {
    id: s.id,
    host: s.host,
    eventAllowlist: s.event_allowlist ?? [],
    createdAt: s.created_at,
  };
}

/**
 * Carga la vista completa de un hostname. Nunca lanza: la analítica es
 * secundaria en esta pantalla y un fallo suyo no puede dejar sin panel a un
 * proyecto que funciona.
 */
export async function loadAnalytics(
  session: SytueSession,
  host: string,
  days = 30,
): Promise<AnalyticsView> {
  const empty: AnalyticsView = {
    live: false,
    points: [],
    topPaths: [],
    topReferrers: [],
    topEvents: [],
    topCountries: [],
    topBrowsers: [],
    topOs: [],
    topDevices: [],
    topOutbound: [],
    topDownloads: [],
    topUtmSource: [],
    topUtmMedium: [],
    topUtmCampaign: [],
    visitorsAreDailySum: true,
  };
  if (!analyticsBaseUrl() || !host) return empty;

  try {
    const site = await findSite(session, host);
    if (!site) return { ...empty, live: true };

    const { from, to } = range(days);
    const q = { from, to };
    const c = http(session);
    const breakdown = (dim: string, limit: string) =>
      c
        .json<{ rows?: AnalyticsRow[] }>({
          path: `/sites/${encodeURIComponent(site.id)}/breakdown`,
          query: { ...q, dim, limit },
        })
        .then((r) => r.rows ?? []);
    const [
      summary,
      series,
      topPaths,
      topReferrers,
      topEvents,
      topCountries,
      topBrowsers,
      topOs,
      topDevices,
      topOutbound,
      topDownloads,
      topUtmSource,
      topUtmMedium,
      topUtmCampaign,
    ] = await Promise.all([
      c.json<{ totals: AnalyticsTotals; visitors_are_daily_sum?: boolean }>({
        path: `/sites/${encodeURIComponent(site.id)}/summary`,
        query: q,
      }),
      c.json<{ points?: AnalyticsPoint[] }>({
        path: `/sites/${encodeURIComponent(site.id)}/timeseries`,
        query: q,
      }),
      breakdown('path', '8'),
      breakdown('referrer', '5'),
      // Sin filtrar `pageview`: el rollup ya lo excluye de la dimension `event`
      // (`WHERE name <> 'pageview'` en store/rollup.go), asi que la lista viene con
      // acciones y nada mas. El campo `pageviews` de cada fila es el RECUENTO de esa
      // accion, no paginas vistas: el nombre de la columna es generico.
      breakdown('event', '8'),
      breakdown('country', '8'),
      breakdown('browser', '6'),
      breakdown('os', '6'),
      breakdown('device', '4'),
      breakdown('outbound', '8'),
      breakdown('download', '8'),
      breakdown('utm_source', '6'),
      breakdown('utm_medium', '6'),
      breakdown('utm_campaign', '6'),
    ]);
    return {
      live: true,
      site,
      totals: summary.totals,
      points: series.points ?? [],
      topPaths,
      topReferrers,
      topEvents,
      topCountries,
      topBrowsers,
      topOs,
      topDevices,
      topOutbound,
      topDownloads,
      topUtmSource,
      topUtmMedium,
      topUtmCampaign,
      visitorsAreDailySum: summary.visitors_are_daily_sum !== false,
    };
  } catch {
    // Sin distinguir causa a propósito: para esta pantalla «no hay analítica» y
    // «la analítica falló» se resuelven igual —se ofrece activarla— y separarlos
    // solo añadiría un estado de error que el usuario no puede accionar.
    return empty;
  }
}

/** Empieza a medir un hostname. Devuelve el sitio creado. */
export async function enableAnalytics(session: SytueSession, host: string): Promise<AnalyticsSite> {
  const raw = await http(session).postJson<RawSite>('/sites', { host, event_allowlist: [] });
  return mapSite(raw);
}

/**
 * Declara qué nombres de evento se aceptan de `data-sytue-event`.
 *
 * Existe la verja porque sin ella `data-sytue-event="pepe@correo.com"` convierte
 * un almacén anónimo en un canal de exfiltración: `domain.CleanEventName` descarta
 * todo nombre que no sea automático ni esté declarado aquí. Los automáticos
 * —saliente, descarga, mailto, tel— NO se declaran y no se pueden quitar.
 */
export async function updateEventAllowlist(
  session: SytueSession,
  siteId: string,
  names: string[],
): Promise<AnalyticsSite> {
  const raw = await http(session).json<RawSite>({
    method: 'PUT',
    path: `/sites/${encodeURIComponent(siteId)}/events`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ event_allowlist: names }),
  });
  return mapSite(raw);
}

/** Deja de medir: borra el sitio y, con él, sus agregados. */
export async function disableAnalytics(session: SytueSession, siteId: string): Promise<void> {
  await http(session).json<void>({
    method: 'DELETE',
    path: `/sites/${encodeURIComponent(siteId)}`,
  });
}
