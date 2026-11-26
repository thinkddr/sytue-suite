/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La actividad de una base (su traza de auditoría, solo para el propietario) contada
 * para personas, y la búsqueda de «Mis bases».
 *
 * La traza guarda `action` con la forma del servicio (`record.update`, `field.rename`…)
 * y quién lo hizo por su `sub`. Aquí se traduce cada acción a una frase del catálogo
 * (varias acciones técnicas comparten frase: cambiar la fórmula o las opciones de un
 * campo es «cambiar un campo»), se saca de qué trata (el nombre que guarda la propia
 * entrada o, si no, la tabla) y se pone nombre a quien la hizo con lo que ya se sabe
 * de los permisos. Un `sub` que no se conoce NO se enseña: «Otra persona».
 *
 * Es el espejo de `apps/fyne/apps/bases/actividad.go`: las dos plataformas cuentan la
 * misma entrada con la misma frase.
 */
import { contiene } from '@sytue/i18n';
import type {
  BasesAuditEntry,
  BasesBase,
  BasesPermission,
  BasesTable,
} from '@sytue/api-clients/bases';
import type { BasesMessageKey, T } from '../i18n/index.js';

/** Cuántas entradas se piden cada vez («Cargar más» pide las siguientes). */
export const PAGINA_ACTIVIDAD = 50;

/** Las claves del catálogo con la frase de cada acción. */
export type ClaveAccion = Extract<BasesMessageKey, `activity.do.${string}`>;

const FRASES: Record<string, ClaveAccion> = {
  'base.create': 'activity.do.baseCreate',
  'base.update': 'activity.do.baseUpdate',
  'base.transfer': 'activity.do.baseTransfer',
  'table.create': 'activity.do.tableCreate',
  'table.rename': 'activity.do.tableRename',
  'table.delete': 'activity.do.tableDelete',
  'table.import': 'activity.do.tableImport',
  'table.export': 'activity.do.tableExport',
  'field.create': 'activity.do.fieldCreate',
  'field.rename': 'activity.do.fieldRename',
  'field.delete': 'activity.do.fieldDelete',
  'field.options': 'activity.do.fieldChange',
  'field.formula': 'activity.do.fieldChange',
  'field.default': 'activity.do.fieldChange',
  'field.reorder': 'activity.do.fieldReorder',
  'record.create': 'activity.do.recordCreate',
  'record.create.form': 'activity.do.recordForm',
  'record.update': 'activity.do.recordUpdate',
  'record.link': 'activity.do.recordUpdate',
  'record.delete': 'activity.do.recordDelete',
  'view.create': 'activity.do.viewCreate',
  'view.update': 'activity.do.viewUpdate',
  'view.delete': 'activity.do.viewDelete',
  'permission.grant': 'activity.do.permissionGrant',
  'permission.revoke': 'activity.do.permissionRevoke',
  'comment.create': 'activity.do.commentCreate',
  'comment.delete': 'activity.do.commentDelete',
  'attachment.add': 'activity.do.attachmentAdd',
  'attachment.delete': 'activity.do.attachmentDelete',
  'form.link.create': 'activity.do.formLink',
  'form.link.revoke': 'activity.do.formLink',
  'connector.grant': 'activity.do.connection',
  'connector.revoke': 'activity.do.connection',
  'connector.rotate': 'activity.do.connection',
  'connector.read': 'activity.do.connectionUse',
  'connector.denied': 'activity.do.connectionUse',
};

/** Lo que ya se sabe de la base para contar sus entradas. */
export interface ContextoActividad {
  /** El `sub` de quien mira: sus entradas son «Tú». */
  yo: string;
  tablas: readonly BasesTable[];
  permisos: readonly BasesPermission[];
}

/** Una entrada lista para pintar. */
export interface EntradaContada {
  id: number;
  quien: string;
  /** La frase de la acción. */
  que: string;
  /** De qué trata: «Nombre · Clientes», o vacío si la entrada no lo dice. */
  sobre: string;
  at: string;
}

function nombreDe(x: unknown): string {
  if (x && typeof x === 'object' && 'name' in x) {
    if (typeof x.name === 'string') return x.name.trim();
  }
  return '';
}

function campoDe(x: unknown, clave: string): string {
  if (x && typeof x === 'object' && clave in x) {
    const v = (x as Record<string, unknown>)[clave];
    if (typeof v === 'string') return v;
  }
  return '';
}

/** Cómo se llama una persona por su `sub`: «Tú», su nombre del directorio o «Otra persona». */
export function nombreDePersona(sub: string, ctx: ContextoActividad, t: T): string {
  if (sub && sub === ctx.yo) return t('activity.you');
  const p = ctx.permisos.find(
    (x) =>
      (x.principal_kind === 'user' || x.principal_kind === 'external') && x.principal_id === sub,
  );
  return p?.principal_label || t('activity.someone');
}

/** A quién se dio o quitó acceso, con las mismas palabras que el diálogo de compartir. */
function destinatario(e: BasesAuditEntry, ctx: ContextoActividad, t: T): string {
  const datos = e.after ?? e.before;
  const clase = campoDe(datos, 'kind');
  const quien = campoDe(datos, 'principal');
  if (clase === 'tenant') return t('share.kind.tenant');
  if (clase === 'group') return t('share.groupLabel', { name: quien });
  return quien ? nombreDePersona(quien, ctx, t) : '';
}

/** Cuenta una entrada de la traza. */
export function contarEntrada(e: BasesAuditEntry, ctx: ContextoActividad, t: T): EntradaContada {
  const clave = FRASES[e.action];
  const que = clave ? t(clave) : t('activity.do.other', { action: e.action });
  const tabla = e.table_id ? (ctx.tablas.find((x) => x.id === e.table_id)?.name ?? '') : '';
  let propio = '';
  if (e.action.startsWith('permission.')) propio = destinatario(e, ctx, t);
  else if (!e.action.startsWith('base.')) propio = nombreDe(e.after) || nombreDe(e.before);
  // En las de tabla el nombre propio YA es el de la tabla: no se repite.
  const partes = e.action.startsWith('table.') ? [propio || tabla] : [propio, tabla];
  return {
    id: e.id,
    quien: nombreDePersona(e.actor, ctx, t),
    que,
    sobre: partes.filter(Boolean).join(' · '),
    at: e.at,
  };
}

/**
 * Las bases cuyo nombre o descripción contienen lo escrito, con la regla de `plegar`
 * («cafe» encuentra «Café»). Sin texto, todas.
 */
export function filtrarBases(bases: readonly BasesBase[], q: string): BasesBase[] {
  const limpio = q.trim();
  if (!limpio) return [...bases];
  return bases.filter((b) => contiene(b.name, limpio) || contiene(b.description ?? '', limpio));
}

/** El tope de la descripción de una base: el mismo que el servicio. */
export const MAX_DESCRIPCION = 1000;
