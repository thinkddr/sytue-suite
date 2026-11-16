/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cuadro de nombres de la barra de fórmulas: enseña la celda activa (o el rango) y,
 * como en Excel y en Sheets, se escribe en él —«B12», «Hoja2!A1:C5» o un nombre
 * definido— y con Intro se salta allí. Escape vuelve a la hoja sin moverse. Ctrl+G y
 * F5 traen el foco aquí desde la rejilla (`atajosHoja.ts`). El ▾ despliega los nombres
 * definidos que la hoja activa ve; elegir uno es como escribirlo.
 *
 * Qué significa lo escrito lo decide `irA.ts`; aquí solo vive el campo. La app nativa
 * tiene el mismo cuadro en `apps/fyne/calcus/barra.go`.
 */
import type { JSX, RefObject } from 'preact';
import { useState } from 'preact/hooks';
import { Dropdown, MenuItem } from '@sytue/ui';
import { useT } from '../context';
import { EditorIcon } from '../icons';

export interface CuadroNombresProps {
  /** La celda activa o el rango, como «B2» o «AA1000:AB1200». */
  rotulo: string;
  /** Los nombres del desplegable (`nombresParaIrA`); vacío, no hay ▾. */
  nombres: readonly string[];
  /** Salta a lo escrito. Devuelve si se ha ido (o no había nada que hacer): entonces el
   *  foco vuelve a la rejilla; si no, el texto se queda marcado para corregirlo. */
  onIr: (texto: string) => boolean;
  /** Devuelve el foco a la rejilla sin moverse (Escape). */
  onVolver: () => void;
  campoRef: RefObject<HTMLInputElement>;
}

export function CuadroNombres({ campoRef, ...p }: CuadroNombresProps): JSX.Element {
  const t = useT();
  /** Lo que se está escribiendo; null cuando el cuadro solo enseña la selección. */
  const [borrador, setBorrador] = useState<string | null>(null);
  const etiqueta = t('sheet.nameBox');
  const ir = (texto: string, campo: HTMLInputElement | null): void => {
    if (p.onIr(texto)) {
      setBorrador(null);
      return;
    }
    // Lo que no lleva a ninguna parte se queda escrito y marcado, como en Excel, para
    // corregirlo sin volver a teclearlo entero.
    if (!campo) return; // desde el desplegable: el aviso basta, el cuadro sigue igual
    setBorrador(texto);
    campo.focus();
    campo.select();
  };
  return (
    // `w-24`, no el `w-16` de antes: «AA1000:AB1200» no cabía. Lo que aun así no quepa
    // se corta con puntos y se lee entero en el `title`.
    // SALVO EN PANTALLA ESTRECHA, donde sigue en `w-16`: a 320 px los 32 px de más
    // salían del campo de la fórmula, que se quedaba en 40 px —por debajo de los
    // 44 de un objetivo táctil (accesibilidad-office, calcus-light-320)—. Ahí el
    // rango largo se corta con puntos igual y el `title` lo sigue dando entero.
    <div class="border-border bg-bg-subtle text-text flex h-6 w-16 shrink-0 items-center rounded-sm border text-xs font-medium tabular-nums sm:w-24">
      <input
        ref={campoRef}
        type="text"
        class={`sytue-focus-ring h-full w-full min-w-0 truncate rounded-sm bg-transparent px-1 text-center focus:text-left`}
        aria-label={etiqueta}
        title={p.rotulo}
        spellcheck={false}
        autocomplete="off"
        value={borrador ?? p.rotulo}
        onFocus={(e) => {
          setBorrador(p.rotulo);
          e.currentTarget.select();
        }}
        onInput={(e) => setBorrador(e.currentTarget.value)}
        onBlur={() => setBorrador(null)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            ir(e.currentTarget.value, e.currentTarget);
          } else if (e.key === 'Escape') {
            e.preventDefault();
            setBorrador(null);
            p.onVolver();
          }
        }}
      />
      {p.nombres.length > 0 ? (
        <Dropdown
          label={t('sheet.nameBox.names')}
          align="start"
          class="flex h-full shrink-0"
          trigger={(tp) => (
            <button
              type="button"
              {...tp}
              class="sytue-focus-ring text-text-muted hover:text-text flex h-full w-4 shrink-0 items-center justify-center rounded-sm"
              aria-label={t('sheet.nameBox.names')}
              title={t('sheet.nameBox.names')}
            >
              <EditorIcon name="chevronDown" size="sm" />
            </button>
          )}
        >
          {(cerrar) =>
            p.nombres.map((n) => (
              <MenuItem
                key={n}
                size="sm"
                onClick={() => {
                  cerrar();
                  ir(n, null);
                }}
              >
                {n}
              </MenuItem>
            ))
          }
        </Dropdown>
      ) : null}
    </div>
  );
}
