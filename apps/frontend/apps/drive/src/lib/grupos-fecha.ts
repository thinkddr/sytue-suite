/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los tramos de «Recientes»: «Hoy», «Ayer», «Esta semana» y «Antes», como los de
 * Google Drive o el Finder. Una lista de cien ficheros con la fecha en letra pequeña
 * obliga a leer fecha por fecha para saber dónde acaba lo de hoy; con los tramos se ve
 * de un vistazo.
 *
 * Las cuentas son en la hora LOCAL de quien mira: «hoy» es su día de calendario, no
 * las últimas 24 horas (algo cambiado a las 23:50 de anoche es de ayer aunque sean las
 * 00:10). La semana empieza el lunes, como en la norma ISO y en todos los idiomas de la
 * suite. Un lunes, por tanto, «Esta semana» no tiene nada (lo del domingo es «Ayer» y lo
 * de antes, «Antes»), y es lo esperable: el tramo no se inventa días que no le tocan.
 *
 * La pareja nativa es `apps/fyne/apps/drive/grupos.go`: mismas reglas, mismos casos.
 */

/** Tramo de una fecha, en el orden en que se pintan. */
export type GrupoFecha = 'today' | 'yesterday' | 'thisWeek' | 'earlier';

/** Medianoche local del día de `d`, desplazada `dias` días. */
function medianoche(d: Date, dias = 0): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + dias).getTime();
}

/**
 * Tramo de `iso` visto desde `ahora`. Sin fecha (o con una que no se entiende) va a
 * «Antes»: es lo único que no promete nada falso. Una fecha en el futuro (el reloj del
 * servidor por delante del de quien mira) cuenta como «Hoy».
 */
export function grupoDeFecha(iso: string | undefined, ahora: Date = new Date()): GrupoFecha {
  if (!iso) return 'earlier';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return 'earlier';
  const hoy = medianoche(ahora);
  if (t >= hoy) return 'today';
  if (t >= medianoche(ahora, -1)) return 'yesterday';
  // getDay(): 0 es domingo. Días desde el lunes de esta semana.
  const desdeLunes = (ahora.getDay() + 6) % 7;
  if (t >= medianoche(ahora, -desdeLunes)) return 'thisWeek';
  return 'earlier';
}

/** Un tramo con sus elementos, en el orden en que llegaron. */
export interface TramoDeFecha<T> {
  grupo: GrupoFecha;
  items: T[];
}

/**
 * Parte la lista en tramos SIN reordenarla: «Recientes» llega ya del más nuevo al más
 * viejo, y si por lo que sea dos elementos de un mismo tramo vienen separados, se juntan
 * en el suyo (cada tramo sale una sola vez) conservando el orden relativo. Los tramos
 * vacíos no salen: un rótulo sin nada debajo es ruido.
 */
export function agruparPorFecha<T extends { modifiedAt?: string | undefined }>(
  items: readonly T[],
  ahora: Date = new Date(),
): TramoDeFecha<T>[] {
  const orden: GrupoFecha[] = ['today', 'yesterday', 'thisWeek', 'earlier'];
  const porGrupo = new Map<GrupoFecha, T[]>();
  for (const item of items) {
    const g = grupoDeFecha(item.modifiedAt, ahora);
    const lista = porGrupo.get(g);
    if (lista) lista.push(item);
    else porGrupo.set(g, [item]);
  }
  return orden.flatMap((grupo) => {
    const lista = porGrupo.get(grupo);
    return lista ? [{ grupo, items: lista }] : [];
  });
}
