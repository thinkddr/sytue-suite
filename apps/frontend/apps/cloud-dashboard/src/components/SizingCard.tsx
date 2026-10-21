/**
 * SizingCard — el punto de coste óptimo del proyecto, en una tarjeta y un botón.
 *
 * NO es un informe: no hay gráficas que explorar ni tablas que interpretar. Dice
 * qué pasa y ofrece arreglarlo. Si no hay nada que hacer, casi no se ve.
 *
 * Subir y bajar NO se confirman igual. Equivocarse al subir le cuesta unos euros
 * al cliente; equivocarse al bajar le tira el servicio. Así que bajar enseña qué
 * se arriesga, con qué medida lo sabemos y cómo se deshace, y no habilita el
 * botón hasta que se reconoce. Subir se queda en el aviso de reinicio.
 *
 * El dato viene de GET /projects/{id}/sizing, que decide sobre consumo REAL
 * (muestras de metrics.k8s.io). Aplicarlo reusa /api/project-runtime, que es el
 * MISMO camino que el resto de la configuración de ejecución: valida contra el
 * catálogo, audita y respeta la cuota del plan. Un atajo propio se habría saltado
 * alguna de las tres. Solo @sytue/ui. §14.
 */
import { useId, useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Button,
  Badge,
  Checkbox,
  Icon,
  InlineHelp,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { SizingAdvice } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff } from '../lib/bff-client.js';
import { confirmationFor } from '../lib/sizing.js';

/** Importe mensual en euros, con el formato del idioma del usuario. */
function eur(locale: Locale, value: number): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'EUR',
    maximumFractionDigits: 2,
  }).format(Math.abs(value));
}

interface Props {
  projectSlug: string;
  advice: SizingAdvice;
  /** Nombre legible de cada tamaño del catálogo, para no enseñar el slug crudo. */
  sizeLabels: Record<string, string>;
  live: boolean;
  locale: Locale;
}

/**
 * El ToastProvider es propio y no heredado: esta tarjeta es una isla aparte de
 * OverviewPanel, y el contexto de Preact no cruza de una isla a otra. Sin él
 * `useToast` cae al no-op del design-system y el aviso se pierde — justo en la
 * única acción de la pantalla, donde «aplicado» y «no se pudo aplicar» se veían
 * exactamente igual: la tarjeta desaparecía o se quedaba quieta, sin decir nada.
 */
export default function SizingCard(props: Props) {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <SizingCardInner {...props} t={t} />
    </ToastProvider>
  );
}

function SizingCardInner({ projectSlug, advice, sizeLabels, live, locale, t }: Props & { t: T }) {
  const toast = useToast();
  const [applying, setApplying] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [done, setDone] = useState(false);
  // Enlaza el botón con el bloque que abre (patrón «disclosure»): sin esto un
  // lector de pantalla no dice que el botón despliega algo ni qué despliega.
  const confirmId = useId();

  // Sin control-plane vivo no se pinta NADA. Una recomendación inventada aquí es
  // peor que el silencio, porque el usuario la aplicaría.
  if (!live || done) return null;

  const label = (name: string) => sizeLabels[name] ?? name;

  async function apply() {
    setApplying(true);
    try {
      await postBff('/api/project-runtime', { project: projectSlug, instance_size: advice.to });
      toast.notify({ tone: 'success', title: t('sizing.applied', { size: label(advice.to) }) });
      setDone(true);
    } catch {
      toast.notify({ tone: 'danger', title: t('sizing.applyFailed') });
    } finally {
      setApplying(false);
      setConfirming(false);
      setAcknowledged(false);
    }
  }

  // --- Aún midiendo: honesto, discreto y sin botón -------------------------
  // Si hay cohorte, se añade lo que sí sabemos —el consumo de las apps
  // comparables— sin dar a entender que hemos medido ESTA. Es dato de terceros y
  // se dice así; sin botón, porque aquí no hay ahorro que ofrecer.
  if (advice.reason === 'insufficient_data') {
    return (
      <Card>
        <CardBody>
          <p class="text-text-subtle flex items-center gap-2 text-sm">
            <Icon name="info" size={16} />
            {t('sizing.measuring', { hours: advice.window_hours })}
          </p>
          {advice.benchmark && (
            <p class="text-text-subtle mt-1 pl-6 text-sm">
              {t('sizing.cohort.hint', {
                percent: advice.benchmark.p50_memory_percent,
                tenants: advice.benchmark.tenants,
              })}
            </p>
          )}
        </CardBody>
      </Card>
    );
  }

  // --- Nada que hacer: confirmación mínima, no pide atención ---------------
  if (advice.action === 'keep' && advice.reason !== 'peak_blocks_downgrade') {
    return (
      <Card>
        <CardBody>
          <p class="text-text-subtle flex items-center gap-2 text-sm">
            <Icon name="check" size={16} />
            {t('sizing.wellSized', {
              size: label(advice.from),
              percent: advice.p95_memory_percent,
            })}
          </p>
        </CardBody>
      </Card>
    );
  }

  // Sobre qué se decidió. Una recomendación de dinero que no enseña su medida no
  // se puede contrastar, y la que dice «no te dejo ahorrar» menos todavía.
  const evidenceLine =
    advice.samples > 0 ? (
      <p class="text-text-subtle mt-1 text-xs tabular-nums">
        {t('sizing.evidence', { samples: advice.samples, hours: advice.window_hours })}
      </p>
    ) : null;

  // --- El régimen cabría, el pico no: se DICE, no se esconde ---------------
  // El cliente ve un consumo bajo y merece saber por qué no le ofrecemos el
  // ahorro. «No puedes ahorrar POR ESTO» es lo que le permite decidir.
  if (advice.reason === 'peak_blocks_downgrade') {
    return (
      <Card>
        <CardBody>
          <div class="flex items-start gap-3">
            <Icon name="warning" size={20} />
            <div>
              <p class="text-sm font-medium">{t('sizing.peakBlocks.title')}</p>
              <p class="text-text-subtle mt-1 text-sm">
                {t('sizing.peakBlocks.body', {
                  usual: advice.p95_memory_percent,
                  peak: advice.peak_memory_percent,
                })}
              </p>
              {evidenceLine}
            </div>
          </div>
        </CardBody>
      </Card>
    );
  }

  // Bajar (= ahorrar) es la única dirección que puede tirar el servicio, así que
  // es la única que pide reconocimiento explícito. La política, en lib/sizing.ts.
  const { requiresAck: saving, evidence } = confirmationFor(advice);
  const amount = eur(locale, advice.monthly_delta_eur);
  // Sin datos propios el ahorro sale de las apps comparables, y el texto tiene
  // que decirlo: «usas el 0 %» sería falso —no hemos medido nada— y además el
  // motivo por el que el cliente dejaría de fiarse de la siguiente recomendación.
  const cohort = evidence === 'cohort' ? advice.benchmark : undefined;

  return (
    <Card>
      <CardBody>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <Badge tone={saving ? 'success' : 'warning'}>
                {t(saving ? 'sizing.badge.save' : 'sizing.badge.risk')}
              </Badge>
            </div>
            {/* El titular es la CIFRA. Es lo primero que se lee y lo único que
                hace falta para decidir; los porcentajes van debajo. */}
            <p class="mt-2 text-base font-medium">
              {cohort
                ? t('sizing.cohort.title', { amount })
                : saving
                  ? t('sizing.down.title', { amount })
                  : t('sizing.up.title', { size: label(advice.to) })}
            </p>
            <p class="text-text-subtle mt-1 text-sm">
              {cohort
                ? t('sizing.cohort.body', {
                    percent: cohort.p50_memory_percent,
                    tenants: cohort.tenants,
                    from: label(advice.from),
                    to: label(advice.to),
                  })
                : saving
                  ? t('sizing.down.body', {
                      percent: advice.p95_memory_percent,
                      from: label(advice.from),
                      to: label(advice.to),
                    })
                  : t('sizing.up.body', { percent: advice.p95_memory_percent, amount })}
            </p>
            {cohort ? null : evidenceLine}
          </div>
          <Button
            variant="primary"
            onClick={() => setConfirming(true)}
            disabled={applying}
            aria-expanded={confirming}
            aria-controls={confirmId}
            aria-label={t('sizing.applyAria', { size: label(advice.to) })}
          >
            {t(saving ? 'sizing.action.down' : 'sizing.action.up', { size: label(advice.to) })}
          </Button>
        </div>

        {confirming && (
          <div class="mt-4" id={confirmId}>
            {saving ? (
              /* BAJAR: lo que se arriesga, con qué lo sabemos y cómo se deshace.
                 Equivocarse aquí no cuesta dinero, tira el servicio — así que
                 esta rama pide un reconocimiento que la de subir no pide. */
              <InlineHelp tone="warning" title={t('sizing.down.risk.title')}>
                <p>
                  {cohort
                    ? t('sizing.cohort.risk.body', { to: label(advice.to) })
                    : t('sizing.down.risk.body', {
                        peak: advice.peak_memory_percent,
                        from: label(advice.from),
                        to: label(advice.to),
                      })}
                </p>
                <p class="mt-1">{t('sizing.restartWarning')}</p>
                <p class="mt-1">{t('sizing.down.reversible', { from: label(advice.from) })}</p>
                <Checkbox
                  class="mt-2"
                  label={t('sizing.down.ack')}
                  checked={acknowledged}
                  disabled={applying}
                  onChange={(e) => setAcknowledged(e.currentTarget.checked)}
                />
              </InlineHelp>
            ) : (
              /* SUBIR: la única consecuencia es el reinicio y el importe, que ya
                 va en el titular. Se dice sin dramatizar y sin fricción extra. */
              <InlineHelp tone="warning">{t('sizing.restartWarning')}</InlineHelp>
            )}
            <div class="mt-3 flex gap-2">
              {/* `void`: `apply` es async y su promesa no la observa nadie. Sin esto el
                  rechazo se perdería en silencio — y ese rechazo es «no se ha aplicado
                  el cambio de tamaño», que el usuario tiene que ver. `apply` ya captura
                  su error y lo pinta; el void solo dice que aquí no se espera. */}
              <Button
                variant="primary"
                onClick={() => void apply()}
                loading={applying}
                disabled={applying || (saving && !acknowledged)}
              >
                {applying ? t('sizing.applying') : t('sizing.confirm')}
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  setConfirming(false);
                  setAcknowledged(false);
                }}
                disabled={applying}
              >
                {t('common.cancel')}
              </Button>
            </div>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
