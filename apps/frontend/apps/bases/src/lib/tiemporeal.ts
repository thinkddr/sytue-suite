/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Coedición en tiempo real, lado del navegador (contrato v1 del servicio).
 *
 * El flujo dice QUÉ ha cambiado, no trae datos. Aquí se decide qué hay que volver a
 * pedir (`planDe`, pura y probada) y se mantiene abierto el `EventSource` contra el BFF:
 * el navegador reconecta solo y manda `Last-Event-ID`, así que el servicio le cuenta lo
 * que se perdió. Cuando el servicio se despide por falta de acceso, se deja de reintentar.
 */
import {
  BASES_REALTIME_VERSION,
  type BasesChange,
  type BasesPresence,
} from '@sytue/api-clients/bases';

/** Qué hacer ante un cambio que llega por el flujo. */
export type Plan =
  | { tipo: 'nada' }
  /** Volver a pedir ese registro y sustituirlo (o añadirlo) en lo que hay en pantalla. */
  | { tipo: 'registro'; id: string; nuevo: boolean }
  | { tipo: 'quitar'; id: string }
  /** Recargar base, tablas y campos (y con ello el rol). */
  | { tipo: 'estructura' }
  | { tipo: 'comentarios'; registro: string };

export interface Contexto {
  /** La tabla que se está mirando. */
  tabla: string;
  /** Las operaciones de celda que ha lanzado ESTA pestaña: su eco no se vuelve a pedir. */
  propias: ReadonlySet<string>;
}

/** Decide qué pedir de nuevo ante un cambio. Lo de otra tabla no toca lo que se ve. */
export function planDe(c: BasesChange, ctx: Contexto): Plan {
  if (c.action === 'access' || c.action === 'base.update') return { tipo: 'estructura' };
  if (c.action.startsWith('table.') || c.action.startsWith('field.')) return { tipo: 'estructura' };
  if (c.action.startsWith('view.'))
    return c.table_id === ctx.tabla ? { tipo: 'estructura' } : { tipo: 'nada' };
  if (c.table_id !== ctx.tabla || !c.record_id) return { tipo: 'nada' };
  if (c.action.startsWith('comment.')) return { tipo: 'comentarios', registro: c.record_id };
  if (c.op_id && ctx.propias.has(c.op_id)) return { tipo: 'nada' };
  switch (c.action) {
    case 'record.delete':
      return { tipo: 'quitar', id: c.record_id };
    case 'record.create':
      return { tipo: 'registro', id: c.record_id, nuevo: true };
    case 'record.update':
    case 'record.link':
      return { tipo: 'registro', id: c.record_id, nuevo: false };
    default:
      return { tipo: 'nada' };
  }
}

/** Quién está en cada celda de una tabla (clave `registro:campo`), sin la propia pestaña. */
export function presentesPorCelda(
  presentes: readonly BasesPresence[],
  tabla: string,
  miConn: string,
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const p of presentes) {
    if (p.conn === miConn || p.table_id !== tabla || !p.record_id) continue;
    const clave = `${p.record_id}:${p.field_id ?? ''}`;
    const nombres = out.get(clave) ?? [];
    const nombre = p.name || p.sub.slice(0, 8);
    if (!nombres.includes(nombre)) nombres.push(nombre);
    out.set(clave, nombres);
  }
  return out;
}

/** Las personas presentes en la base (una por persona, sin contarse a una misma pestaña). */
export function personasPresentes(presentes: readonly BasesPresence[], miConn: string): string[] {
  const vistas = new Set<string>();
  const out: string[] = [];
  for (const p of presentes) {
    if (p.conn === miConn || vistas.has(p.sub)) continue;
    vistas.add(p.sub);
    out.push(p.name || p.sub.slice(0, 8));
  }
  return out;
}

/** Un identificador aleatorio de pestaña o de operación. */
export function idAleatorio(): string {
  return crypto.randomUUID();
}

export interface Oyentes {
  /** El flujo está abierto (también tras reconectar). */
  alAbrir: () => void;
  alCambiar: (c: BasesChange) => void;
  alPresencia: (p: BasesPresence[]) => void;
  /** Se quedó demasiado atrás: hay que recargar todo. */
  alResincronizar: () => void;
  /** El servicio cerró por falta de acceso: no se reintenta. */
  alPerderAcceso: () => void;
  /** La conexión se ha caído; el navegador reintenta solo. */
  alCaer: () => void;
}

/** Abre el flujo de una base. Devuelve cómo cerrarlo. */
export function seguir(url: string, conn: string, o: Oyentes): () => void {
  const es = new EventSource(`${url}?v=${BASES_REALTIME_VERSION}&conn=${encodeURIComponent(conn)}`);
  const datos = <T>(e: Event): T | null => {
    try {
      return JSON.parse((e as MessageEvent<string>).data) as T;
    } catch {
      return null;
    }
  };
  es.addEventListener('hello', () => o.alAbrir());
  es.addEventListener('change', (e) => {
    const c = datos<BasesChange>(e);
    if (c && typeof c.action === 'string') o.alCambiar(c);
  });
  es.addEventListener('presence', (e) => {
    const p = datos<BasesPresence[]>(e);
    if (Array.isArray(p)) o.alPresencia(p);
  });
  es.addEventListener('resync', () => o.alResincronizar());
  es.addEventListener('bye', (e) => {
    if (datos<{ reason?: string }>(e)?.reason === 'access') {
      es.close();
      o.alPerderAcceso();
    }
    // `token` o `error`: el servidor cierra y EventSource reconecta con Last-Event-ID.
  });
  // La conexión se ha caído (o el servidor la cerró tras despedirse): el navegador
  // reintenta solo, con `Last-Event-ID`, salvo que se haya cerrado aquí por falta de acceso.
  es.onerror = () => {
    if (es.readyState !== EventSource.CLOSED) o.alCaer();
  };
  return () => es.close();
}
