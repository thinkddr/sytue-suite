/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Actividad» (solo el propietario): quién cambió qué y cuándo en el documento,
 * a partir de GET /api/audit. Una página de 100 operaciones a la vez (la API pagina por
 * `before_seq`), con «Cargar más»; filtros por persona y por tipo sobre lo ya cargado.
 * Nunca enseña el contenido de lo escrito: el servicio no lo manda.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Button, FieldGroup, Select } from '@sytue/ui';
import { formatDateTime } from '@sytue/i18n';
import {
  TIPOS_ACTIVIDAD,
  autorDeActividad,
  filtrarActividad,
  personasDeActividad,
  queToco,
} from '../state/actividad';
import type { AuditEntry, EditorContext } from '../types';

/** Operaciones que resume cada página (el servicio acota por su lado). */
const PAGINA = 100;

interface Pagina {
  entries?: AuditEntry[];
  next_before_seq?: number;
  names?: Record<string, string>;
}

export function ActividadPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const [entradas, setEntradas] = useState<AuditEntry[]>([]);
  const [nombres, setNombres] = useState<Record<string, string>>({});
  const [siguiente, setSiguiente] = useState(0);
  const [cargando, setCargando] = useState(true);
  const [fallo, setFallo] = useState(false);
  const [persona, setPersona] = useState('');
  const [tipo, setTipo] = useState('');
  const [aviso, setAviso] = useState('');
  /** Quita las respuestas de una carga anterior si el documento cambia a media petición. */
  const vuelo = useRef(0);

  const cargar = (antesDe: number): void => {
    const mio = ++vuelo.current;
    setCargando(true);
    setFallo(false);
    fetch(`/api/audit?id=${encodeURIComponent(ctx.doc.id)}&limit=${PAGINA}&before_seq=${antesDe}`)
      .then((r) =>
        r.ok ? (r.json() as Promise<Pagina>) : Promise.reject(new Error(`${r.status}`)),
      )
      .then((b) => {
        if (mio !== vuelo.current) return;
        const nuevas = b.entries ?? [];
        setEntradas((prev) => (antesDe === 0 ? nuevas : [...prev, ...nuevas]));
        setNombres((prev) => ({ ...prev, ...b.names }));
        setSiguiente(b.next_before_seq ?? 0);
        setAviso(t('activity.loaded', { count: nuevas.length }));
      })
      .catch(() => {
        if (mio === vuelo.current) setFallo(true);
      })
      .finally(() => {
        if (mio === vuelo.current) setCargando(false);
      });
  };

  useEffect(() => {
    cargar(0);
    return () => {
      vuelo.current += 1;
    };
    // Solo al abrir o al cambiar de documento.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx.doc.id]);

  const personas = useMemo(() => personasDeActividad(entradas), [entradas]);
  const visibles = useMemo(
    () => filtrarActividad(entradas, persona, tipo),
    [entradas, persona, tipo],
  );
  const etiquetaTipo = (k: string): string =>
    (TIPOS_ACTIVIDAD as readonly string[]).includes(k)
      ? t(`activity.kind.${k as (typeof TIPOS_ACTIVIDAD)[number]}`)
      : k;
  const fecha = (iso: string): string =>
    formatDateTime(ctx.locale, iso, { dateStyle: 'short', timeStyle: 'short' });

  return (
    <div class="flex min-h-full flex-col gap-3 p-3">
      <div class="flex flex-col gap-2">
        <FieldGroup label={t('activity.person')} htmlFor="actividad-persona">
          <Select
            id="actividad-persona"
            size="sm"
            value={persona}
            onChange={(e) => setPersona(e.currentTarget.value)}
          >
            <option value="">{t('activity.allPeople')}</option>
            {personas.map((sub) => (
              <option key={sub} value={sub}>
                {autorDeActividad(sub, nombres)}
              </option>
            ))}
          </Select>
        </FieldGroup>
        <FieldGroup label={t('activity.kind')} htmlFor="actividad-tipo">
          <Select
            id="actividad-tipo"
            size="sm"
            value={tipo}
            onChange={(e) => setTipo(e.currentTarget.value)}
          >
            <option value="">{t('activity.allKinds')}</option>
            {TIPOS_ACTIVIDAD.map((k) => (
              <option key={k} value={k}>
                {t(`activity.kind.${k}`)}
              </option>
            ))}
          </Select>
        </FieldGroup>
      </div>

      {/* Lo que cambia sin que se mueva el foco (cargar más, error) se anuncia aquí. */}
      <p class="sr-only" role="status" aria-live="polite">
        {fallo ? t('audit.error') : aviso}
      </p>

      {fallo ? (
        <p class="text-danger-text text-sm" role="alert">
          {t('audit.error')}
        </p>
      ) : visibles.length === 0 ? (
        <p class="text-text-muted text-sm">
          {cargando
            ? t('activity.loading')
            : entradas.length === 0
              ? t('audit.empty')
              : t('activity.noMatch')}
        </p>
      ) : (
        <ul class="flex flex-col gap-2" aria-label={t('audit.history')}>
          {visibles.map((e) => (
            <li
              key={`${e.seq_desde}-${e.author_sub}-${e.key ?? e.kind}`}
              class="border-border rounded-md border p-2"
            >
              <div class="flex items-baseline gap-1.5">
                <span class="text-text min-w-0 truncate text-xs font-semibold">
                  {autorDeActividad(e.author_sub, nombres)}
                </span>
                <time
                  class="sytue-meta-dense ml-auto shrink-0 pl-1.5"
                  dateTime={e.at}
                  title={fecha(e.at)}
                >
                  {fecha(e.at)}
                </time>
              </div>
              <p class="text-text-muted mt-1 text-xs break-words">
                {etiquetaTipo(e.kind)} · {queToco(e)} ×{e.count}
              </p>
            </li>
          ))}
        </ul>
      )}

      {siguiente > 0 && !fallo && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={cargando}
          onClick={() => cargar(siguiente)}
        >
          {cargando ? t('activity.loading') : t('activity.more')}
        </Button>
      )}
      {fallo && (
        <Button type="button" size="sm" variant="ghost" onClick={() => cargar(siguiente)}>
          {t('activity.retry')}
        </Button>
      )}
    </div>
  );
}
