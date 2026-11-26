/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Compartir mi disponibilidad»: los huecos libres de los próximos días dentro de la
 * jornada de los ajustes, listos para copiar (`lib/disponibilidad.ts`). Se elige cuánto
 * tiene que durar un hueco para contar y cuántos días mirar; lo que se va a copiar se
 * ve antes, tal cual.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, EmptyState, FieldGroup, Modal, Select, Skeleton, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { suggestSlots, type FreeSlot } from '../client/calendarApi.js';
import {
  DURACIONES_DISPONIBILIDAD,
  VENTANAS_DISPONIBILIDAD,
  diasDisponibles,
  textoDisponibilidad,
} from '../lib/disponibilidad.js';
import type { AjustesCalendario } from '../lib/preferencias.js';
import type { T } from '../i18n/index.js';
import { browserZone } from './tz.js';

interface Props {
  t: T;
  locale: Locale;
  open: boolean;
  /** La jornada y los fines de semana de los ajustes: fuera de ellos no se ofrece. */
  ajustes: AjustesCalendario;
  onClose: () => void;
}

const dosCifras = (h: number): string => `${String(h).padStart(2, '0')}:00`;

export function Disponibilidad({ t, locale, open, ajustes, onClose }: Props): JSX.Element {
  const toast = useToast();
  const [duracion, setDuracion] = useState(DURACIONES_DISPONIBILIDAD[0]!);
  const [dias, setDias] = useState(VENTANAS_DISPONIBILIDAD[0]!);
  const [huecos, setHuecos] = useState<FreeSlot[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const [intento, setIntento] = useState(0);
  const zona = browserZone();

  useEffect(() => {
    if (!open) return;
    let stale = false;
    setHuecos(null);
    setFallo(false);
    const desde = new Date();
    const hasta = new Date(desde.getTime() + dias * 86_400_000);
    void suggestSlots({
      start: desde.toISOString(),
      end: hasta.toISOString(),
      durationMinutes: duracion,
      tz: zona,
      workStart: dosCifras(ajustes.inicioLaboral),
      workEnd: dosCifras(ajustes.finLaboral),
      weekdays: !ajustes.finesDeSemana,
      // El servicio devuelve tramos libres, no huecos sueltos: con el tope de 50
      // caben de sobra dos semanas de jornada.
    })
      .then(({ slots }) => {
        if (!stale) setHuecos(slots);
      })
      .catch(() => {
        if (!stale) setFallo(true);
      });
    return () => {
      stale = true;
    };
  }, [
    open,
    duracion,
    dias,
    ajustes.inicioLaboral,
    ajustes.finLaboral,
    ajustes.finesDeSemana,
    zona,
    intento,
  ]);

  const porDia = useMemo(
    () => (huecos ? diasDisponibles(huecos, zona, locale) : []),
    [huecos, zona, locale],
  );
  const texto = textoDisponibilidad(t, porDia, zona);

  async function copiar(): Promise<void> {
    try {
      await navigator.clipboard.writeText(texto);
      toast.notify({ tone: 'success', title: t('availability.copied') });
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('availability.copyFailed') });
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('availability.title')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => void copiar()} disabled={!texto}>
            {t('availability.copy')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('availability.description')}</p>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FieldGroup label={t('availability.duration')} htmlFor="disp-duracion">
            <Select
              id="disp-duracion"
              value={String(duracion)}
              onChange={(e) => setDuracion(Number(e.currentTarget.value))}
            >
              {DURACIONES_DISPONIBILIDAD.map((m) => (
                <option key={m} value={String(m)}>
                  {m % 60 === 0
                    ? t('settings.durationHours', { count: m / 60 })
                    : t('settings.durationMinutes', { count: m })}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('findTime.window')} htmlFor="disp-dias">
            <Select
              id="disp-dias"
              value={String(dias)}
              onChange={(e) => setDias(Number(e.currentTarget.value))}
            >
              {VENTANAS_DISPONIBILIDAD.map((d) => (
                <option key={d} value={String(d)}>
                  {t('findTime.windowDays', { count: d })}
                </option>
              ))}
            </Select>
          </FieldGroup>
        </div>
        {fallo ? (
          <EmptyState
            compact
            icon="warning"
            title={t('findTime.error')}
            action={
              <Button variant="secondary" size="sm" onClick={() => setIntento((n) => n + 1)}>
                {t('common.retry')}
              </Button>
            }
          />
        ) : huecos === null ? (
          <div class="flex flex-col gap-2" aria-busy="true" aria-label={t('findTime.loading')}>
            <Skeleton class="h-4 w-2/3" />
            <Skeleton class="h-4 w-1/2" />
            <Skeleton class="h-4 w-3/5" />
          </div>
        ) : porDia.length === 0 ? (
          <EmptyState compact icon="calendar" title={t('findTime.empty')} />
        ) : (
          // Lo que se va a copiar, tal cual: sin sorpresas al pegarlo.
          <pre class="border-border bg-bg-subtle text-text rounded-md border p-3 font-sans text-sm whitespace-pre-wrap">
            {texto}
          </pre>
        )}
      </div>
    </Modal>
  );
}
