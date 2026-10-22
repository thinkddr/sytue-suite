/**
 * AnalyticsPanel — visitas del proyecto, activables con un clic.
 *
 * Dos estados y nada más: o el host no se mide y hay un botón, o se mide y hay
 * cifras. No hay configuración intermedia porque no hace falta: la ruta `/_sytue`
 * ya la publica el control-plane en el Ingress del proyecto, así que activar es
 * literalmente dar de alta el hostname.
 *
 * Ese botón es TODO el cableado, y esa es la parte que no se ve aquí: la etiqueta
 * `<script>` ya está en el HTML —Sytue DevX la inyecta en cada build de lo que
 * construye— y el colector devuelve 404 mientras el hostname no esté dado de alta.
 * Por eso activar no obliga a tocar el código ni a redesplegar: lo que cambia es
 * la respuesta del colector, no la app.
 *
 * Tres cosas que esta pantalla NO hace, y son deliberadas:
 *
 *  - **No pide aceptar cookies** ni ofrece un banner, porque no hay nada que
 *    consentir: la medición no usa cookies, no toca `localStorage` y el
 *    identificador diario se destruye con su sal (ADR 0026).
 *  - **No dice «visitantes únicos» a secas.** La cifra es la SUMA DE ÚNICOS
 *    DIARIOS y se etiqueta como tal. Deduplicar entre días exigiría conservar
 *    correlación, que es justo lo que la arquitectura impide.
 *  - **No hay exploración libre.** Las visitas por día, las páginas más vistas y
 *    de dónde llegan cubren la pregunta real; un explorador de dimensiones sería
 *    un informe, y los informes no los lee nadie.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  BarChart,
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  CardHeader,
  CardTitle,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { AnalyticsRow, AnalyticsSite, AnalyticsView } from '../lib/analytics.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, type BffOk } from '../lib/bff-client.js';
import { CopyField } from './CopyField.js';

interface Props {
  projectSlug: string;
  /** Hostname que se mide (o se ofrecería medir): la URL de producción del proyecto. */
  host: string;
  view: AnalyticsView;
  locale: Locale;
}

export default function AnalyticsPanel(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} t={useT(props.locale)} />
    </ToastProvider>
  );
}

/** Cifra grande con su etiqueta debajo. El número manda; el texto explica. */
function Kpi({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div class="min-w-28">
      <p class="text-2xl font-semibold tabular-nums">{value}</p>
      <p class="text-text-subtle text-sm">{label}</p>
      {hint ? <p class="sytue-meta mt-0.5">{hint}</p> : null}
    </div>
  );
}

/**
 * Lista con barra de proporción. La barra va DETRÁS del texto y no al lado: con
 * rutas largas, una columna de barras aparte obliga a leer en zigzag.
 */
function TopList({ rows, empty }: { rows: AnalyticsRow[]; empty: string }) {
  if (rows.length === 0) return <p class="text-text-subtle py-4 text-sm">{empty}</p>;
  const max = Math.max(1, ...rows.map((r) => r.pageviews));
  return (
    <ul class="flex flex-col gap-1">
      {rows.map((r) => (
        <li
          key={r.value}
          class="relative flex items-center justify-between gap-4 rounded px-2 py-1.5"
        >
          {/* `bg-selected` y no `accent-100`: el escalón crudo no se reasigna en
              tema oscuro y la barra salía como una franja clara con el texto
              encima. Este token se calcula con color-mix sobre la superficie
              vigente, así que sigue siendo un fondo tenue en los dos temas. */}
          <span
            class="bg-selected absolute inset-y-0 left-0 rounded"
            style={{ width: `${Math.max(2, Math.round((r.pageviews / max) * 100))}%` }}
            aria-hidden="true"
          />
          <span class="relative min-w-0 truncate text-sm">{r.value || '/'}</span>
          <span class="text-text-subtle relative shrink-0 text-sm tabular-nums">{r.pageviews}</span>
        </li>
      ))}
    </ul>
  );
}

function Inner({ projectSlug, host, view, locale, t }: Props & { t: T }) {
  const toast = useToast();
  const [site, setSite] = useState(view.site);
  const [busy, setBusy] = useState(false);
  // Los nombres declarados se editan como una lista separada por comas: son dos o
  // tres, y un editor de fichas para tres cadenas es mas UI que funcion.
  const [events, setEvents] = useState((view.site?.eventAllowlist ?? []).join(', '));
  const nf = new Intl.NumberFormat(locale);
  const df = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' });

  async function enable() {
    setBusy(true);
    try {
      const res = await postBff<BffOk & { site?: AnalyticsSite }>('/api/analytics', {
        project: projectSlug,
        host,
      });
      setSite(res.site);
      toast.notify({ tone: 'success', title: t('analytics.enabled', { host }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('analytics.enableFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    if (!site) return;
    setBusy(true);
    try {
      await postBff('/api/analytics', { site_id: site.id, delete: true });
      setSite(undefined);
      toast.notify({ tone: 'success', title: t('analytics.disabled') });
    } catch {
      toast.notify({ tone: 'danger', title: t('analytics.disableFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function saveEvents() {
    if (!site) return;
    setBusy(true);
    try {
      const names = events
        .split(',')
        .map((e) => e.trim())
        .filter(Boolean);
      const res = await postBff<BffOk & { site?: AnalyticsSite }>('/api/analytics', {
        site_id: site.id,
        events: names,
      });
      if (res.site) {
        setSite(res.site);
        setEvents(res.site.eventAllowlist.join(', '));
      }
      toast.notify({ tone: 'success', title: t('analytics.events.saved') });
    } catch {
      toast.notify({ tone: 'danger', title: t('analytics.events.saveFailed') });
    } finally {
      setBusy(false);
    }
  }

  // --- Analítica no cableada en este entorno ------------------------------
  if (!view.live) {
    return (
      <Card>
        <CardBody>
          <EmptyState
            illustration={<Icon name="chart" size={32} />}
            title={t('analytics.unavailable.title')}
            description={t('analytics.unavailable.desc')}
          />
        </CardBody>
      </Card>
    );
  }

  // --- Aún no se mide: un botón, y lo que va a pasar dicho antes ----------
  if (!site) {
    return (
      <Card>
        <CardHeader>
          <div class="min-w-0">
            <CardTitle as="h2">{t('analytics.off.title')}</CardTitle>
            <p class="text-text-muted mt-0.5 text-sm">{t('analytics.off.desc', { host })}</p>
          </div>
        </CardHeader>
        <CardBody>
          <InlineHelp tone="info" title={t('analytics.privacy.title')}>
            <ul class="list-disc pl-5 text-sm">
              <li>{t('analytics.privacy.noCookies')}</li>
              <li>{t('analytics.privacy.firstParty')}</li>
              <li>{t('analytics.privacy.daily')}</li>
            </ul>
          </InlineHelp>
          <div class="mt-4">
            <Button variant="primary" onClick={() => void enable()} disabled={busy}>
              {busy ? t('analytics.enabling') : t('analytics.enable')}
            </Button>
          </div>
        </CardBody>
      </Card>
    );
  }

  const totals = view.totals;
  const bounceRate =
    totals && totals.visits > 0 ? Math.round((totals.bounces / totals.visits) * 100) : 0;

  return (
    <div class="flex flex-col gap-4">
      <Card>
        <CardHeader class="flex-wrap items-start">
          <div class="min-w-0">
            <CardTitle as="h2">{t('analytics.on.title')}</CardTitle>
            <p class="text-text-muted mt-0.5 text-sm">
              {t('analytics.on.desc', { host: site.host, days: view.points.length })}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <Badge tone="success">{t('analytics.badge.active')}</Badge>
            <Button variant="ghost" onClick={() => void disable()} disabled={busy}>
              {t('analytics.disable')}
            </Button>
          </div>
        </CardHeader>
        <CardBody>
          <div class="flex flex-wrap gap-8">
            <Kpi label={t('analytics.kpi.pageviews')} value={nf.format(totals?.pageviews ?? 0)} />
            {/* La etiqueta honesta va pegada a la cifra, no en una nota al pie:
                sin ella el número parece un único de rango y está inflado. */}
            <Kpi
              label={t('analytics.kpi.visitors')}
              value={nf.format(totals?.visitors ?? 0)}
              {...(view.visitorsAreDailySum ? { hint: t('analytics.kpi.visitorsHint') } : {})}
            />
            <Kpi label={t('analytics.kpi.visits')} value={nf.format(totals?.visits ?? 0)} />
            <Kpi label={t('analytics.kpi.bounce')} value={`${bounceRate} %`} />
          </div>

          <div class="mt-6">
            <BarChart
              points={view.points.map((p) => ({
                label: df.format(new Date(p.at)),
                value: p.pageviews,
              }))}
              seriesLabel={t('analytics.kpi.pageviews')}
              emptyLabel={t('analytics.chart.empty')}
              tableHeaders={[t('analytics.chart.day'), t('analytics.kpi.pageviews')]}
              height={140}
            />
          </div>
        </CardBody>
      </Card>

      <div class="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <div class="min-w-0">
              <CardTitle as="h2">{t('analytics.top.paths')}</CardTitle>
            </div>
          </CardHeader>
          <CardBody>
            <div>
              <TopList rows={view.topPaths} empty={t('analytics.top.empty')} />
            </div>
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <div class="min-w-0">
              <CardTitle as="h2">{t('analytics.top.referrers')}</CardTitle>
            </div>
          </CardHeader>
          <CardBody>
            <div>
              <TopList rows={view.topReferrers} empty={t('analytics.top.empty')} />
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Acciones. La dimension `event` ya la materializaba el rollup y NADIE la
          pedia: Sytue contaba los clics salientes y las descargas de todos sus
          clientes sin ensenarselos a ninguno. Debajo, la verja que decide que
          nombres propios se aceptan: sin ella `data-sytue-event` seria un canal de
          exfiltracion, y con ella vacia —como estaba— no llega ni uno. */}
      <Card>
        <CardHeader>
          <div class="min-w-0">
            <CardTitle as="h2">{t('analytics.events.title')}</CardTitle>
            <p class="text-text-muted mt-0.5 text-sm">{t('analytics.events.desc')}</p>
          </div>
        </CardHeader>
        <CardBody>
          <div>
            <TopList rows={view.topEvents} empty={t('analytics.top.empty')} />
          </div>
          <div class="border-border-subtle mt-5 border-t pt-4">
            <FieldGroup label={t('analytics.events.allowlist')} htmlFor="an-events">
              <Input
                id="an-events"
                value={events}
                placeholder={t('analytics.events.placeholder')}
                aria-describedby="an-events-hint"
                onInput={(e) => setEvents(e.currentTarget.value)}
              />
              <FieldHint id="an-events-hint">{t('analytics.events.allowlistHint')}</FieldHint>
            </FieldGroup>
            <div class="mt-3">
              <Button
                variant="secondary"
                onClick={() => void saveEvents()}
                disabled={busy || events === site.eventAllowlist.join(', ')}
              >
                {t('analytics.events.save')}
              </Button>
            </div>
          </div>
        </CardBody>
      </Card>

      {/* Nueve dimensiones que el rollup ya agregaba y nadie pedía: geografía,
          cliente y campaña. Mismo patrón que Rutas/Orígenes de arriba (TopList),
          solo que antes esta pantalla se detenía en tres de las catorce que el
          servicio expone (`GET /dimensions`). */}
      <div>
        <h2 class="text-text text-base font-semibold">{t('analytics.detail.title')}</h2>
        <p class="text-text-muted mt-0.5 text-sm">{t('analytics.detail.desc')}</p>
      </div>
      <div class="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.country')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topCountries} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.browser')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topBrowsers} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.os')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topOs} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.device')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topDevices} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.outbound')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topOutbound} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.download')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topDownloads} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.utmSource')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topUtmSource} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.utmMedium')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topUtmMedium} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle as="h3">{t('analytics.dim.utmCampaign')}</CardTitle>
          </CardHeader>
          <CardBody>
            <TopList rows={view.topUtmCampaign} empty={t('analytics.top.empty')} />
          </CardBody>
        </Card>
      </div>

      {/* El fragmento solo aparece cuando NO hay ni una visita: si las hay, el
          script ya está puesto y recordárselo sería ruido.

          Y casi nunca hará falta: Sytue DevX lo inyecta en todo el HTML que
          construye (builder.analyticsInjectStep), así que esto queda para quien
          trae su propia imagen —Dockerfile o Go—, donde el HTML no pasa por
          nuestras manos. Se enseña IGUAL en los dos casos en vez de decidir por
          el framework: un proyecto que fuerza `build_strategy=buildpacks` no
          lleva nuestro Dockerfile, y afirmarle que ya está cableado sería
          mandarlo a buscar el fallo justo donde no está. */}
      {(totals?.pageviews ?? 0) === 0 ? (
        <Card>
          <CardHeader>
            <div class="min-w-0">
              <CardTitle as="h2">{t('analytics.snippet.title')}</CardTitle>
              <p class="text-text-muted mt-0.5 text-sm">{t('analytics.snippet.desc')}</p>
            </div>
          </CardHeader>
          <CardBody>
            <div>
              <CopyField
                t={t}
                label={t('analytics.snippet.label')}
                value={`<script defer src="/_sytue/s.js"></script>`}
              />
            </div>
            <p class="text-text-subtle mt-3 flex items-center gap-2 text-sm">
              <Icon name="info" size={16} />
              {t('analytics.snippet.sameOrigin')}
            </p>
          </CardBody>
        </Card>
      ) : null}
    </div>
  );
}
