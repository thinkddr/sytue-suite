/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Una carpeta de una cuenta externa en el panel principal, y los buzones unificados
 * («Todos los buzones de entrada», «Todos los enviados», «Todos los borradores»): lo
 * que MailApp necesita para pintarlos con la misma lista y el mismo lector que las
 * carpetas de Sytue. Es puro; Fyne tiene lo mismo en apps/fyne/apps/mail/vista_externa.go,
 * con los mismos casos de mezcla (apps/fyne/apps/mail/testdata/mezcla-de-entradas.json).
 */
import type { ExternalMailMessage } from '@sytue/api-clients';
import type { EmailSummary } from '../client/mailApi.js';
import type { BuzonUnificado, FuenteDeBuzon } from './buzones.js';

/**
 * Los buzones unificados que se abren en el panel, mezclados por fecha (fase B, punto
 * 5): la Entrada, los Enviados y los Borradores. Archivo, Spam y Papelera solo se
 * despliegan en la carpeta de cada cuenta: vaciar o borrar para siempre es de una
 * carpeta, no de una mezcla. Fyne: `rolesMezclados`.
 */
export const ROLES_MEZCLADOS = ['inbox', 'drafts', 'sent'] as const;
export type RolMezclado = (typeof ROLES_MEZCLADOS)[number];

/** Si el buzón unificado de ese rol se abre en el panel, o solo se despliega. */
export function esMezclado(rol: string): rol is RolMezclado {
  return (ROLES_MEZCLADOS as readonly string[]).includes(rol);
}

/**
 * Lo que se mira en el panel cuando no es una carpeta de Sytue: una carpeta de una
 * cuenta, o la carpeta de un rol de todas mezclada por fecha.
 */
export type VistaExterna =
  { tipo: 'carpeta'; cuenta: string; carpeta: string } | { tipo: 'unificado'; rol: RolMezclado };

/**
 * Las carpetas que se piden para un buzón unificado: las de SU rol y ninguna más. El
 * sondeo de 30 s y «Cargar más» piden solo estas, las de la carpeta a la vista: con
 * «Todos los enviados» abierto no se vuelve a pedir ninguna Entrada.
 */
export function fuentesDeUnificado(
  unificados: readonly BuzonUnificado[],
  rol: RolMezclado,
): FuenteDeBuzon[] {
  return [...(unificados.find((u) => u.rol === rol)?.fuentes ?? [])];
}

/**
 * Si abrir una fila reabre el redactor en vez del lector: en Borradores de Sytue,
 * y en «Todos los borradores» con un borrador de Sytue. Uno de una cuenta se lee: su
 * borrador vive en su proveedor, y el redactor guarda en Borradores de Sytue.
 *
 * Con una búsqueda, nunca: la búsqueda es de todo el buzón y no quita la vista, así
 * que un resultado puede ser un recibido aunque se esté en Borradores; abierto en el
 * redactor, guardarlo (PUT /drafts/{id}) borraría el original.
 */
export function abreRedactor(
  vista: VistaExterna | null,
  rolSytue: string | undefined,
  id: string,
  enBusqueda: boolean,
): boolean {
  if (enBusqueda) return false;
  if (!vista) return rolSytue === 'drafts';
  return vista.tipo === 'unificado' && vista.rol === 'drafts' && origenExterno(id) === null;
}

/** Dónde vive un mensaje externo: su cuenta, su carpeta y su id en el proveedor. */
export interface OrigenExterno {
  cuenta: string;
  carpeta: string;
  id: string;
}

const PREFIJO = 'ext:';

/**
 * El id de un mensaje externo en la lista: lleva su cuenta y su carpeta, porque cada
 * acción (leer, mover, abrir) las pide, y en un buzón unificado los de
 * Sytue y los de cada cuenta van mezclados. Nunca choca con uno de Sytue (un UUID).
 */
export function idExterno(origen: OrigenExterno): string {
  return PREFIJO + [origen.cuenta, origen.carpeta, origen.id].map(encodeURIComponent).join(':');
}

/** El origen de un id de `idExterno`; null si el mensaje es de Sytue. */
export function origenExterno(id: string): OrigenExterno | null {
  if (!id.startsWith(PREFIJO)) return null;
  const partes = id.slice(PREFIJO.length).split(':');
  if (partes.length !== 3) return null;
  try {
    const [cuenta, carpeta, propio] = partes.map(decodeURIComponent) as [string, string, string];
    return cuenta && propio ? { cuenta, carpeta, id: propio } : null;
  } catch {
    return null;
  }
}

/**
 * Si el mensaje es de una cuenta sin acciones en su proveedor (POP3): se lee, no se
 * tría. En un buzón unificado la vista no es de solo lectura, pero su Entrada se
 * mezcla con las demás; un mensaje de Sytue nunca lo es.
 */
export function sinAccionesExternas(
  id: string,
  accionesDe: (cuenta: string) => boolean | undefined,
): boolean {
  const origen = origenExterno(id);
  return origen !== null && accionesDe(origen.cuenta) === false;
}

/**
 * Si la vista es la carpeta de una cuenta que ya no está entre `cuentas` (se
 * desconectó en el diálogo): hay que volver a la Entrada de Sytue.
 */
export function vistaSinCuenta(
  vista: VistaExterna | null,
  cuentas: readonly { instance_id: string }[],
): boolean {
  return vista?.tipo === 'carpeta' && !cuentas.some((c) => c.instance_id === vista.cuenta);
}

/**
 * Deshace un quitado optimista que falló: vuelven los `quitados`, en su sitio de
 * `antes`, y nada más. Lo que otra acción quitó entre tanto (los de una cuenta, que
 * sí se movieron en su proveedor) no reaparece, y lo de ahora conserva su estado.
 */
export function devolverQuitados<T extends { id: string }>(
  antes: readonly T[],
  ahora: readonly T[],
  quitados: ReadonlySet<string>,
): T[] {
  const vivos = new Map(ahora.map((e) => [e.id, e]));
  const vuelta = antes.flatMap((e) => (quitados.has(e.id) ? [e] : (vivos.get(e.id) ?? [])));
  const puestos = new Set(vuelta.map((e) => e.id));
  return [...vuelta, ...ahora.filter((e) => !puestos.has(e.id))];
}

/**
 * El sobre de un mensaje externo, como uno de Sytue: así lo pinta `MessageList` y lo
 * abre `MessageReader`. Sin hilo —el proveedor no lo da—: cada mensaje es una
 * conversación de uno (`groupByThread`). La fecha pasa a ISO para que la lista y la
 * mezcla la comparen como un instante; si el proveedor dio una que no se lee, se
 * queda tal cual y la mezcla la trata como la más vieja.
 */
export function sobreExterno(
  cuenta: string,
  carpeta: string,
  m: ExternalMailMessage,
): EmailSummary {
  const instante = Date.parse(m.date);
  return {
    id: idExterno({ cuenta, carpeta, id: m.id }),
    threadId: '',
    subject: m.subject,
    from: m.from,
    to: m.to,
    cc: m.cc ?? [],
    preview: m.snippet ?? '',
    receivedAt: Number.isNaN(instante) ? m.date : new Date(instante).toISOString(),
    keywords: { $seen: m.seen, $flagged: m.flagged },
    hasAttachment: (m.attachments?.length ?? 0) > 0,
    mailboxId: carpeta,
  };
}

/**
 * Una fuente de un buzón unificado: la carpeta de Sytue (`clave` vacía) o la de una
 * cuenta, con lo pedido y aún no enseñado (del más nuevo al más viejo, en el
 * orden de su servidor) y el cursor de su página siguiente («» = no hay más).
 */
export interface FuenteMezcla<T> {
  clave: string;
  cursor: string;
  pendientes: T[];
}

interface Fechado {
  id: string;
  receivedAt: string;
}

/** El instante de un sobre; uno ilegible cuenta como el más viejo. */
function instante(x: Fechado): number {
  const n = Date.parse(x.receivedAt);
  return Number.isNaN(n) ? 0 : n;
}

/**
 * Mezcla por fecha lo que las fuentes tienen pendiente. Saca siempre el más nuevo de
 * las cabezas y se para en cuanto una fuente que aún tiene páginas se queda sin
 * pendientes: lo siguiente podría ser suyo, y enseñar antes uno más viejo de otra lo
 * dejaría fuera de su sitio para siempre (la lista no se reordena al pedir más). La
 * fecha empatada la decide el orden de las fuentes, Sytue primero.
 */
export function mezclarPorFecha<T extends Fechado>(
  fuentes: readonly FuenteMezcla<T>[],
): { salida: T[]; fuentes: FuenteMezcla<T>[] } {
  const quedan = fuentes.map((f) => ({ ...f, pendientes: [...f.pendientes] }));
  const salida: T[] = [];
  for (;;) {
    if (quedan.some((f) => f.pendientes.length === 0 && f.cursor)) break;
    let mejor = -1;
    for (let i = 0; i < quedan.length; i++) {
      const cabeza = quedan[i]?.pendientes[0];
      if (!cabeza) continue;
      const actual = mejor < 0 ? undefined : quedan[mejor]?.pendientes[0];
      if (!actual || instante(cabeza) > instante(actual)) mejor = i;
    }
    const elegida = quedan[mejor];
    if (!elegida) break;
    salida.push(elegida.pendientes.shift() as T);
  }
  return { salida, fuentes: quedan };
}

/** Las fuentes cuya página siguiente hace falta para seguir mezclando. */
export function porPedir<T>(fuentes: readonly FuenteMezcla<T>[]): string[] {
  return fuentes.filter((f) => f.pendientes.length === 0 && f.cursor).map((f) => f.clave);
}

/** Si queda algo por enseñar: pendientes o páginas sin pedir. */
export function quedanEnMezcla<T>(fuentes: readonly FuenteMezcla<T>[]): boolean {
  return fuentes.some((f) => f.pendientes.length > 0 || f.cursor !== '');
}
