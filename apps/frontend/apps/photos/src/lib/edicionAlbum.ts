/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Lo que se decide al renombrar un álbum o al elegir su portada desde el visor.
 * La misma regla vive en Fyne (`tituloRenombrado` y `esPortada` en
 * apps/fyne/apps/photos/app_secciones.go).
 */

/**
 * El título que hay que mandar al renombrar, o `null` si no hay nada que guardar:
 * cancelado, vacío o igual al de ahora. Sin esto, aceptar el diálogo sin tocarlo
 * gastaba una petición y, en un álbum compartido para editar, le avisaba a la
 * dueña de un «cambio» que no existe.
 */
export function tituloRenombrado(antes: string, escrito: string | null): string | null {
  const nuevo = escrito?.trim() ?? '';
  return nuevo === '' || nuevo === antes ? null : nuevo;
}

/**
 * Si esa foto es ya la portada. El servicio devuelve `coverAssetId` resuelto —la
 * elegida o, si nadie eligió, la primera del álbum—, así que basta compararlo: lo
 * que se ve en la tarjeta de «Álbumes» es exactamente esto.
 */
export function esPortada(album: { coverAssetId: string | null } | null, foto: string): boolean {
  return album !== null && album.coverAssetId === foto;
}

/**
 * El mando de portada del visor de un álbum. NO es un conmutador: pulsarlo en la
 * que ya es la portada no quita nada, así que ahí no lleva `aria-pressed` —un
 * «botón de alternar, pulsado» que no se puede soltar—: dice que lo es y va
 * `apagado` (`aria-disabled`, no `disabled`: uno apagado de verdad se salta con
 * el tabulador y no diría cuál es la portada). Fyne hace lo mismo con `Apagado`
 * en `botonPortada`.
 */
export function mandoPortada(
  album: { coverAssetId: string | null } | null,
  foto: string,
): { clave: 'album.isCover' | 'album.setCover'; apagado: boolean } {
  const ya = esPortada(album, foto);
  return { clave: ya ? 'album.isCover' : 'album.setCover', apagado: ya };
}
