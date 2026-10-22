/**
 * LabelsModal — gestión de etiquetas de Sytue Mail (estilo Gmail «Gestionar
 * etiquetas»). Lista las etiquetas de la cuenta y permite renombrarlas,
 * recolorearlas (paleta sobria) y borrarlas, además de crear
 * nuevas. Las etiquetas son keywords JMAP/IMAP en el backend; aquí solo el CRUD
 * vía BFF. El color nunca es el único indicador: el nombre siempre acompaña. AA.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  Icon,
  SettingsSection,
  Spinner,
  useToast,
} from '@sytue/ui';
import {
  listLabels,
  createLabel,
  updateLabel,
  deleteLabel,
  type Label,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT, type MailMessageKey, type T } from '../i18n/index.js';
import { LABEL_COLOR_TOKENS, labelDot } from './utils.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  onClose: () => void;
  /** Aviso al padre de que las etiquetas cambiaron (para refrescar el sidebar). */
  onChanged: () => void;
}

/** Clave del nombre de cada token de color, para el aria-label del selector. */
const COLOR_KEYS: Record<string, MailMessageKey> = {
  slate: 'colour.slate',
  brass: 'colour.brass',
  sage: 'colour.sage',
  clay: 'colour.clay',
  blue: 'colour.blue',
  plum: 'colour.plum',
  teal: 'colour.teal',
  rose: 'colour.rose',
};

/** Selector de color (puntos): color + nombre (title/aria), no solo color. */
function ColorPicker({
  t,
  value,
  onSelect,
  class: cls,
}: {
  t: T;
  value: string;
  onSelect: (c: string) => void;
  class?: string;
}): JSX.Element {
  return (
    <div
      class={`flex flex-wrap gap-1.5 pointer-coarse:gap-5 ${cls ?? ''}`}
      role="group"
      aria-label={t('labels.colourGroup')}
    >
      {LABEL_COLOR_TOKENS.map((token) => {
        const active = value === token;
        const key = COLOR_KEYS[token];
        const name = key ? t(key) : token;
        return (
          <button
            key={token}
            type="button"
            // Puntos de 24px separados por 6: con el dedo, los objetivos se solapan y
            // se elige el color de al lado. Con el puntero grueso el BLANCO es de 44
            // (`after:-inset-2.5`) y el paso entre puntos también (`gap-5`): el punto
            // se queda en 24. Agrandado entero, ocho puntos de 44 se comían la fila y
            // el nombre de la etiqueta desaparecía.
            class={`sytue-focus-ring relative grid h-6 w-6 place-items-center rounded-full border-2 transition-colors duration-150 pointer-coarse:after:absolute pointer-coarse:after:-inset-2.5 pointer-coarse:after:content-[''] ${active ? 'border-text' : 'border-transparent'}`}
            style={{ background: labelDot(token) }}
            aria-label={name}
            aria-pressed={active}
            title={name}
            onClick={() => onSelect(token)}
          >
            {active ? <Icon name="check" size={14} class="text-[var(--sytue-neutral-0)]" /> : null}
          </button>
        );
      })}
    </div>
  );
}

export function LabelsModal({ locale, open, onClose, onChanged }: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [labels, setLabels] = useState<Label[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [reload, setReload] = useState(0);
  const [newName, setNewName] = useState('');
  const [newColor, setNewColor] = useState(LABEL_COLOR_TOKENS[0] ?? 'slate');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError(false);
    listLabels()
      .then(({ labels: list }) => active && setLabels(list))
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  // Validación en vivo: el fallo típico es repetir un nombre que ya existe (el
  // servidor lo rechaza y el usuario no sabe por qué).
  const trimmedName = newName.trim();
  const duplicate =
    trimmedName !== '' && labels.some((l) => l.name.toLowerCase() === trimmedName.toLowerCase());
  const canCreate = trimmedName !== '' && !duplicate;

  async function handleCreate(): Promise<void> {
    const name = newName.trim();
    if (!canCreate || saving) return;
    setSaving(true);
    try {
      await createLabel(name, newColor);
      setNewName('');
      setReload((n) => n + 1);
      onChanged();
      toast.notify({ tone: 'success', title: t('labels.created') });
    } catch {
      toast.notify({ tone: 'danger', title: t('labels.createFailed') });
    } finally {
      setSaving(false);
    }
  }

  // Guardado optimista de nombre/color de una etiqueta existente; revierte y avisa
  // si el servidor rechaza (p. ej. nombre duplicado).
  function handleSave(label: Label, name: string, color: string): void {
    const prev = labels;
    setLabels((list) => list.map((l) => (l.id === label.id ? { ...l, name, color } : l)));
    void updateLabel(label.id, name, color)
      .then(() => onChanged())
      .catch(() => {
        setLabels(prev);
        toast.notify({ tone: 'danger', title: t('labels.saveFailed') });
      });
  }

  function handleDelete(label: Label): void {
    const prev = labels;
    setLabels((list) => list.filter((l) => l.id !== label.id));
    void deleteLabel(label.id)
      .then(() => onChanged())
      .catch(() => {
        setLabels(prev);
        toast.notify({ tone: 'danger', title: t('labels.deleteFailed') });
      });
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('labels.title')}
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-6">
        <InlineHelp tone="info" title={t('labels.whatTitle')}>
          {t('labels.whatBody')}
        </InlineHelp>

        <SettingsSection
          plain
          title={t('labels.yoursTitle')}
          description={t('labels.yoursDescription')}
        >
          {loading ? (
            <div class="text-text-subtle flex items-center gap-2 py-4">
              <Spinner size={16} label={t('labels.loading')} />
              <span class="text-sm" aria-hidden="true">
                {t('common.loading')}
              </span>
            </div>
          ) : error ? (
            <div class="flex flex-col items-start gap-2 py-2" role="alert">
              <p class="text-text flex items-start gap-2 text-sm">
                <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
                {t('labels.loadError')}
              </p>
              <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
                {t('common.retry')}
              </Button>
            </div>
          ) : labels.length === 0 ? (
            <EmptyState
              title={t('labels.emptyTitle')}
              description={t('labels.emptyDescription')}
              illustration={<Icon name="bucket" size={32} />}
              action={
                <Button
                  variant="secondary"
                  onClick={() => document.getElementById('lbl-name')?.focus()}
                >
                  {t('labels.emptyAction')}
                </Button>
              }
            />
          ) : (
            <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
              {labels.map((l) => (
                <LabelRow key={l.id} t={t} label={l} onSave={handleSave} onDelete={handleDelete} />
              ))}
            </ul>
          )}
        </SettingsSection>

        <SettingsSection
          plain
          title={t('labels.newTitle')}
          description={t('labels.newDescription')}
          class="border-border border-t pt-5"
        >
          <FieldGroup label={t('common.name')} htmlFor="lbl-name">
            <Input
              id="lbl-name"
              type="text"
              value={newName}
              maxLength={128}
              placeholder={t('labels.namePlaceholder')}
              invalid={duplicate}
              aria-describedby="lbl-name-hint"
              onInput={(e) => setNewName(e.currentTarget.value)}
              onKeyDown={(e: KeyboardEvent) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void handleCreate();
                }
              }}
            />
            <FieldHint id="lbl-name-hint" tone={duplicate ? 'danger' : 'neutral'}>
              {duplicate ? t('labels.nameDuplicate') : t('labels.nameHint')}
            </FieldHint>
          </FieldGroup>
          <FieldGroup label={t('labels.colour')} htmlFor="lbl-color">
            <ColorPicker t={t} value={newColor} onSelect={setNewColor} />
            <FieldHint id="lbl-color-hint">{t('labels.colourHint')}</FieldHint>
          </FieldGroup>
          <div class="flex justify-end">
            <Button
              variant="primary"
              leadingIcon={<Icon name="plus" />}
              loading={saving}
              disabled={!canCreate}
              onClick={() => void handleCreate()}
            >
              {t('labels.addLabel')}
            </Button>
          </div>
        </SettingsSection>
      </div>
    </Modal>
  );
}

/** Fila editable de una etiqueta: nombre (guarda al salir) + color + borrar. */
function LabelRow({
  t,
  label,
  onSave,
  onDelete,
}: {
  t: T;
  label: Label;
  onSave: (l: Label, name: string, color: string) => void;
  onDelete: (l: Label) => void;
}): JSX.Element {
  const [name, setName] = useState(label.name);
  useEffect(() => setName(label.name), [label.name]);

  // Borrar quita el keyword de TODOS los mensajes: pide confirmación en línea.
  const [confirming, setConfirming] = useState(false);
  const confirmId = `lbl-del-${label.id}`;
  useEffect(() => {
    if (confirming) document.getElementById(confirmId)?.focus();
  }, [confirming, confirmId]);

  function commitName(): void {
    const trimmed = name.trim();
    if (!trimmed || trimmed === label.name) {
      setName(label.name);
      return;
    }
    onSave(label, trimmed, label.color);
  }

  return (
    // En el teléfono, el nombre y borrar arriba y los colores debajo, a todo el ancho:
    // en una fila el campo se quedaba en 26 px y no se leía ninguna etiqueta.
    <li class="flex items-center gap-3 px-3 py-2.5 max-md:flex-wrap">
      <span
        class="h-3 w-3 shrink-0 rounded-full"
        style={{ background: labelDot(label.color) }}
        aria-hidden="true"
      />
      <Input
        class="min-w-0 flex-1"
        type="text"
        value={name}
        maxLength={128}
        aria-label={t('labels.rowName', { label: label.name })}
        onInput={(e) => setName(e.currentTarget.value)}
        onBlur={commitName}
        onKeyDown={(e: KeyboardEvent) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            (e.currentTarget as HTMLInputElement).blur();
          }
        }}
      />
      <ColorPicker
        t={t}
        value={label.color}
        onSelect={(c) => onSave(label, label.name, c)}
        class="max-md:order-last max-md:basis-full max-md:pl-6"
      />
      {confirming ? (
        <div class="flex shrink-0 items-center gap-1.5 max-md:basis-full max-md:justify-end">
          <span class="text-text-muted text-xs">{t('labels.confirmDelete')}</span>
          <Button id={confirmId} variant="danger" size="sm" onClick={() => onDelete(label)}>
            {t('common.yes')}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setConfirming(false)}>
            {t('common.no')}
          </Button>
        </div>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          iconOnly
          class="text-text-subtle hover:text-danger-text shrink-0"
          aria-label={t('labels.deleteLabel', { label: label.name })}
          title={t('labels.deleteShort')}
          onClick={() => setConfirming(true)}
        >
          <Icon name="close" size={16} />
        </Button>
      )}
    </li>
  );
}
