/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Deshacer» en el aviso de «Movido a la papelera», del registro de la ficha y del
 * lote de la lista. El borrado del servicio es SUAVE, pero desde la lista sacarlo de
 * la papelera pedía ir a buscarlo allí; el aviso lo devuelve en un clic. Espejo en
 * `apps/fyne/apps/erp/papelera.go`, mismos nombres.
 */
import { DURACION_DESHACER, type useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { api, mensajeDeError } from './api.js';

type Toast = ReturnType<typeof useToast>;

/** Saca de la papelera lo que se acaba de tirar: el lote de `restaurar`, uno o muchos. */
export async function sacarDePapelera(modelo: string, ids: string[]): Promise<void> {
  await api<{ afectados: number }>(`${modelo}/lote`, {
    method: 'POST',
    body: JSON.stringify({ accion: 'restaurar', ids }),
  });
}

/** Lo que hace la barra de lote además de exportar y cambiar el estado. */
export type AccionDeLote = 'estado' | 'borrar' | 'restaurar';

/**
 * El lote de la lista y su aviso: «Movido a la papelera» con «Deshacer» SOLO al tirar;
 * al devolver, «Restaurado», sin «Deshacer»; al cambiar el estado, «Guardado». Los
 * errores los tira para que quien llama los pinte.
 *
 * Si sale bien, llama a `recargar` (y otra vez al deshacer): lo cacheado del modelo deja
 * de valer y la lista abierta se vuelve a pedir, como `recargarModelo` en Fyne. Antes la
 * lista se «refrescaba» poniendo el mismo filtro de estado que ya había, y Preact no
 * repinta un estado igual: lo devuelto o tirado seguía a la vista.
 *
 * Dentro de la papelera la barra ofrece DEVOLVER y no tirar: tirar lo ya tirado no
 * hacía nada (el lote filtra `deleted_at IS NULL`), pero el aviso salía con
 * «Deshacer», y ese «Deshacer» sacaba de la papelera cosas que nadie acababa de tirar.
 */
export async function aplicarLote(
  toast: Toast,
  t: T,
  modelo: string,
  accion: AccionDeLote,
  ids: string[],
  recargar: () => void,
  estado?: string,
): Promise<void> {
  await api<{ afectados: number }>(`${modelo}/lote`, {
    method: 'POST',
    body: JSON.stringify({ accion, ids, estado }),
  });
  if (accion === 'borrar') avisarPapelera(toast, t, modelo, ids, recargar);
  else
    toast.notify({
      tone: 'success',
      title: t(accion === 'restaurar' ? 'toast.restored' : 'toast.saved'),
    });
  recargar();
}

/**
 * Las filas sobre las que actúa la barra de lote: las marcadas QUE SE VEN, en su orden.
 * La selección sobrevive a cambiar de filtro (la lista no se remonta), y contar el
 * conjunto entero dejaba la barra en «2 seleccionados» sin nada marcado a la vista:
 * salir de la papelera con dos marcadas y pulsar «Mover a papelera» volvía a tirarlas,
 * y su «Deshacer» las sacaba de allí. Fyne cuenta igual (`barraLote` y `lote`).
 */
export function marcadasVisibles(
  seleccionados: ReadonlySet<string>,
  registros: readonly { id: string }[],
): string[] {
  return registros.filter((r) => seleccionados.has(r.id)).map((r) => r.id);
}

/**
 * Qué ofrece la barra de lote además de exportar: fuera de la papelera, cambiar el estado
 * (si el modelo tiene) y tirar; dentro, solo devolver. Es la rama de `barraLote` en Fyne.
 */
export function accionesDeLote(
  papelera: boolean,
  hayEstados: boolean,
): { cambiarEstado: boolean; accion: Exclude<AccionDeLote, 'estado'> } {
  return papelera
    ? { cambiarEstado: false, accion: 'restaurar' }
    : { cambiarEstado: hayEstados, accion: 'borrar' };
}

/**
 * El aviso de éxito de un envío a la papelera, con «Deshacer» durante diez segundos.
 * Al deshacer, lo dice —o dice por qué no— y llama a `alDeshacer` para repintar.
 */
export function avisarPapelera(
  toast: Toast,
  t: T,
  modelo: string,
  ids: string[],
  alDeshacer: () => void,
): void {
  toast.notify({
    tone: 'success',
    title: t('toast.deleted'),
    duration: DURACION_DESHACER,
    action: {
      label: t('action.undo'),
      onClick: () => {
        void sacarDePapelera(modelo, ids)
          .then(() => toast.notify({ tone: 'success', title: t('toast.undone') }))
          .catch((err: unknown) =>
            toast.notify({
              tone: 'danger',
              title: t('toast.undoFailed'),
              description: mensajeDeError(err, t),
            }),
          )
          .finally(alDeshacer);
      },
    },
  });
}
