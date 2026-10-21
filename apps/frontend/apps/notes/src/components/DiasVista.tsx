// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * Las vistas del diario que miran más de un día: Semana (siete columnas, una en el
 * teléfono), Días (el diario seguido, del más reciente hacia atrás) y «Este día» (el
 * mismo día de años anteriores). Y la agenda del día, leída de Sytue Calendar.
 *
 * Las notas aquí se LEEN: una línea cada una. Escribir es abrir el día, que es donde
 * vive el editor; así no hay un segundo editor que mantener. Mover una nota de día es
 * soltar su asa, en la lista del día, sobre la barra de semana (`zonaDeDia`).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Icon, Spinner, cx } from '@sytue/ui';
import { intlTag, type Locale } from '@sytue/i18n';
import type { DailyDay, Note } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import {
  diaLocal,
  desplazarDia,
  resumenNota,
  semanaDe,
  TIPO_ARRASTRE_NOTA,
} from '../client/diario.js';
import { ventanaLocal } from '../client/agenda.js';
import * as api from '../client/notesApi.js';

export type VistaDiario = 'day' | 'week' | 'days';
export const VISTAS_DIARIO: readonly VistaDiario[] = ['day', 'week', 'days'];

/** Tramo de la vista Días por petición; «Días anteriores» trae otro. */
const TRAMO_DIAS = 14;

const fecha = (dia: string) => new Date(`${dia}T12:00:00`);

interface ZonaDeDia {
  onDragOver?: (e: DragEvent) => void;
  onDrop?: (e: DragEvent) => void;
}

/** Lo que hace falta para que un día acepte una nota soltada encima. */
export function zonaDeDia(
  dia: string,
  onMoverADia?: (noteId: string, dia: string) => void,
): ZonaDeDia {
  if (!onMoverADia) return {};
  return {
    onDragOver: (e: DragEvent) => {
      if (!e.dataTransfer?.types.includes(TIPO_ARRASTRE_NOTA)) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
    },
    onDrop: (e: DragEvent) => {
      const id = e.dataTransfer?.getData(TIPO_ARRASTRE_NOTA);
      if (!id) return;
      e.preventDefault();
      onMoverADia(id, dia);
    },
  };
}

function FilaNota({ t, note }: { t: T; note: Note }): JSX.Element {
  const texto = resumenNota(note);
  return (
    <li class="notes-day-line">
      {note.kind === 'todo' ? (
        <span
          class={cx('notes-day-box', note.checked && 'notes-day-box--done')}
          role="img"
          aria-label={t(note.checked ? 'filter.checked' : 'filter.unchecked')}
        />
      ) : (
        <span class="notes-day-dot" aria-hidden="true" />
      )}
      <span
        class={cx(
          'min-w-0 flex-1',
          note.kind === 'todo' && note.checked && 'line-through opacity-60',
        )}
      >
        {texto}
      </span>
    </li>
  );
}

function ListaDia({
  t,
  day,
  vacio = true,
}: {
  t: T;
  day: DailyDay | undefined;
  /** Decir «nada todavía» cuando el día está vacío; en la semana se deja en blanco. */
  vacio?: boolean;
}): JSX.Element | null {
  // Una nota que es solo un separador no tiene línea que enseñar aquí.
  const notas = (day?.notes ?? []).filter((n) => resumenNota(n) !== '');
  if (!notas.length) {
    return vacio ? <p class="text-text-subtle px-1 py-1 text-sm">{t('daily.empty')}</p> : null;
  }
  return (
    <ul class="flex flex-col">
      {notas.map((n) => (
        <FilaNota key={n.id} t={t} note={n} />
      ))}
    </ul>
  );
}

interface VistaProps {
  t: T;
  locale: Locale;
  date: string;
  vista: Exclude<VistaDiario, 'day'>;
  /** Cambia cuando algo del diario cambia, para volver a pedirlo. */
  version: number;
  onDay: (date: string) => void;
}

/** Semana o Días: varios días con sus notas en una sola petición por tramo. */
export function DiasVista({ t, locale, date, vista, version, onDay }: VistaProps): JSX.Element {
  const [tramos, setTramos] = useState(1);
  const [dias, setDias] = useState<Map<string, DailyDay> | null>(null);
  const [error, setError] = useState(false);
  useEffect(() => setTramos(1), [date, vista]);
  const fechas =
    vista === 'week'
      ? semanaDe(date)
      : Array.from({ length: TRAMO_DIAS * tramos }, (_, i) => desplazarDia(date, -i));
  const desde = vista === 'week' ? fechas[0]! : fechas[fechas.length - 1]!;
  const hasta = vista === 'week' ? fechas[6]! : date;
  useEffect(() => {
    let vivo = true;
    setError(false);
    // Un tramo por petición (el servicio da 31 días con notas como mucho).
    const peticiones: Promise<DailyDay[]>[] = [];
    for (let fin = hasta; fin >= desde; fin = desplazarDia(fin, -TRAMO_DIAS)) {
      const ini = desplazarDia(fin, -(TRAMO_DIAS - 1));
      peticiones.push(api.dailyRange(ini < desde ? desde : ini, fin, true));
    }
    Promise.all(peticiones)
      .then((r) => vivo && setDias(new Map(r.flat().map((d) => [d.date, d]))))
      .catch(() => vivo && setError(true));
    return () => {
      vivo = false;
    };
  }, [desde, hasta, version]);
  if (error) return <p class="text-text-muted p-6 text-sm">{t('error.loadFailed')}</p>;
  if (!dias) {
    return (
      <div class="grid place-items-center p-6" aria-busy="true">
        <Spinner size={20} />
      </div>
    );
  }
  const hoy = diaLocal();
  const cabecera = (dia: string, largo: boolean) => (
    <button
      type="button"
      class={cx('notes-days-head sytue-focus-ring', largo ? 'text-lg' : 'text-sm')}
      aria-current={dia === hoy ? 'date' : undefined}
      onClick={() => onDay(dia)}
    >
      {fecha(dia).toLocaleDateString(
        intlTag(locale),
        largo
          ? { weekday: 'long', day: 'numeric', month: 'long' }
          : { weekday: 'short', day: 'numeric' },
      )}
    </button>
  );
  if (vista === 'week') {
    return (
      <section class="notes-week-view" aria-label={t('daily.view.week')}>
        {fechas.map((dia) => (
          <div key={dia} class="notes-week-col">
            {cabecera(dia, false)}
            <ListaDia t={t} day={dias.get(dia)} vacio={false} />
          </div>
        ))}
      </section>
    );
  }
  // Días: solo los que tienen algo, como un diario seguido; el abierto, siempre.
  const conAlgo = fechas.filter((d) => d === date || dias.has(d));
  return (
    <section class="notes-col notes-days-view" aria-label={t('daily.view.days')}>
      {conAlgo.map((dia) => (
        <div key={dia} class="notes-days-entry">
          {cabecera(dia, true)}
          <ListaDia t={t} day={dias.get(dia)} />
        </div>
      ))}
      <div class="-ml-2 flex">
        <Button variant="ghost" size="sm" onClick={() => setTramos((n) => n + 1)}>
          {t('daily.more')}
        </Button>
      </div>
    </section>
  );
}

/** «Este día»: el mismo día de años anteriores, solo si hay algo que enseñar. */
export function EsteDia({
  t,
  locale,
  date,
  onDay,
}: {
  t: T;
  locale: Locale;
  date: string;
  onDay: (date: string) => void;
}): JSX.Element | null {
  const [dias, setDias] = useState<DailyDay[]>([]);
  useEffect(() => {
    let vivo = true;
    setDias([]);
    api
      .thisDay(date)
      .then((r) => vivo && setDias(r))
      .catch(() => undefined);
    return () => {
      vivo = false;
    };
  }, [date]);
  if (!dias.length) return null;
  return (
    <section class="notes-col" aria-label={t('onThisDay.title')}>
      <div class="notes-this-day">
        <h3 class="notes-group">{t('onThisDay.title')}</h3>
        {dias.map((d) => (
          <div key={d.date} class="notes-days-entry">
            <button
              type="button"
              class="notes-days-head sytue-focus-ring text-base"
              onClick={() => onDay(d.date)}
            >
              {fecha(d.date).toLocaleDateString(intlTag(locale), { dateStyle: 'long' })}
            </button>
            <ListaDia t={t} day={d} />
          </div>
        ))}
      </div>
    </section>
  );
}

/** La agenda del día desde Sytue Calendar. Sin eventos (o sin Calendar), nada. */
export function AgendaDia({
  t,
  locale,
  date,
  version,
}: {
  t: T;
  locale: Locale;
  date: string;
  version: number;
}): JSX.Element | null {
  const [eventos, setEventos] = useState<api.EventoAgenda[]>([]);
  useEffect(() => {
    let vivo = true;
    const { from, to } = ventanaLocal(date);
    api
      .agenda(date, from, to)
      .then((r) => vivo && setEventos(r))
      .catch(() => vivo && setEventos([]));
    return () => {
      vivo = false;
    };
  }, [date, version]);
  if (!eventos.length) return null;
  const hora = (iso: string) =>
    new Date(iso).toLocaleTimeString(intlTag(locale), { hour: '2-digit', minute: '2-digit' });
  return (
    <ul class="notes-agenda" aria-label={t('agenda.title')}>
      {eventos.map((e) => (
        <li key={e.id + e.start} class="notes-agenda-item">
          <span class="notes-stream-glyph mt-0!">
            <Icon name="calendar" size={14} />
          </span>
          {e.url ? (
            <a
              class="sytue-focus-ring min-w-0 flex-1 truncate rounded-sm hover:underline"
              href={e.url}
            >
              {e.name}
            </a>
          ) : (
            <span class="min-w-0 flex-1 truncate">{e.name}</span>
          )}
          <span class="notes-agenda-time">{e.allDay ? t('agenda.allDay') : hora(e.start)}</span>
        </li>
      ))}
    </ul>
  );
}
