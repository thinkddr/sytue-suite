/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel derecho de ajustes contextual (constructor B9). Con la selección en
 * texto/párrafo muestra ajustes de párrafo (espaciado antes/después,
 * interlineado exacto y sangrías izquierda/derecha/primera línea con steppers
 * numéricos); con el cursor dentro de una tabla muestra ajustes de tabla
 * (bordes, cabecera alternada, ancho de columnas automático). Se pliega y
 * despliega con el icono de engranaje (ctx.ui.settingsOpen).
 *
 * Decisiones no cubiertas por el blueprint (§0.10, se documentan aquí):
 * - Este panel usa el token de ancho compartido --editor-panel-w (288 px, el
 *   mismo que el panel del LeftRail) para que ambos lados queden simétricos.
 * - Espaciado y sangrías no existen en EditorCommand (§2): la mutación vive
 *   aquí como estilo inline sobre los bloques de la selección (el HTML ES el
 *   modelo) y termina emitiendo `input` sobre el root para que el autosave la
 *   recoja — mismo patrón que los caminos manuales de exec.ts (§3).
 * - «Cabecera alternada» se materializa como primera fila destacada (negrita
 *   + fondo) y bandas alternas en el resto, con estilos inline y la marca
 *   data-cabecera-alternada en la <table> para poder releer el estado.
 * - Unidades: espaciado en pt, sangrías en cm, interlineado como múltiplo;
 *   los valores actuales se leen de getComputedStyle (px @96 dpi) para que
 *   los steppers reflejen también los valores por defecto de la hoja §8.
 */
import type { JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import type { EditorContext } from '../types';
import { useT } from '../context';
import { getEditorRoot } from '../canvas/exec';
import { EditorIcon } from '../icons';

/* ---------- Utilidades internas ---------- */

/** Receta única de foco visible (regla de oro §0.6). */
// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Botón-icono sobre la cabecera sólida del producto (bg-header): texto claro AA. */
const HEADER_ICON_BTN =
  'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-sm text-header-fg ' +
  'hover:bg-chrome-hover active:bg-chrome-active focus-visible:outline-none ' +
  'sytue-focus-ring-chrome';

/** Botón del raíl plegado: misma métrica que el RAIL_BTN del LeftRail (h-8 w-8)
 *  para que los dos raíles del editor queden simétricos. */
const RAIL_BTN =
  'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
  FOCUS_RING;

const PX_POR_PT = 96 / 72; // 1 pt = 1,333 px @96 dpi
const PX_POR_CM = 96 / 2.54; // 1 cm ≈ 37,8 px @96 dpi

/** Bloques de párrafo sobre los que se aplican los ajustes (§3.1: p, h1–h3, li, td). */
const SELECTOR_BLOQUES = 'p,h1,h2,h3,li,td,th';

/** Colores de CONTENIDO de la cabecera y las bandas de tabla: viajan en el
 *  HTML del documento (mismo criterio que CONTENT_PALETTE de §5; no son
 *  tokens de UI, el papel es blanco siempre). */
const FONDO_CABECERA = '#E2E8F0';
const FONDO_BANDA = '#F1F5F9';

/** El autosave escucha `input` en el root; las mutaciones manuales lo emiten a mano (§3). */
function emitirInput(root: HTMLElement): void {
  root.dispatchEvent(new Event('input', { bubbles: true }));
}

/** Bloques que interseca la selección actual dentro del folio, solo los más
 *  internos (aplicar a la vez a un <li> y a su <p> duplicaría márgenes). */
function bloquesEnSeleccion(root: HTMLElement): HTMLElement[] {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return [];
  const range = sel.getRangeAt(0);
  if (!root.contains(range.commonAncestorContainer)) return [];
  const candidatos = Array.from(root.querySelectorAll<HTMLElement>(SELECTOR_BLOQUES)).filter((el) =>
    range.intersectsNode(el),
  );
  return candidatos.filter((el) => !candidatos.some((otro) => otro !== el && el.contains(otro)));
}

/*
 * Última diana conocida: mientras se teclea en un stepper del panel el foco
 * vive fuera del folio, así que se recuerda la última resolución válida de la
 * selección (mismo espíritu que la memoria de Range de exec.ts §3).
 * ponytail: memoria de módulo porque hay UN editor por página; pasar a estado
 * del componente si algún día conviven varios editores.
 */
let objetivosRecordados: HTMLElement[] = [];

function resolverObjetivos(): HTMLElement[] {
  const root = getEditorRoot();
  if (!root) return [];
  const vivos = bloquesEnSeleccion(root);
  if (vivos.length > 0) objetivosRecordados = vivos;
  return objetivosRecordados.filter((el) => root.contains(el));
}

function limitar(valor: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, valor));
}

/* ---------- Lectura y aplicación: párrafo ---------- */

interface ValoresParrafo {
  antes: number; // pt
  despues: number; // pt
  interlineado: number; // múltiplo (sin unidad)
  izquierda: number; // cm
  derecha: number; // cm
  primera: number; // cm
}

const PARRAFO_VACIO: ValoresParrafo = {
  antes: 0,
  despues: 0,
  interlineado: 1.5, // valor base de .sytue-page (§8)
  izquierda: 0,
  derecha: 0,
  primera: 0,
};

function leerParrafo(bloque: HTMLElement | null): ValoresParrafo {
  if (!bloque) return PARRAFO_VACIO;
  const estilo = getComputedStyle(bloque);
  const px = (v: string): number => Number.parseFloat(v) || 0;
  const fuente = px(estilo.fontSize) || 16;
  return {
    antes: px(estilo.marginTop) / PX_POR_PT,
    despues: px(estilo.marginBottom) / PX_POR_PT,
    interlineado: estilo.lineHeight === 'normal' ? 1.2 : px(estilo.lineHeight) / fuente,
    izquierda: px(estilo.marginLeft) / PX_POR_CM,
    derecha: px(estilo.marginRight) / PX_POR_CM,
    primera: px(estilo.textIndent) / PX_POR_CM,
  };
}

type PropParrafo =
  'marginTop' | 'marginBottom' | 'lineHeight' | 'marginLeft' | 'marginRight' | 'textIndent';

/** Aplica un estilo inline a todos los bloques de la selección y avisa al autosave. */
function aplicarParrafo(prop: PropParrafo, css: string): boolean {
  const root = getEditorRoot();
  const bloques = resolverObjetivos();
  if (!root || bloques.length === 0) return false;
  for (const bloque of bloques) bloque.style[prop] = css;
  emitirInput(root);
  return true;
}

/* ---------- Lectura y aplicación: tabla ---------- */

interface EstadoTabla {
  bordes: boolean;
  cabecera: boolean;
  anchoAuto: boolean;
}

function tablaActual(): HTMLTableElement | null {
  const root = getEditorRoot();
  if (!root) return null;
  const tabla = resolverObjetivos()[0]?.closest('table') ?? null;
  return tabla !== null && root.contains(tabla) ? tabla : null;
}

function leerTabla(tabla: HTMLTableElement | null): EstadoTabla {
  if (!tabla) return { bordes: true, cabecera: false, anchoAuto: false };
  const celda = tabla.querySelector('td,th');
  return {
    bordes: celda === null || getComputedStyle(celda).borderTopStyle !== 'none',
    cabecera: tabla.hasAttribute('data-cabecera-alternada'),
    anchoAuto: tabla.style.tableLayout === 'auto',
  };
}

/** Bordes on/off: estilo inline por celda que anula (o restaura) el borde de .sytue-page td (§8). */
function aplicarBordes(tabla: HTMLTableElement, visibles: boolean): void {
  for (const celda of Array.from(tabla.querySelectorAll<HTMLElement>('td,th'))) {
    celda.style.border = visibles ? '' : 'none';
  }
}

/** Cabecera alternada: primera fila destacada + bandas alternas en el cuerpo. */
function aplicarCabecera(tabla: HTMLTableElement, activa: boolean): void {
  // ponytail: estilos inline fila a fila, así que las filas insertadas después
  // no heredan la banda hasta re-alternar el conmutador; regla CSS de contenido
  // propia cuando el editor tenga su hoja de estilos exportable.
  Array.from(tabla.rows).forEach((fila, i) => {
    fila.style.backgroundColor = !activa
      ? ''
      : i === 0
        ? FONDO_CABECERA
        : i % 2 === 0
          ? FONDO_BANDA
          : '';
    if (i === 0) {
      for (const celda of Array.from(fila.cells)) celda.style.fontWeight = activa ? '600' : '';
    }
  });
  tabla.toggleAttribute('data-cabecera-alternada', activa);
}

/** Ancho de columnas automático: anula el table-layout: fixed de .sytue-page table (§8). */
function aplicarAnchoAuto(tabla: HTMLTableElement, auto: boolean): void {
  tabla.style.tableLayout = auto ? 'auto' : '';
  tabla.style.width = auto ? 'auto' : '';
}

/* ---------- Controles del panel ---------- */

const BOTON_PASO =
  'inline-flex h-7 w-7 shrink-0 items-center justify-center border border-border bg-surface text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

interface StepperProps {
  etiqueta: string;
  /** Valor actual leído del documento. */
  valor: number;
  min: number;
  max: number;
  paso: number;
  /** Decimales al mostrar (los ceros finales se recortan). */
  decimales: number;
  disabled?: boolean;
  onCommit: (valor: number) => void;
}

/** Stepper numérico: botones −/+ (no roban el foco del folio) + entrada libre. */
function Stepper(props: StepperProps): JSX.Element {
  const t = useT();
  const id = useId();
  const texto = String(Number(props.valor.toFixed(props.decimales)));
  const saltar = (signo: 1 | -1): void => {
    const siguiente = Number((props.valor + signo * props.paso).toFixed(props.decimales));
    props.onCommit(limitar(siguiente, props.min, props.max));
  };
  return (
    <div class="flex items-center justify-between gap-2 px-3 py-1">
      <label for={id} class="sytue-label min-w-0 truncate">
        {props.etiqueta}
      </label>
      <div class="flex shrink-0 items-center">
        <button
          type="button"
          class={`${BOTON_PASO} rounded-l-sm`}
          aria-label={t('settings.decrease', { name: props.etiqueta })}
          title={t('settings.decrease', { name: props.etiqueta })}
          disabled={props.disabled}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => saltar(-1)}
        >
          <EditorIcon name="minus" size="sm" />
        </button>
        <input
          id={id}
          type="number"
          inputMode="decimal"
          class={
            'border-border bg-surface text-text h-7 w-16 border-y px-1 text-center text-xs tabular-nums ' +
            '[appearance:textfield] disabled:opacity-40 [&::-webkit-inner-spin-button]:appearance-none ' +
            '[&::-webkit-outer-spin-button]:appearance-none ' +
            FOCUS_RING
          }
          min={props.min}
          max={props.max}
          step={props.paso}
          disabled={props.disabled}
          // El valor se escribe a mano para no pisar lo que el usuario teclea:
          // solo se refresca cuando la entrada no tiene el foco.
          ref={(el) => {
            if (el && document.activeElement !== el) el.value = texto;
          }}
          onChange={(e) => {
            const v = Number.parseFloat(e.currentTarget.value);
            if (Number.isFinite(v)) props.onCommit(limitar(v, props.min, props.max));
            else e.currentTarget.value = texto;
          }}
        />
        <button
          type="button"
          class={`${BOTON_PASO} -ml-px rounded-r-sm`}
          aria-label={t('settings.increase', { name: props.etiqueta })}
          title={t('settings.increase', { name: props.etiqueta })}
          disabled={props.disabled}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => saltar(1)}
        >
          <EditorIcon name="plus" size="sm" />
        </button>
      </div>
    </div>
  );
}

interface ConmutadorProps {
  etiqueta: string;
  activo: boolean;
  disabled?: boolean;
  onToggle: () => void;
}

/**
 * Interruptor accesible (role="switch"); no roba el foco del folio.
 *
 * No es el `Switch` de `@sytue/ui` a propósito: aquel envuelve un `<input type=checkbox>`
 * real, y pulsarlo se lleva el foco —y con él la selección del texto— fuera del folio, que
 * es justo lo que no puede pasar al activar una opción sobre lo que hay seleccionado. Lo
 * que SÍ se copia es su aspecto, que es donde este se había quedado atrás: el carril
 * apagado es un componente de interfaz y WCAG 1.4.11 le pide 3:1 (`bg-border` daba 1,3:1,
 * o sea, no se veía que hubiera un interruptor), y el pomo sale de `surface` porque un
 * blanco fijo no invierte en tema oscuro.
 */
function Conmutador(props: ConmutadorProps): JSX.Element {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={props.activo}
      disabled={props.disabled}
      class={
        'text-text flex h-8 w-full items-center justify-between gap-2 rounded-sm px-2 text-left text-sm ' +
        'hover:bg-bg-subtle disabled:pointer-events-none disabled:opacity-40 ' +
        FOCUS_RING
      }
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => props.onToggle()}
    >
      <span class="min-w-0 truncate">{props.etiqueta}</span>
      <span
        aria-hidden="true"
        class={`inline-flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition-colors ${
          props.activo ? 'bg-primary' : 'bg-border-strong ring-1 ring-neutral-500'
        }`}
      >
        <span
          class={`bg-surface h-4 w-4 rounded-full shadow-[var(--shadow-xs)] transition-transform ${
            props.activo ? 'translate-x-4' : ''
          }`}
        />
      </span>
    </button>
  );
}

function TituloSeccion({ children }: { children: string }): JSX.Element {
  return <h3 class="sytue-overline text-text-subtle px-3 pt-3 pb-1">{children}</h3>;
}

/* ---------- Secciones contextuales ---------- */

function SeccionParrafo({ alAplicar }: { alAplicar: () => void }): JSX.Element {
  const t = useT();
  // Lectura en render: el panel se re-renderiza con cada cambio de ctx (la
  // selección refresca fmt en EditorApp) y tras cada ajuste (alAplicar).
  const bloque = resolverObjetivos()[0] ?? null;
  const v = leerParrafo(bloque);
  const sinBloque = bloque === null;
  const fijar = (prop: PropParrafo, css: string): void => {
    if (aplicarParrafo(prop, css)) alAplicar();
  };
  return (
    <>
      <TituloSeccion>{t('settings.spacing')}</TituloSeccion>
      <Stepper
        etiqueta={t('settings.spacing.before')}
        valor={v.antes}
        min={0}
        max={96}
        paso={6}
        decimales={1}
        disabled={sinBloque}
        onCommit={(n) => fijar('marginTop', `${n}pt`)}
      />
      <Stepper
        etiqueta={t('settings.spacing.after')}
        valor={v.despues}
        min={0}
        max={96}
        paso={6}
        decimales={1}
        disabled={sinBloque}
        onCommit={(n) => fijar('marginBottom', `${n}pt`)}
      />
      <Stepper
        etiqueta={t('settings.spacing.lineHeight')}
        valor={v.interlineado}
        min={1}
        max={3}
        paso={0.1}
        decimales={2}
        disabled={sinBloque}
        onCommit={(n) => fijar('lineHeight', String(n))}
      />
      <TituloSeccion>{t('settings.indent')}</TituloSeccion>
      <Stepper
        etiqueta={t('settings.indent.left')}
        valor={v.izquierda}
        min={0}
        max={10}
        paso={0.25}
        decimales={2}
        disabled={sinBloque}
        onCommit={(n) => fijar('marginLeft', `${n}cm`)}
      />
      <Stepper
        etiqueta={t('settings.indent.right')}
        valor={v.derecha}
        min={0}
        max={10}
        paso={0.25}
        decimales={2}
        disabled={sinBloque}
        onCommit={(n) => fijar('marginRight', `${n}cm`)}
      />
      <Stepper
        etiqueta={t('settings.indent.firstLine')}
        valor={v.primera}
        min={-5}
        max={5}
        paso={0.25}
        decimales={2}
        disabled={sinBloque}
        onCommit={(n) => fijar('textIndent', `${n}cm`)}
      />
      {sinBloque && <p class="sytue-meta px-3 pt-2">{t('settings.paragraph.hint')}</p>}
    </>
  );
}

function SeccionTabla({ alAplicar }: { alAplicar: () => void }): JSX.Element {
  const t = useT();
  const tabla = tablaActual();
  const estado = leerTabla(tabla);
  const sinTabla = tabla === null;
  const conmutar = (
    aplicar: (tabla: HTMLTableElement, siguiente: boolean) => void,
    siguiente: boolean,
  ): void => {
    const root = getEditorRoot();
    if (!tabla || !root) return;
    aplicar(tabla, siguiente);
    emitirInput(root);
    alAplicar();
  };
  return (
    <>
      <TituloSeccion>{t('settings.table')}</TituloSeccion>
      <div class="flex flex-col gap-0.5 px-1">
        <Conmutador
          etiqueta={t('settings.table.borders')}
          activo={estado.bordes}
          disabled={sinTabla}
          onToggle={() => conmutar(aplicarBordes, !estado.bordes)}
        />
        <Conmutador
          etiqueta={t('settings.table.bandedHeader')}
          activo={estado.cabecera}
          disabled={sinTabla}
          onToggle={() => conmutar(aplicarCabecera, !estado.cabecera)}
        />
        <Conmutador
          etiqueta={t('settings.table.autoWidth')}
          activo={estado.anchoAuto}
          disabled={sinTabla}
          onToggle={() => conmutar(aplicarAnchoAuto, !estado.anchoAuto)}
        />
      </div>
    </>
  );
}

/* ---------- Panel ---------- */

/**
 * Panel derecho de ajustes (firma universal de zona, §2). Plegado muestra solo
 * la franja con el engranaje; desplegado, 280 px con la sección contextual
 * (tabla si el cursor está dentro de una, párrafo en el resto de casos).
 */
export function SettingsPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  // Sello local: tras aplicar un ajuste el documento cambia sin que cambie la
  // selección, así que los valores leídos del DOM se refrescan a mano.
  const [, marcar] = useState(0);
  const alAplicar = (): void => marcar((n) => n + 1);

  if (!ctx.ui.settingsOpen) {
    return (
      <div class="border-border bg-surface flex w-[var(--editor-rail-w)] shrink-0 flex-col items-center border-l py-2">
        <button
          type="button"
          class={RAIL_BTN}
          aria-label={t('settings.panel')}
          title={t('settings.panel')}
          aria-pressed={false}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => ctx.setUi({ settingsOpen: true })}
        >
          <EditorIcon name="settings" />
        </button>
      </div>
    );
  }

  const enTabla = ctx.fmt.inTable;
  return (
    <aside
      aria-label={t('settings.aside')}
      // En <768px vale lo mismo que para el panel izquierdo (ver EditorLayout):
      // 288 px fijos junto a las dos franjas no caben en un teléfono, así que aquí
      // el panel toma el ancho disponible y el lienzo le cede la pantalla.
      class="border-border bg-surface flex w-[var(--editor-panel-w)] min-w-0 shrink-0 flex-col border-l max-md:w-auto max-md:flex-1"
    >
      <div class="bg-header text-header-fg border-chrome-divider flex h-10 shrink-0 items-center justify-between border-b py-1 pr-1.5 pl-3">
        <h2 class="truncate text-sm font-semibold">
          {enTabla ? t('settings.table.heading') : t('settings.paragraph.heading')}
        </h2>
        {/* El engranaje pliega el panel; sobre el acento sólido va con texto claro
            (RibbonButton se reserva para el estado plegado, sobre superficie blanca). */}
        <button
          type="button"
          class={HEADER_ICON_BTN}
          aria-label={t('settings.panel')}
          title={t('settings.panel')}
          aria-pressed={true}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => ctx.setUi({ settingsOpen: false })}
        >
          <EditorIcon name="settings" />
        </button>
      </div>
      {/* Región con scroll: enfocable por sí misma (WCAG 2.1.1). Con el folio sin selección
          los controles de dentro están desactivados y, sin esto, el teclado no podía
          desplazar un panel más alto que la pantalla (`scrollable-region-focusable`). */}
      <div
        role="group"
        aria-label={enTabla ? t('settings.table.heading') : t('settings.paragraph.heading')}
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={0}
        class="sytue-focus-ring min-h-0 flex-1 overflow-y-auto pb-3"
      >
        {enTabla ? (
          <SeccionTabla alAplicar={alAplicar} />
        ) : (
          <SeccionParrafo alAplicar={alAplicar} />
        )}
      </div>
    </aside>
  );
}
