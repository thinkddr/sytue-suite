import { useUltimo } from '@sytue/ui';
import { ruta } from '../lib/rutas.js';
/**
 * El progreso de UN trabajo de IA (diseñar, traducir, exportar): en cola, en curso, hecho o
 * con error, siempre con texto además de color. Consulta /api/ia/jobs/:id hasta que termina.
 */
import { useEffect, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Badge, Button, ProgressBar } from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { fueraDelEEE, useTraduccion } from './comun.js';

export interface AIJob {
  id: string;
  tenant: string;
  site_id: string;
  kind: 'design' | 'translate' | 'export';
  status: 'running' | 'done' | 'failed';
  input?: unknown;
  result?: unknown;
  result_key?: string;
  error?: string;
  provider?: string;
  byok?: boolean;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  created_at: string;
  finished_at?: string;
}

export interface EstadoTrabajoIAProps {
  jobId: string;
  titulo: string;
  locale?: Locale;
  onCompletado?: (job: AIJob) => void;
  /** Con esto, el fallo ofrece «Reintentar»: quien llama relanza el trabajo. */
  onReintentar?: () => void;
  /** Con esto, mientras espera ofrece «Dejar de esperar» y, tras un fallo, «Volver». */
  onCancelar?: () => void;
  /** Cada cuánto se consulta; solo cambia en las pruebas. */
  intervaloMs?: number;
}

type Fase = 'queued' | 'running' | 'done' | 'failed';

const INTERVALO_MS = 1500;
// Un corte de red suelto no es un fallo del trabajo: se aguanta alguno antes de rendirse.
const FALLOS_SEGUIDOS = 3;

const TONO: Record<Fase, BadgeTone> = {
  queued: 'neutral',
  running: 'info',
  done: 'success',
  failed: 'danger',
};

function reloj(segundos: number): string {
  const m = Math.floor(segundos / 60);
  const s = segundos % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}

export function EstadoTrabajoIA({
  jobId,
  titulo,
  locale,
  onCompletado,
  onReintentar,
  onCancelar,
  intervaloMs = INTERVALO_MS,
}: EstadoTrabajoIAProps): JSX.Element {
  const { t } = useTraduccion(locale);
  const [job, setJob] = useState<AIJob | null>(null);
  const [perdido, setPerdido] = useState(false);
  const [segundos, setSegundos] = useState(0);

  // Quien llama pasa una función nueva en cada render: si fuera dependencia del efecto, cada
  // render reiniciaría la consulta y, ya terminado, volvería a avisar de que terminó.
  const alCompletar = useUltimo(onCompletado);

  useEffect(() => {
    let activo = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let fallos = 0;
    setJob(null);
    setPerdido(false);

    async function consultar() {
      try {
        const res = await fetch(ruta(`/api/ia/jobs/${jobId}`));
        if (res.status === 404) throw Object.assign(new Error('404'), { definitivo: true });
        if (!res.ok) throw new Error(String(res.status));
        const data = (await res.json()) as AIJob;
        if (!activo) return;
        fallos = 0;
        setJob(data);
        if (data.status === 'done') alCompletar.current?.(data);
        else if (data.status !== 'failed') timer = setTimeout(() => void consultar(), intervaloMs);
      } catch (err) {
        if (!activo) return;
        fallos += 1;
        if (fallos >= FALLOS_SEGUIDOS || (err as { definitivo?: boolean }).definitivo) {
          setPerdido(true);
        } else {
          timer = setTimeout(() => void consultar(), intervaloMs);
        }
      }
    }

    void consultar();
    return () => {
      activo = false;
      clearTimeout(timer);
    };
  }, [jobId, intervaloMs, alCompletar]);

  const fase: Fase =
    perdido || job?.status === 'failed'
      ? 'failed'
      : job?.status === 'done'
        ? 'done'
        : job
          ? 'running'
          : 'queued';
  const pendiente = fase === 'queued' || fase === 'running';

  useEffect(() => {
    if (!pendiente) return;
    const inicio = Date.now();
    setSegundos(0);
    const id = setInterval(() => setSegundos(Math.floor((Date.now() - inicio) / 1000)), 1000);
    return () => clearInterval(id);
  }, [pendiente, jobId]);

  const texto = {
    queued: t('ai.job.queued_text'),
    running: t('ai.job.running_text'),
    done: t('ai.job.done_text'),
    failed: perdido ? t('ai.job.lost_text') : t('ai.job.failed_text'),
  }[fase];
  const etiqueta = {
    queued: t('ai.job.state_queued'),
    running: t('ai.job.state_running'),
    done: t('ai.job.state_done'),
    failed: t('ai.job.state_failed'),
  }[fase];

  return (
    <section aria-label={titulo} class="border-border bg-surface rounded-lg border p-4">
      <div class="flex flex-wrap items-start justify-between gap-2">
        <div class="min-w-0">
          <h4 class="sytue-h4 text-text">{titulo}</h4>
          {/* Se anuncia solo cada vez que cambia: es lo que oye el lector de pantalla. */}
          <p
            role={fase === 'failed' ? 'alert' : 'status'}
            class={`mt-0.5 text-sm ${fase === 'failed' ? 'text-danger-text' : 'text-text-muted'}`}
          >
            {texto}
          </p>
        </div>
        <Badge tone={TONO[fase]}>{etiqueta}</Badge>
      </div>

      {pendiente ? (
        <div class="mt-3 space-y-1.5">
          <ProgressBar label={titulo} />
          <p class="text-text-muted text-xs tabular-nums">
            {t('ai.job.elapsed', { time: reloj(segundos) })}
          </p>
        </div>
      ) : null}

      {job?.provider ? (
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Badge tone={fueraDelEEE(job.provider) ? 'warning' : 'info'}>
            {fueraDelEEE(job.provider) ? t('ai.job.badge_non_eu') : t('ai.job.badge_eu')}
          </Badge>
          {job.byok ? <Badge>{t('ai.job.badge_byok')}</Badge> : null}
          {job.model ? (
            <span class="text-text-muted text-xs">{t('ai.job.model', { model: job.model })}</span>
          ) : null}
        </div>
      ) : null}

      {fase === 'failed' && job?.error ? (
        <p class="text-text-muted mt-3 text-xs break-words">
          {t('ai.job.detail', { detail: job.error })}
        </p>
      ) : null}

      {(pendiente && onCancelar) || (fase === 'failed' && (onReintentar || onCancelar)) ? (
        <div class="border-border mt-4 flex flex-col gap-2 border-t pt-3 sm:flex-row sm:items-center sm:justify-between">
          {pendiente ? <p class="text-text-muted text-xs">{t('ai.job.stop_help')}</p> : <span />}
          <div class="flex flex-col gap-2 sm:flex-row">
            {onCancelar ? (
              <Button type="button" variant="secondary" onClick={onCancelar}>
                {pendiente ? t('ai.job.stop') : t('ai.job.back')}
              </Button>
            ) : null}
            {fase === 'failed' && onReintentar ? (
              <Button type="button" variant="primary" onClick={onReintentar}>
                {t('ai.job.retry')}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
    </section>
  );
}

export default EstadoTrabajoIA;
