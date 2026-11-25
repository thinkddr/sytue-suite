/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * HistorialLotes — las acciones en lote recientes y a quién no se aplicaron.
 *
 * Sytue ID apunta cada lote como un trabajo —qué, cuántos y cómo acabó cada
 * persona— y nadie podía verlo: en cuanto se cerraba el aviso de «se aplicó a 28
 * de 30», no quedaba forma de saber cuáles eran los dos. Se pide al desplegar: es
 * consulta de después, no algo que haga falta en cada visita al directorio.
 */
import { useState } from 'preact/hooks';
import { Badge, Button, Spinner } from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { useTGestion, type ClaveGestion, type TGestion } from '../i18n/gestion/index.js';
import { fallosDelLote, type ResultadoLote, type TrabajoLote } from '../lib/gestion.js';

/** Motivo legible de cada código estable del lote (admin_bulk.go en Sytue ID). */
const MOTIVO: Record<string, ClaveGestion> = {
  not_found: 'gestion.lote.codigo.not_found',
  self: 'gestion.lote.codigo.self',
  forbidden: 'gestion.lote.codigo.forbidden',
  deleted: 'gestion.lote.codigo.deleted',
  last_owner: 'gestion.lote.codigo.last_owner',
  last_admin: 'gestion.lote.codigo.last_admin',
};

export function motivoDeLote(code: string | undefined, tg: TGestion): string {
  return tg(MOTIVO[code ?? ''] ?? 'gestion.lote.codigo.error');
}

const ESTADO: Record<string, { clave: ClaveGestion; tono: BadgeTone }> = {
  ejecutando: { clave: 'gestion.lotes.estado.ejecutando', tono: 'info' },
  terminado: { clave: 'gestion.lotes.estado.terminado', tono: 'success' },
  cancelado: { clave: 'gestion.lotes.estado.cancelado', tono: 'neutral' },
  interrumpido: { clave: 'gestion.lotes.estado.interrumpido', tono: 'warning' },
};

function accion(a: string, t: T, tg: TGestion): string {
  switch (a) {
    case 'suspend':
      return t('users.bulk.suspend');
    case 'reactivate':
      return t('users.bulk.reactivate');
    case 'revoke_sessions':
      return t('users.bulk.revokeSessions');
    case 'set_role':
      return tg('gestion.lotes.accion.setRole');
    default:
      return a;
  }
}

/** La lista de a quién no se aplicó un lote (la usa también el aviso del directorio). */
export function FallosLote({
  fallos,
  nombre,
  tg,
}: {
  fallos: readonly ResultadoLote[];
  /** Nombre visible de un id, si la pantalla lo tiene cargado. */
  nombre: (id: string) => string;
  tg: TGestion;
}) {
  return (
    <ul class="flex flex-col gap-1">
      {fallos.map((f) => (
        <li key={f.id} class="flex flex-wrap items-baseline gap-x-2 text-sm">
          <span class="text-text font-medium">{nombre(f.id)}</span>
          <span class="text-text-muted">{motivoDeLote(f.code, tg)}</span>
        </li>
      ))}
    </ul>
  );
}

export default function HistorialLotes({
  locale,
  nombre,
}: {
  locale: Locale;
  nombre: (id: string) => string;
}) {
  const t = useT(locale);
  const tg = useTGestion(locale);
  const [estado, setEstado] = useState<'cerrado' | 'cargando' | 'error' | 'ok'>('cerrado');
  const [lotes, setLotes] = useState<TrabajoLote[]>([]);
  const [detalle, setDetalle] = useState<Record<string, TrabajoLote | 'cargando' | 'error'>>({});

  async function cargar() {
    setEstado('cargando');
    try {
      const res = await fetch('/api/admin/jobs', { headers: { accept: 'application/json' } });
      if (!res.ok) throw new Error(String(res.status));
      const d = (await res.json()) as { jobs?: TrabajoLote[] };
      setLotes(d.jobs ?? []);
      setEstado('ok');
    } catch {
      setEstado('error');
    }
  }

  async function verDetalle(id: string) {
    setDetalle((p) => ({ ...p, [id]: 'cargando' }));
    try {
      const res = await fetch(`/api/admin/jobs/${encodeURIComponent(id)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const lote = (await res.json()) as TrabajoLote;
      setDetalle((p) => ({ ...p, [id]: lote }));
    } catch {
      setDetalle((p) => ({ ...p, [id]: 'error' }));
    }
  }

  return (
    <section
      aria-labelledby="lotes-titulo"
      class="border-border bg-surface mx-4 mb-4 rounded-lg border"
    >
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2.5">
        <h2 id="lotes-titulo" class="text-text text-sm font-semibold">
          {tg('gestion.lotes.titulo')}
        </h2>
        <p class="text-text-muted min-w-0 flex-1 text-sm">{tg('gestion.lotes.ayuda')}</p>
        {estado === 'cerrado' || estado === 'error' ? (
          <Button size="sm" variant="secondary" onClick={() => void cargar()}>
            {estado === 'error' ? t('common.tryAgain') : tg('gestion.lotes.ver')}
          </Button>
        ) : null}
      </div>
      {estado === 'cargando' ? (
        <p class="text-text-muted flex items-center gap-2 px-3 pb-3 text-sm" role="status">
          <Spinner size={16} /> {t('common.loading')}
        </p>
      ) : estado === 'error' ? (
        <p class="text-danger-text px-3 pb-3 text-sm" role="alert">
          {tg('gestion.lotes.error')}
        </p>
      ) : estado === 'ok' ? (
        lotes.length === 0 ? (
          <p class="text-text-muted px-3 pb-3 text-sm">{tg('gestion.lotes.vacio')}</p>
        ) : (
          <ul class="border-border divide-border divide-y border-t">
            {lotes.map((l) => {
              const e = ESTADO[l.status];
              const d = detalle[l.id];
              return (
                <li key={l.id} class="flex flex-col gap-2 px-3 py-2">
                  <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <span class="text-text min-w-0 flex-1 text-sm font-medium">
                      {accion(l.action, t, tg)}
                      <span class="sytue-meta ml-2">{formatDateTime(locale, l.created_at)}</span>
                    </span>
                    <span class="text-text-muted text-sm tabular-nums">
                      {tg('gestion.lotes.progreso', { done: l.done, total: l.total })}
                    </span>
                    <Badge tone={e?.tono ?? 'neutral'}>{e ? tg(e.clave) : l.status}</Badge>
                    {d === undefined || d === 'error' ? (
                      <Button size="sm" variant="ghost" onClick={() => void verDetalle(l.id)}>
                        {d === 'error' ? t('common.tryAgain') : tg('gestion.lotes.detalle')}
                      </Button>
                    ) : null}
                  </div>
                  {d === 'cargando' ? (
                    <Spinner size={16} />
                  ) : d && d !== 'error' ? (
                    fallosDelLote(d.results).length === 0 ? (
                      <p class="text-success-text text-sm">{tg('gestion.lotes.todosBien')}</p>
                    ) : (
                      <div class="bg-bg-subtle rounded-md px-3 py-2">
                        <p class="text-text mb-1 text-sm font-medium">
                          {tg('gestion.lote.fallos')}
                        </p>
                        <FallosLote fallos={fallosDelLote(d.results)} nombre={nombre} tg={tg} />
                      </div>
                    )
                  ) : null}
                </li>
              );
            })}
          </ul>
        )
      ) : null}
    </section>
  );
}
