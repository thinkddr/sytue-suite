/**
 * FiltersModal — reglas/filtros de servidor de Sytue Mail. Lista las reglas de la
 * cuenta (activar/desactivar, borrar) y crea nuevas con un ASISTENTE corto:
 * qué correos → qué hacer con ellos → resumen en una frase → crear. Las aplica el
 * servidor al correo entrante; aquí solo el CRUD vía BFF. docs/CONVENTIONS.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  Input,
  InlineHelp,
  ReviewSummary,
  Select,
  Checkbox,
  Switch,
  SettingsSection,
  Modal,
  Icon,
  Spinner,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { IconName, WizardStep } from '@sytue/ui';
import {
  listFilters,
  saveFilter,
  deleteFilter,
  type Mailbox,
  type MailFilter,
  type MailFilterInput,
} from '../client/mailApi.js';
import { formatList, type Locale } from '@sytue/i18n';
import { useT, type MailMessageKey } from '../i18n/index.js';
import { mailboxLabel } from './utils.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  /** Carpetas de la cuenta, para el destino de «Mover a…». */
  mailboxes: Mailbox[];
  onClose: () => void;
  /** Borrador inicial (el remitente del correo abierto): abre el asistente ya relleno. */
  initial?: Partial<Draft> | undefined;
}

/** Estado del formulario de nueva regla. */
interface Draft {
  name: string;
  matchFrom: string;
  matchSubject: string;
  moveTo: string; // '' = no mover
  markRead: boolean;
  markFlagged: boolean;
}

const EMPTY: Draft = {
  name: '',
  matchFrom: '',
  matchSubject: '',
  moveTo: '',
  markRead: false,
  markFlagged: false,
};

/** Una regla es aplicable si tiene al menos una condición y una acción. */
function draftValid(d: Draft): boolean {
  const hasCondition = d.matchFrom.trim() !== '' || d.matchSubject.trim() !== '';
  const hasAction = d.moveTo !== '' || d.markRead || d.markFlagged;
  return hasCondition && hasAction;
}

/**
 * Destinos con nombre propio: cada uno es una consecuencia que se entiende sola.
 * Solo CLAVES: el texto lo resuelve la `t` del componente (el mapa es de módulo y
 * no puede llevar cadenas ya traducidas).
 */
const ROLE_CHOICE: Record<
  string,
  { title: MailMessageKey; description: MailMessageKey; icon: IconName }
> = {
  archive: {
    title: 'rules.archiveTitle',
    description: 'rules.archiveDescription',
    icon: 'bucket',
  },
  junk: {
    title: 'rules.junkTitle',
    description: 'rules.junkDescription',
    icon: 'warning',
  },
  trash: {
    title: 'rules.trashTitle',
    description: 'rules.trashDescription',
    icon: 'close',
  },
};

export function FiltersModal({
  locale,
  open,
  mailboxes,
  onClose,
  initial,
}: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [filters, setFilters] = useState<MailFilter[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState(false);
  // El pie del diálogo, donde el asistente pone su botonera (`footerTarget`): dentro del
  // cuerpo, en el teléfono «Continuar» quedaba fuera de la vista en el paso 2.
  const [pieAsistente, setPieAsistente] = useState<HTMLDivElement | null>(null);
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const [reload, setReload] = useState(0);
  // Asistente de creación: null = viendo la lista; 0..2 = paso del asistente.
  const [step, setStep] = useState<number | null>(null);
  // «Otra carpeta»: el destino se elige en un desplegable aparte.
  const [pickOther, setPickOther] = useState(false);
  // Al abrir: con un borrador inicial («Crear una regla para este remitente») el
  // asistente arranca en el primer paso con el remitente ya puesto; sin él, la lista.
  //
  // Lo decide ESTE efecto y solo este. La carga de la lista ponía también `step` a
  // null y, como corre después, tapaba el asistente: se veía la lista, y «Crear
  // regla» (startWizard) empezaba en blanco, con el remitente perdido.
  useEffect(() => {
    if (!open) return;
    if (initial) {
      setDraft({ ...EMPTY, ...initial });
      setStep(0);
    } else {
      setStep(null);
    }
  }, [open, initial]);

  // Destinos de «Mover a…»: cualquier carpeta salvo Borradores/Enviados (mover
  // correo entrante ahí no tiene sentido).
  const moveTargets = mailboxes.filter((m) => m.role !== 'drafts' && m.role !== 'sent');
  // Carpetas con consecuencia propia (Archivo/Spam/Papelera) → su tarjeta; el resto
  // caen en «Moverlo a otra carpeta» con un desplegable.
  const roleTargets = moveTargets.flatMap((m) => {
    const choice = m.role ? ROLE_CHOICE[m.role] : undefined;
    return choice ? [{ mailbox: m, choice }] : [];
  });
  const otherTargets = moveTargets.filter((m) => !m.role || !ROLE_CHOICE[m.role]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError(false);
    listFilters()
      .then(({ filters: list }) => active && setFilters(list))
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  function mailboxName(id: string): string {
    const mb = mailboxes.find((m) => m.id === id);
    return mb ? mailboxLabel(t, mb.name, mb.role) : t('rules.someFolder');
  }

  // Las enumeraciones las monta `formatList` (Intl.ListFormat): «a, b y c» /
  // «a, b and c» / «a, b und c». Concatenarlas con «y» las rompería fuera del
  // español.
  function conditionText(f: Pick<MailFilter, 'matchFrom' | 'matchSubject'>): string {
    const parts: string[] = [];
    if (f.matchFrom) parts.push(t('rules.conditionFrom', { value: f.matchFrom }));
    if (f.matchSubject) parts.push(t('rules.conditionSubject', { value: f.matchSubject }));
    return parts.length ? formatList(locale, parts) : t('rules.conditionAny');
  }

  function actionText(f: Pick<MailFilter, 'moveTo' | 'markRead' | 'markFlagged'>): string {
    const parts: string[] = [];
    if (f.moveTo) parts.push(t('rules.actionMove', { folder: mailboxName(f.moveTo) }));
    if (f.markRead) parts.push(t('rules.actionRead'));
    if (f.markFlagged) parts.push(t('rules.actionFlag'));
    return parts.length ? formatList(locale, parts) : t('rules.actionNone');
  }

  /** La regla contada como una frase: «A los correos … los …». */
  function ruleSentence(d: Draft): string {
    const from = d.matchFrom.trim();
    const subject = d.matchSubject.trim();
    // Frase entera del catálogo: el orden de «quién» y «qué hago» cambia entre
    // lenguas y armarla a trozos con `+` la rompería.
    const who = from
      ? subject
        ? t('rules.whoFromSubject', { from, subject })
        : t('rules.whoFrom', { from })
      : t('rules.whoSubject', { subject });
    const does: string[] = [];
    if (d.moveTo) does.push(t('rules.doMove', { folder: mailboxName(d.moveTo) }));
    if (d.markRead) does.push(t('rules.doRead'));
    if (d.markFlagged) does.push(t('rules.doFlag'));
    return does.length
      ? t('rules.sentence', { who, actions: formatList(locale, does) })
      : t('rules.sentenceNothing', { who });
  }

  // Alterna activa/inactiva reenviando la regla completa (el servidor reemplaza
  // todos los campos mutables). Optimista con revert por toast.
  function handleToggle(f: MailFilter): void {
    const next = !f.enabled;
    setFilters((prev) => prev.map((x) => (x.id === f.id ? { ...x, enabled: next } : x)));
    void saveFilter({ id: f.id, ...toInput(f), enabled: next }).catch(() => {
      setFilters((prev) => prev.map((x) => (x.id === f.id ? { ...x, enabled: !next } : x)));
      toast.notify({ tone: 'danger', title: t('rules.toggleFailed') });
    });
  }

  function handleDelete(f: MailFilter): void {
    const prev = filters;
    setFilters((list) => list.filter((x) => x.id !== f.id));
    void deleteFilter(f.id).catch(() => {
      setFilters(prev);
      toast.notify({ tone: 'danger', title: t('rules.deleteFailed') });
    });
  }

  async function handleAdd(): Promise<void> {
    if (!draftValid(draft) || saving) return;
    setSaving(true);
    try {
      await saveFilter({
        name: draft.name.trim(),
        enabled: true,
        matchFrom: draft.matchFrom.trim(),
        matchSubject: draft.matchSubject.trim(),
        moveTo: draft.moveTo || null,
        markRead: draft.markRead,
        markFlagged: draft.markFlagged,
      });
      setDraft(EMPTY);
      setPickOther(false);
      setStep(null); // vuelve a la lista, donde ya se ve la regla nueva
      setReload((n) => n + 1); // recarga la lista con la regla nueva (y su id)
      toast.notify({
        tone: 'success',
        title: t('rules.created'),
        description: t('rules.createdDescription'),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('rules.createFailed') });
    } finally {
      setSaving(false);
    }
  }

  function startWizard(): void {
    setDraft(EMPTY);
    setPickOther(false);
    setStep(0);
  }

  const hasCondition = draft.matchFrom.trim() !== '' || draft.matchSubject.trim() !== '';
  const hasAction = draft.moveTo !== '' || draft.markRead || draft.markFlagged;
  const otherPending = pickOther && !draft.moveTo;

  const steps: WizardStep[] = [
    {
      id: 'que',
      title: t('rules.stepWhichTitle'),
      heading: t('rules.stepWhichHeading'),
      description: t('rules.stepWhichDescription'),
      canContinue: hasCondition,
      content: (
        <div class="flex flex-col gap-4">
          <FieldGroup label={t('rules.fromLabel')} htmlFor="flt-from">
            <Input
              id="flt-from"
              type="text"
              value={draft.matchFrom}
              placeholder={t('rules.fromPlaceholder')}
              aria-describedby="flt-from-hint"
              onInput={(e) => setDraft((d) => ({ ...d, matchFrom: e.currentTarget.value }))}
            />
            <FieldHint id="flt-from-hint">{t('rules.fromHint')}</FieldHint>
          </FieldGroup>
          <FieldGroup label={t('rules.subjectLabel')} htmlFor="flt-subject">
            <Input
              id="flt-subject"
              type="text"
              value={draft.matchSubject}
              placeholder={t('rules.subjectPlaceholder')}
              aria-describedby="flt-subject-hint"
              onInput={(e) => setDraft((d) => ({ ...d, matchSubject: e.currentTarget.value }))}
            />
            <FieldHint id="flt-subject-hint" tone={hasCondition ? 'success' : 'neutral'}>
              {hasCondition ? t('rules.subjectHintOk') : t('rules.subjectHintBoth')}
            </FieldHint>
          </FieldGroup>
        </div>
      ),
    },
    {
      id: 'accion',
      title: t('rules.stepActionTitle'),
      heading: t('rules.stepActionHeading'),
      description: t('rules.stepActionDescription'),
      canContinue: hasAction && !otherPending,
      content: (
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-2" role="group" aria-label={t('rules.destinationGroup')}>
            <ChoiceCard
              name="flt-dest"
              title={t('rules.keepInboxTitle')}
              description={t('rules.keepInboxDescription')}
              icon="info"
              selected={!pickOther && draft.moveTo === ''}
              onSelect={() => {
                setPickOther(false);
                setDraft((d) => ({ ...d, moveTo: '' }));
              }}
            />
            {roleTargets.map(({ mailbox, choice }) => (
              <ChoiceCard
                key={mailbox.id}
                name="flt-dest"
                title={t(choice.title)}
                description={t(choice.description)}
                icon={choice.icon}
                selected={!pickOther && draft.moveTo === mailbox.id}
                onSelect={() => {
                  setPickOther(false);
                  setDraft((d) => ({ ...d, moveTo: mailbox.id }));
                }}
              />
            ))}
            {otherTargets.length > 0 ? (
              <ChoiceCard
                name="flt-dest"
                title={t('rules.otherFolderTitle')}
                description={t('rules.otherFolderDescription')}
                icon="menu"
                selected={pickOther}
                onSelect={() => {
                  setPickOther(true);
                  setDraft((d) => ({ ...d, moveTo: '' }));
                }}
              />
            ) : null}
          </div>

          {pickOther ? (
            <FieldGroup label={t('rules.targetFolder')} htmlFor="flt-move">
              <Select
                id="flt-move"
                value={draft.moveTo}
                aria-describedby="flt-move-hint"
                onChange={(e) => setDraft((d) => ({ ...d, moveTo: e.currentTarget.value }))}
              >
                <option value="">{t('rules.chooseFolder')}</option>
                {otherTargets.map((m) => (
                  <option key={m.id} value={m.id}>
                    {mailboxLabel(t, m.name, m.role)}
                  </option>
                ))}
              </Select>
              <FieldHint id="flt-move-hint" tone={otherPending ? 'danger' : 'neutral'}>
                {otherPending
                  ? t('rules.targetPending')
                  : t('rules.targetChosen', { folder: mailboxName(draft.moveTo) })}
              </FieldHint>
            </FieldGroup>
          ) : null}

          <fieldset class="m-0 flex flex-col gap-2 border-0 p-0">
            <legend class="text-text mb-1 p-0 text-sm font-semibold">
              {t('rules.alsoLegend')}
            </legend>
            <Checkbox
              label={t('rules.alsoRead')}
              checked={draft.markRead}
              onChange={(e) => setDraft((d) => ({ ...d, markRead: e.currentTarget.checked }))}
            />
            <Checkbox
              label={t('rules.alsoFlag')}
              checked={draft.markFlagged}
              onChange={(e) => setDraft((d) => ({ ...d, markFlagged: e.currentTarget.checked }))}
            />
          </fieldset>

          {draft.moveTo && mailboxes.find((m) => m.id === draft.moveTo)?.role === 'trash' ? (
            <InlineHelp tone="warning" title={t('rules.trashWarningTitle')}>
              {t('rules.trashWarningBody')}
            </InlineHelp>
          ) : null}
        </div>
      ),
    },
    {
      id: 'resumen',
      title: t('rules.stepSummaryTitle'),
      heading: t('rules.stepSummaryHeading'),
      description: t('rules.stepSummaryDescription'),
      nextLabel: t('rules.create'),
      onNext: () => void handleAdd(),
      content: (
        <div class="flex flex-col gap-4">
          <InlineHelp tone="info">{ruleSentence(draft)}</InlineHelp>
          <ReviewSummary
            items={[
              {
                label: t('rules.summaryWhich'),
                value: conditionText({
                  matchFrom: draft.matchFrom.trim(),
                  matchSubject: draft.matchSubject.trim(),
                }),
                onEdit: () => setStep(0),
              },
              {
                label: t('rules.summaryWhat'),
                value: actionText({
                  moveTo: draft.moveTo,
                  markRead: draft.markRead,
                  markFlagged: draft.markFlagged,
                }),
                onEdit: () => setStep(1),
              },
              { label: t('rules.summaryWhen'), value: t('rules.summaryWhenValue') },
            ]}
          />
          <FieldGroup label={t('rules.nameLabel')} htmlFor="flt-name">
            <Input
              id="flt-name"
              type="text"
              value={draft.name}
              placeholder={t('rules.namePlaceholder')}
              aria-describedby="flt-name-hint"
              onInput={(e) => setDraft((d) => ({ ...d, name: e.currentTarget.value }))}
            />
            <FieldHint id="flt-name-hint">{t('rules.nameHint')}</FieldHint>
          </FieldGroup>
        </div>
      ),
    },
  ];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={step === null ? t('rules.title') : t('rules.newTitle')}
      size="lg"
      {...(step === null
        ? {
            // El botón suelto, sin envoltorio: así el pie del teléfono lo estira a todo
            // el ancho y 44 px (`PIE_TELEFONO` de Modal.tsx), como en Fyne.
            footer: (
              <Button variant="secondary" onClick={onClose}>
                {t('common.close')}
              </Button>
            ),
          }
        : { footer: <div ref={setPieAsistente} class="w-full min-w-0" /> })}
    >
      {step !== null ? (
        <Wizard
          steps={steps}
          current={step}
          onCurrentChange={setStep}
          busy={saving}
          onCancel={() => setStep(null)}
          labels={{ cancel: t('rules.backToRules') }}
          footerTarget={pieAsistente}
        />
      ) : (
        <SettingsSection
          plain
          title={t('rules.yoursTitle')}
          description={t('rules.yoursDescription')}
          actions={
            <Button variant="primary" leadingIcon={<Icon name="plus" />} onClick={startWizard}>
              {t('rules.create')}
            </Button>
          }
        >
          {loading ? (
            <div class="text-text-subtle flex items-center gap-2 py-4">
              <Spinner size={16} label={t('rules.loading')} />
              <span class="text-sm" aria-hidden="true">
                {t('common.loading')}
              </span>
            </div>
          ) : error ? (
            <div class="flex flex-col items-start gap-2 py-2" role="alert">
              <p class="text-text flex items-start gap-2 text-sm">
                <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
                {t('rules.loadError')}
              </p>
              <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
                {t('common.retry')}
              </Button>
            </div>
          ) : filters.length === 0 ? (
            <EmptyState
              title={t('rules.emptyTitle')}
              description={t('rules.emptyDescription')}
              illustration={<Icon name="settings" size={32} />}
              action={
                <Button variant="primary" leadingIcon={<Icon name="plus" />} onClick={startWizard}>
                  {t('rules.emptyAction')}
                </Button>
              }
            />
          ) : (
            <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
              {filters.map((f) => (
                // En el teléfono, la regla entera arriba y «Activa» y borrar debajo: al
                // lado, condición y acción se quedaban en 169 px y no se leían.
                <li
                  key={f.id}
                  class="flex items-start justify-between gap-3 px-3 py-2.5 max-md:flex-col max-md:items-stretch max-md:gap-1"
                >
                  <div class="min-w-0">
                    <p class="text-text truncate text-sm font-medium">
                      {f.name || conditionText(f)}
                    </p>
                    <p class="sytue-meta truncate max-md:whitespace-normal">
                      {conditionText(f)} → {actionText(f)}
                    </p>
                  </div>
                  <div class="flex shrink-0 items-center gap-2 max-md:justify-between">
                    <Switch
                      label={t('rules.enabled')}
                      checked={f.enabled}
                      onChange={() => handleToggle(f)}
                      class="text-sm"
                    />
                    <button
                      type="button"
                      // 28px no se apunta con el dedo, y este botón BORRA una regla y
                      // vive pegado al interruptor de activarla: fallar el objetivo
                      // tenía consecuencias. 44px (WCAG 2.5.8) solo con puntero grueso,
                      // como el Button de @sytue/ui.
                      class="text-text-subtle hover:bg-bg-subtle hover:text-danger-text sytue-focus-ring grid h-7 w-7 shrink-0 place-items-center rounded-sm transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
                      aria-label={t('rules.deleteRule', { rule: f.name || conditionText(f) })}
                      onClick={() => handleDelete(f)}
                    >
                      <Icon name="close" size={16} />
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </SettingsSection>
      )}
    </Modal>
  );
}

/** Extrae los campos editables de una regla como entrada para reenviarla. */
function toInput(f: MailFilter): MailFilterInput {
  return {
    name: f.name,
    priority: f.priority,
    matchFrom: f.matchFrom,
    matchSubject: f.matchSubject,
    moveTo: f.moveTo,
    markRead: f.markRead,
    markFlagged: f.markFlagged,
  };
}
