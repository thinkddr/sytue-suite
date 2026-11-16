/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * En qué orden se ve una carpeta de Drive, y qué se recuerda entre visitas.
 *
 * Las CARPETAS van delante en todos los órdenes, como en cualquier gestor de ficheros:
 * ordenar por tamaño no debería esconder las carpetas entre los ficheros. El desempate es
 * siempre por nombre, para que dos ficheros con la misma fecha no bailen entre recargas.
 *
 * Las carpetas que se paginan («Mi unidad», unidades de equipo) las ordena el SERVICIO
 * (`?orden=&sentido=`, keyset por columna): ordenar en el cliente solo ordenaría lo
 * cargado y enseñaría un «más grande» falso, y rompería la continuidad al encadenar
 * páginas. `ordenarRecursos` queda para los listados que llegan enteros (la carpeta de
 * algo compartido contigo).
 */
import { intlTag, type Locale } from '@sytue/i18n';

export const CAMPOS = ['name', 'modified', 'size'] as const;
export type CampoOrden = (typeof CAMPOS)[number];
export type Sentido = 'asc' | 'desc';
export interface Orden {
  campo: CampoOrden;
  sentido: Sentido;
}

/** El de siempre, y el que el servicio aplica si no se le pide otro. */
export const ORDEN_DEL_SERVICIO: Orden = { campo: 'name', sentido: 'asc' };

const CLAVE_ORDEN = 'sytue.drive.orden';
const CLAVE_VISTA = 'sytue.drive.vista';

interface Ordenable {
  name: string;
  isDirectory: boolean;
  size?: number | undefined;
  modifiedAt?: string | undefined;
}

export function esOrdenDelServicio(o: Orden): boolean {
  return o.campo === ORDEN_DEL_SERVICIO.campo && o.sentido === ORDEN_DEL_SERVICIO.sentido;
}

/** Cómo llama el servicio a cada campo (`?orden=` de los listados de carpeta). */
const CAMPO_DEL_SERVICIO: Record<CampoOrden, string> = {
  name: 'nombre',
  modified: 'modificado',
  size: 'tamano',
};

/**
 * Los parámetros que piden `orden` al servicio. El orden de siempre no lleva ninguno:
 * así la URL de la primera página es la de antes y siguen valiendo la caché sin conexión
 * y los cursores ya emitidos.
 */
export function parametrosDeOrden(o: Orden): { orden: string; sentido: Sentido } | null {
  if (esOrdenDelServicio(o)) return null;
  return { orden: CAMPO_DEL_SERVICIO[o.campo], sentido: o.sentido };
}

/**
 * Lo que el BFF reenvía al servicio de una petición del navegador: solo valores
 * conocidos. Lo demás se descarta, que no es un error de quien mira sino basura en la URL.
 */
export function ordenDeLaPeticion(params: URLSearchParams): { orden?: string; sentido?: Sentido } {
  const orden = params.get('orden') ?? '';
  const sentido = params.get('sentido') ?? '';
  return {
    ...(Object.values(CAMPO_DEL_SERVICIO).includes(orden) ? { orden } : {}),
    ...(sentido === 'asc' || sentido === 'desc' ? { sentido } : {}),
  };
}

/**
 * La pulsación sobre una columna: la misma invierte el sentido; otra empieza por lo que se
 * busca casi siempre (nombre de la A a la Z; fecha y tamaño, lo más reciente y lo más
 * grande primero).
 */
export function siguienteOrden(actual: Orden, campo: CampoOrden): Orden {
  if (actual.campo === campo) return { campo, sentido: actual.sentido === 'asc' ? 'desc' : 'asc' };
  return { campo, sentido: campo === 'name' ? 'asc' : 'desc' };
}

/**
 * Ordena una copia: carpetas delante, luego el campo y, a igualdad, el nombre. Para
 * listados que llegan ENTEROS; los paginados los ordena el servicio.
 */
export function ordenarRecursos<T extends Ordenable>(
  items: readonly T[],
  orden: Orden,
  locale: Locale,
): T[] {
  const col = new Intl.Collator(intlTag(locale), { numeric: true, sensitivity: 'base' });
  const porNombre = (a: T, b: T) => col.compare(a.name, b.name);
  const signo = orden.sentido === 'asc' ? 1 : -1;
  const campo: (a: T, b: T) => number =
    orden.campo === 'name'
      ? porNombre
      : orden.campo === 'size'
        ? (a, b) => (a.size ?? 0) - (b.size ?? 0)
        : (a, b) => fecha(a.modifiedAt) - fecha(b.modifiedAt);
  return [...items].sort(
    (a, b) =>
      Number(b.isDirectory) - Number(a.isDirectory) || signo * campo(a, b) || porNombre(a, b),
  );
}

function fecha(iso: string | undefined): number {
  const n = iso ? Date.parse(iso) : NaN;
  return Number.isNaN(n) ? 0 : n;
}

// Preferencias por navegador. `localStorage` puede no existir o lanzar (modo privado,
// almacenamiento bloqueado): entonces se usa lo de siempre y no se recuerda nada.
export function leerOrden(): Orden {
  try {
    const raw = localStorage.getItem(CLAVE_ORDEN);
    const [campo, sentido] = (raw ?? '').split(':');
    if (
      (CAMPOS as readonly string[]).includes(campo ?? '') &&
      (sentido === 'asc' || sentido === 'desc')
    ) {
      return { campo: campo as CampoOrden, sentido };
    }
  } catch {
    /* sin almacenamiento: orden por defecto */
  }
  return ORDEN_DEL_SERVICIO;
}

export function guardarOrden(o: Orden): void {
  try {
    localStorage.setItem(CLAVE_ORDEN, `${o.campo}:${o.sentido}`);
  } catch {
    /* sin almacenamiento: no se recuerda */
  }
}

export type Vista = 'list' | 'grid';

export function leerVista(): Vista {
  try {
    return localStorage.getItem(CLAVE_VISTA) === 'grid' ? 'grid' : 'list';
  } catch {
    return 'list';
  }
}

export function guardarVista(v: Vista): void {
  try {
    localStorage.setItem(CLAVE_VISTA, v);
  } catch {
    /* sin almacenamiento: no se recuerda */
  }
}
