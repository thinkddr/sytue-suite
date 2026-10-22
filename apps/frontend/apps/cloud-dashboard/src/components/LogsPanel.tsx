/**
 * LogsPanel — registros y analítica del proyecto en lenguaje claro (estilo
 * Vercel). Dos pestañas que ya no se confunden: «Compilación» (el log del último
 * build, histórico, con búsqueda server-side ?q= sobre TODO el log) y «En vivo»
 * (los logs de RUNTIME del despliegue de producción, en streaming, con pausa y
 * auto-scroll). Filtro y resaltado compartidos. Solo @sytue/ui. docs/CONVENTIONS.md §14.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { VNode } from 'preact';
import {
  Card,
  CardBody,
  Badge,
  Button,
  Input,
  Icon,
  Select,
  SectionHeader,
  Spinner,
  Tabs,
  VirtualList,
  withBase,
  useUltimo,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { LogLevel, LogLine, UsageMetric } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { appendCapped, runtimeLine, takeLines } from '../lib/logstream.js';
import DemoBanner from './DemoBanner.js';
import { contiene, rangos } from '@sytue/i18n';

/** Color del mensaje según nivel, derivado de los tokens sobre el fondo oscuro del visor. */
const levelTone: Record<LogLevel, string> = {
  info: 'text-white/85',
  // ≥ 4.5:1 sobre neutral-950 (AA texto pequeño); /50 se quedaba en ~3.5:1.
  debug: 'text-white/70',
  warn: 'text-[color-mix(in_srgb,var(--sytue-warning)_60%,white)]',
  error: 'text-[color-mix(in_srgb,var(--sytue-danger)_60%,white)]',
};

/** Techo del buffer en vivo: un servicio con tráfico escupe miles de líneas/minuto. */
const MAX_LIVE_LINES = 5000;
/** Cadencia de volcado del buffer a estado: agrupa ráfagas en un solo re-render. */
const FLUSH_MS = 300;

/**
 * Parte el mensaje en fragmentos resaltando cada coincidencia de `q`
 * (case-insensitive) con un <mark> de acento latón sobrio sobre el visor oscuro.
 * `q` vacío devuelve el texto tal cual. Todas las ocurrencias, sin regex.
 */
function highlightMatches(text: string, q: string): (string | VNode)[] {
  if (!q) return [text];
  const out: (string | VNode)[] = [];
  let i = 0;
  // Las mismas coincidencias que el filtro: sin mayúsculas ni acentos.
  for (const [idx, fin] of rangos(text, q)) {
    if (idx > i) out.push(text.slice(i, idx));
    out.push(
      <mark class="rounded-[2px] bg-[color-mix(in_srgb,var(--sytue-primary-400)_40%,transparent)] px-0.5 text-white">
        {text.slice(idx, fin)}
      </mark>,
    );
    i = fin;
  }
  if (i < text.length) out.push(text.slice(i));
  return out;
}

/**
 * Botón sobre el visor oscuro: el Button canónico asume superficie clara y no lee
 * ahí. Velos BLANCOS literales y no los del cromo: el visor es oscuro en los dos
 * temas, y los tokens del cromo son de tinta —se confundirían con el propio visor.
 * El foco visible lo aporta el anillo global (theme.css), que contrasta sobre el visor.
 */
function DarkButton({ children, onClick }: { children: string; onClick: () => void }) {
  return (
    <button
      type="button"
      class="rounded-control border border-white/15 px-3 py-1.5 font-sans text-sm font-semibold text-white/80 transition-colors hover:bg-white/10"
      onClick={onClick}
    >
      {children}
    </button>
  );
}

/** Visor virtualizado compartido por las dos pestañas (build y runtime). */
function LogViewer({
  lines,
  query,
  empty,
  stick,
  t,
}: {
  lines: LogLine[];
  query: string;
  empty: VNode | string;
  /** Ancla el scroll al final: solo tiene sentido en el stream en vivo. */
  stick?: boolean;
  t: T;
}) {
  return (
    <Card>
      <CardBody class="p-0">
        {lines.length === 0 ? (
          <div class="flex flex-col items-center gap-3 bg-[var(--sytue-neutral-950)] px-2 py-10 text-center font-mono text-sm">
            {typeof empty === 'string' ? <p class="text-white/50">{empty}</p> : empty}
          </div>
        ) : (
          /* Virtualizado (@sytue/ui): pinta solo la ventana visible — los logs
             reales llegan a decenas de miles de líneas. */
          <VirtualList
            items={lines}
            estimateSize={28}
            overscan={12}
            stickToBottom={stick ?? false}
            aria-label={t('logs.viewerLabel')}
            class="h-[460px] bg-[var(--sytue-neutral-950)] p-3 font-mono text-sm leading-relaxed"
            renderItem={(l) => (
              <div class="flex items-start gap-3 px-1 py-0.5">
                {l.ts ? <span class="shrink-0 text-white/60 tabular-nums">{l.ts}</span> : null}
                <Badge
                  tone={l.level === 'error' ? 'danger' : l.level === 'warn' ? 'warning' : 'neutral'}
                >
                  {l.source}
                </Badge>
                <span class={'min-w-0 flex-1 break-words ' + levelTone[l.level]}>
                  {query ? highlightMatches(l.message, query) : l.message}
                </span>
              </div>
            )}
          />
        )}
      </CardBody>
    </Card>
  );
}

type LiveStatus = 'connecting' | 'streaming' | 'ended' | 'error';

/** Mensaje honesto para cada fallo REAL del control-plane (nunca «modo demostración»). */
function failureText(t: T, f: { status: number; code: string }): string {
  if (f.status === 401) return t('error.unauthorized');
  if (f.status === 403) return t('logs.live.forbidden');
  if (f.status === 404) return t('logs.live.notFound');
  if (f.status === 503) return t('logs.live.unavailable');
  return t('error.unknown', { status: f.status, code: f.code });
}

/**
 * Logs de runtime en vivo. Lee el stream del BFF (/api/log-stream) con fetch +
 * lector de ReadableStream en vez de EventSource: EventSource no puede mandar la
 * cabecera de autorización (por eso la pone el BFF), no deja leer el status de la
 * respuesta —un 403 por falta de rol sería un error mudo— y reconecta solo en
 * bucle. Con fetch el fallo trae status y código reales y se cuentan tal cual.
 */
function LiveLogs({
  project,
  deployment,
  query,
  t,
}: {
  project: string;
  deployment: string;
  query: string;
  t: T;
}) {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [status, setStatus] = useState<LiveStatus>('connecting');
  const [failure, setFailure] = useState<{ status: number; code: string } | null>(null);
  const [paused, setPaused] = useState(false);
  const [queued, setQueued] = useState(0);
  const [attempt, setAttempt] = useState(0);
  // Buffer intermedio: el stream escribe aquí (sin re-render) y un temporizador
  // vuelca por lotes. En pausa deja de volcarse pero la conexión sigue viva, así
  // que al reanudar no falta nada (salvo lo que rebase el techo del buffer).
  const pendingRef = useRef<LogLine[]>([]);
  // Evento SSE que la red partió entre dos lecturas (ver takeLines).
  const carryRef = useRef('');
  const pausedRef = useUltimo(paused);

  useEffect(() => {
    const ctrl = new AbortController();
    let cancelled = false;
    setStatus('connecting');
    setFailure(null);

    void (async () => {
      try {
        const params = new URLSearchParams({ project, deployment });
        const res = await fetch(`/api/log-stream?${params.toString()}`, { signal: ctrl.signal });
        if (!res.ok || !res.body) {
          const code = await res
            .json()
            .then((b: { error?: string }) => b.error ?? 'backend_error')
            .catch(() => 'backend_error');
          if (!cancelled) {
            setFailure({ status: res.status, code });
            setStatus('error');
          }
          return;
        }
        if (!cancelled) setStatus('streaming');
        const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
        carryRef.current = '';
        for (;;) {
          const { value, done } = await reader.read();
          if (done || cancelled) break;
          const cut = takeLines(carryRef.current, value);
          carryRef.current = cut.carry;
          for (const raw of cut.lines) pendingRef.current.push(runtimeLine(raw));
          // También en pausa: el buffer no puede crecer sin límite.
          if (pendingRef.current.length > MAX_LIVE_LINES) {
            pendingRef.current.splice(0, pendingRef.current.length - MAX_LIVE_LINES);
          }
        }
        if (!cancelled) setStatus('ended');
      } catch {
        // El abort del cleanup también entra aquí: no es un fallo que contar.
        if (!cancelled) {
          setFailure(null);
          setStatus('error');
        }
      }
    })();

    return () => {
      cancelled = true;
      ctrl.abort();
    };
  }, [project, deployment, attempt]);

  useEffect(() => {
    const id = setInterval(() => {
      // No hace falta cerrar nada por silencio: el servidor cierra CADA LÍNEA con su
      // propio evento y hace flush, así que lo que quede en `carry` son bytes en
      // vuelo de milisegundos, no una línea esperando a que llegue la siguiente.
      //
      // En pausa solo se publica cuánto se está acumulando (el ref no re-renderiza).
      if (pausedRef.current) {
        setQueued(pendingRef.current.length);
        return;
      }
      if (pendingRef.current.length === 0) return;
      const batch = pendingRef.current;
      pendingRef.current = [];
      setQueued(0);
      setLines((prev) => appendCapped(prev, batch, MAX_LIVE_LINES));
    }, FLUSH_MS);
    return () => clearInterval(id);
  }, [pausedRef]);

  const q = query.toLowerCase();
  const shown = useMemo(
    () => (q ? lines.filter((l) => contiene(l.message, q)) : lines),
    [lines, q],
  );

  const reconnect = () => {
    pendingRef.current = [];
    setQueued(0);
    setAttempt((n) => n + 1);
  };

  return (
    <>
      <div class="mb-3 flex flex-wrap items-center gap-3">
        <Button
          variant="secondary"
          size="sm"
          aria-pressed={paused}
          disabled={status === 'error'}
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? t('logs.resume') : t('logs.pause')}
        </Button>
        {status === 'error' || status === 'ended' ? (
          <Button variant="secondary" size="sm" onClick={reconnect}>
            {t('logs.reconnect')}
          </Button>
        ) : null}
        <span
          class="text-text-muted flex items-center gap-2 text-sm tabular-nums"
          role="status"
          aria-live="polite"
        >
          {status === 'connecting' ? (
            <>
              <Spinner size={16} /> {t('logs.connecting')}
            </>
          ) : status === 'streaming' ? (
            paused ? (
              <>
                <Badge tone="warning">{t('logs.paused')}</Badge>
                {t('logs.queued', { count: queued })}
              </>
            ) : (
              <>
                <Badge tone="success">{t('logs.streaming')}</Badge>
                {t('logs.lines', { count: lines.length })}
              </>
            )
          ) : status === 'ended' ? (
            <>
              <Badge tone="neutral">{t('logs.streamClosed')}</Badge>
              {t('logs.linesReceived', { count: lines.length })}
            </>
          ) : (
            <Badge tone="danger">{t('logs.offline')}</Badge>
          )}
        </span>
      </div>

      {status === 'error' ? (
        <div
          class="border-danger bg-danger-bg text-danger-text mb-3 rounded-md border px-3 py-2 text-sm"
          role="alert"
        >
          {failure ? failureText(t, failure) : t('logs.connectionLost')}
        </div>
      ) : null}

      <LogViewer
        lines={shown}
        query={query}
        stick={!paused}
        t={t}
        empty={
          status === 'error'
            ? t('logs.empty.liveError')
            : query
              ? t('logs.empty.liveQuery')
              : t('logs.empty.liveWaiting')
        }
      />
    </>
  );
}

export default function LogsPanel({
  project,
  logs,
  usage,
  live,
  error,
  deployment,
  locale,
}: {
  /** Slug del proyecto — para pedir la búsqueda server-side al BFF. */
  project: string;
  logs: LogLine[];
  usage: UsageMetric[];
  live: boolean;
  /** Error REAL del control-plane en la carga SSR (403/500), si lo hubo. */
  error?: { status: number; code: string } | undefined;
  /** Despliegue cuyo runtime se sigue en vivo; ausente si el proyecto no tiene ninguno. */
  deployment?: { id: string; label: string } | undefined;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}) {
  const t = useT(locale);
  const [query, setQuery] = useState('');
  const [source, setSource] = useState<'all' | LogLine['source']>('all');
  // Resultado de la última búsqueda server-side (con la `q` que lo produjo) y su
  // estado. `null` = aún no hay búsqueda para la query actual (caja vacía o en vuelo).
  const [result, setResult] = useState<{ q: string; lines: LogLine[] } | null>(null);
  const [status, setStatus] = useState<'idle' | 'searching' | 'error'>('idle');

  const trimmed = query.trim();

  // Búsqueda server-side con debounce: filtra sobre TODO el log (?q=), no solo el
  // tramo cargado por SSR. Caja vacía → sin búsqueda (se muestra el tail SSR).
  useEffect(() => {
    if (!trimmed) {
      setResult(null);
      setStatus('idle');
      return;
    }
    let cancelled = false;
    setStatus('searching');
    const timer = setTimeout(() => {
      // El origen elegido decide a qué registro se pregunta: «Build» al log de la
      // compilación, cualquier otro al de ejecución. Antes esta búsqueda iba SIEMPRE al
      // de compilación, así que preguntar por un error de producción devolvía el log de
      // cuando se construyó — dos cosas que se parecen lo justo para no notarlo.
      const params = new URLSearchParams({ project, q: trimmed });
      if (source === 'build') params.set('source', 'build');
      fetch(`/api/logs?${params.toString()}`)
        .then((res) => (res.ok ? res.json() : Promise.reject(new Error('http'))))
        .then((body: { logs: LogLine[] }) => {
          if (cancelled) return;
          setResult({ q: trimmed, lines: body.logs });
          setStatus('idle');
        })
        .catch(() => {
          // Sin búsqueda server-side: caemos al filtrado cliente del tramo cargado
          // (estado honesto abajo). No dejamos la vista vacía por un fallo de red.
          if (!cancelled) setStatus('error');
        });
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [trimmed, project, source]);

  // Resultado del servidor que corresponde a la query actual (null si la caja está
  // vacía o la búsqueda aún no ha llegado: entonces se filtra en cliente).
  const matched = trimmed !== '' && result?.q === trimmed ? result : null;
  // Query con la que resaltar/contar: la del resultado servido, o la tecleada
  // mientras llega (filtrado cliente de feedback inmediato sobre el tramo cargado).
  const activeQuery = matched ? matched.q : trimmed;

  const shown = useMemo(() => {
    const base = matched ? matched.lines : logs;
    const q = activeQuery.toLowerCase();
    return base.filter(
      (l) =>
        (source === 'all' || l.source === source) &&
        // Con resultado del servidor las líneas ya casan; si no, filtra por `q` en
        // cliente sobre el tramo cargado (feedback inmediato).
        (matched !== null || !q || contiene(l.message, q)),
    );
  }, [logs, matched, activeQuery, source]);

  const buildTab = (
    <>
      <div class="mb-3 flex flex-wrap items-center gap-2">
        <Select
          aria-label={t('logs.sourceLabel')}
          value={source}
          class="w-44"
          onChange={(e) => setSource(e.currentTarget.value as typeof source)}
        >
          <option value="all">{t('logs.source.all')}</option>
          <option value="build">Build</option>
          <option value="runtime">Runtime</option>
          <option value="edge">Edge</option>
        </Select>
        {trimmed ? (
          <span
            class="text-text-muted flex items-center gap-1.5 text-sm tabular-nums"
            role="status"
            aria-live="polite"
          >
            {status === 'searching' ? (
              <>
                <Spinner size={16} /> {t('logs.searching')}
              </>
            ) : status === 'error' ? (
              <span class="text-warning-text">{t('logs.searchDegraded')}</span>
            ) : (
              t('logs.matches', { count: shown.length })
            )}
          </span>
        ) : null}
      </div>
      <LogViewer
        lines={shown}
        query={activeQuery}
        t={t}
        empty={
          trimmed || source !== 'all' ? (
            <>
              <p class="text-white/50">{t('logs.empty.filtered')}</p>
              <DarkButton
                onClick={() => {
                  setQuery('');
                  setSource('all');
                }}
              >
                {t('logs.clearFilters')}
              </DarkButton>
            </>
          ) : (
            t('logs.empty.build')
          )
        }
      />
    </>
  );

  const liveTab = deployment ? (
    <LiveLogs project={project} deployment={deployment.id} query={query} t={t} />
  ) : (
    <p class="text-text-muted text-sm">{t('logs.noRuntime')}</p>
  );

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="audit" size={20} />}
        title={t('logs.title')}
        subtitle={t('logs.subtitle')}
      />
      <div class="cloud-content">
        <div class="mb-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
          {usage.map((m) => (
            <Card key={m.label} raised={false}>
              <CardBody>
                <p class="text-text-muted truncate text-sm">{m.label}</p>
                <p class="sytue-h4 text-text mt-1 tabular-nums">{m.value}</p>
              </CardBody>
            </Card>
          ))}
        </div>

        {/* La búsqueda es común a las dos pestañas: se queda escrita al cambiar. */}
        <div class="mb-3 flex flex-wrap items-center gap-2">
          <Input
            type="search"
            value={query}
            placeholder={t('logs.search')}
            aria-label={t('logs.search')}
            leadingIcon={<Icon name="search" size={16} />}
            class="w-72"
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
          {/* Descarga lo MISMO que se está viendo (el filtro viaja en la URL), no
              «todo»: si alguien busca «timeout» y se lleva el log entero, el
              fichero no responde a la pregunta que hizo. Enlace normal y no
              fetch: el navegador ya sabe guardar una descarga, y así funciona
              también con el botón derecho. Sin atributo `download`: lo que
              fuerza el guardado es el `Content-Disposition` que manda el
              servidor, y el atributo no existe en un <button>. */}
          <Button
            variant="secondary"
            size="sm"
            href={withBase(
              import.meta.env.BASE_URL,
              `/api/logs?project=${encodeURIComponent(project)}&download=1${
                trimmed ? `&q=${encodeURIComponent(trimmed)}` : ''
              }`,
            )}
          >
            {t('logs.download')}
          </Button>
        </div>

        <Tabs
          items={[
            {
              id: 'build',
              label: t('logs.tab.build'),
              content: (
                <>
                  <p class="text-text-muted mb-3 text-sm">{t('logs.build.desc')}</p>
                  {buildTab}
                </>
              ),
            },
            {
              id: 'live',
              label: t('logs.tab.live'),
              content: (
                <>
                  {/* Dos variantes completas, no una frase con un trozo opcional
                      cosido en medio: el paréntesis no cae en el mismo sitio en
                      todos los idiomas. */}
                  <p class="text-text-muted mb-3 text-sm">
                    {deployment
                      ? t('logs.live.desc', { deployment: deployment.label })
                      : t('logs.live.desc.noDeployment')}
                  </p>
                  {liveTab}
                </>
              ),
            },
          ]}
        />
      </div>
    </>
  );
}
