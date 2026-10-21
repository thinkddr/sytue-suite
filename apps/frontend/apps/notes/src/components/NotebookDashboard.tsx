// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// La cabecera del cuaderno con la referencia de Twos: el día es la página y su
// título sale UNA vez, la semana es una tira sin caja con un punto en los días que
// tienen algo, y la captura es una sola fila. Poco cromo: los controles son
// fantasma y lo que se lee es el texto. Todo lo que no es escribir va en dos líneas:
// la del título (con ir de un día a otro) y la del año (con la vista).
import type { JSX } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { Button, Dropdown, Icon, IconButton, Input, MenuItem, cx, useAtajo } from '@sytue/ui';
import { appUrl, orgActual } from '@sytue/config';
import { intlTag, type Locale } from '@sytue/i18n';
import type { DailyDay, Note, NoteInput, Notebook } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import {
  capturaRapida,
  diaLocal,
  desplazarDia,
  ENFOQUES,
  expandirCaptura,
  type Enfoque,
} from '../client/diario.js';
import { PLANTILLAS, type Plantilla } from '../client/plantillas.js';
import { Glyph } from './glyphs.js';
import { dailyRange } from '../client/notesApi.js';
import { AgendaDia, VISTAS_DIARIO, zonaDeDia, type VistaDiario } from './DiasVista.js';
import type { CapturaPendiente } from '../client/cola.js';

interface Props {
  t: T;
  locale: Locale;
  notebook: Notebook;
  notes: Note[];
  disabled: boolean;
  enfoque: Enfoque;
  openingDay: boolean;
  initialDraft: string;
  /**
   * Las notas en pantalla son ya las de este diario y es mío: su punto lo deciden
   * ellas (el recuento del servidor no se entera de lo que acaba de pasar). Mientras
   * carga aún son las del día anterior y no dicen nada de este.
   */
  notasPropiasCargadas?: boolean;
  onDraft: (text: string) => void;
  onEnfoque: (v: Enfoque) => void;
  /** Ocultar las tareas hechas o canceladas; si no, se ven tachadas. */
  hideFinished: boolean;
  onHideFinished: (v: boolean) => void;
  onDay: (date: string) => void;
  /** «Trasladar pendientes a hoy» desde un día pasado. */
  onCarry: () => void;
  onCapture: (input: NoteInput) => Promise<boolean>;
  onTemplate: (p: Plantilla) => Promise<boolean>;
  /** Día, Semana o Días (solo en un diario). */
  vista?: VistaDiario;
  onVista?: (v: VistaDiario) => void;
  /** Soltar una nota sobre un día de la barra la mueve allí. */
  onMoverADia?: ((noteId: string, dia: string) => void) | undefined;
  /** Cambia cuando cambia algo del diario: los puntos de la semana se vuelven a pedir. */
  version?: number;
  /** Lo capturado aquí que aún no ha llegado al servicio (client/cola.ts). */
  pendientes?: readonly CapturaPendiente[];
  onDescartar?: (clientId: string) => void;
  /**
   * Lo que en un cuaderno corriente va en su barra (orden, exportar, compartir,
   * ajustes) y en un día cabe en un menú al final de la navegación.
   */
  acciones?: JSX.Element | null | undefined;
}

/** El progreso del día: un aro pequeño y «2/5»; la frase entera, para quien no ve. */
function Aro({ hechas, total, rotulo }: { hechas: number; total: number; rotulo: string }) {
  const r = 6;
  const c = 2 * Math.PI * r;
  return (
    <span class="notes-ring text-text-subtle text-xs tabular-nums" title={rotulo}>
      <svg viewBox="0 0 16 16" fill="none" stroke-width="2.5" aria-hidden="true">
        <circle class="notes-ring-track" cx="8" cy="8" r={r} />
        <circle
          class="notes-ring-value"
          cx="8"
          cy="8"
          r={r}
          stroke-linecap="round"
          stroke-dasharray={c}
          stroke-dashoffset={c * (1 - hechas / total)}
        />
      </svg>
      <span aria-hidden="true">
        {hechas}/{total}
      </span>
      <span class="sr-only" role="status">
        {rotulo}
      </span>
    </span>
  );
}

/** El texto de una captura pendiente, sin HTML: es lo que se escribió. */
export function textoPendiente(c: CapturaPendiente): string {
  const i = c.input;
  if (i.title) return i.title;
  if (i.content_cipher) return '';
  return new DOMParser().parseFromString(i.content ?? '', 'text/html').body.textContent ?? '';
}

const fecha = (dia: string) => new Date(`${dia}T12:00:00`);

export function NotebookDashboard({
  t,
  locale,
  notebook,
  notes,
  disabled,
  enfoque,
  openingDay,
  initialDraft,
  notasPropiasCargadas = false,
  onDraft,
  onEnfoque,
  hideFinished,
  onHideFinished,
  onDay,
  onCarry,
  onCapture,
  onTemplate,
  vista = 'day',
  onVista,
  onMoverADia,
  version = 0,
  pendientes = [],
  onDescartar,
  acciones,
}: Props): JSX.Element {
  const mounted = useRef(true);
  useLayoutEffect(
    () => () => {
      mounted.current = false;
    },
    [],
  );
  const [draft, setDraft] = useState(initialDraft);
  const changeDraft = (text: string) => {
    setDraft(text);
    onDraft(text);
  };
  const [kind, setKind] = useState<'text' | 'todo'>('text');
  const [saving, setSaving] = useState(false);
  const atajoHoy = useAtajo('Ctrl+D');
  // Las canceladas no cuentan: ni pendientes ni para el progreso.
  const tasks = notes.filter((n) => n.kind === 'todo' && !n.cancelled_at);
  const done = tasks.filter((n) => n.checked).length;
  const date = notebook.daily_date;
  const hoy = diaLocal();
  const tag = intlTag(locale);
  // Los puntos de la barra: qué días de la semana tienen notas y si queda algo
  // pendiente. El día abierto se cuenta aquí mismo, con lo que ya está en pantalla.
  const [marcas, setMarcas] = useState<Map<string, DailyDay>>(new Map());
  const [sobre, setSobre] = useState<string | null>(null);
  useEffect(() => {
    if (!date) return;
    let vivo = true;
    dailyRange(desplazarDia(date, -3), desplazarDia(date, 3))
      .then((r) => vivo && setMarcas(new Map(r.map((d) => [d.date, d]))))
      .catch(() => undefined);
    return () => {
      vivo = false;
    };
  }, [date, version]);
  const focusLabels = {
    all: t('filter.all'),
    pending: t('filter.unchecked'),
    done: t('filter.checked'),
    pinned: t('focus.pinned'),
  };
  const counts = {
    all: notes.length,
    pending: tasks.length - done,
    done,
    pinned: notes.filter((n) => n.pinned).length,
  };
  const marcaDe = (day: string): Pick<DailyDay, 'total' | 'pending'> | undefined =>
    day === date && notasPropiasCargadas
      ? { total: notes.length, pending: counts.pending }
      : marcas.get(day);
  const submit = async () => {
    if (saving || disabled) return;
    const input = capturaRapida(draft, kind);
    if (!input) return;
    setSaving(true);
    try {
      if (await onCapture(input)) {
        onDraft('');
        if (mounted.current) {
          setDraft('');
          onEnfoque('all');
        }
      }
    } finally {
      setSaving(false);
    }
  };
  const template = async (p: Plantilla) => {
    if (saving || disabled) return;
    setSaving(true);
    try {
      if ((await onTemplate(p)) && mounted.current) onEnfoque('all');
    } finally {
      setSaving(false);
    }
  };
  // En lista y en árbol todo va en la columna de lectura; en columnas y en el lienzo
  // libre la captura ocupa el ancho del cuaderno, como sus notas.
  const lectura = notebook.layout === 'list' || notebook.layout === 'tree';
  // Los filtros hablan de tareas y de fijadas: en un día, en cuanto hay algo; en un
  // cuaderno corriente, solo si tiene tareas.
  // (Y siempre que haya un enfoque puesto: un filtro que no se ve esconde notas.)
  const conFiltros = date
    ? vista === 'day' && notes.length > 0
    : tasks.length > 0 || enfoque !== 'all';
  return (
    <section
      class={cx('notes-day', lectura ? 'notes-col' : 'px-4 sm:px-6')}
      aria-label={t('capture.label')}
    >
      {date ? (
        <>
          <header class="notes-day-head">
            <div class="notes-day-row">
              <h2 class="notes-page-title min-w-0">
                {fecha(date).toLocaleDateString(tag, {
                  weekday: 'long',
                  day: 'numeric',
                  month: 'long',
                })}
              </h2>
              <div class="notes-day-nav">
                <IconButton
                  label={t('daily.previous')}
                  disabled={openingDay}
                  onClick={() => onDay(desplazarDia(date, -1))}
                >
                  <Icon name="chevronLeft" size={16} />
                </IconButton>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={openingDay}
                  title={atajoHoy ? `${t('daily.today')} (${atajoHoy})` : t('daily.today')}
                  onClick={() => onDay(hoy)}
                >
                  {t('daily.today')}
                </Button>
                <IconButton
                  label={t('daily.next')}
                  disabled={openingDay}
                  onClick={() => onDay(desplazarDia(date, 1))}
                >
                  <Icon name="chevronRight" size={16} />
                </IconButton>
                <Button
                  variant="ghost"
                  size="sm"
                  iconOnly
                  aria-label="Sytue Calendar"
                  title="Sytue Calendar"
                  href={appUrl(
                    'calendar',
                    `?date=${encodeURIComponent(date)}&all_day=1`,
                    orgActual(import.meta.env.BASE_URL),
                  )}
                >
                  <Icon name="calendar" size={16} />
                </Button>
                {acciones}
              </div>
            </div>
            <div class="notes-day-row">
              <p class="notes-day-sub">
                {fecha(date).getFullYear()}
                {date === hoy ? ` · ${t('daily.today')}` : ''}
              </p>
              {onVista ? (
                <div class="notes-seg -mr-2" role="group" aria-label={t('daily.view')}>
                  {VISTAS_DIARIO.map((v) => (
                    <button
                      type="button"
                      key={v}
                      class="notes-seg-item sytue-focus-ring"
                      aria-pressed={vista === v}
                      onClick={() => onVista(v)}
                    >
                      {t(`daily.view.${v}`)}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
          </header>
          {/* En «Semana» los siete días ya están debajo, con su cabecera: la tira sobraba. */}
          {vista === 'week' ? null : (
            <div class="notes-week" role="group" aria-label={t('daily.label')}>
              {Array.from({ length: 7 }, (_, i) => desplazarDia(date, i - 3)).map((day) => {
                const d = fecha(day);
                const m = marcaDe(day);
                // El día abierto no acepta: la nota ya está en él.
                const zona = zonaDeDia(day, day !== date ? onMoverADia : undefined);
                const soltar = zona.onDrop;
                const arrastre: {
                  onDragOver?: (e: DragEvent) => void;
                  onDragEnter?: () => void;
                  onDragLeave?: () => void;
                  onDrop?: (e: DragEvent) => void;
                } = soltar
                  ? {
                      ...zona,
                      onDragEnter: () => setSobre(day),
                      onDragLeave: () => setSobre((x) => (x === day ? null : x)),
                      onDrop: (e: DragEvent) => {
                        setSobre(null);
                        soltar(e);
                      },
                    }
                  : {};
                return (
                  <button
                    type="button"
                    key={day}
                    class="notes-week-day sytue-focus-ring"
                    aria-pressed={day === date}
                    aria-current={day === hoy ? 'date' : undefined}
                    aria-label={`${d.toLocaleDateString(tag, { dateStyle: 'full' })}${m?.total ? ` · ${t('daily.marks', { total: m.total, pending: m.pending })}` : ''}`}
                    disabled={openingDay}
                    onClick={() => onDay(day)}
                    {...(sobre === day ? { 'data-drop': 'true' } : {})}
                    // El asa de una nota (arrastre de puntero, NoteCanvas) la suelta aquí.
                    {...(soltar ? { 'data-drop-day': day } : {})}
                    {...arrastre}
                  >
                    <span class="notes-week-name">
                      {d.toLocaleDateString(tag, { weekday: 'short' })}
                    </span>
                    <span class="notes-week-num">{d.getDate()}</span>
                    <span
                      class={cx(
                        'notes-week-dot',
                        !!m?.total && 'notes-week-dot--on',
                        !!m?.pending && 'notes-week-dot--pending',
                      )}
                      aria-hidden="true"
                    />
                  </button>
                );
              })}
            </div>
          )}
        </>
      ) : acciones ? (
        // Un cuaderno en lista es una página como la de un día: su título en grande y
        // sus acciones al lado, sin barra encima.
        <header class="notes-day-head mb-5!">
          <div class="notes-day-row">
            <h2 class="notes-page-title min-w-0">{notebook.title}</h2>
            <div class="notes-day-nav">{acciones}</div>
          </div>
        </header>
      ) : null}
      <div class="notes-capture-wrap">
        <form
          class="notes-capture"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          {/* El conmutador de tarea ocupa la columna del glifo de las filas: lo que se
              escribe empieza donde empieza el texto de las notas. */}
          <IconButton
            class="-ml-1"
            label={t('note.newTodo')}
            pressed={kind === 'todo'}
            disabled={disabled || saving}
            onClick={() => setKind(kind === 'todo' ? 'text' : 'todo')}
          >
            <Glyph name="todo" size={16} />
          </IconButton>
          <Input
            name="capture"
            class="notes-capture-input"
            aria-label={t('capture.label')}
            aria-describedby="notes-capture-hint"
            placeholder={t('capture.placeholder')}
            value={draft}
            disabled={disabled || saving}
            maxLength={kind === 'todo' ? 400 : 10000}
            onInput={(e) => changeDraft(expandirCaptura(draft, e.currentTarget.value))}
          />
          <Dropdown
            label={t('template.label')}
            align="end"
            trigger={(props) => (
              <IconButton {...props} label={t('template.label')} disabled={disabled || saving}>
                <Icon name="ellipsis" size={16} />
              </IconButton>
            )}
          >
            {PLANTILLAS.map((p) => (
              <MenuItem key={p} disabled={disabled || saving} onSelect={() => void template(p)}>
                {t(`template.${p}`)}
              </MenuItem>
            ))}
          </Dropdown>
          <IconButton
            type="submit"
            label={saving ? t('note.saving') : t('capture.add')}
            disabled={disabled || saving || !draft.trim()}
          >
            <Icon name="plus" size={16} />
          </IconButton>
        </form>
        <p id="notes-capture-hint" class="notes-capture-hint">
          {t('capture.submitHint')}
        </p>
      </div>
      {pendientes.length > 0 ? (
        <ul class="notes-pending">
          {pendientes.map((c) => (
            <li key={c.client_id} class="notes-pending-item">
              <Icon
                name={c.rechazada ? 'warning' : 'clock'}
                size={14}
                class={c.rechazada ? 'text-danger-text' : 'text-text-subtle'}
              />
              <span class="text-text-muted min-w-0 flex-1 truncate">{textoPendiente(c)}</span>
              <span class={cx('shrink-0', c.rechazada ? 'text-danger-text' : 'text-text-subtle')}>
                {t(c.rechazada ? 'capture.rejected' : 'capture.pending')}
              </span>
              {c.rechazada && onDescartar ? (
                <Button
                  variant="ghost"
                  size="xs"
                  aria-label={`${t('capture.discard')}: ${textoPendiente(c)}`}
                  onClick={() => onDescartar(c.client_id)}
                >
                  {t('capture.discard')}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {date && date < hoy && counts.pending > 0 ? (
        <div class="notes-carry-bar" role="status">
          <span>{t('daily.pendingLeft', { count: counts.pending })}</span>
          <Button
            variant="ghost"
            size="xs"
            disabled={disabled || openingDay}
            leadingIcon={<Icon name="chevronRight" size={14} />}
            onClick={onCarry}
          >
            {t('daily.carryToToday')}
          </Button>
        </div>
      ) : null}
      {conFiltros ? (
        <div class="notes-day-filters">
          <div class="notes-seg" role="group" aria-label={t('filter.placeholder')}>
            {ENFOQUES.map((f) => (
              <button
                type="button"
                key={f}
                class="notes-seg-item sytue-focus-ring"
                aria-pressed={enfoque === f}
                onClick={() => onEnfoque(f)}
              >
                {focusLabels[f]} <span class="notes-seg-count">{counts[f]}</span>
              </button>
            ))}
          </div>
          <div class="ml-auto flex items-center gap-2 pr-1">
            {tasks.length > 0 ? (
              <Aro
                hechas={done}
                total={tasks.length}
                rotulo={t('daily.progress', { done, total: tasks.length })}
              />
            ) : null}
            {tasks.length > 0 ? (
              <IconButton
                label={t('daily.hideFinished')}
                pressed={hideFinished}
                onClick={() => onHideFinished(!hideFinished)}
              >
                <Icon name={hideFinished ? 'ojoOff' : 'ojo'} size={16} />
              </IconButton>
            ) : null}
          </div>
        </div>
      ) : null}
      {date && vista === 'day' ? (
        <AgendaDia t={t} locale={locale} date={date} version={version} />
      ) : null}
    </section>
  );
}
