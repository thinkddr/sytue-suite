/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que «Trae tus datos» decide sin pintar nada: qué botones ofrecer, cuándo llevan la
 * cuenta y qué trabajos van en cada línea del resumen. Aparte del componente para
 * probarlo sin DOM, y con los mismos nombres que en Fyne (`apps/fyne/ui/importar.go`).
 */
import type { ImportDomain, ImportJob, ImportOrigin, ImportSource } from '@sytue/api-clients';

/** Marcas comerciales: no se traducen. */
export const MARCA: Record<ImportSource, string> = { google: 'Google', m365: 'Microsoft 365' };

/** ¿Sigue trabajando? Mientras lo haga se vuelve a preguntar. */
export function enCurso(j: ImportJob): boolean {
  return j.status === 'queued' || j.status === 'running';
}

/**
 * ¿Es este trabajo de este origen? Por la cuenta, si se sabe de las dos partes; si no,
 * por la conexión, mirando TODAS las de la cuenta: un trabajo lanzado con una conexión
 * vieja de la misma cuenta es de este origen.
 */
export function esDeOrigen(j: ImportJob, o: ImportOrigin): boolean {
  if (j.source !== o.source) return false;
  if (j.accountKey && o.accountKey) return j.accountKey === o.accountKey;
  return o.instanceIds.includes(j.instanceId);
}

/**
 * Proveedores con más de un origen. Solo ahí el botón lleva la cuenta («Traerlo de
 * Google · acme.com»): con una sola cuenta, el rótulo de siempre ya es inequívoco.
 */
export function proveedoresRepetidos(origins: ImportOrigin[]): Set<ImportSource> {
  const vistos = new Set<ImportSource>();
  const repetidos = new Set<ImportSource>();
  for (const o of origins) {
    if (vistos.has(o.source)) repetidos.add(o.source);
    vistos.add(o.source);
  }
  return repetidos;
}

/** Cómo se nombra la cuenta de un origen: su dominio o, sin él, el nombre de la conexión. */
export function cuentaDeOrigen(o: ImportOrigin): string {
  return o.accountLabel || o.name;
}

export interface Reparto {
  /** En cola o copiando. */
  trabajando: ImportJob[];
  /** Terminados: una línea por contenido (y cuenta) con lo traído y cuándo. */
  hechos: ImportJob[];
  /** Fallidos: se pueden reintentar, y siguen donde se quedaron. */
  fallidos: ImportJob[];
  /** Orígenes que se pueden ofrecer: los que no tienen ya una importación en marcha. */
  libres: ImportOrigin[];
  /** ¿Hay trabajos de más de una cuenta? Entonces cada línea dice de cuál. */
  variasCuentas: boolean;
}

/**
 * Reparte lo que hay. Los trabajos llegan ya reducidos al vigente de cada cuenta y
 * contenido (`latestImports`), así que aquí no se suma nada dos veces.
 *
 * Un origen con una importación en marcha de este contenido NO se ofrece: un segundo
 * clic sobre la misma cuenta no copia nada nuevo (el servidor devuelve la que ya corre),
 * y un botón que parece hacer algo y no hace nada confunde más que no verlo.
 */
export function repartir(
  origins: ImportOrigin[],
  jobs: ImportJob[],
  domains: ImportDomain[],
): Reparto {
  const mios = jobs.filter((j) => domains.includes(j.domain));
  const trabajando = mios.filter(enCurso);
  const cuentas = new Set(mios.map((j) => j.accountKey || `instance:${j.instanceId}`));
  return {
    trabajando,
    hechos: mios.filter((j) => j.status === 'done'),
    fallidos: mios.filter((j) => j.status === 'error'),
    libres: origins.filter((o) => !trabajando.some((j) => esDeOrigen(j, o))),
    variasCuentas: cuentas.size > 1,
  };
}

/** Total de elementos traídos por una tanda de trabajos. */
export function contarTraidos(jobs: ImportJob[]): number {
  return jobs.reduce((n, j) => n + j.migrated, 0);
}

/**
 * Qué decir de un fallo: el motivo con arreglo, si lo tiene. Una conexión caducada o
 * sin delegación no se arregla «inténtalo dentro de un momento», y decirlo así deja a
 * la persona reintentando para siempre.
 */
export function claveDeFallo(
  code: string,
): 'import.unavailable' | 'import.needsAdmin' | 'import.someFailed' {
  if (code === 'connection_expired' || code === 'import_connection_unavailable')
    return 'import.unavailable';
  if (code === 'needs_admin') return 'import.needsAdmin';
  return 'import.someFailed';
}
