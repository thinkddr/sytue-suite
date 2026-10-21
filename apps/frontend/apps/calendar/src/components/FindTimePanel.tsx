/**
 * FindTimePanel — «buscar hueco» dentro del EventModal: consulta los huecos LIBRES
 * comunes (free/busy) en una ventana y una franja laboral, y ofrece horas de
 * inicio clicables que fijan el evento en ese hueco. El calculo (interseccion de
 * ocupados + franja laboral + DST) vive en el servicio Go; aqui solo se pide y se
 * pinta. El organizador (usuario autenticado) se incluye SIEMPRE en el servidor.
 *
 * Solo Tailwind/tokens @sytue/ui — hairlines, sin degradados (anti AI-slop). El
 * color no es el unico indicador; cada hueco es un boton con texto legible (AA).
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, Select, Skeleton, Switch, cx } from '@sytue/ui';
import { intlTag, type Locale } from '@sytue/i18n';
import { suggestSlots, type FreeSlot } from '../client/calendarApi.js';
import type { T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  /** Idioma negociado en el servidor: horas y días en su convención. */
  locale: Locale;
  /** Inicio elegido del evento: ancla la ventana de busqueda (desde su dia). */
  anchorStart: Date;
  /** Duracion del evento en minutos (el hueco debe caberla). */
  durationMin: number;
  /** Zona IANA del evento: franja laboral y formato de horas. */
  tzid: string;
  /** Fija el evento en el hueco elegido (start + duracion). */
  onPick: (start: Date, end: Date) => void;
}

const WORK_START = '09:00';
const WORK_END = '17:00';

/** Ventanas de búsqueda ofrecidas, en días. */
const WINDOWS = [7, 14, 30] as const;

/** Genera horas de inicio candidatas dentro de cada hueco libre (paso 30 min).
 *  El cursor se llama `ms` y no `t`: `t` es la función de traducción. */
function candidateStarts(slots: FreeSlot[], durationMin: number): Date[] {
  const dur = durationMin * 60_000;
  const step = 30 * 60_000;
  const out: Date[] = [];
  for (const s of slots) {
    const blockEnd = new Date(s.end).getTime();
    let k = 0;
    for (let ms = new Date(s.start).getTime(); ms + dur <= blockEnd && k < 4; ms += step, k++) {
      out.push(new Date(ms));
      if (out.length >= 16) return out;
    }
  }
  return out;
}

export function FindTimePanel({
  t,
  locale,
  anchorStart,
  durationMin,
  tzid,
  onPick,
}: Props): JSX.Element {
  const [weekdays, setWeekdays] = useState(true);
  const [windowDays, setWindowDays] = useState(7);
  const [slots, setSlots] = useState<FreeSlot[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  const zone = tzid || undefined;
  // Los formateadores llevan el idioma del usuario, no un 'es-ES' fijo: la hora
  // sale en la convención de cada lengua (24 h en Europa continental) y el día
  // con el nombre que da CLDR, en la ZONA del evento.
  const timeFmt = useMemo(
    () =>
      new Intl.DateTimeFormat(intlTag(locale), {
        hour: '2-digit',
        minute: '2-digit',
        timeZone: zone,
      }),
    [locale, zone],
  );
  const dayFmt = useMemo(
    () =>
      new Intl.DateTimeFormat(intlTag(locale), {
        weekday: 'short',
        day: 'numeric',
        month: 'short',
        timeZone: zone,
      }),
    [locale, zone],
  );

  useEffect(() => {
    // Ventana: desde el inicio del dia del ancla (o ahora, si ya paso) + N dias.
    const dayStart = new Date(anchorStart);
    dayStart.setHours(0, 0, 0, 0);
    const from = new Date(Math.max(Date.now(), dayStart.getTime()));
    const to = new Date(dayStart.getTime());
    to.setDate(to.getDate() + windowDays);

    let stale = false;
    setLoading(true);
    setError(false);
    void suggestSlots({
      start: from.toISOString(),
      end: to.toISOString(),
      durationMinutes: Math.max(15, Math.round(durationMin)),
      ...(zone ? { tz: zone } : {}),
      workStart: WORK_START,
      workEnd: WORK_END,
      weekdays,
      // Invitados adicionales por sub cuando exista el directorio del tenant; hoy
      // el servidor ya incluye al organizador (self).
    })
      .then(({ slots: s }) => {
        if (!stale) setSlots(s);
      })
      .catch(() => {
        if (!stale) setError(true);
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [anchorStart, durationMin, zone, weekdays, windowDays, reloadKey]);

  const starts = slots ? candidateStarts(slots, durationMin) : [];
  // Agrupa por dia natural (en la zona del evento) para una lista densa y legible.
  const groups = new Map<string, Date[]>();
  for (const s of starts) {
    const key = dayFmt.format(s);
    const bucket = groups.get(key);
    if (bucket) bucket.push(s);
    else groups.set(key, [s]);
  }

  return (
    <div class="border-border bg-bg-subtle flex flex-col gap-3 rounded-md border p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="text-text-muted text-xs">
          {t('findTime.summary', {
            start: WORK_START,
            end: WORK_END,
            duration: Math.max(15, Math.round(durationMin)),
          })}
        </p>
        <div class="flex items-center gap-3">
          <Switch
            checked={weekdays}
            onChange={() => setWeekdays((v) => !v)}
            label={t('findTime.weekdaysOnly')}
          />
          <Select
            aria-label={t('findTime.window')}
            value={String(windowDays)}
            class="h-8"
            onChange={(e) => setWindowDays(Number(e.currentTarget.value))}
          >
            {WINDOWS.map((d) => (
              <option key={d} value={String(d)}>
                {t('findTime.windowDays', { count: d })}
              </option>
            ))}
          </Select>
        </div>
      </div>

      {loading ? (
        <div class="flex flex-col gap-1.5" role="status" aria-label={t('findTime.loading')}>
          <Skeleton height="1.75rem" />
          <Skeleton height="1.75rem" width="70%" />
        </div>
      ) : error ? (
        <div class="flex items-center justify-between gap-2">
          <span class="text-text-subtle text-sm">{t('findTime.error')}</span>
          <Button variant="secondary" size="sm" onClick={() => setReloadKey((k) => k + 1)}>
            {t('common.retry')}
          </Button>
        </div>
      ) : starts.length === 0 ? (
        <p class="text-text-subtle py-2 text-sm">{t('findTime.empty')}</p>
      ) : (
        <ul class="flex flex-col gap-2">
          {[...groups.entries()].map(([day, times]) => (
            <li key={day} class="flex flex-col gap-1">
              {/* Solo la primera letra: `capitalize` ponía en mayúscula también el mes («Lun, 19 Oct»). */}
              <span class="sytue-meta font-medium first-letter:uppercase">{day}</span>
              <div class="flex flex-wrap gap-1.5">
                {times.map((start) => {
                  const end = new Date(start.getTime() + durationMin * 60_000);
                  return (
                    <button
                      key={start.getTime()}
                      type="button"
                      onClick={() => onPick(start, end)}
                      class={cx(
                        'border-border bg-surface text-text rounded-control inline-flex items-center gap-1 border px-2.5 py-1 text-sm tabular-nums pointer-coarse:min-h-[max(44px,2.75rem)]',
                        'hover:border-border-strong hover:bg-bg-subtle transition-colors duration-150',
                        'sytue-focus-ring',
                      )}
                    >
                      {timeFmt.format(start)}
                    </button>
                  );
                })}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
