/**
 * LogsPanel — qué está fallando en la plataforma, en lenguaje de personas.
 *
 * No es un visor de logs: un volcado crudo se abre una vez y no se vuelve a
 * abrir. Lo que hace falta al entrar aquí es saber en diez segundos si hay algo
 * roto, desde cuándo y a qué servicio le pasa — y eso son problemas AGRUPADOS
 * con su recuento, no líneas.
 *
 * De ahí las tres piezas: las cifras de arriba responden «¿va todo bien?», la
 * tendencia por hora responde «¿es ahora o lleva pasando?», y la lista responde
 * «¿qué exactamente?». El detalle técnico de cada uno queda plegado, porque
 * enseñarlo de entrada es volver al volcado que nadie lee.
 */
import { Fragment } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  EmptyState,
  Input,
  useSondeo,
} from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { LogGroup, LogStats } from '../lib/platform.js';

/** Cada cuánto se rehidrata. Diez segundos es lo que tarda alguien en leer las
 *  cifras de arriba: más rápido parpadea, más lento parece congelado. */
const REFRESCO_MS = 10_000;

/** Lo que se espera tras la última tecla antes de consultar. Por debajo de ~250 ms
 *  se vuelve a consultar por sílabas; por encima de ~500 ms se nota el retraso. */
const ESPERA_TECLEO_MS = 300;

/**
 * Una petición de plataforma, con el 403 tratado como lo que significa aquí: la
 * elevación se ha acabado mientras la pantalla seguía abierta.
 *
 * Esta consola se deja abierta y la elevación caduca sola (una hora por
 * defecto). Sin esto, el sondeo seguía preguntando cada diez segundos para
 * siempre: la pantalla se quedaba enseñando cifras viejas sin decir que ya no se
 * actualizan, y cada intento dejaba un aviso en el registro de plataforma —el
 * mismo que se lee en esta pestaña—. Recargar devuelve al gate de elevación, que
 * es la única salida y tiene el botón para volver a entrar.
 */
async function call(path: string): Promise<Response> {
  const res = await fetch(`/api/platform${path}`);
  if (res.status === 403) window.location.reload();
  return res;
}

/** Cifra de cabecera. El tono avisa sin gritar: solo se enciende si hay algo. */
function Kpi({
  label,
  value,
  tone = 'neutral',
  hint,
}: {
  label: string;
  value: number | string;
  tone?: 'neutral' | 'danger' | 'warning';
  hint?: string;
}) {
  const color =
    tone === 'danger' && value !== 0
      ? 'text-danger-text'
      : tone === 'warning' && value !== 0
        ? 'text-warning-text'
        : 'text-text';
  return (
    <Card raised={false}>
      <CardBody>
        <p class="text-text-muted truncate text-sm">{label}</p>
        <p class={`sytue-h3 mt-1 tabular-nums ${color}`}>{value}</p>
        {hint ? <p class="sytue-meta mt-0.5">{hint}</p> : null}
      </CardBody>
    </Card>
  );
}

/**
 * Tendencia de las últimas 24 horas, en SVG y sin librería de gráficos.
 *
 * Son veinticuatro barras: traerse una dependencia de gráficos para esto pesaría
 * más que toda la pantalla. Las horas sin nada se dibujan igual —el servidor las
 * manda a cero— porque si solo salieran las horas con datos, dos picos separados
 * por un día de calma parecerían una crisis continua.
 */
function Tendencia({ stats, locale, t }: { stats: LogStats; locale: Locale; t: T }) {
  const barras = stats.by_hour;
  const techo = Math.max(1, ...barras.map((b) => b.errors + b.warns));
  const ancho = 100 / Math.max(1, barras.length);
  return (
    <div>
      <svg
        viewBox="0 0 100 30"
        preserveAspectRatio="none"
        class="h-24 w-full"
        role="img"
        aria-label={t('logs.trend.label')}
      >
        {barras.map((b, i) => {
          const total = b.errors + b.warns;
          const h = (total / techo) * 28;
          const hErr = total > 0 ? (b.errors / total) * h : 0;
          const x = i * ancho;
          const w = ancho * 0.7;
          return (
            <g key={b.hour}>
              {/* Los avisos abajo y los errores encima: lo que preocupa queda
                  arriba, donde se mira primero. */}
              {/* Sin alpha: una marca de gráfico necesita 3:1 (WCAG 1.4.11) y al 40 %
                  el aviso quedaba en 1,84:1. Los roles `-text` sí invierten con el
                  tema, así que a opacidad plena dan 6,16:1 y 6,76:1 en claro y ≥6:1
                  en oscuro; oliva y rojo se siguen distinguiendo entre sí. */}
              <rect
                x={x}
                y={30 - h}
                width={w}
                height={Math.max(0, h - hErr)}
                class="fill-warning-text"
              />
              <rect x={x} y={30 - hErr} width={w} height={hErr} class="fill-danger-text" />
              <title>
                {formatDateTime(locale, b.hour)} — {t('logs.trend.errors', { count: b.errors })},{' '}
                {t('logs.trend.warns', { count: b.warns })}
              </title>
            </g>
          );
        })}
      </svg>
      <p class="sytue-meta mt-1">{t('logs.trend.footer')}</p>
    </div>
  );
}

/** Un problema, plegado. El detalle técnico se abre solo si alguien lo pide. */
function Problema({ g, locale, t }: { g: LogGroup; locale: Locale; t: T }) {
  const [abierto, setAbierto] = useState(false);
  const esError = g.level === 'ERROR';
  const attrs = Object.entries(g.sample ?? {}).filter(([k]) => k !== 'service' && k !== 'version');
  return (
    <li class="border-border border-b last:border-0">
      <div class="flex items-start gap-3 px-4 py-3">
        <Badge tone={esError ? 'danger' : 'warning'}>
          {esError ? t('logs.level.error') : t('logs.level.warn')}
        </Badge>
        <div class="min-w-0 flex-1">
          <p class="text-text font-medium">{g.message}</p>
          <p class="sytue-meta mt-0.5">
            {g.service} · {t('logs.since', { when: formatDateTime(locale, g.first) })} ·{' '}
            {t('logs.last', { when: formatDateTime(locale, g.last) })}
          </p>
          {abierto && attrs.length > 0 ? (
            <dl class="bg-bg-subtle mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-md p-2 text-xs">
              {/* La clave va en el Fragment, no en los hijos: un fragmento sin clave
                  se reconcilia por índice y las de dentro no hacen nada. */}
              {attrs.map(([k, v]) => (
                <Fragment key={k}>
                  <dt class="text-text-muted font-mono">{k}</dt>
                  <dd class="text-text font-mono break-all">{v}</dd>
                </Fragment>
              ))}
            </dl>
          ) : null}
        </div>
        {/* El recuento es la información principal de la fila: una vez es un
            incidente, mil veces es una avería, y el número es lo que separa
            «mirar cuando pueda» de «mirar ahora». */}
        <span class="text-text shrink-0 text-right tabular-nums">
          <strong>{g.count}</strong>
          <span class="sytue-meta block">{t('logs.times')}</span>
        </span>
        {attrs.length > 0 ? (
          <Button size="sm" variant="ghost" onClick={() => setAbierto(!abierto)}>
            {abierto ? t('logs.hideDetail') : t('logs.showDetail')}
          </Button>
        ) : null}
      </div>
    </li>
  );
}

export default function LogsPanel({ locale, t }: { locale: Locale; t: T }) {
  const [stats, setStats] = useState<LogStats | null>(null);
  const [grupos, setGrupos] = useState<LogGroup[] | null>(null);
  const [nivel, setNivel] = useState('');
  const [servicio, setServicio] = useState('');
  const [busqueda, setBusqueda] = useState('');
  const [fallo, setFallo] = useState(false);

  // Los servicios que han fallado salen de lo que hay, no de una lista fija: una
  // lista fija se queda desactualizada en cuanto nace un servicio nuevo, y
  // enseñaría filtros que no encuentran nada.
  const servicios = useMemo(
    () => [...new Set((grupos ?? []).map((g) => g.service))].sort(),
    [grupos],
  );

  // El buscador es un campo de texto y entraba tal cual en las dependencias de
  // la consulta: CADA tecla cancelaba el sondeo, creaba otro y lanzaba las dos
  // agregaciones. Escribir «database» eran dieciséis consultas al servicio de
  // plataforma para enseñar solo la última. Se consulta al dejar de teclear.
  const [busquedaAplicada, setBusquedaAplicada] = useState('');
  useEffect(() => {
    const id = setTimeout(() => setBusquedaAplicada(busqueda), ESPERA_TECLEO_MS);
    return () => clearTimeout(id);
  }, [busqueda]);

  const montadoRef = useRef(true);
  useEffect(() => {
    montadoRef.current = true;
    return () => {
      montadoRef.current = false;
    };
  }, []);

  const traer = useCallback(async () => {
    const p = new URLSearchParams();
    if (nivel) p.set('level', nivel);
    if (servicio) p.set('service', servicio);
    if (busquedaAplicada.trim()) p.set('q', busquedaAplicada.trim());
    const qs = p.toString();
    try {
      const [rl, rs] = await Promise.all([call(`/logs${qs ? `?${qs}` : ''}`), call('/logs/stats')]);
      if (!montadoRef.current) return;
      if (!rl.ok || !rs.ok) {
        setFallo(true);
        return;
      }
      setFallo(false);
      setGrupos(((await rl.json()) as { items: LogGroup[] }).items ?? []);
      setStats((await rs.json()) as LogStats);
    } catch {
      if (montadoRef.current) setFallo(true);
    }
  }, [nivel, servicio, busquedaAplicada]);

  // Al entrar y cada vez que cambian los filtros.
  useEffect(() => {
    void traer();
  }, [traer]);

  // Se refresca solo: esta pantalla se mira mientras algo va mal, y tener que
  // recargarla a mano para saber si sigue yendo mal es justo lo que no puede
  // pasar aquí. Con la pestaña de fondo no hay nadie a quien enseñárselo, y eran
  // 720 consultas a la hora (dos cada diez segundos) de una pantalla que nadie
  // está mirando.
  useSondeo(() => void traer(), REFRESCO_MS);

  if (fallo) {
    return <EmptyState title={t('logs.unavailable')} description={t('logs.unavailableHint')} />;
  }

  return (
    <div class="flex flex-col gap-4">
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Kpi
          label={t('logs.kpi.errors1h')}
          value={stats?.errors_1h ?? '—'}
          tone="danger"
          hint={t('logs.kpi.errors1hHint')}
        />
        <Kpi label={t('logs.kpi.errors24h')} value={stats?.errors_24h ?? '—'} tone="danger" />
        <Kpi label={t('logs.kpi.warns24h')} value={stats?.warns_24h ?? '—'} tone="warning" />
        <Kpi
          label={t('logs.kpi.services')}
          value={stats?.services_affected ?? '—'}
          tone="danger"
          hint={t('logs.kpi.servicesHint')}
        />
      </div>

      {stats ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('logs.trend.title')}</CardTitle>
          </CardHeader>
          <CardBody>
            <Tendencia stats={stats} locale={locale} t={t} />
          </CardBody>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t('logs.list.title')}</CardTitle>
          <div class="flex flex-wrap gap-2">
            <select
              class="sy-select"
              value={nivel}
              aria-label={t('logs.filter.level')}
              onChange={(e) => setNivel((e.target as HTMLSelectElement).value)}
            >
              <option value="">{t('logs.filter.allLevels')}</option>
              <option value="error">{t('logs.level.error')}</option>
              <option value="warn">{t('logs.level.warn')}</option>
            </select>
            <select
              class="sy-select"
              value={servicio}
              aria-label={t('logs.filter.service')}
              onChange={(e) => setServicio((e.target as HTMLSelectElement).value)}
            >
              <option value="">{t('logs.filter.allServices')}</option>
              {servicios.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
            <Input
              type="search"
              value={busqueda}
              // El placeholder NO es nombre accesible: desaparece al escribir y no
              // todos los lectores lo anuncian. Los dos <select> de al lado sí lo
              // llevan, así que esto era un olvido, no un criterio.
              aria-label={t('logs.filter.search')}
              placeholder={t('logs.filter.search')}
              onInput={(e) => setBusqueda((e.target as HTMLInputElement).value)}
            />
          </div>
        </CardHeader>
        <CardBody class="p-0">
          {grupos === null ? (
            <p class="sy-hint p-4">{t('common.loading')}</p>
          ) : grupos.length === 0 ? (
            // Sin errores NO es un estado vacío triste: es la buena noticia, y
            // se dice como tal.
            <EmptyState title={t('logs.empty')} description={t('logs.emptyHint')} />
          ) : (
            <ul>
              {grupos.map((g) => (
                <Problema
                  key={`${g.service}|${g.level}|${g.message}`}
                  g={g}
                  locale={locale}
                  t={t}
                />
              ))}
            </ul>
          )}
        </CardBody>
      </Card>
    </div>
  );
}
