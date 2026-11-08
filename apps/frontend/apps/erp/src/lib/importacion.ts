/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Importar con VISTA PREVIA y con marcha atrás.
 *
 * El servicio ya sabía las dos cosas —`?simular=1` hace la importación entera
 * contra la base y la deshace, y cada lote queda apuntado para revertirlo—, pero
 * la interfaz importaba a ciegas y el único «deshacer» era borrar a mano lo que
 * hubiera entrado. Aquí está lo que comparten la lista y su espejo de Fyne
 * (`apps/fyne/apps/erp/importacion.go`, mismos nombres): qué ruta se pide, cómo se
 * cuenta lo que pasaría y cómo se deshace un lote.
 *
 * Solo los CSV pasan por la vista previa y por el deshacer: una factura en XML no
 * entra como lote (ver `importarFacturas` en el servicio). El servicio decide por el
 * CONTENIDO: un XML o un Norma 43 con extensión `.csv` contestan a la simulación 422
 * `importacion_no_simulable` sin escribir nada, y entran sin vista previa.
 */
import { api, consulta, ErrorApi } from './api.js';

/** El resultado de un fichero; `id` es el lote, vacío en una simulación o en un XML. */
export interface ResultadoLote {
  id?: string;
  filas: number;
  creados: number;
  actualizados: number;
  ignoradas: string[] | null;
  fallos: { fila: number; error: string }[] | null;
}

/** Lo que dice el servicio al revertir un lote (`store.Reversion`). */
export interface Reversion {
  borrados: number;
  restaurados: number;
  /** Los que alguien tocó DESPUÉS de importar: no se pisan y se cuentan aparte. */
  intactos: number;
}

/** ¿Es una factura electrónica? Las facturas no tienen vista previa ni lote. */
export function esXML(nombre: string, tipo = ''): boolean {
  return /\.xml$/i.test(nombre) || tipo.includes('xml');
}

/** Un fichero elegido, ya leído; `error` si no se pudo leer (movido, sin permiso…). */
export interface FicheroLeido {
  nombre: string;
  xml: boolean;
  cuerpo: string;
  error?: unknown;
}

/**
 * Lee los ficheros elegidos, cada uno por su lado: uno que no se puede leer se queda
 * con su error y lo cuenta la importación como un fallo más, en vez de tumbarla
 * entera sin decir nada (`leerFicheros` de Fyne).
 */
export async function leerFicheros(
  ficheros: { name: string; type: string; text: () => Promise<string> }[],
): Promise<FicheroLeido[]> {
  return Promise.all(
    ficheros.map(async (f) => {
      const base = { nombre: f.name, xml: esXML(f.name, f.type) };
      try {
        return { ...base, cuerpo: await f.text() };
      } catch (error) {
        return { ...base, cuerpo: '', error };
      }
    }),
  );
}

/** El POST de un fichero ya leído, de verdad o simulado. */
export function subirFichero(
  modelo: string,
  f: FicheroLeido,
  simular: boolean,
): Promise<ResultadoLote> {
  return api<ResultadoLote>(rutaImportar(modelo, f.nombre, simular), {
    method: 'POST',
    headers: { 'content-type': f.xml ? 'application/xml' : 'text/csv' },
    body: f.cuerpo,
  });
}

/**
 * Los 4xx que hablan del FICHERO: ilegible, sin columnas o no simulable (422,
 * `csv_invalido`, `csv_sin_columnas`, `importacion_no_simulable`), demasiado grande o
 * petición mala (400: el servicio corta el cuerpo con `MaxBytesReader`), sin permiso
 * para importar ese modelo (403), demasiado grande antes de llegar al servicio (413) o
 * de un tipo que no se admite (415). Un 401 de sesión caducada, un 408 o un 429 de
 * límite no dicen nada de él. Espejo de `codigosDelFichero` en `apps/fyne/apps/erp/importacion.go`.
 */
const CODIGOS_DEL_FICHERO: ReadonlySet<number> = new Set([400, 403, 413, 415, 422]);

/**
 * ¿La simulación de un fichero dice algo de ESE fichero? Uno de `CODIGOS_DEL_FICHERO`
 * lo diría igual la importación de verdad, así que no se pregunta por él. Un corte de
 * red, un 5xx o un 4xx de otra cosa (sesión, límite) no dice nada del fichero: no se
 * importa a ciegas.
 */
export function falloDelFichero(err: unknown): boolean {
  return err instanceof ErrorApi && CODIGOS_DEL_FICHERO.has(err.estado);
}

/** La simulación no se pudo hacer: no se importa nada y se dice. */
export class SimulacionFallida extends Error {
  readonly causa: unknown;
  constructor(causa: unknown) {
    super(causa instanceof Error ? causa.message : String(causa));
    this.name = 'SimulacionFallida';
    this.causa = causa;
  }
}

/**
 * Simula los CSV legibles y suma lo que pasaría. Lanza `SimulacionFallida` si alguno
 * falla por algo que no es del fichero (`falloDelFichero`).
 */
export async function simularLeidos(
  modelo: string,
  leidos: FicheroLeido[],
  fila: (n: number, error: string) => string,
): Promise<Previa> {
  const csv = leidos.filter((f) => !f.xml && f.error === undefined);
  const simulados = await Promise.all(
    csv.map(async (f) => {
      try {
        return { nombre: f.nombre, res: await subirFichero(modelo, f, true) };
      } catch (err) {
        if (falloDelFichero(err)) return null;
        throw new SimulacionFallida(err);
      }
    }),
  );
  return sumarPrevia(
    simulados.filter((s) => s !== null),
    fila,
  );
}

/** Lo que pasó con todos los ficheros de una importación (`importacion` de Fyne). */
export interface Importacion {
  creados: number;
  actualizados: number;
  ignoradas: string[];
  fallos: string[];
  /** Los lotes que entraron: lo que deshace «Deshacer». */
  lotes: string[];
}

/**
 * Sube los ficheros uno tras otro y suma lo que pasó. Un fichero ilegible o que el
 * servicio rechaza es un fallo más; con varios, cada fallo lleva su nombre delante.
 */
export async function importarLeidos(
  modelo: string,
  leidos: FicheroLeido[],
  fila: (n: number, error: string) => string,
  motivo: (err: unknown) => string,
): Promise<Importacion> {
  const r: Importacion = { creados: 0, actualizados: 0, ignoradas: [], fallos: [], lotes: [] };
  const varios = leidos.length > 1;
  const fallo = (f: FicheroLeido, err: unknown): void => {
    r.fallos.push(varios ? `${f.nombre}: ${motivo(err)}` : motivo(err));
  };
  for (const f of leidos) {
    if (f.error !== undefined) {
      fallo(f, f.error);
      continue;
    }
    try {
      const res = await subirFichero(modelo, f, false);
      r.creados += res.creados;
      r.actualizados += res.actualizados;
      if (res.id && res.creados + res.actualizados > 0) r.lotes.push(res.id);
      for (const c of res.ignoradas ?? []) if (!r.ignoradas.includes(c)) r.ignoradas.push(c);
      for (const x of res.fallos ?? []) {
        const texto = fila(x.fila, x.error);
        r.fallos.push(varios ? `${f.nombre} · ${texto}` : texto);
      }
    } catch (err) {
      fallo(f, err);
    }
  }
  return r;
}

/** La ruta del POST: el nombre del fichero para reconocer el lote, y `simular=1` si toca. */
export function rutaImportar(modelo: string, fichero: string, simular: boolean): string {
  return `${modelo}/importar${consulta({ fichero, simular: simular ? 1 : undefined })}`;
}

/** Lo que pasaría con todos los CSV juntos. */
export interface Previa {
  creados: number;
  actualizados: number;
  /** Una línea por fila que no entraría (con el fichero delante si hay varios). */
  fallos: string[];
}

/**
 * Suma las simulaciones de varios ficheros. `fila` pone el texto de una fila
 * fallida («Fila 4: …»); con más de un fichero, cada una lleva su nombre delante.
 */
export function sumarPrevia(
  resultados: { nombre: string; res: ResultadoLote }[],
  fila: (n: number, error: string) => string,
): Previa {
  const p: Previa = { creados: 0, actualizados: 0, fallos: [] };
  for (const { nombre, res } of resultados) {
    p.creados += res.creados;
    p.actualizados += res.actualizados;
    for (const f of res.fallos ?? []) {
      const texto = fila(f.fila, f.error);
      p.fallos.push(resultados.length > 1 ? `${nombre} · ${texto}` : texto);
    }
  }
  return p;
}

/** ¿Hay algo que preguntar? Si nada entraría, la vista previa no aporta: se importa y se ven los fallos. */
export function merecePreguntar(p: Previa): boolean {
  return p.creados + p.actualizados > 0;
}

/**
 * ¿Se ofrece «Deshacer» en el aviso? Importar pide `administrar` en el módulo del
 * modelo, pero revertir lo pide en CONTABILIDAD (`exigirVerboContable` del servicio):
 * quien administra Ventas y no Contabilidad recibiría siempre «sin permiso».
 */
export function ofrecerDeshacer(
  lotes: string[],
  puedeAccion: (modulo: string, verbo: string) => boolean,
): boolean {
  return lotes.length > 0 && puedeAccion('contabilidad', 'administrar');
}

/**
 * Deshace los lotes de una importación, uno tras otro, y suma lo que pasó. Si uno
 * falla se sigue con los demás —lo que sí se pudo deshacer no se queda a medias—
 * y al final se lanza el primer error.
 */
export async function revertirImportaciones(ids: string[]): Promise<Reversion> {
  const total: Reversion = { borrados: 0, restaurados: 0, intactos: 0 };
  let primero: Error | null = null;
  for (const id of ids) {
    try {
      const r = await api<Reversion>(`importaciones/${encodeURIComponent(id)}/revertir`, {
        method: 'POST',
      });
      total.borrados += r.borrados;
      total.restaurados += r.restaurados;
      total.intactos += r.intactos;
    } catch (err) {
      primero ??= err instanceof Error ? err : new Error(String(err));
    }
  }
  if (primero !== null) throw primero;
  return total;
}
