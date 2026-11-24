/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BarraDeBloque — la barra flotante del bloque donde está el cursor, como en Confluence:
 * debajo de la tabla, del código, del panel, del estado o de la fecha (encima si abajo
 * no cabe), con lo que se hace con ESE bloque. Sale sola al entrar y se va al salir.
 *
 * Se pinta en un portal con posición fija, así que no empuja el texto ni cambia la altura
 * de la nota: tapa, como mucho, la línea de debajo del bloque mientras se trabaja en él.
 * Es una `Toolbar` (una sola parada de tabulación, flechas dentro) y cada botón lleva su
 * ayuda. Apoyar en un botón no saca el cursor del texto (`preventDefault` al apoyar).
 *
 * Lo que cambia aquí es el DOM del bloque (un atributo, una celda): no entra en la pila de
 * deshacer del navegador, como no entraban las acciones de tabla de siempre.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import {
  BOTON_DE_BARRA,
  Dropdown,
  Icon,
  MenuItem,
  Popover,
  SEPARADOR_DE_BARRA,
  Toolbar,
  Tooltip,
  cx,
} from '@sytue/ui';
import type { NotesMessageKey, T } from '../i18n/index.js';
import {
  COLORES_DE_BLOQUE,
  LENGUAJES,
  TIPOS_DE_PANEL,
  type ColorDeBloque,
  type Contexto,
  type TipoDePanel,
} from './bloques.js';
import { alineacionDe, tieneCabeceraDeColumna, tieneCabeceraDeFila } from './tabla-notas.js';
import { Glyph, type GlyphName } from './glyphs.js';

/** El nombre de cada color de bloque (los de la paleta de tokens). */
export const NOMBRE_DE_COLOR: Record<ColorDeBloque, NotesMessageKey> = {
  gris: 'editor.statusGray',
  azul: 'editor.statusBlue',
  verde: 'editor.statusGreen',
  amarillo: 'editor.statusYellow',
  rojo: 'editor.statusRed',
  morado: 'editor.statusPurple',
};

const NOMBRE_DE_PANEL: Record<TipoDePanel, NotesMessageKey> = {
  info: 'editor.panelInfo',
  nota: 'editor.panelNote',
  exito: 'editor.panelSuccess',
  aviso: 'editor.panelWarning',
  error: 'editor.panelError',
};

/** Los lenguajes con su nombre propio (no se traducen); «texto» sí. */
const NOMBRE_DE_LENGUAJE: Record<string, string> = {
  bash: 'Bash',
  c: 'C',
  cpp: 'C++',
  csharp: 'C#',
  css: 'CSS',
  go: 'Go',
  html: 'HTML',
  java: 'Java',
  javascript: 'JavaScript',
  json: 'JSON',
  kotlin: 'Kotlin',
  markdown: 'Markdown',
  php: 'PHP',
  python: 'Python',
  ruby: 'Ruby',
  rust: 'Rust',
  sql: 'SQL',
  swift: 'Swift',
  typescript: 'TypeScript',
  xml: 'XML',
  yaml: 'YAML',
};

/** Lo que la barra pide a quien edita: cada acción ya sabe sobre qué bloque actúa. */
export interface AccionesDeBloque {
  tabla: {
    fila: (lado: 'antes' | 'despues') => void;
    columna: (lado: 'antes' | 'despues') => void;
    quitarFila: () => void;
    quitarColumna: () => void;
    quitarTabla: () => void;
    cabeceraFila: () => void;
    cabeceraColumna: () => void;
    numerada: () => void;
    fondo: (c: ColorDeBloque | null) => void;
    alinear: (lado: 'left' | 'center' | 'right') => void;
  };
  codigo: {
    lenguaje: (l: string) => void;
    lineas: () => void;
    ajuste: () => void;
    quitar: () => void;
  };
  panel: { tipo: (t: TipoDePanel) => void; quitar: () => void };
  estado: { color: (c: ColorDeBloque) => void; quitar: () => void };
  fecha: { dia: (iso: string) => void; quitar: () => void };
  expandir: { quitar: () => void };
}

/** Un botón de la barra del bloque, con su ayuda. */
function Boton({
  rotulo,
  dibujo,
  pulsado,
  onPulsar,
}: {
  rotulo: string;
  dibujo: JSX.Element;
  pulsado?: boolean;
  onPulsar: () => void;
}): JSX.Element {
  return (
    <Tooltip label={rotulo}>
      <button
        type="button"
        class={BOTON_DE_BARRA}
        aria-label={rotulo}
        {...(pulsado === undefined ? {} : { 'aria-pressed': pulsado })}
        onMouseDown={(e) => e.preventDefault()}
        onClick={onPulsar}
      >
        {dibujo}
      </button>
    </Tooltip>
  );
}

const g = (name: GlyphName) => <Glyph name={name} size={16} />;
const Sep = () => <span class={SEPARADOR_DE_BARRA} aria-hidden="true" />;

/** La muestra de un color de bloque: el fondo del token, como lo pinta la nota. */
export function Muestra({ color }: { color: ColorDeBloque | null }): JSX.Element {
  return (
    <span
      class={cx(
        'border-border inline-block h-4 w-4 shrink-0 rounded-sm border',
        color && `notes-fondo-${color}`,
      )}
      aria-hidden="true"
    />
  );
}

export function BarraDeBloque({
  t,
  contexto,
  acciones,
}: {
  t: T;
  contexto: Contexto;
  acciones: AccionesDeBloque;
}): JSX.Element | null {
  const caja = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const el = contexto.el;

  // Debajo del bloque, o encima si abajo no cabe; dentro de la ventana siempre. Se
  // recoloca al desplazar (en captura: la nota se desplaza dentro de su panel) y al
  // cambiar de tamaño.
  useLayoutEffect(() => {
    const colocar = () => {
      const r = el.getBoundingClientRect();
      const b = caja.current?.getBoundingClientRect();
      const alto = b?.height ?? 36;
      const ancho = b?.width ?? 240;
      const debajo = r.bottom + 6 + alto <= window.innerHeight - 8;
      setPos({
        top: debajo ? r.bottom + 6 : Math.max(8, r.top - 6 - alto),
        left: Math.max(8, Math.min(r.left, window.innerWidth - ancho - 8)),
      });
    };
    colocar();
    window.addEventListener('scroll', colocar, true);
    window.addEventListener('resize', colocar);
    return () => {
      window.removeEventListener('scroll', colocar, true);
      window.removeEventListener('resize', colocar);
    };
  }, [el, contexto.tipo]);

  let contenido: ComponentChildren = null;
  let rotulo = t('editor.blockTools');
  switch (contexto.tipo) {
    case 'tabla': {
      const a = acciones.tabla;
      const tabla = contexto.el;
      const alineada = alineacionDe(contexto.celda);
      const fondo = (contexto.celda.getAttribute('data-fondo') ?? null) as ColorDeBloque | null;
      rotulo = t('editor.tableTools');
      contenido = (
        <>
          <Boton
            rotulo={t('editor.headerRow')}
            dibujo={g('cabeceraFila')}
            pulsado={tieneCabeceraDeFila(tabla)}
            onPulsar={a.cabeceraFila}
          />
          <Boton
            rotulo={t('editor.headerColumn')}
            dibujo={g('cabeceraColumna')}
            pulsado={tieneCabeceraDeColumna(tabla)}
            onPulsar={a.cabeceraColumna}
          />
          <Boton
            rotulo={t('editor.numberedRows')}
            dibujo={g('numberedList')}
            pulsado={tabla.hasAttribute('data-numerada')}
            onPulsar={a.numerada}
          />
          <Sep />
          <Popover
            label={t('editor.cellBackground')}
            trigger={(props) => (
              <Tooltip label={t('editor.cellBackground')}>
                <button
                  {...props}
                  type="button"
                  class={cx(BOTON_DE_BARRA, 'aria-expanded:bg-bg-subtle')}
                  aria-label={t('editor.cellBackground')}
                  onMouseDown={(e) => e.preventDefault()}
                >
                  <Muestra color={fondo} />
                </button>
              </Tooltip>
            )}
          >
            {(cerrar) => (
              <div
                class="grid grid-cols-4 gap-1.5"
                role="group"
                aria-label={t('editor.cellBackground')}
              >
                {[null, ...COLORES_DE_BLOQUE].map((c) => (
                  <button
                    key={c ?? 'ninguno'}
                    type="button"
                    aria-label={c ? t(NOMBRE_DE_COLOR[c]) : t('editor.noBackground')}
                    title={c ? t(NOMBRE_DE_COLOR[c]) : t('editor.noBackground')}
                    aria-pressed={fondo === c}
                    class="sytue-focus-ring hover:bg-bg-subtle aria-pressed:bg-selected grid h-8 w-8 place-items-center rounded-md"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => {
                      a.fondo(c);
                      cerrar();
                    }}
                  >
                    <Muestra color={c} />
                  </button>
                ))}
              </div>
            )}
          </Popover>
          <Dropdown
            label={t('editor.align')}
            trigger={(props) => (
              <Tooltip label={t('editor.align')}>
                <button
                  {...props}
                  type="button"
                  class={cx(BOTON_DE_BARRA, 'aria-expanded:bg-bg-subtle')}
                  aria-label={t('editor.align')}
                  onMouseDown={(e) => e.preventDefault()}
                >
                  {g(
                    alineada === 'center'
                      ? 'alignCenter'
                      : alineada === 'right'
                        ? 'alignRight'
                        : 'alignLeft',
                  )}
                </button>
              </Tooltip>
            )}
          >
            {(['left', 'center', 'right'] as const).map((lado) => (
              <MenuItem
                key={lado}
                size="sm"
                role="menuitemradio"
                aria-checked={alineada === lado}
                icon={g(
                  lado === 'left' ? 'alignLeft' : lado === 'center' ? 'alignCenter' : 'alignRight',
                )}
                onMouseDown={(e: MouseEvent) => e.preventDefault()}
                onClick={() => a.alinear(lado)}
              >
                {t(
                  lado === 'left'
                    ? 'editor.alignLeft'
                    : lado === 'center'
                      ? 'editor.alignCenter'
                      : 'editor.alignRight',
                )}
              </MenuItem>
            ))}
          </Dropdown>
          <Sep />
          <Boton
            rotulo={t('editor.rowAbove')}
            dibujo={g('filaArriba')}
            onPulsar={() => a.fila('antes')}
          />
          <Boton
            rotulo={t('editor.rowBelow')}
            dibujo={g('filaAbajo')}
            onPulsar={() => a.fila('despues')}
          />
          <Boton
            rotulo={t('editor.colBefore')}
            dibujo={g('columnaAntes')}
            onPulsar={() => a.columna('antes')}
          />
          <Boton
            rotulo={t('editor.colAfter')}
            dibujo={g('columnaDespues')}
            onPulsar={() => a.columna('despues')}
          />
          <Sep />
          <Boton rotulo={t('editor.rowDelete')} dibujo={g('filaQuitar')} onPulsar={a.quitarFila} />
          <Boton
            rotulo={t('editor.colDelete')}
            dibujo={g('columnaQuitar')}
            onPulsar={a.quitarColumna}
          />
          <Boton rotulo={t('editor.tableDelete')} dibujo={g('trash')} onPulsar={a.quitarTabla} />
        </>
      );
      break;
    }
    case 'codigo': {
      const a = acciones.codigo;
      const pre = contexto.el;
      const lenguaje = pre.querySelector('code')?.getAttribute('data-lenguaje') ?? 'texto';
      const nombre = (l: string) =>
        l === 'texto' ? t('editor.codeLanguageNone') : (NOMBRE_DE_LENGUAJE[l] ?? l);
      rotulo = t('editor.codeBlock');
      contenido = (
        <>
          <Dropdown
            label={t('editor.codeLanguage')}
            panelClass="max-h-72"
            trigger={(props) => (
              <Tooltip label={t('editor.codeLanguage')}>
                <button
                  {...props}
                  type="button"
                  class={cx(
                    BOTON_DE_BARRA,
                    'aria-expanded:bg-bg-subtle w-auto gap-1 px-1.5 text-sm',
                  )}
                  aria-label={`${t('editor.codeLanguage')}: ${nombre(lenguaje)}`}
                  onMouseDown={(e) => e.preventDefault()}
                >
                  <Icon name="code" size={16} />
                  <span class="max-w-28 truncate">{nombre(lenguaje)}</span>
                  <Icon name="chevronDown" size={12} />
                </button>
              </Tooltip>
            )}
          >
            {LENGUAJES.map((l) => (
              <MenuItem
                key={l}
                size="sm"
                role="menuitemradio"
                aria-checked={lenguaje === l}
                onMouseDown={(e: MouseEvent) => e.preventDefault()}
                onClick={() => a.lenguaje(l)}
              >
                <span class="min-w-0 flex-1">{nombre(l)}</span>
                {lenguaje === l ? <Icon name="check" size={14} class="text-text-muted" /> : null}
              </MenuItem>
            ))}
          </Dropdown>
          <Sep />
          <Boton
            rotulo={t('editor.lineNumbers')}
            dibujo={g('numberedList')}
            pulsado={pre.hasAttribute('data-lineas')}
            onPulsar={a.lineas}
          />
          <Boton
            rotulo={t('editor.wrap')}
            dibujo={g('ajuste')}
            pulsado={pre.hasAttribute('data-ajuste')}
            onPulsar={a.ajuste}
          />
          <Sep />
          <Boton rotulo={t('editor.delete')} dibujo={g('trash')} onPulsar={a.quitar} />
        </>
      );
      break;
    }
    case 'panel': {
      const a = acciones.panel;
      const actual = contexto.el.getAttribute('data-panel');
      rotulo = t(NOMBRE_DE_PANEL[(actual as TipoDePanel) ?? 'info'] ?? 'editor.panelInfo');
      contenido = (
        <>
          {TIPOS_DE_PANEL.map((tipo) => (
            <Boton
              key={tipo}
              rotulo={t(NOMBRE_DE_PANEL[tipo])}
              dibujo={<span class={`notes-tinta-${tipo}`}>{g('panel')}</span>}
              pulsado={actual === tipo}
              onPulsar={() => a.tipo(tipo)}
            />
          ))}
          <Sep />
          <Boton rotulo={t('editor.delete')} dibujo={g('trash')} onPulsar={a.quitar} />
        </>
      );
      break;
    }
    case 'estado': {
      const a = acciones.estado;
      const actual = contexto.el.getAttribute('data-estado');
      rotulo = t('editor.status');
      contenido = (
        <>
          {COLORES_DE_BLOQUE.map((c) => (
            <Boton
              key={c}
              rotulo={t(NOMBRE_DE_COLOR[c])}
              dibujo={<Muestra color={c} />}
              pulsado={actual === c}
              onPulsar={() => a.color(c)}
            />
          ))}
          <Sep />
          <Boton rotulo={t('editor.delete')} dibujo={g('trash')} onPulsar={a.quitar} />
        </>
      );
      break;
    }
    case 'fecha': {
      const a = acciones.fecha;
      rotulo = t('editor.date');
      contenido = (
        <>
          <input
            type="date"
            class="border-border bg-surface text-text sytue-focus-ring rounded-control h-7 border px-1.5 text-sm"
            aria-label={t('editor.date')}
            value={contexto.el.getAttribute('datetime') ?? ''}
            onChange={(e) => {
              const v = e.currentTarget.value;
              if (v) a.dia(v);
            }}
          />
          <Sep />
          <Boton rotulo={t('editor.delete')} dibujo={g('trash')} onPulsar={a.quitar} />
        </>
      );
      break;
    }
    case 'expandir':
      rotulo = t('editor.expand');
      contenido = (
        <Boton
          rotulo={t('editor.delete')}
          dibujo={g('trash')}
          onPulsar={acciones.expandir.quitar}
        />
      );
      break;
  }

  if (typeof document === 'undefined') return null;
  return createPortal(
    <div
      ref={caja}
      class="fixed"
      data-notes-editor-tools
      style={{
        zIndex: 'var(--z-dropdown)',
        top: pos ? `${pos.top}px` : '-9999px',
        left: pos ? `${pos.left}px` : '-9999px',
      }}
    >
      <Toolbar
        aria-label={rotulo}
        omitir='[role="dialog"]'
        class="sytue-pop-in sytue-menu-surface border-border flex items-center gap-0.5 rounded-lg border p-1 shadow-[var(--shadow-md)]"
      >
        {contenido}
      </Toolbar>
    </div>,
    document.body,
  );
}
