/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La cola de subida, sin red: qué fichero va, en qué estado está cada uno y qué se
 * rechaza antes de mandar un solo byte. La red la pone `client/subir.ts`.
 *
 * DE UNO EN UNO. Dos originales de gigas a la vez se reparten el mismo ancho de
 * subida y ninguno termina antes; en serie el primero se puede ver procesando
 * mientras sube el segundo.
 */
import type { Visibilidad } from './tipos.js';

/** Tope por vídeo (CAST-ENCARGO §1.6): 8 GiB. */
export const MAX_BYTES = 8 * 1024 ** 3;

export type EstadoSubida = 'esperando' | 'subiendo' | 'procesando' | 'fallo';
export type MotivoFallo = 'tipo' | 'grande' | 'cuota' | 'red' | 'servicio' | 'cancelada';

export interface Elemento {
  clave: string;
  nombre: string;
  bytes: number;
  titulo: string;
  visibilidad: Visibilidad;
  estado: EstadoSubida;
  /** 0–1 mientras sube. */
  progreso: number;
  videoId?: string;
  motivo?: MotivoFallo;
  /** Calidades elegidas antes de subir; sin ellas, la recomendada. */
  calidades?: number[];
}

/** Lo que el navegador reconoce como vídeo, o un contenedor conocido sin tipo. */
export function esVideo(nombre: string, tipo: string): boolean {
  if (tipo.startsWith('video/')) return true;
  return tipo === '' && /\.(mp4|m4v|mov|mkv|webm|avi|mpe?g|ogv|3gp)$/i.test(nombre);
}

/** El título de partida: el nombre del fichero sin extensión ni guiones bajos. */
export function tituloDe(nombre: string): string {
  const sin = nombre.replace(/\.[a-z0-9]{2,4}$/i, '').replace(/[_]+/g, ' ');
  const limpio = sin.replace(/\s+/g, ' ').trim();
  return (limpio || nombre).slice(0, 200);
}

/** Un elemento nuevo; los que no pueden subir nacen ya con su motivo. */
export function nuevo(
  clave: string,
  f: { name: string; size: number; type: string },
  visibilidad: Visibilidad,
  calidades?: number[],
): Elemento {
  const base: Elemento = {
    clave,
    nombre: f.name,
    bytes: f.size,
    titulo: tituloDe(f.name),
    visibilidad,
    estado: 'esperando',
    progreso: 0,
    ...(calidades?.length ? { calidades } : {}),
  };
  if (!esVideo(f.name, f.type)) return { ...base, estado: 'fallo', motivo: 'tipo' };
  if (f.size > MAX_BYTES) return { ...base, estado: 'fallo', motivo: 'grande' };
  return base;
}

/** El siguiente que toca, si no hay ninguno subiendo. */
export function siguiente(cola: readonly Elemento[]): Elemento | undefined {
  if (cola.some((e) => e.estado === 'subiendo')) return undefined;
  return cola.find((e) => e.estado === 'esperando');
}

export function cambiar(
  cola: readonly Elemento[],
  clave: string,
  cambio: Partial<Elemento>,
): Elemento[] {
  return cola.map((e) => (e.clave === clave ? { ...e, ...cambio } : e));
}

/** Fracción total subida, por bytes, de lo que sigue en la cola. */
export function progresoTotal(cola: readonly Elemento[]): number | undefined {
  const vivos = cola.filter((e) => e.estado !== 'fallo');
  const total = vivos.reduce((s, e) => s + e.bytes, 0);
  if (total === 0) return undefined;
  const hecho = vivos.reduce(
    (s, e) => s + (e.estado === 'procesando' ? e.bytes : e.bytes * e.progreso),
    0,
  );
  return hecho / total;
}
