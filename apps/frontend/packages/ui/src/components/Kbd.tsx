/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Kbd — la tecla dibujada: la pastilla con el nombre de un atajo («⌘K», «Esc», «j»).
 *
 * Había una receta de `<kbd>` por sitio —la ventana de atajos, el buscador global, la
 * paleta de Cloud, el lateral de Rooms, el del ERP…— y ninguna se parecía a la de al
 * lado: píldora o cuadrada, mono o sans, con fondo o sin él, a 11 px o a `text-2xs`.
 * Ahora hay DOS, por el fondo en el que se posa: `surface` sobre el papel (diálogos,
 * pies de paleta, barras de herramientas) y `chrome` sobre el cromo (barra superior y
 * menú lateral). Su espejo nativo es `ui.TeclaSuperficie` / `ui.TeclaCromo`.
 *
 * Es DECORACIÓN, siempre: va con `aria-hidden` porque el atajo lo nombra el
 * `aria-label`, el `title` o el texto del control que la lleva. Un lector de pantalla
 * que lee «⌘K» no dice nada útil; quien la pone tiene que seguir nombrando el atajo con
 * palabras.
 *
 * `class` es para COLOCARLA (margen, posición, visibilidad), no para cambiar su
 * aspecto: `cx` no fusiona utilidades y dos esquinas o dos altos pelearían por orden de
 * hoja. Un tercer aspecto se añade aquí como tono, no en el sitio que lo necesita.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

export type KbdTone = 'surface' | 'chrome';

export interface KbdProps extends Omit<JSX.HTMLAttributes<HTMLElement>, 'aria-hidden'> {
  /** El fondo en el que se posa: el papel (por defecto) o el cromo. */
  tone?: KbdTone;
}

/*
 * La medida es una sola: `h-5 min-w-5` (una letra suelta sale cuadrada, «j k» no se
 * leen como una palabra), `px-1.5`, `text-xs` y el filete de 1 px. La cara es la de la
 * suite y no una mono —Fyne no embebe ninguna y la paridad manda—, con cifras
 * tabulares para que «F1»…«F12» o «Ctrl+1» no bailen de ancho.
 */
const BASE =
  'inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded border px-1.5 font-sans text-xs leading-none font-normal whitespace-nowrap tabular-nums';

/*
 * En el cromo no hay fondo propio: deja ver la barra o el carril (o el botón con pinta
 * de campo que la lleva), y el filete es el del cromo, que en oscuro es translúcido.
 */
const TONOS: Record<KbdTone, string> = {
  surface: 'border-border bg-bg-subtle text-text-muted',
  chrome: 'border-chrome-border text-on-chrome-muted bg-transparent',
};

export function Kbd({ tone = 'surface', class: cls, children, ...rest }: KbdProps): JSX.Element {
  return (
    <kbd {...rest} aria-hidden="true" class={cx(BASE, TONOS[tone], cls)}>
      {children}
    </kbd>
  );
}
