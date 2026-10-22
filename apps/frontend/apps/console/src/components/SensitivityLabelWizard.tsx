/**
 * SensitivityLabelWizard — asistente corto para crear (o editar) una etiqueta de
 * sensibilidad.
 *
 * Cuatro preguntas, una por paso, en lenguaje llano: cómo se llama y para qué es, qué
 * nivel tiene respecto a las etiquetas que ya existen, qué debe impedir (contado por su
 * consecuencia real, no por el nombre técnico de la política) y un resumen antes de
 * guardar. Nada de «rank», «policy» ni casillas sueltas.
 *
 * Usa los endpoints BFF (POST/PATCH /api/admin/sensitivity-labels). La taxonomía vive
 * en Sytue ID; el bloqueo real de la compartición lo aplica Sytue Drive sobre los
 * archivos etiquetados.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { SensitivityLabel, SensitivityLabelInput } from '../lib/types.js';

export interface SensitivityLabelWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Etiqueta que se edita; ausente = alta. */
  editing?: SensitivityLabel | undefined;
  /** Taxonomía actual (para colocar la nueva etiqueta entre las que ya hay). */
  labels: SensitivityLabel[];
  onSaved: (label: SensitivityLabel) => void;
  onCancel: () => void;
  onDone: () => void;
}

/** Paleta corta y sobria; el color solo sirve para reconocer la etiqueta de un vistazo. */
const COLORS: { value: string; label: ConsoleMessageKey }[] = [
  { value: '', label: 'wizard.label.color.none' },
  { value: '#4b5563', label: 'wizard.label.color.grey' },
  { value: '#1d4ed8', label: 'wizard.label.color.blue' },
  { value: '#047857', label: 'wizard.label.color.green' },
  { value: '#b45309', label: 'wizard.label.color.amber' },
  { value: '#b91c1c', label: 'wizard.label.color.red' },
];

/**
 * Nivel a partir de la POSICIÓN elegida en la lista: el admin coloca la etiqueta entre
 * las que ya tiene («más sensible que Interno») y aquí se traduce a un número. Deja
 * huecos de 10 para poder intercalar después sin renumerar.
 */
function rankForPosition(others: SensitivityLabel[], pos: number): number {
  if (others.length === 0) return 10;
  const first = others[0] as SensitivityLabel;
  const last = others[others.length - 1] as SensitivityLabel;
  if (pos <= 0) return Math.max(0, first.rank - 10);
  if (pos >= others.length) return last.rank + 10;
  const prev = (others[pos - 1] as SensitivityLabel).rank;
  const next = (others[pos] as SensitivityLabel).rank;
  const mid = Math.floor((prev + next) / 2);
  return mid > prev && mid < next ? mid : prev + 1;
}

/** Posición inicial al editar: dónde está hoy la etiqueta dentro del resto. */
function positionOf(others: SensitivityLabel[], rank: number): number {
  const i = others.findIndex((l) => l.rank > rank);
  return i < 0 ? others.length : i;
}

interface FormState {
  name: string;
  description: string;
  color: string;
  position: number;
  block: boolean;
}

export default function SensitivityLabelWizard({
  t,
  editing,
  labels,
  onSaved,
  onCancel,
  onDone,
}: SensitivityLabelWizardProps) {
  const toast = useToast();
  // Al editar, la propia etiqueta no cuenta como referencia de nivel.
  const others = useMemo(
    () => labels.filter((l) => l.id !== editing?.id).sort((a, b) => a.rank - b.rank),
    [labels, editing],
  );
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<FormState>(() =>
    editing
      ? {
          name: editing.name,
          description: editing.description,
          color: editing.color,
          position: positionOf(others, editing.rank),
          block: editing.blockExternalShare,
        }
      : { name: '', description: '', color: '', position: others.length, block: false },
  );

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    setForm((f) => ({ ...f, [k]: v }));
  }

  const name = form.name.trim();

  /** La posición, contada en llano («más sensible que Interno»). */
  function positionText(pos: number): string {
    if (others.length === 0) return t('wizard.label.level.first');
    if (pos <= 0) return t('wizard.label.level.lowest', { name: others[0]?.name ?? '' });
    if (pos >= others.length)
      return t('wizard.label.level.highest', {
        name: others[others.length - 1]?.name ?? '',
      });
    return t('wizard.label.level.between', {
      lower: others[pos - 1]?.name ?? '',
      upper: others[pos]?.name ?? '',
    });
  }

  const effectText = t(
    form.block ? 'wizard.label.effect.summaryBlock' : 'wizard.label.effect.summaryInform',
  );

  async function save() {
    if (busy) return;
    setBusy(true);
    const payload: SensitivityLabelInput = {
      name,
      description: form.description.trim(),
      color: form.color,
      rank: rankForPosition(others, form.position),
      block_external_share: form.block,
    };
    try {
      const res = await fetch(
        editing
          ? `/api/admin/sensitivity-labels/${encodeURIComponent(editing.id)}`
          : '/api/admin/sensitivity-labels',
        {
          method: editing ? 'PATCH' : 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(payload),
        },
      );
      if (!res.ok) throw new Error(String(res.status));
      onSaved((await res.json()) as SensitivityLabel);
      setStep(4);
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('wizard.label.toast.saveFailed'),
        description: t(
          code === '409' ? 'wizard.label.toast.nameTaken' : 'wizard.label.toast.retry',
        ),
      });
    } finally {
      setBusy(false);
    }
  }

  const stepWhat: WizardStep = {
    id: 'what',
    title: t('wizard.label.step.what'),
    heading: t('wizard.label.what.heading'),
    description: t('wizard.label.what.description'),
    canContinue: name.length > 0,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <FieldGroup label={t('wizard.label.what.nameLabel')} htmlFor="lbl-name">
          <Input
            id="lbl-name"
            value={form.name}
            placeholder={t('wizard.label.what.namePlaceholder')}
            aria-describedby="lbl-name-hint"
            onInput={(e) => set('name', e.currentTarget.value)}
          />
          <FieldHint id="lbl-name-hint">{t('wizard.label.what.nameHint')}</FieldHint>
        </FieldGroup>

        <FieldGroup label={t('wizard.label.what.purposeLabel')} htmlFor="lbl-desc">
          <Textarea
            id="lbl-desc"
            rows={3}
            value={form.description}
            placeholder={t('wizard.label.what.purposePlaceholder')}
            aria-describedby="lbl-desc-hint"
            onInput={(e) => set('description', e.currentTarget.value)}
          />
          <FieldHint id="lbl-desc-hint">{t('wizard.label.what.purposeHint')}</FieldHint>
        </FieldGroup>

        <fieldset class="flex flex-col gap-2">
          <legend class="text-text-muted text-xs font-medium">
            {t('wizard.label.what.colourLegend')}
          </legend>
          <div class="flex flex-wrap items-center gap-2">
            {COLORS.map((c) => (
              <button
                key={c.value || 'none'}
                type="button"
                aria-pressed={form.color === c.value}
                aria-label={t(c.label)}
                title={t(c.label)}
                onClick={() => set('color', c.value)}
                class={`border-border rounded-control flex h-8 items-center gap-2 border px-2.5 text-xs transition-colors ${
                  form.color === c.value ? 'border-primary bg-bg-subtle' : 'hover:bg-bg-subtle'
                }`}
              >
                <span
                  aria-hidden="true"
                  class="border-border-subtle h-3 w-3 rounded-full border"
                  style={c.value ? { backgroundColor: c.value } : undefined}
                />
                {t(c.label)}
              </button>
            ))}
          </div>
          <FieldHint>{t('wizard.label.what.colourHint')}</FieldHint>
        </fieldset>
      </div>
    ),
  };

  const stepLevel: WizardStep = {
    id: 'level',
    title: t('wizard.label.step.level'),
    heading: t('wizard.label.level.heading'),
    description: t('wizard.label.level.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-3">
        {others.length === 0 ? (
          <InlineHelp tone="info" title={t('wizard.label.level.firstTitle')}>
            {t('wizard.label.level.firstBody')}
          </InlineHelp>
        ) : (
          <div class="flex flex-col gap-2">
            {Array.from({ length: others.length + 1 }, (_, pos) => (
              <ChoiceCard
                key={pos}
                dense
                name="sytue-label-position"
                value={String(pos)}
                selected={form.position === pos}
                title={positionText(pos)}
                description={t(
                  pos <= 0
                    ? 'wizard.label.level.hintLowest'
                    : pos >= others.length
                      ? 'wizard.label.level.hintHighest'
                      : 'wizard.label.level.hintMiddle',
                )}
                onSelect={() => set('position', pos)}
              />
            ))}
          </div>
        )}
        <p class="sytue-meta leading-relaxed">
          {t('wizard.label.level.scale', {
            scale: others.length
              ? others.map((l) => l.name).join(' · ')
              : t('wizard.label.level.scaleEmpty'),
          })}
        </p>
      </div>
    ),
  };

  const stepEffect: WizardStep = {
    id: 'effect',
    title: t('wizard.label.step.effect'),
    heading: t('wizard.label.effect.heading'),
    description: t('wizard.label.effect.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ChoiceCard
          name="sytue-label-effect"
          selected={!form.block}
          title={t('wizard.label.effect.informTitle')}
          description={t('wizard.label.effect.informBody')}
          onSelect={() => set('block', false)}
          icon="info"
        />
        <ChoiceCard
          name="sytue-label-effect"
          selected={form.block}
          title={t('wizard.label.effect.blockTitle')}
          description={t('wizard.label.effect.blockBody')}
          onSelect={() => set('block', true)}
          icon="lock"
        />
        {form.block ? (
          <InlineHelp tone="warning" title={t('wizard.label.effect.warnTitle')}>
            {t('wizard.label.effect.warnBody')}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    { label: t('wizard.label.summary.name'), value: name || '—', onEdit: () => setStep(0) },
    {
      label: t('wizard.label.summary.purpose'),
      value: form.description.trim() || t('wizard.label.summary.noPurpose'),
      onEdit: () => setStep(0),
    },
    {
      label: t('wizard.label.summary.level'),
      value: positionText(form.position),
      onEdit: () => setStep(1),
    },
    { label: t('wizard.label.summary.effect'), value: effectText, onEdit: () => setStep(2) },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.label.step.review'),
    heading: t(editing ? 'wizard.label.review.headingEdit' : 'wizard.label.review.headingNew'),
    description: t('wizard.label.review.description'),
    nextLabel: t(editing ? 'wizard.label.review.saveEdit' : 'wizard.label.review.saveNew'),
    canContinue: name.length > 0,
    onNext: () => void save(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={summary} />
        {editing ? (
          <InlineHelp tone="info" title={t('wizard.label.review.editNoteTitle')}>
            {t('wizard.label.review.editNoteBody')}
          </InlineHelp>
        ) : (
          <InlineHelp tone="neutral" title={t('wizard.label.review.newNoteTitle')}>
            {t('wizard.label.review.newNoteBody')}
          </InlineHelp>
        )}
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('wizard.label.step.done'),
    heading: t(editing ? 'wizard.label.done.headingEdit' : 'wizard.label.done.headingNew'),
    done: true,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <InlineHelp tone="success" title={name}>
          {t(form.block ? 'wizard.label.done.bodyBlock' : 'wizard.label.done.bodyInform')}
        </InlineHelp>
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('wizard.label.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWhat, stepLevel, stepEffect, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}
