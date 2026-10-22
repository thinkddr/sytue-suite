/**
 * CalendarModal — crear o editar (renombrar/color) un calendario con el design
 * system Sytue (Modal + Form de @sytue/ui). El color se elige de la paleta de
 * 14 tonos joya --sytue-* (valores hex: el servicio Go exige #RRGGBB) mediante
 * un radiogroup accesible (roving tabindex, flechas). docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  Modal,
  cx,
  useToast,
} from '@sytue/ui';
import {
  createCalendar,
  deleteCalendar,
  updateCalendar,
  type Calendar,
} from '../client/calendarApi.js';
import type { CalendarMessageKey, T } from '../i18n/index.js';

/**
 * Paleta de 14 tonos joya de la suite (mismos hex que los tokens
 * --sytue-product-* / --sytue-header-* de @sytue/config): AA-safe como relleno
 * con icono blanco. Hex literal porque el backend persiste calendar-color CalDAV.
 *
 * El NOMBRE del tono es texto de interfaz (lo lee el lector de pantalla y sale
 * en el `title`), así que aquí solo va su clave de catálogo.
 */
export const CALENDAR_PALETTE: ReadonlyArray<{ name: CalendarMessageKey; value: string }> = [
  { name: 'colour.terracotta', value: '#b0470f' }, // --sytue-product-one
  { name: 'colour.slate', value: '#334155' }, // --sytue-product-id
  { name: 'colour.pine', value: '#146a47' }, // --sytue-product-drive
  { name: 'colour.inkBlue', value: '#1e50a8' }, // --sytue-product-docs
  { name: 'colour.jade', value: '#0f7052' }, // --sytue-product-sheets
  { name: 'colour.ochre', value: '#94590f' }, // --sytue-product-slides
  { name: 'colour.garnet', value: '#a31f38' }, // --sytue-product-mail
  { name: 'colour.plum', value: '#6b3e9e' }, // --sytue-product-calendar
  { name: 'colour.petrol', value: '#0e6c84' }, // --sytue-product-contacts
  { name: 'colour.orchid', value: '#a23e8c' }, // --sytue-product-chat
  { name: 'colour.purple', value: '#8446a8' }, // --sytue-product-meet
  { name: 'colour.indigo', value: '#4338ca' }, // --sytue-product-rooms
  { name: 'colour.steel', value: '#2e5a8c' }, // --sytue-product-cloud
  { name: 'colour.viridian', value: '#0e7268' }, // --sytue-product-hub
];

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  open: boolean;
  /** Calendario a editar, o null para crear uno nuevo. */
  editing: Calendar | null;
  onClose: () => void;
  /** Tras guardar con exito (el llamante recarga la lista). */
  onSaved: () => void;
}

export function CalendarModal({ t, open, editing, onClose, onSaved }: Props): JSX.Element | null {
  const toast = useToast();
  const [name, setName] = useState('');
  const [color, setColor] = useState(CALENDAR_PALETTE[0]!.value);
  const [busy, setBusy] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);

  // Al (re)abrir, sincroniza el formulario con el calendario en edicion.
  useEffect(() => {
    if (!open) return;
    setName(editing?.displayName ?? '');
    setColor(editing?.color ?? CALENDAR_PALETTE[0]!.value);
    setNameError(null);
  }, [open, editing]);

  const isEdit = Boolean(editing?.id);

  async function handleSave(): Promise<void> {
    const trimmed = name.trim();
    if (!trimmed) {
      setNameError(t('calendar.nameRequired'));
      document.getElementById('cal-name')?.focus();
      return;
    }
    setBusy(true);
    try {
      if (isEdit) {
        await updateCalendar(editing!.id!, { displayName: trimmed, color });
        toast.notify({ tone: 'success', title: t('toast.calendarUpdated') });
      } else {
        await createCalendar({ displayName: trimmed, color });
        toast.notify({ tone: 'success', title: t('toast.calendarCreated') });
      }
      onSaved();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.calendarSaveFailed') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t(isEdit ? 'calendar.editTitle' : 'calendar.newTitle')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={() => void handleSave()} loading={busy}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        {isEdit ? null : (
          <InlineHelp tone="info" title={t('calendar.helpTitle')}>
            {t('calendar.helpBody')}
          </InlineHelp>
        )}
        {/* El error va en el FieldHint (mismo sitio que la ayuda), no duplicado. */}
        <FieldGroup label={t('calendar.name')} htmlFor="cal-name" required>
          <Input
            id="cal-name"
            value={name}
            placeholder={t('calendar.namePlaceholder')}
            invalid={Boolean(nameError)}
            aria-describedby="cal-name-hint"
            onInput={(e) => {
              setName(e.currentTarget.value);
              if (nameError) setNameError(null);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void handleSave();
              }
            }}
          />
          <FieldHint id="cal-name-hint" tone={nameError ? 'danger' : 'neutral'}>
            {nameError ?? t('calendar.nameHint')}
          </FieldHint>
        </FieldGroup>
        <div class="flex flex-col gap-1.5">
          <span id="cal-color-label" class="text-text text-sm font-semibold">
            {t('common.colour')}
          </span>
          <ColorSwatches t={t} value={color} onChange={setColor} labelledBy="cal-color-label" />
          <FieldHint>{t('calendar.colourHint')}</FieldHint>
        </div>
      </div>
    </Modal>
  );
}

/**
 * DeleteCalendarModal — confirmacion de borrado de un calendario (accion
 * destructiva: se pierden sus eventos). El calendario por defecto nunca llega
 * aqui (el kebab no ofrece «Eliminar»); si llegara, el servicio Go responde 403.
 */
export function DeleteCalendarModal({
  t,
  calendar,
  onClose,
  onDeleted,
}: {
  t: T;
  calendar: Calendar | null;
  onClose: () => void;
  onDeleted: () => void;
}): JSX.Element | null {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  async function handleDelete(): Promise<void> {
    if (!calendar?.id) return;
    setBusy(true);
    try {
      await deleteCalendar(calendar.id);
      toast.notify({ tone: 'success', title: t('toast.calendarDeleted') });
      onDeleted();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.calendarDeleteFailed') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={Boolean(calendar)}
      onClose={onClose}
      title={t('calendar.deleteTitle')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" onClick={() => void handleDelete()} loading={busy}>
            {t('common.delete')}
          </Button>
        </>
      }
    >
      {/* Frase completa con el nombre interpolado: en alemán las comillas son
          „…“ y el verbo se va al final, así que no vale pegar trozos. */}
      <p>{t('calendar.deleteBody', { name: calendar?.displayName ?? '' })}</p>
    </Modal>
  );
}

/**
 * Selector de color como radiogroup accesible: un radio por tono, roving tabindex
 * y flechas para moverse; el elegido lleva anillo + check (estado no comunicado
 * solo por color). Reutilizado por el calendario y por el evento (color por
 * evento): `idPrefix` evita colisiones de id y `allowNone` antepone un swatch
 * «color del calendario» (valor vacio).
 */
export function ColorSwatches({
  t,
  value,
  onChange,
  idPrefix = 'cal-color',
  allowNone = false,
  labelledBy,
}: {
  t: T;
  value: string;
  onChange: (v: string) => void;
  idPrefix?: string;
  allowNone?: boolean;
  labelledBy?: string | undefined;
}): JSX.Element {
  const items: ReadonlyArray<{ name: CalendarMessageKey; value: string }> = allowNone
    ? [{ name: 'colour.inherit', value: '' }, ...CALENDAR_PALETTE]
    : CALENDAR_PALETTE;
  const idx = items.findIndex((p) => p.value.toLowerCase() === value.toLowerCase());
  const tabStop = idx < 0 ? 0 : idx; // color fuera de paleta: primer swatch tabulable
  const idOf = (v: string): string => `${idPrefix}-${v ? v.slice(1) : 'none'}`;

  function onKeyDown(e: KeyboardEvent): void {
    const delta =
      e.key === 'ArrowRight' || e.key === 'ArrowDown'
        ? 1
        : e.key === 'ArrowLeft' || e.key === 'ArrowUp'
          ? -1
          : 0;
    if (!delta) return;
    e.preventDefault();
    const next = items[(tabStop + delta + items.length) % items.length]!;
    onChange(next.value);
    document.getElementById(idOf(next.value))?.focus();
  }

  return (
    <div
      role="radiogroup"
      {...(labelledBy ? { 'aria-labelledby': labelledBy } : {})}
      tabIndex={-1}
      // Con el dedo, cada tono es un blanco de 44 (el círculo de 28 y 8 de cada lado,
      // con `after:`) y el paso entre tonos, 44: se tocan sin pisarse.
      class="flex flex-wrap gap-2 pointer-coarse:gap-4"
      onKeyDown={onKeyDown}
    >
      {items.map((p, i) => {
        const selected = i === idx;
        const none = p.value === '';
        const name = t(p.name);
        return (
          <button
            key={p.value || 'none'}
            id={idOf(p.value)}
            type="button"
            role="radio"
            aria-checked={selected}
            aria-label={name}
            title={name}
            tabIndex={i === tabStop ? 0 : -1}
            onClick={() => onChange(p.value)}
            class={cx(
              'relative grid h-7 w-7 place-items-center rounded-full transition-shadow duration-150',
              "pointer-coarse:after:absolute pointer-coarse:after:-inset-2 pointer-coarse:after:content-['']",
              'sytue-focus-ring-inset',
              selected &&
                'shadow-[0_0_0_2px_var(--sytue-surface),0_0_0_4px_var(--sytue-border-strong)]',
              // Contorno en TODOS, no solo en el «sin color». Sobre la superficie
              // elevada del tema oscuro (#22262b) ninguno de los 14 colores llega a
              // 3:1, así que sin borde el swatch no tiene forma reconocible: se lee
              // como una mancha, y el de menos contraste (slate) casi desaparece.
              none ? 'border-border border border-dashed' : 'border-border border',
            )}
            style={none ? undefined : { backgroundColor: p.value }}
          >
            {selected ? (
              <Icon name="check" size={14} class={none ? 'text-text' : 'text-neutral-0'} />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
