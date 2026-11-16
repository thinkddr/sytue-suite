/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
/**
 * Los cambios sin guardar de la ficha.
 *
 * Cerrar la ficha —el botón, «atrás» del móvil, abrir otro registro, cambiar de
 * modelo o de vista— tiraba lo escrito sin preguntar: media factura rellenada se
 * perdía por un clic en la barra lateral. Aquí está lo que lo evita, en tres
 * piezas separadas para poder probarlas sin montar el ERP entero:
 *
 * - `hayCambios` decide si lo que hay en el formulario difiere de con lo que nació.
 * - `useAvisoAlSalir` pide al navegador su aviso nativo al recargar o cerrar la
 *   pestaña, SOLO mientras haya cambios: un `beforeunload` permanente hace que el
 *   navegador pregunte también cuando no hay nada que perder.
 * - `useGuardaDeFicha` es la puerta por la que pasa cualquier salida de la ficha.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { useConfirm } from '@sytue/ui';

/**
 * Si el formulario ha cambiado. Una clave que falta cuenta como vacía: un campo que
 * el registro no traía y que se escribe y se borra no es un cambio.
 */
export function hayCambios(
  iniciales: Readonly<Record<string, string>>,
  actuales: Readonly<Record<string, string>>,
): boolean {
  const claves = new Set([...Object.keys(iniciales), ...Object.keys(actuales)]);
  for (const clave of claves) {
    if ((iniciales[clave] ?? '') !== (actuales[clave] ?? '')) return true;
  }
  return false;
}

/** El aviso del navegador al salir de la página, mientras `activo`. */
export function useAvisoAlSalir(activo: boolean): void {
  useEffect(() => {
    if (!activo || typeof window === 'undefined') return;
    const alSalir = (ev: BeforeUnloadEvent): void => {
      ev.preventDefault();
      // Los navegadores que aún no atienden `preventDefault` piden un valor de
      // retorno; el texto lo ignoran todos y enseñan el suyo.
      ev.returnValue = '';
    };
    window.addEventListener('beforeunload', alSalir);
    return () => window.removeEventListener('beforeunload', alSalir);
  }, [activo]);
}

export interface TextosDescarte {
  titulo: string;
  mensaje: string;
  descartar: string;
  seguir: string;
}

/**
 * La puerta de salida de la ficha. La ficha avisa con `marcar(sucia)` de si tiene
 * cambios; quien la cierra pasa la salida por `dejar(seguir)`, que la ejecuta en el
 * acto si no hay nada que perder y, si lo hay, pregunta antes.
 *
 * Es un ref y no un estado porque la respuesta se necesita al pulsar, no al pintar:
 * con un estado, cada tecla de la ficha repintaría el ERP entero.
 *
 * `descartes` cuenta los «Descartar» y va en el `key` de la ficha. Hay salidas que no
 * cambian de ficha —«Nuevo» con un alta en blanco abierta, pulsar en la lista la fila
 * que ya está abierta—: sin volver a montarla, lo escrito seguía en el formulario
 * mientras la guarda ya lo daba por descartado, y el siguiente cierre lo tiraba sin
 * preguntar.
 */
export function useGuardaDeFicha(textos: TextosDescarte): {
  marcar: (sucia: boolean) => void;
  dejar: (seguir: () => void) => void;
  dialogo: JSX.Element;
  descartes: number;
} {
  const suciaRef = useRef(false);
  const [descartes, setDescartes] = useState(0);
  const { confirmar, dialogo } = useConfirm();
  const marcar = useCallback((s: boolean) => {
    suciaRef.current = s;
  }, []);
  const { titulo, mensaje, descartar, seguir: seguirEditando } = textos;
  const dejar = useCallback(
    (seguir: () => void) => {
      if (!suciaRef.current) {
        seguir();
        return;
      }
      void confirmar({
        titulo,
        mensaje,
        confirmLabel: descartar,
        cancelLabel: seguirEditando,
        tone: 'danger',
      }).then((ok) => {
        if (!ok) return;
        // Se ha descartado: la salida siguiente (la del `seguir`) ya no pregunta,
        // aunque la ficha tarde un render en desmontarse y avisar.
        suciaRef.current = false;
        setDescartes((n) => n + 1);
        seguir();
      });
    },
    [confirmar, titulo, mensaje, descartar, seguirEditando],
  );
  return { marcar, dejar, dialogo, descartes };
}
