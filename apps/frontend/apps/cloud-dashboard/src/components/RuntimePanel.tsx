/**
 * RuntimePanel — escala y contrato de ejecución de la app (isla).
 *
 * Hasta ahora esto solo se podía tocar por API: el dashboard no tenía NINGUNA
 * pantalla para cambiar la configuración de un proyecto ya creado. Aquí se editan
 * el tamaño de instancia, el número de instancias, el puerto y la ruta de la sonda
 * de salud, contra POST /api/project-runtime.
 *
 * El catálogo de tamaños llega del control-plane (no hay copia local): así la
 * interfaz nunca ofrece un tamaño que el backend haya retirado. Solo @sytue/ui.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  FieldGroup,
  Input,
  describedBy,
  useToast,
  ToastProvider,
  Radio,
} from '@sytue/ui';
import type { InstanceSize } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import { useT } from '../i18n/index.js';
import type { Framework, ProjectRuntime } from '../lib/types.js';
import type { HealthProbeReport } from '../lib/cloud.js';
import { formatEur } from '../lib/format.js';
import { humanResources, monthlyComputeEur, typicalPortFor } from '../lib/runtime.js';
import { round2 } from '../lib/pricing.js';

export interface RuntimePanelProps {
  project: string;
  runtime: ProjectRuntime;
  sizes: InstanceSize[];
  maxCount: number;
  /** Framework detectado: de ahí sale el puerto que se ofrece de un clic. */
  framework: Framework;
  /** false = sin control-plane conectado (los cambios no salen de la pantalla). */
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function RuntimePanel(props: RuntimePanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ project, runtime, sizes, maxCount, framework, live, locale }: RuntimePanelProps) {
  const t = useT(locale);

  /** Nombre del tamaño en el idioma de la sesión. El `label` que manda el
   *  backend viene en castellano —`services/cloud/internal/domain/instancesize.go`—
   *  y solo se usa de respaldo si aparece un tamaño que este panel no conoce. */
  const sizeLabel = (s: { name: string; label: string }): string => {
    const clave = `runtime.size.${s.name}`;
    const texto = t(clave as Parameters<typeof t>[0]);
    return texto === clave ? s.label : texto;
  };
  const toast = useToast();
  // `saved` es lo que hay en el servidor; `draft`, lo que se está editando. La
  // diferencia entre ambos es LO ÚNICO que se envía: la API trata cada campo
  // ausente como «no lo toques», así que mandar el objeto entero machacaría
  // configuración que no se está tocando.
  const [saved, setSaved] = useState<ProjectRuntime>(runtime);
  const [draft, setDraft] = useState<ProjectRuntime>(runtime);
  const [busy, setBusy] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probe, setProbe] = useState<HealthProbeReport | null>(null);
  const [errors, setErrors] = useState<{ count?: string; port?: string; path?: string }>({});

  const patch = useMemo(() => {
    const p: Record<string, string | number> = {};
    if (draft.instanceSize !== saved.instanceSize) p.instance_size = draft.instanceSize;
    if (draft.instanceCount !== saved.instanceCount) p.instance_count = draft.instanceCount;
    if (draft.appPort !== saved.appPort) p.app_port = draft.appPort;
    if (draft.healthPath !== saved.healthPath) p.health_path = draft.healthPath;
    return p;
  }, [draft, saved]);
  const dirty = Object.keys(patch).length > 0;

  // Coste de la elección, en vivo. Elegir tamaño e instancias sin ver lo que
  // cuesta es elegir a ciegas: es la única consecuencia que no se puede deducir
  // del propio formulario.
  const memoryOf = (name: string) => sizes.find((s) => s.name === name)?.memory_gib ?? 0;
  const costNow = monthlyComputeEur(memoryOf(saved.instanceSize), saved.instanceCount);
  const costNext = monthlyComputeEur(memoryOf(draft.instanceSize), draft.instanceCount);
  const costKnown = costNext > 0 || costNow > 0;
  const delta = round2(costNext - costNow);

  // Puerto típico del framework detectado: se ofrece de un clic en vez de dejar
  // que se adivine, que es como una app acaba reiniciándose en bucle.
  const suggestedPort = typicalPortFor(framework);
  const showPortHint = suggestedPort !== undefined && draft.appPort !== suggestedPort;

  async function detectPath() {
    setProbing(true);
    try {
      // El BFF devuelve el informe del control-plane tal cual (más un `ok`), pero
      // el tipo de postBff es el genérico de las respuestas del BFF: se estrecha
      // aquí, que es donde se conoce la forma de esta ruta concreta.
      const report = (await postBff('/api/probe-health', {
        project,
      })) as unknown as HealthProbeReport;
      setProbe(report);
      // Adoptar la recomendada de una vez es el caso normal: para eso se pulsa el
      // botón. La lista queda debajo por si se prefiere otra de las que respondió.
      if (report.recommended) {
        setDraft({ ...draft, healthPath: report.recommended });
        toast.notify({
          tone: 'success',
          title: t('runtime.probe.found', { path: report.recommended }),
          description: report.weak ? t('runtime.probe.weak') : t('runtime.probe.checked'),
        });
      } else {
        toast.notify({
          tone: 'warning',
          title: report.reachable ? t('runtime.probe.none') : t('runtime.probe.unreachable'),
        });
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('runtime.probe.fail') });
    } finally {
      setProbing(false);
    }
  }

  function setInstances(n: number) {
    setDraft({ ...draft, instanceCount: Math.min(maxCount, Math.max(1, n)) });
    if (errors.count) {
      const { count: _quitado, ...resto } = errors;
      setErrors(resto);
    }
  }

  function validate(): boolean {
    const next: typeof errors = {};
    if (!Number.isInteger(draft.instanceCount) || draft.instanceCount < 1) {
      next.count = t('runtime.error.count');
    } else if (draft.instanceCount > maxCount) {
      next.count = t('runtime.error.max', { max: maxCount });
    }
    if (!Number.isInteger(draft.appPort) || draft.appPort < 1 || draft.appPort > 65535) {
      next.port = t('runtime.error.port');
    }
    if (!draft.healthPath.startsWith('/')) {
      next.path = t('runtime.error.path');
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  }

  async function save(e: Event) {
    e.preventDefault();
    if (!validate()) return;
    setBusy(true);
    try {
      const { live: applied } = await postBff('/api/project-runtime', { project, ...patch });
      setSaved(draft);
      toast.notify({
        tone: 'success',
        title: t('runtime.saved'),
        // Honestidad sobre CUÁNDO surte efecto: el controlador reaplica la
        // producción en su siguiente ciclo, no en el momento de guardar.
        ...demoDesc(t, applied, t('runtime.saved.desc')),
      });
      notifyDemoIfNeeded(t, toast, applied);
    } catch {
      toast.notify({ tone: 'danger', title: t('runtime.save.fail') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form class="flex flex-col gap-4" onSubmit={(e) => void save(e)}>
      <Card>
        <CardHeader>
          <CardTitle as="h2">{t('runtime.scale')}</CardTitle>
          <Badge tone="neutral">
            {draft.instanceCount} × {draft.instanceSize}
          </Badge>
        </CardHeader>
        <CardBody class="flex flex-col gap-4">
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text mb-1 text-sm font-medium">{t('runtime.size')}</legend>
            {sizes.map((s) => {
              const recursos = humanResources(locale, s.cpu_limit, s.memory_limit);
              const mensual = monthlyComputeEur(s.memory_gib, draft.instanceCount);
              return (
                <label
                  key={s.name}
                  class={
                    'border-border flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2 ' +
                    (draft.instanceSize === s.name ? 'border-primary-600 bg-selected' : '')
                  }
                >
                  <Radio
                    class="accent-primary"
                    name="instance-size"
                    value={s.name}
                    checked={draft.instanceSize === s.name}
                    onChange={() => setDraft({ ...draft, instanceSize: s.name })}
                  />
                  <span class="text-text flex-1 text-sm">{sizeLabel(s)}</span>
                  {recursos ? (
                    <span class="text-text-muted text-xs tabular-nums">{recursos}</span>
                  ) : null}
                  {/* w-28 y sin partir: «/Monat» es más largo que «/mes» y a w-24
                      el importe alemán se caía a una segunda línea. */}
                  {mensual > 0 ? (
                    <span class="text-text w-28 shrink-0 text-right text-xs whitespace-nowrap tabular-nums">
                      {t('runtime.perMonth', { price: formatEur(locale, mensual) })}
                    </span>
                  ) : null}
                </label>
              );
            })}
          </fieldset>

          <FieldGroup
            label={t('runtime.count')}
            htmlFor="instance-count"
            help={t('runtime.count.help', { max: maxCount })}
            {...(errors.count ? { error: errors.count } : {})}
          >
            {/* Los botones hacen el caso normal (subir o bajar una) de un clic; el
                campo sigue ahí para saltar a un número concreto. */}
            <span class="flex items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                aria-label={t('runtime.count.less')}
                disabled={draft.instanceCount <= 1}
                onClick={() => setInstances(draft.instanceCount - 1)}
              >
                −
              </Button>
              <Input
                id="instance-count"
                type="number"
                min="1"
                max={String(maxCount)}
                step="1"
                inputMode="numeric"
                class="w-24 text-center"
                value={String(draft.instanceCount)}
                invalid={errors.count != null}
                aria-describedby={describedBy('instance-count', 'help', errors.count)}
                onInput={(e) =>
                  setDraft({
                    ...draft,
                    instanceCount: Number((e.target as HTMLInputElement).value),
                  })
                }
              />
              <Button
                type="button"
                variant="secondary"
                aria-label={t('runtime.count.more')}
                disabled={draft.instanceCount >= maxCount}
                onClick={() => setInstances(draft.instanceCount + 1)}
              >
                +
              </Button>
            </span>
          </FieldGroup>

          {/* La frase entera es UNA clave (con o sin diferencia): el paréntesis de
              la comparación no cae en el mismo sitio en todos los idiomas. El signo
              sí se pega al importe: es notación, no prosa. */}
          {costKnown ? (
            <p class="text-text-muted border-border border-t pt-3 text-sm tabular-nums">
              {delta !== 0
                ? t('runtime.cost.delta', {
                    cost: formatEur(locale, costNext),
                    delta: `${delta > 0 ? '+' : '−'}${formatEur(locale, Math.abs(delta))}`,
                  })
                : t('runtime.cost', { cost: formatEur(locale, costNext) })}
            </p>
          ) : null}
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle as="h2">{t('runtime.contract')}</CardTitle>
        </CardHeader>
        <CardBody class="grid gap-4 sm:grid-cols-2">
          <FieldGroup
            label={t('runtime.port')}
            htmlFor="app-port"
            help={t('runtime.port.help')}
            {...(errors.port ? { error: errors.port } : {})}
          >
            <Input
              id="app-port"
              type="number"
              min="1"
              max="65535"
              step="1"
              inputMode="numeric"
              value={String(draft.appPort)}
              invalid={errors.port != null}
              aria-describedby={describedBy('app-port', 'help', errors.port)}
              onInput={(e) =>
                setDraft({ ...draft, appPort: Number((e.target as HTMLInputElement).value) })
              }
            />
            {showPortHint ? (
              <Button
                type="button"
                variant="ghost"
                class="mt-1"
                onClick={() => setDraft({ ...draft, appPort: suggestedPort })}
              >
                {/* El puerto va como CADENA: un número se formatearía por locale
                    y 3000 saldría como «3.000», que no es un puerto. */}
                {t('runtime.port.use', { framework, port: String(suggestedPort) })}
              </Button>
            ) : null}
          </FieldGroup>
          <FieldGroup
            label={t('runtime.healthPath')}
            htmlFor="health-path"
            help={t('runtime.healthPath.help')}
            {...(errors.path ? { error: errors.path } : {})}
          >
            <Input
              id="health-path"
              value={draft.healthPath}
              invalid={errors.path != null}
              aria-describedby={describedBy('health-path', 'help', errors.path)}
              onInput={(e) =>
                setDraft({ ...draft, healthPath: (e.target as HTMLInputElement).value })
              }
            />
            {/* Este campo era el único que obligaba a teclear una cadena mágica a
                ciegas, y equivocarse no da un error: da una app reiniciándose en
                bucle. Aquí no se propone nada: se pregunta a la app. */}
            <Button
              type="button"
              variant="secondary"
              class="mt-1"
              loading={probing}
              disabled={probing || !live}
              onClick={() => void detectPath()}
            >
              {t('runtime.detect')}
            </Button>
          </FieldGroup>
        </CardBody>
        {probe ? (
          <CardBody class="border-border border-t">
            {!probe.reachable ? (
              <p class="text-text-muted text-sm">
                {t('runtime.probe.unreachable.body', { url: probe.url })}
              </p>
            ) : (
              <div class="flex flex-col gap-2">
                <ul class="flex flex-col gap-1">
                  {probe.results.map((r) => (
                    <li key={r.path} class="flex items-center gap-2 text-sm">
                      <Badge tone={r.healthy ? 'success' : 'neutral'}>
                        {r.status === 0 ? t('runtime.probe.noResponse') : r.status}
                      </Badge>
                      <span class="text-text flex-1">{r.path}</span>
                      {r.status > 0 ? (
                        <span class="text-text-muted text-xs tabular-nums">
                          {t('common.ms', { count: r.latency_ms })}
                        </span>
                      ) : null}
                      {r.healthy && r.path !== draft.healthPath ? (
                        <Button
                          type="button"
                          variant="ghost"
                          aria-label={t('runtime.probe.usePath', { path: r.path })}
                          onClick={() => setDraft({ ...draft, healthPath: r.path })}
                        >
                          {t('runtime.probe.use')}
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
                {probe.recommended === '' ? (
                  <p class="text-text-muted text-sm">{t('runtime.probe.none.body')}</p>
                ) : probe.weak ? (
                  <p class="text-text-muted text-sm">{t('runtime.probe.weak.body')}</p>
                ) : null}
              </div>
            )}
          </CardBody>
        ) : null}
      </Card>

      <div class="flex items-center gap-3">
        <Button type="submit" variant="primary" loading={busy} disabled={busy || !dirty}>
          {t('common.saveChanges')}
        </Button>
        {dirty ? (
          <Button
            type="button"
            variant="ghost"
            disabled={busy}
            onClick={() => {
              setDraft(saved);
              setErrors({});
            }}
          >
            {t('runtime.discard')}
          </Button>
        ) : null}
        <p class="text-text-muted text-sm">{live ? t('runtime.live') : t('runtime.demo')}</p>
      </div>
    </form>
  );
}
