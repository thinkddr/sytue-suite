/**
 * VacationModal — autorespuesta / vacaciones (respuesta automática de
 * fuera-de-oficina). Configuración GUIADA: explica en llano qué va a pasar, dice
 * si está respondiendo AHORA MISMO, valida las fechas en vivo y enseña el mensaje
 * tal y como lo va a recibir quien escriba. La ENTREGA ENTRANTE del servicio
 * encola UNA autorespuesta por remitente respetando RFC 3834 (no responde a
 * listas, rebotes ni auto-generados) y un dedupe por ventana; aquí solo el
 * get/set vía BFF. docs/CONVENTIONS.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  Textarea,
  Switch,
  Modal,
  Spinner,
  useToast,
} from '@sytue/ui';
import { getVacation, saveVacation } from '../client/mailApi.js';
import { formatDate, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  onClose: () => void;
}

/** ISO (UTC) → valor de <input type="date"> (Y-M-D en hora local). */
function toDateInput(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 10);
}

/** Valor de <input type="date"> → ISO. `endOfDay` incluye el día completo. */
function fromDateInput(value: string, endOfDay: boolean): string {
  if (!value) return '';
  const d = new Date(`${value}T${endOfDay ? '23:59:59' : '00:00:00'}`);
  return Number.isNaN(d.getTime()) ? '' : d.toISOString();
}

/** Hoy en formato de <input type="date">, para comparar rangos como texto. */
function todayInput(): string {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
}

/**
 * «14 de marzo» / «14 March» / «14. März» a partir de un valor de <input
 * type="date"> (hora local). El día y el mes bastan: el año lo pone el contexto
 * (unas vacaciones se configuran para el año en curso).
 */
function humanDate(locale: Locale, value: string): string {
  const d = new Date(`${value}T12:00:00`);
  return Number.isNaN(d.getTime())
    ? value
    : formatDate(locale, d, { day: 'numeric', month: 'long' });
}

/** En qué situación está la autorespuesta AHORA, en lenguaje llano. */
interface Status {
  tone: 'success' | 'warning' | 'neutral';
  label: string;
  detail: string;
}

function statusOf(t: T, locale: Locale, enabled: boolean, start: string, end: string): Status {
  const today = todayInput();
  if (!enabled) {
    return {
      tone: 'neutral',
      label: t('vacation.statusOffLabel'),
      detail: t('vacation.statusOffDetail'),
    };
  }
  if (end && end < today) {
    return {
      tone: 'warning',
      label: t('vacation.statusEndedLabel'),
      detail: t('vacation.statusEndedDetail', { date: humanDate(locale, end) }),
    };
  }
  if (start && start > today) {
    return {
      tone: 'warning',
      label: t('vacation.statusScheduledLabel'),
      detail: t('vacation.statusScheduledDetail', { date: humanDate(locale, start) }),
    };
  }
  return {
    tone: 'success',
    label: t('vacation.statusOnLabel'),
    detail: end
      ? t('vacation.statusOnDetailUntil', { date: humanDate(locale, end) })
      : t('vacation.statusOnDetailOpen'),
  };
}

export function VacationModal({ locale, open, onClose }: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // Si la carga falla no se muestra el formulario (evita guardar en blanco y
  // borrar la autorespuesta real del usuario). reload dispara el reintento.
  const [loadError, setLoadError] = useState(false);
  const [reload, setReload] = useState(0);
  const [enabled, setEnabled] = useState(false);
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setLoadError(false);
    getVacation()
      .then((v) => {
        if (!active) return;
        setEnabled(v.enabled);
        setSubject(v.subject);
        setBody(v.body);
        setStartDate(toDateInput(v.startsAt));
        setEndDate(toDateInput(v.endsAt));
      })
      .catch(() => active && setLoadError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  // Validación en vivo: el fallo típico es poner la vuelta antes de la salida, o
  // dejar un período ya pasado (la autorespuesta no se dispararía nunca).
  const badRange = Boolean(startDate && endDate && endDate < startDate);
  const status = statusOf(t, locale, enabled, startDate, endDate);
  const previewSubject = subject.trim() || t('vacation.subjectPlaceholder');
  const previewBody = body.trim();

  async function handleSave(): Promise<void> {
    // Nunca guardar sin una carga exitosa: evita sobrescribir con blancos.
    if (saving || loadError || badRange) return;
    setSaving(true);
    try {
      await saveVacation({
        enabled,
        subject: subject.trim(),
        body,
        startsAt: fromDateInput(startDate, false),
        endsAt: fromDateInput(endDate, true),
      });
      toast.notify({ tone: 'success', title: t('vacation.saved') });
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('vacation.saveFailed') });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('vacation.title')}
      size="md"
      footer={
        <>
          <Button variant="ghost" disabled={saving} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={saving}
            disabled={loadError || loading || badRange}
            onClick={() => void handleSave()}
          >
            {t('common.save')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div class="text-text-subtle flex items-center gap-2 py-6">
          <Spinner size={16} label={t('vacation.loading')} />
          <span class="text-sm" aria-hidden="true">
            {t('common.loading')}
          </span>
        </div>
      ) : loadError ? (
        <div class="flex flex-col items-start gap-2 py-2" role="alert">
          <p class="text-text flex items-start gap-2 text-sm">
            <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
            {t('vacation.loadError')}
          </p>
          <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
            {t('common.retry')}
          </Button>
        </div>
      ) : (
        <div class="flex flex-col gap-5">
          {/* Qué va a pasar, antes de tocar nada. */}
          <InlineHelp tone="info" title={t('vacation.explainTitle')}>
            {t('vacation.explainBody')}
          </InlineHelp>

          <div class="border-border flex items-start justify-between gap-3 rounded-md border p-3">
            <div class="min-w-0">
              <p class="text-text flex flex-wrap items-center gap-2 text-sm font-semibold">
                {t('vacation.autoReply')}
                <Badge tone={status.tone}>{status.label}</Badge>
              </p>
              <p class="text-text-muted mt-1 text-sm leading-snug">{status.detail}</p>
            </div>
            <Switch
              label={t('vacation.enabled')}
              checked={enabled}
              onChange={(e) => setEnabled(e.currentTarget.checked)}
              class="shrink-0 text-sm"
            />
          </div>

          <FieldGroup label={t('common.subject')} htmlFor="vac-subject">
            <Input
              id="vac-subject"
              type="text"
              value={subject}
              placeholder={t('vacation.subjectPlaceholder')}
              aria-describedby="vac-subject-hint"
              onInput={(e) => setSubject(e.currentTarget.value)}
            />
            <FieldHint id="vac-subject-hint">{t('vacation.subjectHint')}</FieldHint>
          </FieldGroup>

          <FieldGroup label={t('common.message')} htmlFor="vac-body">
            <Textarea
              id="vac-body"
              rows={5}
              value={body}
              placeholder={t('vacation.bodyPlaceholder')}
              aria-describedby="vac-body-hint"
              onInput={(e) => setBody(e.currentTarget.value)}
            />
            <FieldHint id="vac-body-hint">{t('vacation.bodyHint')}</FieldHint>
          </FieldGroup>

          <div class="grid gap-4 sm:grid-cols-2">
            <FieldGroup label={t('vacation.from')} htmlFor="vac-start">
              <Input
                id="vac-start"
                type="date"
                value={startDate}
                aria-describedby="vac-start-hint"
                onInput={(e) => setStartDate(e.currentTarget.value)}
              />
              <FieldHint id="vac-start-hint">
                {startDate
                  ? t('vacation.fromHintSet', { date: humanDate(locale, startDate) })
                  : t('vacation.fromHintEmpty')}
              </FieldHint>
            </FieldGroup>
            <FieldGroup label={t('vacation.until')} htmlFor="vac-end">
              <Input
                id="vac-end"
                type="date"
                value={endDate}
                invalid={badRange}
                {...(startDate ? { min: startDate } : {})}
                aria-describedby="vac-end-hint"
                onInput={(e) => setEndDate(e.currentTarget.value)}
              />
              <FieldHint id="vac-end-hint" tone={badRange ? 'danger' : 'neutral'}>
                {badRange
                  ? t('vacation.untilHintBadRange')
                  : endDate
                    ? t('vacation.untilHintSet', { date: humanDate(locale, endDate) })
                    : t('vacation.untilHintEmpty')}
              </FieldHint>
            </FieldGroup>
          </div>

          {/* Previsualización: el mensaje tal y como lo recibirán. */}
          <section class="flex flex-col gap-2">
            <p class="text-text text-sm font-semibold">{t('vacation.previewTitle')}</p>
            <div class="border-border bg-bg-subtle rounded-md border p-3">
              <p class="text-text truncate text-sm font-semibold">{previewSubject}</p>
              {previewBody ? (
                <p class="text-text-muted mt-1.5 text-sm leading-relaxed whitespace-pre-wrap">
                  {previewBody}
                </p>
              ) : (
                <p class="text-text-subtle mt-1.5 text-sm italic">{t('vacation.previewEmpty')}</p>
              )}
            </div>
          </section>
        </div>
      )}
    </Modal>
  );
}
