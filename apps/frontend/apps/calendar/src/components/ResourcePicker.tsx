/**
 * ResourcePicker — «Salas y recursos» dentro del EventModal (patrón resource-calendar
 * de Exchange/Google). Lista los recursos reservables del tenant con su disponibilidad
 * (LIBRE/OCUPADO) en la franja elegida del evento y permite añadirlos a la reserva; al
 * guardar el evento, el EventModal reserva cada uno y refleja aceptada/rechazada. La
 * comprobación de disponibilidad y el anti doble-reserva atómico viven en el servicio Go.
 *
 * Sobrio (design system Sytue: Select + Badge + Icon de @sytue/ui, hairlines, sin
 * degradados/emojis). El estado no es solo color: cada recurso lleva TEXTO
 * («Libre»/«Ocupado»/«Reservada»/«Ocupado») además del tono del Badge (WCAG AA).
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Badge, Icon, Select, Skeleton } from '@sytue/ui';
import { listResources, type ResourceWithBusy } from '../client/calendarApi.js';
import type { T } from '../i18n/index.js';

/** Estado confirmado de una reserva tras guardar el evento. */
export type BookingStatus = 'accepted' | 'declined' | 'pending';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  /** Inicio/fin elegidos del evento (para calcular libre/ocupado en esa franja). */
  start: Date;
  end: Date;
  /** IDs de los recursos ya añadidos a la reserva. */
  selected: string[];
  onChange: (ids: string[]) => void;
  /** Estado confirmado por recurso tras guardar (id → accepted|declined|pending). */
  status?: Record<string, BookingStatus>;
  /** Rótulo de la sección; la pinta el propio selector para poder callarse entero. */
  titulo: string;
}

/** ¿Solapan los intervalos semiabiertos [aS,aE) y [bS,bE)? */
function overlaps(aS: number, aE: number, bS: number, bE: number): boolean {
  return aS < bE && bS < aE;
}

/** ¿El recurso está OCUPADO en [start,end) según sus periodos ocupados? */
function isBusy(r: ResourceWithBusy, startMs: number, endMs: number): boolean {
  return r.busy.some((p) => overlaps(startMs, endMs, Date.parse(p.start), Date.parse(p.end)));
}

const KIND_ICON = { room: 'building', equipment: 'grid' } as const;

export function ResourcePicker({
  t,
  start,
  end,
  selected,
  onChange,
  status,
  titulo,
}: Props): JSX.Element | null {
  const [resources, setResources] = useState<ResourceWithBusy[] | null>(null);
  const [error, setError] = useState(false);

  const startMs = start.getTime();
  const endMs = Math.max(end.getTime(), startMs + 60_000);
  // Clave de la franja: refresca la disponibilidad cuando cambian inicio/fin.
  const rangeKey = `${startMs}|${endMs}`;

  useEffect(() => {
    let stale = false;
    setError(false);
    void listResources({
      start: new Date(startMs).toISOString(),
      end: new Date(endMs).toISOString(),
    })
      .then(({ resources: list }) => {
        if (!stale) setResources(list ?? []);
      })
      .catch(() => {
        if (!stale) setError(true);
      });
    return () => {
      stale = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rangeKey]);

  const byId = useMemo(() => {
    const m = new Map<string, ResourceWithBusy>();
    for (const r of resources ?? []) m.set(r.id, r);
    return m;
  }, [resources]);

  const available = (resources ?? []).filter((r) => !selected.includes(r.id));

  function add(id: string): void {
    if (id && !selected.includes(id)) onChange([...selected, id]);
  }
  function remove(id: string): void {
    onChange(selected.filter((x) => x !== id));
  }

  // Sin salas ni recursos, la sección entera sobra: «no hay salas, un
  // administrador puede darlas de alta» era ruido en cada evento de quien no
  // administra nada, y quien administra tiene el botón de recursos en el lateral.
  if (resources !== null && resources.length === 0) return null;

  const cabecera = <span class="text-text text-sm font-semibold">{titulo}</span>;
  if (resources === null) {
    return (
      <div class="flex flex-col gap-1.5">
        {cabecera}
        {error ? (
          <p class="text-text-subtle text-sm">{t('picker.error')}</p>
        ) : (
          <Skeleton height="2.25rem" />
        )}
      </div>
    );
  }

  return (
    <div class="flex flex-col gap-1.5">
      {cabecera}
      <div class="flex flex-col gap-2">
        {selected.length > 0 ? (
          <ul class="flex flex-col gap-1.5" aria-label={t('picker.list')}>
            {selected.map((id) => {
              const r = byId.get(id);
              const st = status?.[id];
              const busy = r ? isBusy(r, startMs, endMs) : false;
              return (
                <li
                  key={id}
                  class="border-border flex items-center gap-2 rounded-md border px-2.5 py-2"
                >
                  <Icon
                    name={r ? KIND_ICON[r.kind] : 'building'}
                    size={16}
                    class="text-text-muted shrink-0"
                  />
                  <span class="text-text min-w-0 flex-1 truncate text-sm">{r?.name ?? id}</span>
                  <ResourceBadge t={t} busy={busy} {...(st ? { status: st } : {})} />
                  <button
                    type="button"
                    aria-label={t('picker.remove', { name: r?.name ?? t('picker.fallbackName') })}
                    title={t('picker.removeTitle')}
                    onClick={() => remove(id)}
                    class="text-text-muted hover:bg-danger-bg hover:text-danger-text sytue-focus-ring rounded-control grid h-6 w-6 shrink-0 place-items-center transition-colors"
                  >
                    <Icon name="close" size={14} />
                  </button>
                </li>
              );
            })}
          </ul>
        ) : null}

        {available.length > 0 ? (
          <Select
            aria-label={t('picker.add')}
            value=""
            onChange={(e) => {
              add(e.currentTarget.value);
              e.currentTarget.value = '';
            }}
          >
            <option value="">{t('picker.addPlaceholder')}</option>
            {available.map((r) => {
              // La opción se compone con una plantilla completa: el aforo lleva su
              // forma plural y el estado su palabra, sin pegar cachos con `+`.
              const state = t(isBusy(r, startMs, endMs) ? 'picker.busy' : 'picker.free');
              return (
                <option key={r.id} value={r.id}>
                  {r.capacity > 0
                    ? t('picker.optionWithCapacity', {
                        name: r.name,
                        capacity: t('resources.people', { count: r.capacity }),
                        state,
                      })
                    : t('picker.option', { name: r.name, state })}
                </option>
              );
            })}
          </Select>
        ) : (
          <p class="text-text-subtle text-sm">{t('picker.allAdded')}</p>
        )}
      </div>
    </div>
  );
}

/** Badge de estado del recurso: confirmado tras guardar, o previsión libre/ocupado. */
function ResourceBadge({
  t,
  status,
  busy,
}: {
  t: T;
  status?: BookingStatus;
  busy: boolean;
}): JSX.Element {
  if (status === 'accepted') return <Badge tone="success">{t('picker.badgeBooked')}</Badge>;
  if (status === 'declined') return <Badge tone="danger">{t('picker.badgeBusy')}</Badge>;
  if (status === 'pending') return <Badge tone="warning">{t('picker.badgePending')}</Badge>;
  return busy ? (
    <Badge tone="danger">{t('picker.badgeBusy')}</Badge>
  ) : (
    <Badge tone="success">{t('picker.badgeFree')}</Badge>
  );
}
