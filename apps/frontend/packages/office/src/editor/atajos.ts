/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Atajos de suite ofimática que el navegador se quedaba (§6.8 solo cubría los del
 * folio). Ctrl/Cmd+S abría el «Guardar página como…» del navegador —que descarga el
 * HTML de la aplicación— en un editor que ya guarda solo: aquí adelanta el autosave y
 * el chip lo cuenta. Ctrl/Cmd+F abría el buscador del navegador, que no ve más que el
 * folio visible y no sabe reemplazar, teniendo el panel Buscar del editor a un lado.
 * Ctrl/⌘ +, − y 0 ampliaban la página entera en vez del documento (atajosZoom.ts).
 * Van en window y no en el folio para que respondan también con el foco en la cinta
 * o en un panel.
 *
 * Se dejan pasar los del navegador que no compiten (Ctrl+P imprime, y aquí no hay
 * nada mejor todavía) y los del folio (B/I/U, deshacer nativo).
 *
 * Es un hook aparte de EditorApp para que el ENGANCHE se pueda probar sin montar el
 * editor entero (atajos.test.tsx): probar solo `zoomDeAtajo` dejaba sin vigilar el
 * `preventDefault`, y sin él Ctrl+= volvía a ampliar la página del navegador.
 */
import type { Dispatch, StateUpdater } from 'preact/hooks';
import { useEffect } from 'preact/hooks';
import { zoomDeAtajo } from './atajosZoom';
import type { EditorUi } from './types';

export function useAtajosDelEditor(
  saveNow: () => void,
  setUiState: Dispatch<StateUpdater<EditorUi>>,
): void {
  useEffect(() => {
    const onKey = (event: KeyboardEvent): void => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
      // Funcional para leer el zoom de AHORA sin volver a enganchar el oyente.
      if (zoomDeAtajo(event, 100) !== null) {
        event.preventDefault();
        setUiState((prev) => {
          const zoom = zoomDeAtajo(event, prev.zoom) ?? prev.zoom;
          return zoom === prev.zoom ? prev : { ...prev, zoom };
        });
        return;
      }
      const key = event.key.toLowerCase();
      if (key === 's') {
        event.preventDefault();
        saveNow();
      } else if (key === 'f' && !event.shiftKey) {
        event.preventDefault();
        setUiState((prev) => ({ ...prev, panel: 'buscar' }));
        // Con el panel recién abierto lo enfoca su propio autofocus; con el panel
        // YA abierto (y el cursor de vuelta en el folio) hay que traer el foco a
        // mano, o el segundo Ctrl+F no haría nada visible. El tiempo 0 espera al
        // render, mismo truco que el panel Esquema.
        window.setTimeout(() => {
          const field = document.getElementById('editor-find-input');
          if (field instanceof HTMLInputElement) {
            field.focus();
            field.select(); // teclear sustituye la búsqueda anterior
          }
        }, 0);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [saveNow, setUiState]);
}
