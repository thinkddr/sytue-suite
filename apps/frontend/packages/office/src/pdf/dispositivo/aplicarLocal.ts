/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «APLICAR» EN EL DISPOSITIVO: el `/api/pdf-aplicar` del estudio para un PDF de la
 * bóveda (ADR 0086, tercera entrega).
 *
 * El estudio pide cada cambio con el mismo cuerpo de siempre (`{op, datos, campos,
 * rutas, sufijo}`, ver `pages/api/pdf-aplicar.ts`). Con el PDF en el Drive del
 * servidor, el BFF lo baja, llama al servicio de Docs y sube el resultado. Con el PDF
 * en la bóveda, eso no puede pasar: el servidor no lo ve. Esta pieza hace lo mismo
 * aquí, con el MISMO motor en WebAssembly (`motorPdf.ts`) y la MISMA tabla —qué
 * operación del motor, en qué campo van los datos, cuáles dejan siempre copia—, y
 * devuelve los bytes para que el estudio los guarde cifrados (`editarEnBoveda.ts`).
 *
 * Lo que solo sabe hacer el servidor (reconocer texto), lo que aún no se ha traído al
 * dispositivo (proteger, censurar, editar texto e imágenes, sellos y marcas de agua,
 * índice, importar datos) y firmar —que en la bóveda va por su propio diálogo,
 * `FirmarEnDispositivo.tsx`— contestan `pdf_local_unavailable` en vez de intentarlo.
 */
import { ErrorPDF } from '../api';
import type { EdicionLocal, MotorPdf } from './motorPdf';

type Motor = 'anotar' | 'formulario' | 'paginas' | 'herramienta';

/** op del estudio → operación del motor, su `op` dentro y si deja siempre copia. */
const LOCALES: Record<string, { motor: Motor; op?: string; datos?: string; copia?: boolean }> = {
  anotar: { motor: 'anotar', op: 'poner', datos: 'anotaciones' },
  'borrar-anotacion': { motor: 'anotar', op: 'borrar' },
  'aplanar-anotaciones': { motor: 'anotar', op: 'aplanar', copia: true },
  responder: { motor: 'anotar', op: 'responder' },
  marcar: { motor: 'anotar', op: 'marcar' },
  rellenar: { motor: 'formulario', op: 'rellenar', datos: 'valores' },
  'crear-campos': { motor: 'formulario', op: 'crear', datos: 'campos', copia: true },
  'aplanar-formulario': { motor: 'formulario', op: 'aplanar', copia: true },
  paginas: { motor: 'paginas', copia: true },
  comprimir: { motor: 'herramienta', op: 'comprimir', copia: true },
  recortar: { motor: 'herramienta', op: 'recortar', copia: true },
  archivo: { motor: 'herramienta', op: 'archivo', copia: true },
};

/** ¿Se hace esta operación del estudio en el dispositivo? */
export const seHaceEnElDispositivo = (op: string) => Object.hasOwn(LOCALES, op);

/** Cuántos PDF se combinan con el abierto, como el BFF (`MAX_COMBINAR`). */
export const MAX_COMBINAR_LOCAL = 20;

export interface AplicadoLocal extends EdicionLocal {
  /** El resultado va a un fichero nuevo «nombre (sufijo).pdf», no a una versión. */
  copia: boolean;
  /** La palabra del nombre de la copia, ya acotada. */
  sufijo: string;
}

/**
 * La palabra que el estudio manda para el nombre de la copia, acotada como en el BFF
 * (`limpiarSufijo`): letras, números, espacios y guiones, y como mucho 24.
 */
export function limpiarSufijo(v: unknown): string {
  if (typeof v !== 'string') return '';
  return v
    .normalize('NFC')
    .replace(/[^\p{L}\p{N} -]/gu, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 24)
    .trim();
}

const noAqui = () => new ErrorPDF('pdf_local_unavailable', 'pdf_local_unavailable');

/** Los `campos` del cuerpo como texto, que es como los lee el motor (y el BFF). */
function camposDe(cuerpo: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  const campos = cuerpo.campos;
  if (campos && typeof campos === 'object')
    for (const [k, v] of Object.entries(campos as Record<string, unknown>))
      if (typeof v === 'string' || typeof v === 'number') out[k] = String(v);
  return out;
}

/**
 * Aplica `cuerpo` al documento `doc` del motor. `abrirOtro` descifra y abre en el motor
 * un PDF de la bóveda por su ruta (los otros de una combinación); se cierran al acabar.
 */
export async function aplicarEnDispositivo(options: {
  motor: MotorPdf;
  doc: string;
  nombre: string;
  cuerpo: Record<string, unknown>;
  autor: string;
  abrirOtro: (ruta: string) => Promise<string>;
}): Promise<AplicadoLocal> {
  const { motor, doc, cuerpo } = options;
  const op = typeof cuerpo.op === 'string' ? cuerpo.op : '';
  const conf = LOCALES[op];
  if (!conf) throw noAqui();
  const campos = camposDe(cuerpo);
  if (conf.op) campos.op = conf.op;
  if (conf.datos && cuerpo.datos !== undefined) campos[conf.datos] = JSON.stringify(cuerpo.datos);

  let edicion: EdicionLocal;
  switch (conf.motor) {
    case 'anotar':
      edicion = await motor.anotar(doc, campos, options.autor);
      break;
    case 'formulario':
      edicion = await motor.formulario(doc, campos, options.nombre);
      break;
    case 'herramienta':
      edicion = await motor.herramienta(doc, campos);
      break;
    case 'paginas': {
      // Los otros de una combinación, en el orden escrito y detrás del abierto.
      const rutas = Array.isArray(cuerpo.rutas)
        ? cuerpo.rutas.filter((r): r is string => typeof r === 'string' && r.trim() !== '')
        : [];
      if (rutas.length > MAX_COMBINAR_LOCAL) throw new ErrorPDF('too_many_files', 'too_many_files');
      const otros: string[] = [];
      try {
        for (const ruta of rutas) otros.push(await options.abrirOtro(ruta.trim()));
        edicion = await motor.paginas([doc, ...otros], campos);
      } finally {
        for (const otro of otros) await motor.cerrar(otro);
      }
      break;
    }
  }
  const copia = conf.copia === true || cuerpo.destino === 'copia';
  return { ...edicion, copia, sufijo: limpiarSufijo(cuerpo.sufijo) || op };
}
