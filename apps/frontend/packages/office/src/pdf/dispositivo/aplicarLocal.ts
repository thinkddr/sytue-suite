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
 * Desde la cuarta entrega también proteger con contraseña (AES-256), quitarla, las
 * propiedades y sanear, CENSURAR (que borra de verdad: lo prueba el motor), editar el
 * texto y las imágenes, buscar y reemplazar, las capas opcionales, el índice, los
 * sellos y marcas de agua (las capas de `paginas`) e importar los datos de un XFDF.
 *
 * Lo que solo sabe hacer el servidor (reconocer texto y su capa) y firmar —que en la
 * bóveda va por su propio diálogo, `FirmarEnDispositivo.tsx`— contestan
 * `pdf_local_unavailable` en vez de intentarlo.
 */
import { ErrorPDF } from '../api';
import type { EdicionLocal, MotorPdf } from './motorPdf';

type Motor = 'anotar' | 'formulario' | 'paginas' | 'herramienta' | 'proteger' | 'editar';

/** op del estudio → operación del motor, su `op` dentro y si deja siempre copia. */
const LOCALES: Record<string, { motor: Motor; op?: string; datos?: string; copia?: boolean }> = {
  anotar: { motor: 'anotar', op: 'poner', datos: 'anotaciones' },
  'borrar-anotacion': { motor: 'anotar', op: 'borrar' },
  'aplanar-anotaciones': { motor: 'anotar', op: 'aplanar', copia: true },
  responder: { motor: 'anotar', op: 'responder' },
  marcar: { motor: 'anotar', op: 'marcar' },
  rellenar: { motor: 'formulario', op: 'rellenar', datos: 'valores' },
  'crear-campos': { motor: 'formulario', op: 'crear', datos: 'campos', copia: true },
  'importar-datos': { motor: 'formulario', op: 'importar-datos', copia: true },
  'aplanar-formulario': { motor: 'formulario', op: 'aplanar', copia: true },
  // Organizar y también las capas (sellos, marcas de agua, numeración): las dos van por
  // `/pdf/pages` en el servicio, y las dos dejan copia.
  paginas: { motor: 'paginas', copia: true },
  comprimir: { motor: 'herramienta', op: 'comprimir', copia: true },
  recortar: { motor: 'herramienta', op: 'recortar', copia: true },
  archivo: { motor: 'herramienta', op: 'archivo', copia: true },
  proteger: { motor: 'proteger', op: 'proteger', copia: true },
  desproteger: { motor: 'proteger', op: 'desproteger', copia: true },
  sanear: { motor: 'proteger', op: 'sanear', copia: true },
  metadatos: { motor: 'proteger', op: 'metadatos', datos: 'metadatos' },
  // Censurar destruye contenido: SIEMPRE copia, pida lo que pida quien llama.
  redactar: { motor: 'proteger', op: 'redactar', datos: 'areas', copia: true },
  'editar-texto': { motor: 'editar', op: 'texto', datos: 'cambios', copia: true },
  'reemplazar-texto': { motor: 'editar', op: 'buscar-reemplazar', copia: true },
  'editar-imagen': { motor: 'editar', op: 'imagen', copia: true },
  capas: { motor: 'editar', op: 'capas' },
  indice: { motor: 'editar', op: 'indice', datos: 'marcadores', copia: true },
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

/**
 * La imagen de sustitución llega como dataURL (como al BFF). Solo `data:image/…;base64`:
 * lo demás se rechaza con el mismo código que el BFF (`bad_image`). Sin imagen, el motor
 * QUITA la que había, que es el mismo botón visto desde el otro lado.
 */
function imagenDe(valor: unknown): Uint8Array<ArrayBuffer> | undefined {
  if (typeof valor !== 'string' || valor === '') return undefined;
  const coma = valor.indexOf(',');
  const cabecera = coma > 0 ? valor.slice(0, coma) : '';
  if (!cabecera.startsWith('data:image/') || !cabecera.includes(';base64'))
    throw new ErrorPDF('bad_image', 'bad_image', 400);
  try {
    return Uint8Array.from(atob(valor.slice(coma + 1)), (c) => c.charCodeAt(0));
  } catch {
    throw new ErrorPDF('bad_image', 'bad_image', 400);
  }
}

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
    case 'formulario': {
      // Importar lleva el XFDF como texto en el cuerpo, como al BFF, y el motor lo
      // quiere como fichero: aquí, sus bytes.
      let xfdf: Uint8Array | undefined;
      if (op === 'importar-datos') {
        const texto = typeof cuerpo.xfdf === 'string' ? cuerpo.xfdf.trim() : '';
        if (texto === '') throw new ErrorPDF('missing_data', 'missing_data', 400);
        xfdf = new TextEncoder().encode(texto);
      }
      edicion = await motor.formulario(doc, campos, options.nombre, xfdf);
      break;
    }
    case 'herramienta':
      edicion = await motor.herramienta(doc, campos);
      break;
    case 'proteger':
      edicion = await motor.proteger(doc, campos);
      break;
    case 'editar': {
      if (op === 'reemplazar-texto') {
        campos.aplicar = '1';
        campos.busqueda = JSON.stringify(cuerpo.busqueda ?? {});
        if (Array.isArray(cuerpo.elegidas)) campos.elegidas = JSON.stringify(cuerpo.elegidas);
      }
      const imagen = op === 'editar-imagen' ? imagenDe(cuerpo.imagen) : undefined;
      try {
        edicion = await motor.editar(doc, campos, imagen);
      } finally {
        imagen?.fill(0);
      }
      break;
    }
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
