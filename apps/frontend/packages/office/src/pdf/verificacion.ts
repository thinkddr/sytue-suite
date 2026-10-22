/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Verificación de las firmas de un PDF recibido: los tipos del informe que da
 * `/api/v1/pdf/verify` y lo poco que la pantalla decide por su cuenta. TODO el
 * juicio (integridad, cambios posteriores, sello, confianza) es del motor; aquí
 * solo se traduce a claves de catálogo y a un tono, para que una firma que
 * falla nunca se pinte igual que una que no se sabe comprobar.
 */
import type { FirmaLeida } from './api';
import type { DocsKey } from '../i18n';

export type Veredicto = 'valida' | 'indeterminada' | 'modificada' | 'invalida';

/** Códigos estables de `domain.Motivo*`. */
export type Motivo =
  | 'rango'
  | 'formato'
  | 'integridad'
  | 'certificacion'
  | 'cambios'
  | 'sin_anclas'
  | 'no_confiable'
  | 'caducado'
  | 'sello_invalido'
  | 'sello_no_confiable'
  | 'limite'
  | 'serie_negativa'
  | 'sin_atributos';

export interface CertificadoLeido {
  sujeto: string;
  organizacion?: string;
  emisor: string;
  serie: string;
  desde: string;
  hasta: string;
  cualificado?: boolean;
  dispositivo_seguro?: boolean;
}

export interface SelloLeido {
  fecha?: string;
  autoridad?: string;
  integro: boolean;
  confianza: 'confiable' | 'no_confiable' | 'sin_anclas';
}

export interface CambiosPosteriores {
  bytes_firmados: number;
  bytes_totales: number;
  revisiones: number;
  alcance: 'ninguno' | 'permitidos' | 'otros';
  paginas?: number[];
  objetos?: number;
}

export interface FirmaVerificada extends FirmaLeida {
  tipo: string;
  veredicto: Veredicto;
  motivos?: string[];
  integra: boolean;
  firmante?: CertificadoLeido;
  cadena?: CertificadoLeido[];
  confianza: 'confiable' | 'no_confiable' | 'sin_anclas';
  hora_referencia?: string;
  sello?: SelloLeido;
  cambios: CambiosPosteriores;
  certificacion?: number;
}

export interface InformeVerificacion {
  firmas: FirmaVerificada[] | null;
  /** Falso: el despliegue no tiene en quién confiar y la identidad no se juzga. */
  anclas: boolean;
}

const MOTIVOS: Record<Motivo, DocsKey> = {
  rango: 'pdf.verify.reason.rango',
  formato: 'pdf.verify.reason.formato',
  integridad: 'pdf.verify.reason.integridad',
  certificacion: 'pdf.verify.reason.certificacion',
  cambios: 'pdf.verify.reason.cambios',
  sin_anclas: 'pdf.verify.reason.sin_anclas',
  no_confiable: 'pdf.verify.reason.no_confiable',
  caducado: 'pdf.verify.reason.caducado',
  sello_invalido: 'pdf.verify.reason.sello_invalido',
  sello_no_confiable: 'pdf.verify.reason.sello_no_confiable',
  limite: 'pdf.verify.reason.limite',
  serie_negativa: 'pdf.verify.reason.serie_negativa',
  sin_atributos: 'pdf.verify.reason.sin_atributos',
};

/** Clave del texto de un motivo; uno que el motor añada y aquí no se conozca
 *  cae en el genérico en vez de enseñar un código en bruto. */
export function claveDeMotivo(m: string): DocsKey {
  return (MOTIVOS as Record<string, DocsKey>)[m] ?? 'pdf.verify.reason.formato';
}

const VEREDICTOS: Record<Veredicto, DocsKey> = {
  valida: 'pdf.verify.verdict.valida',
  indeterminada: 'pdf.verify.verdict.indeterminada',
  modificada: 'pdf.verify.verdict.modificada',
  invalida: 'pdf.verify.verdict.invalida',
};

export function claveDeVeredicto(v: Veredicto): DocsKey {
  return VEREDICTOS[v] ?? VEREDICTOS.indeterminada;
}

/** Tono visual del veredicto: solo «válida» es verde; «indeterminada» no es un
 *  fallo ni un visto bueno, así que se queda en neutro. */
export function tonoDeVeredicto(v: Veredicto): 'ok' | 'neutro' | 'peligro' {
  if (v === 'valida') return 'ok';
  if (v === 'indeterminada') return 'neutro';
  return 'peligro';
}

/** El peor veredicto de una lista, que es el que manda para decir «este
 *  documento está bien». Sin firmas no hay veredicto. */
export function peorVeredicto(firmas: readonly FirmaVerificada[]): Veredicto | null {
  const orden: Veredicto[] = ['invalida', 'modificada', 'indeterminada', 'valida'];
  for (const v of orden) if (firmas.some((f) => f.veredicto === v)) return v;
  return null;
}

/** «3, 5-7, 9»: las páginas cambiadas sin una lista interminable. */
export function rangoDePaginas(paginas: readonly number[]): string {
  const orden = [...new Set(paginas)].sort((a, b) => a - b);
  const trozos: string[] = [];
  for (let i = 0; i < orden.length;) {
    let j = i;
    while (j + 1 < orden.length && orden[j + 1] === (orden[j] ?? 0) + 1) j++;
    trozos.push(j > i ? `${orden[i]}-${orden[j]}` : `${orden[i]}`);
    i = j + 1;
  }
  return trozos.join(', ');
}

/** Fecha ISO del motor a fecha legible; si no se puede leer, se deja tal cual. */
export function fechaCorta(iso: string | undefined, locale: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(d);
}
