/**
 * CalendarModal — crear o editar (renombrar/color) un calendario con el design
 * system Sytue (Modal + Form de @sytue/ui). El color se elige de la paleta de
 * colores de usuario de la suite (valores hex: el servicio Go exige #RRGGBB)
 * mediante un radiogroup accesible (roving tabindex, flechas). docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { PALETA_DE_USUARIO, claveDeColorDeUsuario, colorDeUsuario } from '@sytue/config';
import { useI18n } from '@sytue/i18n/preact';
import {
  Button,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  SelectorDeColor,
  useToast,
} from '@sytue/ui';
import {
  createCalendar,
  deleteCalendar,
  updateCalendar,
  type Calendar,
} from '../client/calendarApi.js';
import type { T } from '../i18n/index.js';

/**
 * Los tonos de un calendario: la paleta de colores de usuario de la suite
 * (`PALETA_DE_USUARIO` de @sytue/config), la misma que la de las etiquetas de
 * Mail o los cuadernos de Notes, medida en los cuatro modos. Hex porque el
 * backend persiste calendar-color CalDAV. Un color guardado fuera de ella (de
 * la paleta de antes o de otro cliente) se PINTA con su tono más cercano
 * (`colorDeUsuario`) y no se reescribe hasta que su dueño elige otro.
 */
export const CALENDAR_PALETTE = PALETA_DE_USUARIO;

/** El tono de un calendario nuevo y el de una suscripción, por su id. */
export const COLOR_NUEVO = PALETA_DE_USUARIO.find((c) => c.id === 'blue')!.valor;
export const COLOR_SUSCRIPCION = PALETA_DE_USUARIO.find((c) => c.id === 'plum')!.valor;

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
  const [color, setColor] = useState(COLOR_NUEVO);
  const [busy, setBusy] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);

  // Al (re)abrir, sincroniza el formulario con el calendario en edicion.
  useEffect(() => {
    if (!open) return;
    setName(editing?.displayName ?? '');
    setColor(editing?.color ?? COLOR_NUEVO);
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
 * Selector de color del calendario: el común de la suite (`SelectorDeColor`) con
 * la paleta y los nombres de Calendar. Reutilizado por el calendario y por el
 * evento (color por evento): `allowNone` antepone un swatch «color del
 * calendario» (valor vacío).
 */
export function ColorSwatches({
  t,
  value,
  onChange,
  allowNone = false,
  labelledBy,
}: {
  t: T;
  value: string;
  onChange: (v: string) => void;
  allowNone?: boolean;
  labelledBy?: string | undefined;
}): JSX.Element {
  // Los nombres de los tonos son del catálogo común: los comparten todas las apps.
  const tc = useI18n().t;
  const items: ReadonlyArray<{ nombre: string; valor: string }> = [
    ...(allowNone ? [{ nombre: t('colour.inherit'), valor: '' }] : []),
    ...CALENDAR_PALETTE.map((c) => ({ nombre: tc(claveDeColorDeUsuario(c.id)), valor: c.valor })),
  ];
  // Un color guardado fuera de la paleta se ve, y se marca, como su tono más
  // cercano; el valor guardado no se toca hasta que su dueño elige otro.
  const pintado = colorDeUsuario(value) ?? '';
  return (
    <SelectorDeColor
      colores={items}
      valor={pintado}
      onChange={onChange}
      etiquetadoPor={labelledBy}
    />
  );
}
