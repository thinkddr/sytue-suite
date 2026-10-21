/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Primitivas del ribbon (BLUEPRINT.md §5): botón-icono 28×28, grupo con rótulo,
 * select compacto, muestra de color con popover de paleta y separador vertical.
 * Regla de oro: los controles de icono hacen preventDefault en mousedown para
 * NO robar el foco (ni la selección) del lienzo; el <select> es la excepción
 * (necesita foco) y exec() restaura la selección él solo (§3).
 */
import { createContext } from 'preact';
import type { ComponentChildren, JSX, RefObject } from 'preact';
import { EditorCtx, useT } from '../context';
import { useContext, useEffect, useRef, useState } from 'preact/hooks';
import {
  BOTON_DE_BARRA,
  CAMPO_DE_BARRA_BASE,
  atajoDelSistema,
  cx,
  esMac,
  useMobileBack,
} from '@sytue/ui';
import type { DocsT } from '../../i18n';
import { COMANDOS, ariaAtajo, atajoDe, bloqueo } from '../comandos';
import type { IdComando } from '../comandos';
import { EditorIcon } from '../icons';
import { CLASE_POPOVER } from './popovers';
import type { EditorIconName } from '../icons';

/* ---------- Paletas de CONTENIDO (§5) ----------
 * Colores que el usuario aplica a su documento; no son tokens de UI. Únicos
 * hex permitidos fuera de editor.css, fijados aquí por el blueprint. */

export const CONTENT_PALETTE = [
  '#0A0A0A',
  '#475569',
  '#94A3B8',
  '#FFFFFF',
  '#F97316',
  '#DC2626',
  '#D97706',
  '#16A34A',
  '#2563EB',
  '#7C3AED',
  '#DB2777',
  '#0D9488',
] as const;

/**
 * Los mismos colores, con nombre.
 *
 * Los sitios que pintan SERIES —barras de datos, minigráficos, escalas de color— tenían
 * cada uno su lista de hex, copiada de esta y ya con vida propia. Con nombre se pueden
 * usar sin volver a escribirlos y sin contar posiciones en un array.
 */
export const SERIE = {
  tinta: CONTENT_PALETTE[0],
  pizarra: CONTENT_PALETTE[1],
  blanco: CONTENT_PALETTE[3],
  naranja: CONTENT_PALETTE[4],
  rojo: CONTENT_PALETTE[5],
  ambar: CONTENT_PALETTE[6],
  verde: CONTENT_PALETTE[7],
  azul: CONTENT_PALETTE[8],
  violeta: CONTENT_PALETTE[9],
  rosa: CONTENT_PALETTE[10],
  teal: CONTENT_PALETTE[11],
} as const;

export const HIGHLIGHT_PALETTE = [
  '#FEF08A',
  '#FED7AA',
  '#BBF7D0',
  '#BFDBFE',
  '#E9D5FF',
  '#FBCFE8',
  '#E2E8F0',
  'transparent', // transparent = quitar resaltado
] as const;

/* ---------- Utilidades internas ---------- */

/** Receta única de foco visible (regla de oro §0.6; nunca outline:none sin reemplazo). */
// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Clases del botón-icono 28×28 (receta literal del blueprint §5). */
// EL ASPECTO VIVE EN `@sytue/ui`, no aquí. Lo escribió este ribbon, pero el
// redactor de Sytue Mail lo necesitaba igual y la alternativa era copiar las
// cadenas a mano y verlas divergir. Lo que queda en este fichero es el ARMAZÓN
// —los grupos con su rótulo debajo, los popovers—, que sí es propio del ribbon.
const BUTTON_CLASS = BOTON_DE_BARRA;

/**
 * Carcasa de un popover del ribbon: el panel flotante que cuelga de su disparador.
 *
 * Estaba escrita a mano en 29 sitios con las MISMAS ocho clases, y esa repetición ya
 * había divergido fuera del ribbon: dos paneles del lienzo usan `z-10` (=10) en vez del
 * `--z-popover` del sistema (=1070), por debajo de dropdown (1000) y modal (1060).
 *
 * Es `fixed` y no `absolute`: la banda de grupos se desplaza en horizontal y un contenedor con
 * `overflow-x` recorta lo que cuelga de él. `usePopoversFijos` (popovers.ts) lo coloca bajo su
 * disparador; el `left-0`/`right-0` que llevan algunos sitios es de cuando era `absolute` y ya
 * no manda (la posición la pone ese hook). Ver el porqué completo en popovers.ts.
 *
 * Aquí va SOLO lo común. Lo que varía de verdad —ancho, alineación (`left-0`/`right-0`) y
 * densidad (`p-1` en menús de filas, `p-2` en paneles)— lo pone cada sitio, porque meterlo
 * aquí obligaría a sobrescribirlo y dos utilidades del mismo tipo en un elemento no las
 * resuelve el orden de la cadena sino el de la hoja generada.
 */
export const POPOVER_SHELL =
  `${CLASE_POPOVER} sytue-menu-surface border-border fixed mt-1 rounded-lg border ` +
  'z-[var(--z-popover)] shadow-[var(--shadow-md)]';

/**
 * Campo de texto del ribbon.
 *
 * La altura es `h-7` porque esa es la fila del ribbon: `RibbonButton` y `RibbonSelect`
 * miden 28 px, y un campo más bajo no alinea con ellos. Es la razón por la que aquí NO
 * vale el `Input size="sm"` del sistema (24 px) — no es que el ribbon diverja del DS,
 * es que su fila mide otra cosa.
 *
 * Estaba escrito a mano en siete sitios y ya había producido TRES alturas distintas en
 * la misma barra: 28 px, 24 px, y 26 px en los dos que no declaraban altura y la dejaban
 * salir de `py-1` + el interlineado de `text-xs`. Esa tercera no la eligió nadie.
 *
 * El foco usa la utilidad del sistema y no la sombra escrita a mano que llevaban cinco de
 * los siete: en un campo de texto `:focus-visible` se activa también al hacer clic, así
 * que no se pierde nada y se gana no volver a copiar el anillo.
 */
export const FIELD_BASE = CAMPO_DE_BARRA_BASE;

/**
 * El campo del ribbon: la base + la altura de la fila.
 *
 * La altura va aparte porque hay un sitio que necesita otra —las celdas de la rejilla de
 * tabla, densas dentro de su popover— y dos `h-*` en el mismo elemento NO los resuelve el
 * orden de la cadena sino el de la hoja generada: `h-7` se emite después de `h-6` y gana
 * siempre. Esa celda pedía 24 px y llevaba renderizándose a 28 desde antes de que esto se
 * unificara; con `FIELD_BASE` ya mide lo que dice.
 */
export const FIELD_CLASS = `h-7 ${FIELD_BASE}`;

/* ---------- usePopover ---------- */

/** Estado abierto/cerrado de un popover del ribbon + cierre por clic-fuera y
 *  Escape (§5), con listeners solo mientras está abierto. Encapsula el efecto de
 *  descarte que se repetía en cada disparador con panel flotante (muestra de
 *  color, popovers de pestaña). El disparador hace el preventDefault en mousedown
 *  él mismo si necesita conservar el caret/selección del lienzo. */
export function usePopover(): {
  open: boolean;
  setOpen: (value: boolean | ((prev: boolean) => boolean)) => void;
  rootRef: RefObject<HTMLDivElement>;
} {
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);
  return { open, setOpen, rootRef };
}

/* ---------- Comandos del registro ---------- */

/**
 * Un atajo como lo lee quien tiene delante el teclado: «⌘⇧X» en un Mac y, en el resto,
 * con los nombres de tecla del idioma («Strg+Umschalt+X», «Ctrl+Mayús+X»).
 */
export function textoAtajo(keys: string, t: DocsT, mac: boolean = esMac()): string {
  if (mac) return atajoDelSistema(keys, true).replace(/Shift\+/g, '⇧');
  return keys
    .replace(/^Ctrl\+/, `${t('shortcut.ctrl')}+`)
    .replace(/Shift\+/, `${t('shortcut.shift')}+`);
}

/**
 * Lo que el registro dice de un control: rótulo, atajo y si está disponible ahora (solo
 * lectura, selección). Lo comparten el botón, el selector y la muestra de color para que
 * los tres se apaguen por la misma regla.
 */
function useComando(
  commandId: IdComando | undefined,
  label: string | undefined,
): { label: string; atajo: string | undefined; bloqueado: boolean } {
  const t = useT();
  // `null` fuera del proveedor (una prueba, un ejemplo): entonces no se apaga nada.
  const ctx = useContext(EditorCtx);
  if (!commandId) return { label: label ?? '', atajo: undefined, bloqueado: false };
  return {
    label: label ?? t(COMANDOS[commandId].labelKey),
    atajo: atajoDe(commandId),
    bloqueado:
      ctx !== null && bloqueo(commandId, { soloLectura: ctx.soloLectura, fmt: ctx.fmt }) !== null,
  };
}

/**
 * ¿Está apagado ahora ese comando? Para los disparadores de popover escritos a mano, que
 * no son un `RibbonButton` pero abren un panel que despacha ese comando.
 */
export function useBloqueado(commandId: IdComando | undefined): boolean {
  return useComando(commandId, '').bloqueado;
}

/** Rótulo propio o comando del registro (que lo trae): uno de los dos, obligatorio. */
type ConRotulo =
  { commandId: IdComando; label?: string } | { commandId?: undefined; label: string };

/* ---------- RibbonButton ---------- */

/** Botón-icono compacto 28×28. onMouseDown SIEMPRE hace preventDefault para no
 *  robar el foco (y la selección) del lienzo.
 *
 *  Con `commandId`, el rótulo, el atajo (en el `title` y en `aria-keyshortcuts`) y la
 *  disponibilidad salen del registro (`editor/comandos.json`); `label` solo hace falta
 *  cuando el rótulo cambia con el estado («Ocultar comentarios»). */
export type RibbonButtonProps = ConRotulo & {
  icon: EditorIconName;
  active?: boolean; // aria-pressed
  disabled?: boolean;
  onAction: () => void;
};

export function RibbonButton(props: RibbonButtonProps): JSX.Element {
  const t = useT();
  const { label, atajo, bloqueado } = useComando(props.commandId, props.label);
  const title = atajo ? `${label} (${textoAtajo(atajo, t)})` : label;
  return (
    <button
      type="button"
      class={BUTTON_CLASS}
      aria-label={label}
      title={title}
      aria-pressed={props.active}
      aria-keyshortcuts={atajo ? ariaAtajo(atajo) : undefined}
      disabled={props.disabled || bloqueado}
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => props.onAction()}
    >
      <EditorIcon name={props.icon} />
    </button>
  );
}

/* ---------- La banda: plegada o desplegada ---------- */

/**
 * ¿Está la banda desplegada con «Más»? Lo da `Ribbon` y lo leen el grupo y el separador,
 * que se pintan distinto en cada caso. Es un contexto y no una clase de grupo de Tailwind
 * (`group-data-*`) porque las dos reglas que dependen de él chocan con una de anchura
 * (`max-md:hidden` contra «desplegada: a la vista»), y el orden en que Tailwind emite dos
 * variantes no lo decide la cadena: aquí lo decide el código, que se lee.
 */
export const BandaDesplegada = createContext(false);

/* ---------- RibbonGroup ---------- */

/** Grupo del ribbon: fila de controles (44px) + rótulo inferior (text-2xs). */
export interface RibbonGroupProps {
  label: string; // visible bajo el grupo y aria-label del contenedor (role="group")
  children: ComponentChildren;
}

export function RibbonGroup(props: RibbonGroupProps): JSX.Element {
  const desplegada = useContext(BandaDesplegada);
  return (
    // `shrink-0`: la banda de la cinta es UNA línea que se desplaza, y un grupo que se
    // encogiera repartiría sus botones en dos líneas en vez de dejarse deslizar.
    //
    // Y por la misma razón sus controles NO envuelven, tampoco en un teléfono. Envolvían
    // de cuando la banda era `flex-wrap` y un grupo podía medir más que la pantalla; con
    // la banda deslizándose eso dejó de servir y pasó a estorbar: medido el 2026-10-16 a
    // 390 px, «Fuente» se partía en dos líneas DENTRO de una banda que ya desliza, la cinta
    // de Inicio subía a 216 px y Negrita y Cursiva quedaban debajo de «Más» mientras
    // Subrayado y Tachado sí se veían. En una línea, la banda mide 53 px en el teléfono y
    // lo que no cabe está a un deslizamiento, en su orden.
    //
    // Desplegada («Más») es otra cosa: los grupos se reparten en líneas y uno que mida más
    // que la pantalla sí reparte sus controles (`max-w-full` y `flex-wrap`), alineados al
    // principio para que su rótulo quede bajo su primer control y no en mitad de la línea.
    //
    // En el teléfono el rótulo del grupo NO se pinta con la banda plegada: son 13 px de los
    // 53, sin pantalla de sobra, y cada control lleva su nombre (`aria-label` y `title`).
    // El grupo lo sigue diciendo a quien no ve (`aria-label`), y con la banda desplegada,
    // que es donde se busca algo, vuelve a verse.
    <div
      role="group"
      aria-label={props.label}
      class={cx(
        'flex h-full shrink-0 flex-col items-center gap-0.5 px-1.5 pt-1.5 pb-1 max-md:items-start',
        desplegada ? 'max-w-full' : 'max-md:py-1',
      )}
    >
      <div
        class={cx(
          'flex min-h-11 items-center justify-center gap-0.5 gap-y-1 max-md:justify-start',
          desplegada && 'max-w-full flex-wrap',
        )}
      >
        {props.children}
      </div>
      <div class={cx('sytue-meta-dense mt-auto leading-none', !desplegada && 'max-md:hidden')}>
        {props.label}
      </div>
    </div>
  );
}

/* ---------- RibbonSelect ---------- */

/** Select nativo compacto (h-7). En onMouseDown NO se hace preventDefault (el
 *  select necesita foco); exec() restaura la selección del lienzo él solo (§3). */
export type RibbonSelectProps = ConRotulo & {
  value: string;
  options: { value: string; label: string }[];
  width?: number; // px; por defecto 120
  disabled?: boolean;
  onSelect: (value: string) => void;
};

export function RibbonSelect(props: RibbonSelectProps): JSX.Element {
  const { label, bloqueado } = useComando(props.commandId, props.label);
  return (
    // `min-w-0` Y NO SOLO EL `width`: como hijo de un flex, este div tiene
    // `min-width: auto`, así que su tamaño mínimo es el CONTENIDO intrínseco del
    // `<select>` —el ancho de su opción más larga— y el `width` declarado no se
    // respeta. En alemán, «Kein Übergang» y «Überblenden» estiraban el selector de
    // transición de Diapos hasta sacar la cinta entera de la pantalla: el barrido
    // móvil lo medía en +55, +64, +73 y +75 px de scroll lateral en los cuatro
    // teléfonos. Con `min-w-0` el ancho manda y el propio `select` recorta.
    <div class="relative min-w-0" style={{ width: `${props.width ?? 120}px` }}>
      <select
        class={
          'border-border bg-surface text-text hover:bg-bg-subtle h-7 w-full cursor-pointer appearance-none rounded-sm border pr-6 pl-1.5 text-xs disabled:pointer-events-none disabled:opacity-40 ' +
          FOCUS_RING
        }
        aria-label={label}
        title={label}
        value={props.value}
        disabled={props.disabled || bloqueado}
        onChange={(e) => props.onSelect(e.currentTarget.value)}
      >
        {props.options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      {/* Chevron de marca (appearance-none oculta la flecha nativa del SO); receta del DS (Form.tsx). */}
      <span
        aria-hidden="true"
        class="text-text-subtle pointer-events-none absolute top-1/2 right-1.5 -translate-y-1/2"
      >
        <EditorIcon name="chevronDown" size="sm" />
      </span>
    </div>
  );
}

/* ---------- RibbonColorSwatch ---------- */

/** Botón con barrita de color + popover de paleta (grid 6×2 de muestras 20×20).
 *  Todos los mousedown del popover hacen preventDefault (el foco no sale del lienzo). */
export type RibbonColorSwatchProps = ConRotulo & {
  icon: EditorIconName; // 'textColor' | 'highlight'
  color: string; // color actual (barrita inferior de 3px)
  palette?: readonly string[]; // por defecto CONTENT_PALETTE
  onPick: (color: string) => void;
};

export function RibbonColorSwatch(props: RibbonColorSwatchProps): JSX.Element {
  const { open, setOpen, rootRef } = usePopover();
  const { label, bloqueado } = useComando(props.commandId, props.label);
  // Los rótulos de la paleta también son texto visible (§13.1). La `t` se LEE del
  // contexto en vez de recibirse por props: este componente vive al final de
  // cintas muy anidadas y pasarla a mano obligaría a tocar catorce llamadas y las
  // firmas de todo lo que hay por el camino.
  const t = useT();
  const palette = props.palette ?? CONTENT_PALETTE;

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'text-text-muted relative inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text disabled:pointer-events-none disabled:opacity-40 ' +
          FOCUS_RING
        }
        aria-label={label}
        title={label}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen((v) => !v)}
      >
        <EditorIcon name={props.icon} />
        {/* Barrita del color actual; si es 'transparent' se pinta neutra para que no desaparezca. */}
        <span
          aria-hidden="true"
          class="absolute bottom-1 left-1/2 h-[3px] w-4 -translate-x-1/2 rounded-[1px]"
          style={{
            backgroundColor: props.color === 'transparent' ? 'var(--sytue-border)' : props.color,
          }}
        />
      </button>
      {open && (
        // onMouseDown solo evita que el popover robe el foco del lienzo editable; no es
        // una interacción de usuario (cada color es un <button> accesible).
        // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
        <div
          role="group"
          aria-label={t('palette.label', { name: label })}
          class={`${POPOVER_SHELL} left-0 grid w-max grid-cols-6 gap-1 p-2`}
          onMouseDown={(e) => e.preventDefault()}
        >
          {palette.map((c) => {
            const transparent = c === 'transparent';
            const name = transparent ? t('palette.none') : t('palette.color', { hex: c });
            return (
              <button
                key={c}
                type="button"
                class={'border-border relative h-6 w-6 rounded-[3px] border ' + FOCUS_RING}
                style={transparent ? undefined : { backgroundColor: c }}
                aria-label={name}
                title={name}
                onClick={() => {
                  props.onPick(c);
                  setOpen(false);
                }}
              >
                {transparent && (
                  /* Diagonal "sin color" (token danger, sin hex de UI). */
                  <span
                    aria-hidden="true"
                    class="absolute inset-0.5"
                    style={{
                      background:
                        'linear-gradient(to top right, transparent 44%, var(--sytue-danger) 44% 56%, transparent 56%)',
                    }}
                  />
                )}
                {c === props.color && (
                  /* Check sobre el color actual (§5); difference garantiza contraste sobre claro y oscuro. */
                  <span class="absolute inset-0 grid place-items-center text-white mix-blend-difference">
                    <EditorIcon name="check" size="sm" />
                  </span>
                )}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ---------- RibbonSeparator ---------- */

/**
 * Separador vertical entre grupos.
 *
 * Va centrado en la FILA DE CONTROLES, no en la banda entera. Centrado en los 68 px de la
 * banda caía en 14-54, mientras que los iconos ocupan 6-50 y el rótulo empieza en 53: le
 * faltaban 8 px por arriba, le sobraban 4 por abajo y se metía en la línea del rótulo.
 * Ahora ocupa 12-44, centrado en la fila de 44 px que de verdad separa.
 *
 * `shrink-0`, porque la banda es una fila flexible que se desplaza y lo que NO lleva
 * `shrink-0` absorbe lo que falta: medido el 2026-10-16, en cuanto la banda no cabía
 * —Inicio a 1280, Datos ya a 1440, todas las pestañas en una tableta— los once
 * separadores de Datos medían 0 px y los grupos se leían como una sola lista.
 *
 * En el teléfono el separador baja 2 px con la fila (`max-md:py-1` del grupo). Con la banda
 * desplegada no se pinta: los grupos se reparten en líneas, cada uno con su rótulo, y un
 * separador que cae al final de una línea no separa dos grupos, separa un grupo del borde.
 *
 * Y no se pinta al principio ni al final de la banda, ni detrás de otro separador. Hay
 * grupos que esperan a la red —los bloques, el tema, las macros— y no pintan nada hasta
 * que llega (o si no llega): el separador de cada lado quedaba pegado al del otro y la
 * banda enseñaba un «‖» entre «Formulario» y «Comentario» (medido el 2026-10-16). La regla
 * vive aquí y no en cada pestaña porque cualquier grupo condicional la necesita.
 */
export function RibbonSeparator(): JSX.Element | null {
  if (useContext(BandaDesplegada)) return null;
  return (
    <div
      aria-hidden="true"
      data-separador
      class="bg-border mx-1.5 mt-3 h-8 w-px shrink-0 self-start first:hidden last:hidden max-md:mt-2.5 [[data-separador]+&]:hidden"
    />
  );
}
