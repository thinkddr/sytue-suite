/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Anfitrión de un script EN BASES: traduce cada llamada de `sytue.v1` a UNA petición
 * corriente de la API de Bases, hecha con la sesión de quien ejecuta. No hay camino
 * propio ni credencial de script: el servidor autoriza cada operación como autoriza
 * cualquier otra de esa persona, así que un script nunca puede más que quien lo lanza.
 *
 * El bucle (sandbox, tiempo, operaciones, ritmo, tamaño, saneado de lo devuelto) es el
 * común, `ejecutarAislado` de `@sytue/scripts`. Lo que añade este fichero es lo que el
 * servidor NO puede saber, porque para él son peticiones de la persona:
 *  - el manifiesto: la operación ha de estar declarada, y si escribe, la escritura;
 *  - el CONFINAMIENTO A LA BASE del script: toda tabla ha de ser de esa base (y del
 *    manifiesto, si acota), y todo registro se pide antes y ha de ser de una tabla
 *    permitida. Sin esto, un script aprobado en una base podría leer otra base a la que
 *    quien lo ejecuta tiene acceso y copiarla a la primera, donde su autor sí lee;
 *  - nunca se fuerza un borrado ni se toca nada que no sean registros.
 * Una llamada denegada —aquí o por el servidor— ABORTA la ejecución con su motivo.
 */
import { ejecutarAislado } from '@sytue/scripts';
import type { Atencion, Escalar, ResultadoScript, Sandbox } from '@sytue/scripts';
import { HttpError } from '@sytue/api-clients';
import type { BasesClient, BasesRecord, BasesTable, BasesValue } from '@sytue/api-clients/bases';

import { RE_UUID, validarManifiesto } from './manifiesto.js';
import type { ManifiestoScript } from './manifiesto.js';
import { OPERACIONES, escribe } from './superficie.js';
import type { Operacion } from './superficie.js';

/** Lo que el anfitrión usa de la API: registros y lo necesario para encontrarlos. Nada más. */
export type ApiDeScripts = Pick<
  BasesClient,
  | 'listTables'
  | 'listViews'
  | 'listRecords'
  | 'getRecord'
  | 'createRecord'
  | 'updateRecord'
  | 'deleteRecord'
>;

/** Registros por página que puede pedir un script. */
export const MAX_PAGINA_SCRIPT = 100;
const MAX_CURSOR = 4096;

export interface OpcionesDeBase {
  codigo: string;
  /** Manifiesto de origen no fiable: se vuelve a validar aquí. */
  manifiesto: unknown;
  baseId: string;
  api: ApiDeScripts;
  crearSandbox: () => Sandbox;
  ahora?: () => number;
}

const no = (codigo: string, mensaje: string): Atencion => ({ tipo: 'rechazo', codigo, mensaje });
const valor = (v: unknown): Atencion => ({ tipo: 'valor', valor: v });

const esId = (x: Escalar | undefined): x is string => typeof x === 'string' && RE_UUID.test(x);

/** Un objeto llano leído de un argumento JSON; `null` si no lo es. */
function objeto(x: Escalar | undefined): Record<string, unknown> | null {
  if (typeof x !== 'string') return null;
  try {
    const v: unknown = JSON.parse(x);
    return typeof v === 'object' && v !== null && !Array.isArray(v)
      ? (v as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

/** Lo que se devuelve de un registro: sus datos, no quién lo tocó ni sus adjuntos. */
const registro = (r: BasesRecord) => ({
  id: r.id,
  table_id: r.table_id,
  data: r.data,
  links: r.links,
  rev: r.rev,
});

/** Un fallo de la API como motivo de aborto. Lo que no es de la API se relanza. */
function deLaApi(e: unknown): Atencion {
  if (!(e instanceof HttpError)) throw e;
  if (e.status === 403) return no('sin_permiso', 'No tienes permiso para esa operación.');
  if (e.status === 404) return no('no_existe', 'Eso no existe o no puedes verlo.');
  if (e.status === 409)
    return no(
      e.code === 'rev_conflict' ? 'conflicto' : 'rechazado_por_la_base',
      e.code ?? 'conflicto',
    );
  if (e.status === 429)
    return { tipo: 'limite', codigo: 'ritmo_api', mensaje: 'Demasiadas peticiones.' };
  if (e.status === 422 || e.status === 400)
    return no('invalido', 'El servidor no admite esos datos.');
  throw e;
}

export function ejecutarScriptDeBase(o: OpcionesDeBase): Promise<ResultadoScript> {
  const v = validarManifiesto(o.manifiesto);
  if (!v.ok)
    return Promise.resolve({
      outcome: 'denied',
      errorCode: 'manifiesto_invalido',
      message: v.error,
      durationMs: 0,
      ops: 0,
      output: [],
    });
  const m: ManifiestoScript = v.manifiesto;

  // Las tablas de la base, pedidas una vez por ejecución con el permiso de quien ejecuta:
  // lo que no sale aquí no es de esta base (o no lo lee) y no existe para el script.
  let tablas: Promise<Map<string, BasesTable>> | undefined;
  const tablasDeLaBase = (): Promise<Map<string, BasesTable>> =>
    (tablas ??= o.api
      .listTables(o.baseId)
      .then((l) => new Map(l.filter((t) => t.base_id === o.baseId).map((t) => [t.id, t]))));
  const tablaPermitida = async (id: string): Promise<BasesTable | undefined> =>
    m.tables.length > 0 && !m.tables.includes(id) ? undefined : (await tablasDeLaBase()).get(id);
  const FUERA = no('ambito', 'Fuera de la base o de las tablas del manifiesto.');

  /** Pide un registro y lo da por inexistente si no es de una tabla permitida. */
  const registroPermitido = async (
    id: string,
  ): Promise<{ r: BasesRecord; tabla: BasesTable } | undefined> => {
    const r = await o.api.getRecord(id);
    const tabla = await tablaPermitida(r.table_id);
    return tabla ? { r, tabla } : undefined;
  };

  /** Separa `{ campo: valor }` en datos y enlaces según el tipo de cada campo de la tabla. */
  const repartir = (
    tabla: BasesTable,
    entrada: Record<string, unknown>,
  ): { data: Record<string, BasesValue>; links: Record<string, string[]> } | null => {
    const data: Record<string, BasesValue> = {};
    const links: Record<string, string[]> = {};
    for (const [campo, x] of Object.entries(entrada)) {
      const def = tabla.fields.find((f) => f.id === campo);
      if (!def) return null;
      if (def.kind === 'link') {
        if (!Array.isArray(x) || !x.every((i) => typeof i === 'string' && RE_UUID.test(i)))
          return null;
        links[campo] = x as string[];
      } else if (
        x === null ||
        typeof x === 'string' ||
        typeof x === 'boolean' ||
        (typeof x === 'number' && Number.isFinite(x)) ||
        (Array.isArray(x) && x.every((i) => typeof i === 'string'))
      ) {
        data[campo] = x;
      } else return null;
    }
    return { data, links };
  };

  const atender = async (op: Operacion, args: readonly Escalar[]): Promise<Atencion> => {
    if (!m.ops.includes(op))
      return no('operacion_no_declarada', `El manifiesto no declara «${op}».`);
    // Segunda barrera: aunque un manifiesto llegara aquí sin validar, sin `write` no se escribe.
    if (escribe(op) && !m.write)
      return no('escritura_no_declarada', `«${op}» escribe y el manifiesto no declara escritura.`);
    const [a, b] = args;
    if (op === 'tables.list') {
      if (args.length > 0) return no('args', 'tables.list() no lleva argumentos.');
      const todas = [...(await tablasDeLaBase()).values()];
      return valor(
        todas
          .filter((t) => m.tables.length === 0 || m.tables.includes(t.id))
          .map((t) => ({ id: t.id, name: t.name })),
      );
    }
    if (!esId(a)) return no('args', 'El primer argumento ha de ser un id.');
    switch (op) {
      case 'fields.list':
      case 'views.list': {
        const tabla = await tablaPermitida(a);
        if (!tabla) return FUERA;
        if (op === 'fields.list')
          return valor(
            tabla.fields.map((f) => ({
              id: f.id,
              name: f.name,
              kind: f.kind,
              ...(f.choices ? { choices: f.choices } : {}),
              ...(f.link_table_id ? { link_table_id: f.link_table_id } : {}),
            })),
          );
        const vistas = await o.api.listViews(a);
        return valor(vistas.map((x) => ({ id: x.id, name: x.name, kind: x.kind })));
      }
      case 'records.list': {
        if (!(await tablaPermitida(a))) return FUERA;
        const q = b === undefined ? {} : objeto(b);
        if (
          !q ||
          Object.keys(q).some((k) => k !== 'view' && k !== 'cursor' && k !== 'limit') ||
          (q.view !== undefined && !(typeof q.view === 'string' && RE_UUID.test(q.view))) ||
          (q.cursor !== undefined &&
            !(typeof q.cursor === 'string' && q.cursor.length <= MAX_CURSOR)) ||
          (q.limit !== undefined &&
            !(
              Number.isInteger(q.limit) &&
              (q.limit as number) >= 1 &&
              (q.limit as number) <= MAX_PAGINA_SCRIPT
            ))
        )
          return no('args', 'records.list(tabla, { view?, cursor?, limit? }).');
        const p = await o.api.listRecords(a, {
          limit: (q.limit as number | undefined) ?? MAX_PAGINA_SCRIPT,
          ...(q.view === undefined ? {} : { view: q.view }),
          ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
        });
        return valor({
          records: p.records.map(registro),
          next_cursor: p.next_cursor ?? null,
        });
      }
      case 'records.get': {
        const x = await registroPermitido(a);
        return x ? valor(registro(x.r)) : FUERA;
      }
      case 'records.create': {
        const tabla = await tablaPermitida(a);
        if (!tabla) return FUERA;
        const entrada = objeto(b);
        const partes = entrada && repartir(tabla, entrada);
        if (!partes) return no('args', 'records.create(tabla, { <campo>: valor }).');
        return valor(registro(await o.api.createRecord(a, partes.data, partes.links)));
      }
      case 'records.update': {
        const x = await registroPermitido(a);
        if (!x) return FUERA;
        const entrada = objeto(b);
        const partes = entrada && repartir(x.tabla, entrada);
        if (!partes) return no('args', 'records.update(registro, { <campo>: valor }).');
        return valor(registro(await o.api.updateRecord(a, x.r.rev, partes.data, partes.links)));
      }
      default: {
        // records.delete: nunca forzado; si otros registros lo enlazan, el servidor lo niega.
        if (!(await registroPermitido(a))) return FUERA;
        await o.api.deleteRecord(a);
        return valor(null);
      }
    }
  };

  return ejecutarAislado({
    codigo: o.codigo,
    operaciones: OPERACIONES,
    maxOps: m.max_ops,
    maxMs: m.max_ms,
    crearSandbox: o.crearSandbox,
    ...(o.ahora ? { ahora: o.ahora } : {}),
    atender: (op, args) => atender(op, args).catch(deLaApi),
  });
}
