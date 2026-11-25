/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SelectorDeSimbolos — el panel de «Símbolos»: recientes, flechas, matemáticas, moneda,
 * tipografía, marcas y emoji, con una búsqueda arriba.
 *
 * TODO CON EL TECLADO. El foco empieza en la búsqueda; ↓ baja a la rejilla, que es UNA
 * parada de tabulación (foco itinerante) y se recorre con las flechas —← → de uno en
 * uno, ↑ ↓ de fila en fila—, Inicio y Fin; Intro o Espacio insertan. Intro en la
 * búsqueda inserta el primero que casa. Escape lo cierra el `Popover` que lo contiene,
 * que devuelve el foco a su botón.
 *
 * Cada botón lleva el propio carácter: el lector de pantalla lo nombra en su idioma
 * («flecha hacia la derecha»), cosa que una etiqueta escrita aquí no haría en 26.
 */
import type { JSX } from 'preact';
import { Fragment } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Input, cx } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import {
  CATEGORIAS,
  FRECUENTES,
  apuntarReciente,
  buscarSimbolos,
  leerRecientes,
  type IdCategoria,
} from '../client/simbolos.js';

/** Columnas de la rejilla: lo que mueven ↑ y ↓. */
export const COLUMNAS = 8;

interface Seccion {
  id: string;
  titulo: string;
  simbolos: readonly string[];
}

export function SelectorDeSimbolos({
  t,
  onElegir,
  enfoque,
}: {
  t: T;
  /** El símbolo elegido; ya se ha apuntado en recientes. */
  onElegir: (s: string) => void;
  /** Abrir mirando solo los emoji (el botón «Emoji» de la barra). */
  enfoque?: IdCategoria;
}): JSX.Element {
  const [q, setQ] = useState('');
  const [recientes] = useState(leerRecientes);
  const [activo, setActivo] = useState(0);
  const rejilla = useRef<HTMLDivElement>(null);

  const secciones: Seccion[] = q.trim()
    ? [
        {
          id: 'resultados',
          titulo: t('editor.symbols'),
          simbolos: buscarSimbolos(q).map((x) => x.s),
        },
      ]
    : [
        ...(enfoque
          ? []
          : [
              {
                id: 'recientes',
                titulo: t('editor.symbolsRecent'),
                simbolos: recientes.length ? recientes : FRECUENTES,
              },
            ]),
        ...CATEGORIAS.filter((c) => !enfoque || c.id === enfoque).map((c) => ({
          id: c.id,
          titulo: t(c.clave),
          simbolos: c.simbolos.map((x) => x.s),
        })),
      ];
  const todos = secciones.flatMap((x) => x.simbolos);
  const actual = Math.min(activo, Math.max(todos.length - 1, 0));

  const elegir = (simbolo: string) => {
    apuntarReciente(simbolo);
    onElegir(simbolo);
  };

  const mover = (i: number) => {
    const n = Math.max(0, Math.min(todos.length - 1, i));
    setActivo(n);
    rejilla.current?.querySelector<HTMLElement>(`[data-i="${n}"]`)?.focus();
  };

  const alTeclearRejilla = (e: JSX.TargetedKeyboardEvent<HTMLButtonElement>) => {
    const pasos: Record<string, number> = {
      ArrowRight: 1,
      ArrowLeft: -1,
      ArrowDown: COLUMNAS,
      ArrowUp: -COLUMNAS,
    };
    const paso = pasos[e.key];
    // Tab desde la rejilla vuelve a la búsqueda: el panel no se deja a medias.
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault();
      rejilla.current?.parentElement?.querySelector<HTMLInputElement>('input')?.focus();
      return;
    }
    if (paso !== undefined) {
      e.preventDefault();
      // ↑ desde la primera fila vuelve a la búsqueda.
      if (paso < 0 && actual + paso < 0 && e.key === 'ArrowUp') {
        rejilla.current?.parentElement?.querySelector<HTMLInputElement>('input')?.focus();
        return;
      }
      mover(actual + paso);
    } else if (e.key === 'Home') {
      e.preventDefault();
      mover(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      mover(todos.length - 1);
    }
  };

  let indice = 0;
  return (
    <div class="flex w-[19rem] max-w-full flex-col gap-2">
      <Input
        type="search"
        size="sm"
        value={q}
        autocomplete="off"
        aria-label={t('editor.symbolsSearch')}
        placeholder={t('editor.symbolsSearch')}
        onInput={(e) => {
          setQ(e.currentTarget.value);
          setActivo(0);
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && todos.length) {
            e.preventDefault();
            mover(0);
          } else if (e.key === 'Enter' && q.trim() && todos[0]) {
            e.preventDefault();
            elegir(todos[0]);
          }
        }}
      />
      <div
        ref={rejilla}
        class="-mx-1 max-h-64 overflow-y-auto px-1"
        // La rejilla se desplaza, pero no es una parada propia: el foco va a sus botones.
        data-toolbar-skip
      >
        {todos.length === 0 ? (
          <p class="text-text-muted px-1 py-2 text-sm" role="status">
            {t('editor.slashNoResults')}
          </p>
        ) : (
          secciones.map((sec) =>
            sec.simbolos.length === 0 ? null : (
              <Fragment key={sec.id}>
                <p id={`notes-simbolos-${sec.id}`} class="sytue-meta px-1 pt-1.5 pb-1">
                  {sec.titulo}
                </p>
                <div
                  role="group"
                  aria-labelledby={`notes-simbolos-${sec.id}`}
                  class="grid grid-cols-8 gap-0.5"
                >
                  {sec.simbolos.map((simbolo) => {
                    const i = indice++;
                    return (
                      <button
                        key={`${sec.id}-${simbolo}`}
                        type="button"
                        data-i={i}
                        tabIndex={i === actual ? 0 : -1}
                        class={cx(
                          'sytue-focus-ring text-text hover:bg-bg-subtle grid h-8 w-full place-items-center rounded-md text-lg leading-none',
                          i === actual && 'bg-bg-subtle',
                        )}
                        onMouseDown={(e) => e.preventDefault()}
                        onFocus={() => setActivo(i)}
                        onKeyDown={alTeclearRejilla}
                        onClick={() => elegir(simbolo)}
                      >
                        {simbolo}
                      </button>
                    );
                  })}
                </div>
              </Fragment>
            ),
          )
        )}
      </div>
    </div>
  );
}
