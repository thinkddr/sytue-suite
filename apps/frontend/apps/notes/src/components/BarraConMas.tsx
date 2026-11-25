/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BarraConMas — la barra del editor en UNA fila, con lo que no cabe en «Más».
 *
 * Antes se envolvía en varias filas en el escritorio y se desplazaba de lado en el
 * teléfono (`FILA_UNICA_EN_MOVIL`), con los controles del final fuera de la vista. Aquí
 * cada pieza declara su grupo y su prioridad, se mide lo que cabe (`client/desborde.ts`)
 * y el resto se ofrece en el menú «Más», con su rótulo y su atajo. La fila tiene siempre
 * la misma altura, así que cambiar de nota no mueve el texto.
 *
 * LAS MEDIDAS SE RECUERDAN ENTRE NOTAS. El editor se remonta al cambiar de nota (`key`);
 * con el reparto anterior a mano, la primera pintura ya sale bien y la barra no parpadea
 * de «todo» a «lo que cabe». Se mide antes de pintar (`useLayoutEffect`) y otra vez
 * cuando la fila cambia de ancho.
 *
 * UNA HERRAMIENTA CON PANEL (enlazar, colores, símbolos) cuyo botón no cabe abre su
 * panel colgado de «Más»: el mismo contenido, el mismo foco y el mismo Escape.
 */
import type { ComponentChildren, JSX } from 'preact';
import { Fragment } from 'preact';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import {
  BARRA_DE_EDICION,
  BOTON_DE_BARRA,
  Dropdown,
  Icon,
  MenuItem,
  MenuSeparator,
  Popover,
  SEPARADOR_DE_BARRA,
  Toolbar,
  Tooltip,
  atajoDelSistema,
  cx,
} from '@sytue/ui';
import { repartirBarra } from '../client/desborde.js';

export interface PanelDeBarra {
  label: string;
  contenido: () => ComponentChildren;
  cerrar: () => void;
}

export interface ItemDeBarra {
  /** Estable: con él se recuerda su ancho. */
  id: string;
  grupo: string;
  /** Más alta, más tarde va a «Más». */
  prioridad: number;
  /** Ancho de reserva hasta que se mide, en px (por defecto, el de un botón). */
  ancho?: number;
  /** Su control en la fila. */
  barra: () => JSX.Element;
  /** Lo que pone dentro de «Más» cuando no cabe: uno o varios ítems de menú. */
  menu: (cerrar: () => void) => ComponentChildren;
  /** Su panel, si lo tiene y está abierto. */
  panel?: PanelDeBarra | null;
}

/** Anchos medidos por pieza (y el de «Más»): valen para la próxima nota. */
const anchos = new Map<string, number>();
/** El último reparto, para que una barra recién montada empiece donde lo dejó la anterior. */
let ultimo: { clave: string; visibles: readonly boolean[] } | null = null;

const BOTON = 28;
const SEPARADOR = 9; // `mx-1` + `w-px`
const MAS = '\0mas';

function numero(v: string | undefined, porDefecto: number): number {
  const n = Number.parseFloat(v ?? '');
  return Number.isFinite(n) ? n : porDefecto;
}

/**
 * Una opción de «Más»: el dibujo y el rótulo del botón que sustituye, y su atajo a la
 * derecha. Conserva el cursor del texto al apoyar (como los botones de la fila) y cierra
 * el menú antes de actuar, para que la acción encuentre el foco donde lo espera.
 */
export function OpcionDeMas({
  dibujo,
  rotulo,
  atajo,
  marcada,
  desactivada,
  cerrar,
  onElegir,
}: {
  dibujo?: JSX.Element;
  rotulo: string;
  atajo?: string | undefined;
  /** Con estado (negrita puesta, lista activa): se anuncia como casilla de menú. */
  marcada?: boolean | undefined;
  desactivada?: boolean | undefined;
  cerrar: () => void;
  onElegir: () => void;
}): JSX.Element {
  return (
    <MenuItem
      size="sm"
      {...(dibujo ? { icon: dibujo } : {})}
      {...(marcada === undefined ? {} : { role: 'menuitemcheckbox', 'aria-checked': marcada })}
      disabled={desactivada}
      onMouseDown={(e: MouseEvent) => e.preventDefault()}
      onClick={() => {
        cerrar();
        onElegir();
      }}
    >
      <span class="min-w-0 flex-1 truncate">{rotulo}</span>
      {marcada ? <Icon name="check" size={14} class="text-text-muted shrink-0" /> : null}
      {atajo ? (
        <span class="sytue-meta shrink-0 tabular-nums">{atajoDelSistema(atajo)}</span>
      ) : null}
    </MenuItem>
  );
}

/** Olvida lo medido (para las pruebas, que cambian el ancho de la fila a mano). */
export function olvidarMedidas(): void {
  anchos.clear();
  ultimo = null;
}

export function BarraConMas({
  items,
  'aria-label': etiqueta,
  etiquetaMas,
  class: clase,
}: {
  items: ItemDeBarra[];
  'aria-label': string;
  /** El rótulo de «Más» (`editor.more`). */
  etiquetaMas: string;
  class?: string;
}): JSX.Element {
  const fila = useRef<HTMLDivElement>(null);
  const clave = items.map((i) => i.id).join(' ');
  const [visibles, setVisibles] = useState<readonly boolean[]>(() =>
    ultimo?.clave === clave ? ultimo.visibles : items.map(() => true),
  );
  const [anchoFila, setAnchoFila] = useState(0);

  // El ancho de la fila, y cada vez que cambia (la ventana, el panel, la tarjeta).
  useLayoutEffect(() => {
    const el = fila.current;
    if (!el) return;
    const medir = () => setAnchoFila(el.clientWidth);
    medir();
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(medir);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useLayoutEffect(() => {
    const el = fila.current;
    if (!el) return;
    // Lo que se ve se mide ahora; lo escondido conserva su última medida.
    for (const pieza of el.querySelectorAll<HTMLElement>('[data-pieza]')) {
      const w = pieza.getBoundingClientRect().width;
      if (w > 0) anchos.set(pieza.dataset['pieza']!, w);
    }
    const estilo = typeof getComputedStyle === 'function' ? getComputedStyle(el) : null;
    const relleno =
      numero(estilo?.paddingLeft, 0) +
      numero(estilo?.paddingRight, 0) +
      numero(estilo?.borderLeftWidth, 0) +
      numero(estilo?.borderRightWidth, 0);
    const nuevos = repartirBarra(
      items.map((i) => ({
        ancho: anchos.get(i.id) ?? i.ancho ?? BOTON,
        grupo: i.grupo,
        prioridad: i.prioridad,
      })),
      {
        disponible: anchoFila > 0 ? anchoFila - relleno : 0,
        anchoMas: anchos.get(MAS) ?? BOTON,
        hueco: numero(estilo?.columnGap, 2),
        separador: SEPARADOR,
      },
    );
    ultimo = { clave, visibles: nuevos };
    setVisibles((antes) =>
      antes.length === nuevos.length && antes.every((v, i) => v === nuevos[i]) ? antes : nuevos,
    );
    // `items` cambia en cada render (son funciones nuevas); lo que importa es cuáles son.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clave, anchoFila]);

  const escondidos = items.filter((_, i) => visibles[i] === false);
  const panel = escondidos.find((i) => i.panel)?.panel ?? null;

  const enFila: JSX.Element[] = [];
  let previo: ItemDeBarra | null = null;
  items.forEach((item, i) => {
    if (visibles[i] === false) return;
    if (previo && previo.grupo !== item.grupo)
      enFila.push(<span key={`sep-${item.id}`} class={SEPARADOR_DE_BARRA} aria-hidden="true" />);
    enFila.push(
      <div key={item.id} data-pieza={item.id} class="flex shrink-0 items-center">
        {item.barra()}
      </div>,
    );
    previo = item;
  });

  const mas = (
    <Dropdown
      label={etiquetaMas}
      align="end"
      trigger={(props) => (
        <Tooltip label={etiquetaMas}>
          <button
            {...props}
            type="button"
            class={cx(BOTON_DE_BARRA, 'aria-expanded:bg-bg-subtle aria-expanded:text-text')}
            aria-label={etiquetaMas}
            onMouseDown={(e) => e.preventDefault()}
          >
            <Icon name="ellipsis" size={16} />
          </button>
        </Tooltip>
      )}
    >
      {(cerrar) =>
        escondidos.map((item, i) => (
          <Fragment key={item.id}>
            {i > 0 && escondidos[i - 1]!.grupo !== item.grupo ? <MenuSeparator /> : null}
            {item.menu(cerrar)}
          </Fragment>
        ))
      }
    </Dropdown>
  );

  return (
    <Toolbar
      innerRef={fila}
      aria-label={etiqueta}
      class={cx(BARRA_DE_EDICION, 'min-w-0 flex-nowrap overflow-hidden', clase)}
    >
      {enFila}
      {escondidos.length > 0 ? (
        <div data-pieza={MAS} class="flex shrink-0 items-center">
          {/* El panel de una herramienta escondida cuelga de «Más». El disparador del
              Popover es el propio menú: no se usa su `onClick`, solo su sitio y su foco. */}
          <Popover
            label={panel?.label ?? etiquetaMas}
            open={panel !== null}
            onOpenChange={(abierto) => {
              if (!abierto) panel?.cerrar();
            }}
            align="end"
            trigger={() => mas}
          >
            {panel ? panel.contenido() : null}
          </Popover>
        </div>
      ) : null}
    </Toolbar>
  );
}
