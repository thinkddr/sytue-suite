/**
 * MetricsCard — el consumo REAL del proyecto, en dos gráficas pequeñas.
 *
 * El control-plane muestrea CPU y memoria de los pods de producción cada 15
 * minutos desde la migración 0026, y hasta ahora esas muestras solo las leía el
 * motor de recomendación de tamaño: el panel podía decir «te sobra tamaño» sin
 * poder enseñar el consumo del que salía esa conclusión. Una recomendación de
 * dinero que no se puede contrastar es la que nadie aplica.
 *
 * # Dos gráficas y no una con dos ejes
 *
 * Memoria (bytes) y CPU (milicores) son escalas distintas. Un solo gráfico con dos
 * ejes deja que la elección de escalas invente cruces que no existen; dos gráficas
 * con el MISMO eje temporal se comparan igual de bien y no mienten.
 *
 * # El techo va dentro
 *
 * Cada serie se pinta contra el límite de SU tamaño: 300 MiB es holgado en
 * «standard» y un OOMKill a punto en «basic». Sin la línea del techo, la gráfica
 * no dice nada útil.
 *
 * # Las cifras van en texto, no solo en la curva
 *
 * ponytail: sin crosshair ni tooltip. El valor de ahora, el p95 y el pico están
 * escritos al lado —que es lo que alguien necesita— y así funciona igual con el
 * dedo, donde el hover no existe. Si hiciera falta leer un punto concreto, el
 * añadido es un crosshair con navegación por teclado, no un `title` por punto.
 *
 * Solo @sytue/ui + tokens del tema (claro y oscuro salen de las variables). §14.
 */
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  Icon,
  EmptyState,
  formatBytes,
} from '@sytue/ui';

/** Percentil por el vecino más cercano, igual que el motor del control-plane. */
function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const s = [...values].sort((a, b) => a - b);
  const idx = Math.max(0, Math.ceil((p / 100) * s.length) - 1);
  return s[Math.min(idx, s.length - 1)] ?? 0;
}
import type { Locale } from '@sytue/i18n';
import type { ProjectMetrics } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';

interface Props {
  metrics: ProjectMetrics;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

/**
 * Color de la serie según lo cerca que esté del techo. Los tres pasos son los
 * mismos umbrales con los que el motor decide subir de tamaño, así que la gráfica
 * y el consejo no pueden contarse cosas distintas.
 *
 * El color NUNCA va solo: al lado hay una insignia con el porcentaje escrito.
 */
function toneFor(pct: number): {
  stroke: string;
  fill: string;
  badge: 'success' | 'warning' | 'danger';
} {
  if (pct >= 95) {
    return { stroke: 'var(--sytue-danger)', fill: 'var(--sytue-danger)', badge: 'danger' };
  }
  if (pct >= 80) {
    return { stroke: 'var(--sytue-warning)', fill: 'var(--sytue-warning)', badge: 'warning' };
  }
  return { stroke: 'var(--sytue-primary-400)', fill: 'var(--sytue-primary-400)', badge: 'success' };
}

/**
 * Una serie contra su techo. `values` va en orden cronológico y `limit` es el
 * máximo del eje: la escala se ancla al LÍMITE y no al máximo observado, que es
 * lo que permite ver de un vistazo cuánto margen queda.
 */
function Sparkline({
  values,
  limit,
  stroke,
  fill,
  label,
}: {
  values: number[];
  limit: number;
  stroke: string;
  fill: string;
  label: string;
}) {
  const w = 320;
  const h = 72;
  const top = limit > 0 ? limit : Math.max(...values, 1);
  const n = values.length;
  const x = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * w);
  const y = (v: number) => h - Math.min(1, v / top) * (h - 4) - 2;

  const line = values
    .map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    .join(' ');
  const area = `${line} L${w},${h} L0,${h} Z`;

  return (
    // role="img" + descripción: quien no ve la curva se lleva las cifras, que van
    // escritas fuera y son lo que de verdad hace falta para decidir.
    <svg
      viewBox={`0 0 ${w} ${h}`}
      class="h-[72px] w-full"
      preserveAspectRatio="none"
      role="img"
      aria-label={label}
    >
      {/* Techo del tamaño: discontinuo y recesivo, no compite con el dato. */}
      <line
        x1="0"
        y1="2"
        x2={w}
        y2="2"
        stroke="var(--sytue-border)"
        stroke-width="1"
        stroke-dasharray="4 4"
      />
      <path d={area} fill={fill} opacity="0.12" />
      <path
        d={line}
        fill="none"
        stroke={stroke}
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
    </svg>
  );
}

/** Una métrica: cifra grande, insignia de porcentaje y la curva debajo. */
function Serie({
  title,
  now,
  p95,
  peak,
  pct,
  values,
  limit,
  limitLabel,
  t,
}: {
  title: string;
  now: string;
  p95: string;
  peak: string;
  pct: number;
  values: number[];
  limit: number;
  limitLabel: string;
  t: T;
}) {
  const tone = toneFor(pct);
  return (
    <figure class="min-w-0">
      <figcaption class="mb-1 flex items-baseline justify-between gap-2">
        <span class="text-text text-sm font-semibold">{title}</span>
        <Badge tone={tone.badge}>{t('metrics.ofLimit', { percent: pct })}</Badge>
      </figcaption>
      <p class="text-text text-2xl font-semibold tabular-nums">{now}</p>
      <p class="text-text-muted mb-2 text-sm tabular-nums">
        {t('metrics.summary', { p95, peak, limit: limitLabel })}
      </p>
      <Sparkline
        values={values}
        limit={limit}
        stroke={tone.stroke}
        fill={tone.fill}
        label={t('metrics.chartAria', { title, p95, peak, limit: limitLabel })}
      />
    </figure>
  );
}

export default function MetricsCard({ metrics, locale }: Props) {
  const t = useT(locale);
  const points = metrics.points;

  if (points.length === 0) {
    return (
      <Card class="mb-4">
        <CardBody class="p-0">
          <EmptyState
            illustration={<Icon name="chart" size={24} />}
            title={t('metrics.empty.title')}
            description={t('metrics.empty.desc')}
          />
        </CardBody>
      </Card>
    );
  }

  const mem = points.map((p) => p.memoryBytes);
  const cpu = points.map((p) => p.cpuMilli);
  const memP95 = percentile(mem, 95);
  const cpuP95 = percentile(cpu, 95);
  const memPeak = Math.max(...mem);
  const cpuPeak = Math.max(...cpu);
  const memNow = mem[mem.length - 1] ?? 0;
  const cpuNow = cpu[cpu.length - 1] ?? 0;
  const pct = (v: number, limit: number) => (limit > 0 ? Math.round((v / limit) * 100) : 0);

  return (
    <Card class="mb-4">
      <CardHeader>
        <div class="min-w-0">
          <CardTitle as="h2">{t('metrics.title')}</CardTitle>
          <p class="text-text-muted mt-0.5 text-sm">
            {t('metrics.subtitle', { hours: metrics.windowHours, size: metrics.instanceSize })}
          </p>
        </div>
      </CardHeader>
      <CardBody class="grid gap-6 sm:grid-cols-2">
        <Serie
          t={t}
          title={t('metrics.memory')}
          now={formatBytes(locale, memNow)}
          p95={formatBytes(locale, memP95)}
          peak={formatBytes(locale, memPeak)}
          limitLabel={formatBytes(locale, metrics.memoryLimitBytes)}
          pct={pct(memP95, metrics.memoryLimitBytes)}
          values={mem}
          limit={metrics.memoryLimitBytes}
        />
        <Serie
          t={t}
          title={t('metrics.cpu')}
          now={`${cpuNow} m`}
          p95={`${cpuP95} m`}
          peak={`${cpuPeak} m`}
          limitLabel={`${metrics.cpuMilliLimit} m`}
          pct={pct(cpuP95, metrics.cpuMilliLimit)}
          values={cpu}
          limit={metrics.cpuMilliLimit}
        />
      </CardBody>
    </Card>
  );
}
