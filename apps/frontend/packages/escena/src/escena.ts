/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Consultas y operaciones PURAS sobre la escena: recorrer, buscar, cajas en el
 * lienzo, rutas de conectores, añadir/quitar/mover/girar nodos, orden Z,
 * alinear/distribuir, agrupar y gestión de capas. Todas devuelven una escena
 * NUEVA (la de entrada no se muta); si la operación no tiene efecto devuelven la
 * misma instancia, para que el editor pueda comparar por identidad.
 */
import { IDENTIDAD, aplicar, componer, girar, girarEn, trasladar } from './matriz.js';
import type { Matriz } from './matriz.js';
import { connectorRoute } from './conector.js';
import type { ConnEnd, ConnectorRoute } from './conector.js';
import { alignBoxes, boundingBox, distributeBoxes, reorderZ } from './organizar.js';
import type { AlignEdge, DistAxis, Rect, ZOrderOp } from './organizar.js';
import { ESCENA_VERSION } from './tipos.js';
import type {
  Capa,
  Escena,
  ExtremoConector,
  Nodo,
  NodoConector,
  NodoGrupo,
  Punto,
} from './tipos.js';

/** Nodos con caja (x/y/w/h/rot). */
export type NodoDeCaja = Extract<Nodo, { w: number }>;

export const esCaja = (n: Nodo): n is NodoDeCaja =>
  n.tipo === 'forma' || n.tipo === 'texto' || n.tipo === 'imagen' || n.tipo === 'grupo';

export function escenaVacia(
  opc: { ancho?: number; alto?: number; nombreCapa?: string; idCapa?: string } = {},
): Escena {
  return {
    v: ESCENA_VERSION,
    ancho: opc.ancho ?? 800,
    alto: opc.alto ?? 600,
    capas: [
      {
        id: opc.idCapa ?? 'capa1',
        nombre: opc.nombreCapa ?? 'Capa 1',
        visible: true,
        bloqueada: false,
        nodos: [],
      },
    ],
  };
}

/* ---------- Recorrido y búsqueda ---------- */

export interface Visita {
  nodo: Nodo;
  capa: Capa;
  /** Ancestros (grupos), del más externo al más interno; vacío = nodo de primer nivel. */
  camino: NodoGrupo[];
}

/** Todos los nodos, en profundidad, capas de abajo arriba y cada lista en orden Z. */
export function* recorrer(escena: Escena): Generator<Visita> {
  function* dentro(capa: Capa, nodos: Nodo[], camino: NodoGrupo[]): Generator<Visita> {
    for (const nodo of nodos) {
      yield { nodo, capa, camino };
      if (nodo.tipo === 'grupo') yield* dentro(capa, nodo.hijos, [...camino, nodo]);
    }
  }
  for (const capa of escena.capas) yield* dentro(capa, capa.nodos, []);
}

export function buscar(escena: Escena, id: string): Visita | undefined {
  for (const v of recorrer(escena)) if (v.nodo.id === id) return v;
  return undefined;
}

/** Id aleatorio corto que no existe aún en la escena (nodos ni capas). Es
 *  aleatorio y no un contador a propósito: un comentario anclado a un nodo
 *  borrado no puede acabar apuntando a otro nodo que reutilice su id. */
export function nuevoId(escena: Escena, prefijo = 'n'): string {
  const usados = new Set<string>(escena.capas.map((c) => c.id));
  for (const v of recorrer(escena)) usados.add(v.nodo.id);
  for (;;) {
    const bytes = crypto.getRandomValues(new Uint8Array(4));
    const id = prefijo + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    if (!usados.has(id)) return id;
  }
}

/* ---------- Geometría de un nodo ---------- */

/** Matriz que lleva el sistema local de un nodo de caja (origen en su esquina
 *  superior izquierda) al de su padre: traslación al centro, giro y vuelta. */
export const matrizDeCaja = (n: {
  x: number;
  y: number;
  w: number;
  h: number;
  rot?: number;
}): Matriz =>
  componer(
    trasladar(n.x + n.w / 2, n.y + n.h / 2),
    componer(girar(n.rot ?? 0), trasladar(-n.w / 2, -n.h / 2)),
  );

/** Matriz nodo → padre (identidad para líneas, trazos y conectores). */
export const matrizDe = (n: Nodo): Matriz => (esCaja(n) ? matrizDeCaja(n) : IDENTIDAD);

/** Matriz del sistema local de los hijos de `camino` al lienzo. */
export const matrizMundo = (camino: readonly NodoGrupo[]): Matriz =>
  camino.reduce((m, g) => componer(m, matrizDeCaja(g)), IDENTIDAD);

const comoCaja = (p: Punto): Rect => ({ x: p.x, y: p.y, w: 0, h: 0 });

/** Puntos que delimitan al nodo, en el sistema de su padre: esquinas (nodos de
 *  caja), extremos (líneas) o puntos del trazo. Los conectores no tienen (los
 *  da la ruta, que necesita la escena). */
export function puntosDe(n: Nodo): Punto[] {
  switch (n.tipo) {
    case 'linea':
      return [
        { x: n.x1, y: n.y1 },
        { x: n.x2, y: n.y2 },
      ];
    case 'trazo': {
      const out: Punto[] = [];
      for (let i = 0; i + 1 < n.puntos.length; i += 2) {
        out.push({ x: n.puntos[i]!, y: n.puntos[i + 1]! });
      }
      return out;
    }
    case 'conector':
      return [];
    default: {
      const m = matrizDeCaja(n);
      return [
        aplicar(m, { x: 0, y: 0 }),
        aplicar(m, { x: n.w, y: 0 }),
        aplicar(m, { x: n.w, y: n.h }),
        aplicar(m, { x: 0, y: n.h }),
      ];
    }
  }
}

/** Extremo de un conector resuelto contra la escena (rect del nodo anclado o punto libre). */
export function resolverExtremo(escena: Escena, e: ExtremoConector): ConnEnd {
  if ('nodo' in e) {
    const caja = cajaMundo(escena, e.nodo);
    if (caja) return { rect: caja, anchor: e.lado ?? 'auto' };
    return { point: { x: 0, y: 0 } };
  }
  return { point: { x: e.x, y: e.y } };
}

/** Ruta de un conector (en coordenadas del lienzo). */
export const rutaDe = (escena: Escena, c: NodoConector): ConnectorRoute =>
  connectorRoute(resolverExtremo(escena, c.desde), resolverExtremo(escena, c.hasta), c.trazado);

/** Rectángulo envolvente de un nodo en coordenadas del lienzo, o `undefined` si no existe. */
export function cajaMundo(escena: Escena, id: string): Rect | undefined {
  const v = buscar(escena, id);
  if (!v) return undefined;
  if (v.nodo.tipo === 'conector') {
    const r = rutaDe(escena, v.nodo);
    return boundingBox([comoCaja(r.start), comoCaja(r.end)]);
  }
  const m = matrizMundo(v.camino);
  return boundingBox(puntosDe(v.nodo).map((p) => comoCaja(aplicar(m, p))));
}

/* ---------- Edición de nodos (inmutable) ---------- */

/** Reconstruye la lista aplicando `fn` a cada nodo: devuelve el mismo nodo para
 *  dejarlo igual (y entonces se desciende a los hijos de un grupo), otro para
 *  sustituirlo (sin descender) o `null` para quitarlo. Conserva las identidades
 *  de lo que no cambia. */
function mapear(nodos: Nodo[], fn: (n: Nodo) => Nodo | null): Nodo[] {
  let cambio = false;
  const out: Nodo[] = [];
  for (const n of nodos) {
    let r = fn(n);
    if (r === n && n.tipo === 'grupo') {
      const hijos = mapear(n.hijos, fn);
      if (hijos !== n.hijos) r = { ...n, hijos };
    }
    if (r !== n) cambio = true;
    if (r) out.push(r);
  }
  return cambio ? out : nodos;
}

function mapearEscena(escena: Escena, fn: (n: Nodo) => Nodo | null): Escena {
  let cambio = false;
  const capas = escena.capas.map((c) => {
    const nodos = mapear(c.nodos, fn);
    if (nodos === c.nodos) return c;
    cambio = true;
    return { ...c, nodos };
  });
  return cambio ? { ...escena, capas } : escena;
}

/** Sustituye el nodo `id` por lo que devuelva `fn`. */
export const actualizarNodo = (escena: Escena, id: string, fn: (n: Nodo) => Nodo): Escena =>
  mapearEscena(escena, (n) => (n.id === id ? fn(n) : n));

export function agregarNodo(escena: Escena, capaId: string, nodo: Nodo): Escena {
  if (!escena.capas.some((c) => c.id === capaId)) return escena;
  return {
    ...escena,
    capas: escena.capas.map((c) => (c.id === capaId ? { ...c, nodos: [...c.nodos, nodo] } : c)),
  };
}

const idsDe = (n: Nodo, out: Set<string>): void => {
  out.add(n.id);
  if (n.tipo === 'grupo') n.hijos.forEach((h) => idsDe(h, out));
};

/** Quita los conectores anclados a algún nodo de `quitados` (borrar una forma
 *  borra sus conectores, como en Diapos). */
function podarConectores(escena: Escena, quitados: Set<string>): Escena {
  const anclado = (e: ExtremoConector): boolean => 'nodo' in e && quitados.has(e.nodo);
  return mapearEscena(escena, (n) =>
    n.tipo === 'conector' && (anclado(n.desde) || anclado(n.hasta)) ? null : n,
  );
}

/** Elimina los nodos `ids` (a cualquier profundidad) y los conectores que dejen huérfanos. */
export function eliminarNodos(escena: Escena, ids: readonly string[]): Escena {
  const sel = new Set(ids);
  const quitados = new Set<string>();
  const sin = mapearEscena(escena, (n) => {
    if (!sel.has(n.id)) return n;
    idsDe(n, quitados);
    return null;
  });
  return quitados.size === 0 ? escena : podarConectores(sin, quitados);
}

/** Traslada un nodo (dx, dy en el sistema de su padre). Los conectores solo
 *  mueven sus extremos libres; los anclados siguen a su nodo. */
export function desplazar(n: Nodo, dx: number, dy: number): Nodo {
  switch (n.tipo) {
    case 'linea':
      return { ...n, x1: n.x1 + dx, y1: n.y1 + dy, x2: n.x2 + dx, y2: n.y2 + dy };
    case 'trazo':
      return { ...n, puntos: n.puntos.map((v, i) => v + (i % 2 === 0 ? dx : dy)) };
    case 'conector': {
      const mv = (e: ExtremoConector): ExtremoConector =>
        'nodo' in e ? e : { x: e.x + dx, y: e.y + dy };
      return { ...n, desde: mv(n.desde), hasta: mv(n.hasta) };
    }
    default:
      return { ...n, x: n.x + dx, y: n.y + dy };
  }
}

/** Mueve los nodos `ids`. Si un grupo y uno de sus hijos están a la vez en `ids`, solo se mueve el grupo. */
export function moverNodos(escena: Escena, ids: readonly string[], dx: number, dy: number): Escena {
  if ((dx === 0 && dy === 0) || ids.length === 0) return escena;
  const sel = new Set(ids);
  return mapearEscena(escena, (n) => (sel.has(n.id) ? desplazar(n, dx, dy) : n));
}

const norm360 = (g: number): number => {
  const r = Math.round((((g % 360) + 360) % 360) * 1000) / 1000;
  return r === 360 ? 0 : r;
};

/** Movimiento rígido (traslación + giro) de un nodo con la matriz `m`: es lo que
 *  hacen girar y desagrupar. */
export function transformarRigido(n: Nodo, m: Matriz): Nodo {
  const p = (x: number, y: number): Punto => aplicar(m, { x, y });
  switch (n.tipo) {
    case 'linea': {
      const a = p(n.x1, n.y1);
      const b = p(n.x2, n.y2);
      return { ...n, x1: a.x, y1: a.y, x2: b.x, y2: b.y };
    }
    case 'trazo': {
      const puntos: number[] = [];
      for (let i = 0; i + 1 < n.puntos.length; i += 2) {
        const q = p(n.puntos[i]!, n.puntos[i + 1]!);
        puntos.push(q.x, q.y);
      }
      return { ...n, puntos };
    }
    case 'conector': {
      const mv = (e: ExtremoConector): ExtremoConector => {
        if ('nodo' in e) return e;
        const q = p(e.x, e.y);
        return { x: q.x, y: q.y };
      };
      return { ...n, desde: mv(n.desde), hasta: mv(n.hasta) };
    }
    default: {
      const c = p(n.x + n.w / 2, n.y + n.h / 2);
      const rot = norm360((n.rot ?? 0) + (Math.atan2(m[1], m[0]) * 180) / Math.PI);
      const { rot: _antes, ...resto } = n;
      const pos = { x: c.x - n.w / 2, y: c.y - n.h / 2 };
      return rot === 0 ? { ...resto, ...pos } : { ...resto, ...pos, rot };
    }
  }
}

/** Gira los nodos `ids` `grados` alrededor del centro de cada uno. */
export function girarNodos(escena: Escena, ids: readonly string[], grados: number): Escena {
  if (grados === 0 || ids.length === 0) return escena;
  const sel = new Set(ids);
  return mapearEscena(escena, (n) => {
    if (!sel.has(n.id) || n.tipo === 'conector') return n;
    const c = boundingBox(puntosDe(n).map(comoCaja));
    return transformarRigido(n, girarEn(grados, c.x + c.w / 2, c.y + c.h / 2));
  });
}

/** Cambia la caja (x/y/w/h) de un nodo de caja. Los grupos no se redimensionan
 *  (se mueven y giran): ponytail: escalar los hijos de un grupo, cuando haga falta. */
export function redimensionar(escena: Escena, id: string, caja: Rect): Escena {
  const w = Math.max(1, caja.w);
  const h = Math.max(1, caja.h);
  return actualizarNodo(escena, id, (n) =>
    esCaja(n) && n.tipo !== 'grupo' ? { ...n, x: caja.x, y: caja.y, w, h } : n,
  );
}

/* ---------- Orden Z, alinear, distribuir ---------- */

/** Reordena en Z los nodos de primer nivel `ids` dentro de su capa. */
export function ordenarZ(escena: Escena, ids: readonly string[], op: ZOrderOp): Escena {
  let cambio = false;
  const capas = escena.capas.map((c) => {
    const nodos = reorderZ(c.nodos, ids, op);
    if (nodos.length === c.nodos.length && nodos.every((n, i) => n === c.nodos[i])) return c;
    cambio = true;
    return { ...c, nodos };
  });
  return cambio ? { ...escena, capas } : escena;
}

type CajaConId = Rect & { id: string };

/** Cajas, en el lienzo, de los nodos de primer nivel `ids` (sin conectores). */
function cajasDe(escena: Escena, ids: readonly string[]): CajaConId[] {
  const out: CajaConId[] = [];
  for (const capa of escena.capas) {
    for (const n of capa.nodos) {
      if (!ids.includes(n.id) || n.tipo === 'conector') continue;
      const c = cajaMundo(escena, n.id);
      if (c) out.push({ id: n.id, ...c });
    }
  }
  return out;
}

function aplicarCajas(escena: Escena, antes: CajaConId[], despues: CajaConId[]): Escena {
  let e = escena;
  antes.forEach((a, i) => {
    const d = despues[i]!;
    e = moverNodos(e, [a.id], d.x - a.x, d.y - a.y);
  });
  return e;
}

export function alinearNodos(
  escena: Escena,
  ids: readonly string[],
  borde: AlignEdge,
  ref: 'seleccion' | 'lienzo',
): Escena {
  const antes = cajasDe(escena, ids);
  const lienzo = { x: 0, y: 0, w: escena.ancho, h: escena.alto };
  const despues = alignBoxes(
    antes,
    antes.map((c) => c.id),
    borde,
    ref === 'lienzo' ? lienzo : 'selection',
    lienzo,
  );
  return aplicarCajas(escena, antes, despues);
}

/** Reparte con huecos iguales (hacen falta 3 o más nodos). */
export function distribuirNodos(escena: Escena, ids: readonly string[], eje: DistAxis): Escena {
  const antes = cajasDe(escena, ids);
  const despues = distributeBoxes(
    antes,
    antes.map((c) => c.id),
    eje,
  );
  return aplicarCajas(escena, antes, despues);
}

/* ---------- Grupos ---------- */

/** Agrupa nodos de primer nivel de UNA misma capa (≥ 2, sin conectores) bajo un
 *  grupo con id `idGrupo`, colocado donde estaba su miembro más al frente. */
export function agrupar(escena: Escena, ids: readonly string[], idGrupo: string): Escena {
  const sel = new Set(ids);
  const capa = escena.capas.find(
    (c) => c.nodos.filter((n) => sel.has(n.id) && n.tipo !== 'conector').length >= 2,
  );
  if (!capa) return escena;
  const miembros = capa.nodos.filter((n) => sel.has(n.id) && n.tipo !== 'conector');
  const caja = boundingBox(miembros.flatMap((n) => puntosDe(n).map(comoCaja)));
  const grupo: NodoGrupo = {
    id: idGrupo,
    tipo: 'grupo',
    x: caja.x,
    y: caja.y,
    w: Math.max(1, caja.w),
    h: Math.max(1, caja.h),
    hijos: miembros.map((n) => desplazar(n, -caja.x, -caja.y)),
  };
  const dentro = new Set(miembros.map((n) => n.id));
  const alto = capa.nodos.reduce((k, n, i) => (dentro.has(n.id) ? i : k), -1);
  const nodos: Nodo[] = [];
  capa.nodos.forEach((n, i) => {
    if (!dentro.has(n.id)) nodos.push(n);
    if (i === alto) nodos.push(grupo);
  });
  return { ...escena, capas: escena.capas.map((c) => (c === capa ? { ...c, nodos } : c)) };
}

/** Deshace el grupo `idGrupo` (de primer nivel): los hijos pasan a la capa con su posición y giro ya compuestos. */
export function desagrupar(escena: Escena, idGrupo: string): Escena {
  const v = buscar(escena, idGrupo);
  if (!v || v.nodo.tipo !== 'grupo' || v.camino.length > 0) return escena;
  const g = v.nodo;
  const m = matrizDeCaja(g);
  const hijos = g.hijos.map((h) => transformarRigido(h, m));
  return {
    ...escena,
    capas: escena.capas.map((c) =>
      c === v.capa ? { ...c, nodos: c.nodos.flatMap((n) => (n === g ? hijos : [n])) } : c,
    ),
  };
}

/* ---------- Capas ---------- */

export function agregarCapa(escena: Escena, capa: Pick<Capa, 'id' | 'nombre'>): Escena {
  if (escena.capas.some((c) => c.id === capa.id)) return escena;
  return {
    ...escena,
    capas: [...escena.capas, { ...capa, visible: true, bloqueada: false, nodos: [] }],
  };
}

const actualizarCapa = (escena: Escena, id: string, fn: (c: Capa) => Capa): Escena => ({
  ...escena,
  capas: escena.capas.map((c) => (c.id === id ? fn(c) : c)),
});

export const renombrarCapa = (escena: Escena, id: string, nombre: string): Escena =>
  actualizarCapa(escena, id, (c) => ({ ...c, nombre }));

export const ajustarCapa = (
  escena: Escena,
  id: string,
  ajuste: { visible?: boolean; bloqueada?: boolean },
): Escena => actualizarCapa(escena, id, (c) => ({ ...c, ...ajuste }));

/** Sube o baja la capa en el orden (misma operación Z que los nodos). */
export const ordenarCapa = (escena: Escena, id: string, op: ZOrderOp): Escena => {
  const capas = reorderZ(escena.capas, [id], op);
  return capas.every((c, i) => c === escena.capas[i]) ? escena : { ...escena, capas };
};

/** Elimina una capa y sus nodos (y los conectores huérfanos). Siempre queda al menos una. */
export function eliminarCapa(escena: Escena, id: string): Escena {
  const capa = escena.capas.find((c) => c.id === id);
  if (!capa || escena.capas.length < 2) return escena;
  const quitados = new Set<string>();
  capa.nodos.forEach((n) => idsDe(n, quitados));
  return podarConectores({ ...escena, capas: escena.capas.filter((c) => c !== capa) }, quitados);
}

/** Lleva los nodos de primer nivel `ids` al frente de la capa `capaId`. */
export function enviarACapa(escena: Escena, ids: readonly string[], capaId: string): Escena {
  const sel = new Set(ids);
  if (!escena.capas.some((c) => c.id === capaId)) return escena;
  const llevados = escena.capas.flatMap((c) => c.nodos.filter((n) => sel.has(n.id)));
  if (llevados.length === 0) return escena;
  return {
    ...escena,
    capas: escena.capas.map((c) => {
      const resto = c.nodos.filter((n) => !sel.has(n.id));
      return c.id === capaId ? { ...c, nodos: [...resto, ...llevados] } : { ...c, nodos: resto };
    }),
  };
}
