/**
 * Modal / Dialog — overlay + card centrada. Focus trap, Esc cierra, retorno de
 * foco al cerrar, role=dialog + aria-modal. Tamaños sm/md/lg/xl.
 * docs/design.md §3.9.
 *
 * Dos presentaciones, decididas por el ANCHO y no por el componente que llama:
 *
 * - Desde 768 px, la tarjeta de siempre —centrada o en el panel derecho— con el radio
 *   de las tarjetas de la forma (`rounded-card`: 14 en «Carril», 16 en «Marco»), no el
 *   20 que la hacía la superficie más redonda de la suite.
 * - Por debajo, una HOJA inferior a todo el ancho, con asa, la zona segura de abajo y el
 *   cuerpo desplazándose entre cabecera y pie fijos. Antes era una tarjeta flotando a
 *   media pantalla, fuera del alcance del pulgar, y desplazaba el panel entero: en un
 *   formulario largo, «Guardar» y el aspa se iban de la vista. Era la presentación de
 *   UN redactor (`mobileBottomSheet`); ahora es la de todos.
 *
 * Cabecera y pie no llevan filete salvo cuando el cuerpo se desplaza: tres bandas con
 * dos líneas para una frase de confirmación pesaban más que lo que decían. El filete
 * aparece justo cuando hace falta decir «hay más por debajo».
 */
import type { ComponentChildren, JSX } from 'preact';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { useUltimo } from '../lib/useUltimo.js';
import { useMobileBack } from '../lib/useMobileBack.js';
import { tecladoEnOtraSuperposicion } from '../lib/atajos.js';
import { Button } from './Button.js';
import { Icon } from './Icon.js';
import { useDialogToasts } from './Toast.js';

export type ModalSize = 'sm' | 'md' | 'lg' | 'xl';

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  size?: ModalSize;
  /** Panel lateral para formularios amplios; comparte foco y cierre con el modal. */
  placement?: 'center' | 'right';
  /** Pie con acciones (p.ej. botones). */
  footer?: ComponentChildren;
  children: ComponentChildren;
  /**
   * Cerrar al hacer click en el overlay (por defecto true). En el teléfono también
   * decide si se puede bajar la hoja con el dedo, y por eso si lleva asa: un asa que no
   * arrastra promete algo que no hace.
   */
  dismissable?: boolean;
  /**
   * Solo el botón «×» usa esto (si viene); clic fuera y Escape siguen llamando a
   * `onClose`. Existe para el redactor de Sytue Mail: clic fuera / Escape
   * autoguarda y cierra sin preguntar, pero la «×» SÍ pregunta si se quiere
   * descartar el borrador en vez de guardarlo — la única de las tres vías
   * pensada para eso. El resto de diálogos de la suite no lo necesita y sigue
   * con el mismo `onClose` para las tres.
   */
  onCloseButtonClick?: () => void;
}

const sizes: Record<ModalSize, string> = {
  sm: 'max-w-[400px]',
  md: 'max-w-[560px]',
  lg: 'max-w-[720px]',
  // Editores con lienzo (dibujo de Docus): necesitan sitio para el lienzo y su panel.
  xl: 'max-w-[1120px]',
};

/**
 * Qué cuenta como enfocable para un trap de Tab.
 *
 * Se exporta porque el `Modal` no es la única superficie que atrapa el foco: el visor de
 * presentación de Docs hace lo mismo y con su propia lista habría dos definiciones de
 * «enfocable» divergiendo en silencio.
 */
export const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * Los CAMPOS donde se escribe o se elige: es donde debe caer el foco al abrir un
 * diálogo, no en el botón «×». Se dejan fuera casillas, radios y ficheros: un
 * diálogo que arranca con una casilla enfocada parece que pide marcarla.
 */
const CAMPO =
  'input:not([disabled]):not([type="hidden"]):not([type="checkbox"]):not([type="radio"]):not([type="file"]):not([type="range"]):not([type="color"]),textarea:not([disabled]),select:not([disabled]),[contenteditable="true"]';

/**
 * Lo que hay que bajar la hoja con el dedo para cerrarla. Menos se confunde con un
 * toque que tiembla; más obliga a cruzar media pantalla.
 */
const CIERRE_ARRASTRE = 80;

/**
 * Un TIRÓN cierra aunque no llegue a `CIERRE_ARRASTRE`, como las hojas del sistema en
 * iOS y Android: más de `VELOCIDAD_CIERRE` px por milisegundo, si ha bajado al menos
 * `TIRON_MINIMO` (menos es un toque que tiembla). Fyne lee los tres de aquí.
 */
const VELOCIDAD_CIERRE = 0.5;
const TIRON_MINIMO = 24;

/**
 * El pie en el teléfono: las acciones a TODO el ancho, de dos en dos y a partes iguales,
 * con el principal el último (abajo a la derecha, donde cae el pulgar) y 44 px de alto.
 * En escritorio no cambia: a la derecha y cada una con su ancho.
 *
 * - Cada acción parte de media fila (`flex-basis` 50 % menos medio hueco) y crece; dos
 *   que caben se quedan exactamente a la mitad.
 * - Un rótulo que no cabe en media fila no se corta: su mínimo es su ancho natural (el
 *   botón es `whitespace-nowrap`), así que la pareja no cabe en la línea y cada una baja
 *   a la suya, a todo el ancho, con el principal debajo.
 * - Con un número IMPAR, la primera va sola arriba (`basis-full`): con tres, «Eliminar»
 *   encima y «Cancelar | Guardar» debajo, lejos del pulgar lo que destruye y la pareja de
 *   siempre en su sitio. Con una sola, a todo el ancho.
 * - Un pie que trae su propio envoltorio (un solo hijo) ocupa la fila entera y se reparte
 *   por dentro: es la salida de los pies que no son una botonera (el redactor de Mail).
 */
const PIE_TELEFONO =
  'max-md:items-stretch max-md:[&>*]:flex-[1_1_calc(50%_-_0.25rem)] max-md:[&>*:first-child:nth-last-child(odd)]:basis-full max-md:[&>button]:min-h-11 max-md:[&>a]:min-h-11';

/** Por debajo de esto el diálogo es una hoja: el `max-md:` de las clases. */
const HOJA = '(width < 48rem)';

/** ¿Se maneja con el dedo? Es el `pointer-coarse:` de la hoja, preguntado desde JS. */
function conElDedo(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches;
}

export function Modal({
  open,
  onClose,
  title,
  size = 'md',
  placement = 'center',
  footer,
  children,
  dismissable = true,
  onCloseButtonClick,
}: ModalProps): JSX.Element | null {
  const t = useT();
  const panelRef = useRef<HTMLDivElement>(null);
  const cuerpoRef = useRef<HTMLDivElement>(null);
  const avisosRef = useDialogToasts();
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const arrastreRef = useRef<{ y: number; dy: number; t: number } | null>(null);
  // Si el cuerpo se desplaza: es lo único que enciende los filetes de cabecera y pie.
  const [desborda, setDesborda] = useState(false);
  // onClose viva via ref: suele ser una closure nueva en cada render del padre.
  // Si fuera dependencia del efecto de foco, cualquier re-render de fondo
  // (cargas, toasts) re-ejecutaria el efecto y ROBARIA el foco al usuario
  // mientras escribe (su texto acabaria en el boton «Cerrar»).
  const onCloseRef = useUltimo(onClose);
  useMobileBack(open, onClose, { nivel: 2 });

  // useLayoutEffect (sincrono al commit), NO useEffect: el foco inicial debe
  // fijarse antes de que el dialogo sea interactuable. Diferido a un frame
  // (rAF), robaria el foco a quien ya hubiera empezado a teclear en un campo
  // (usuarios rapidos / automatizacion): sus pulsaciones acababan en «Cerrar».
  useLayoutEffect(() => {
    if (!open) return;
    previouslyFocusedRef.current = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    // Foco inicial dentro del diálogo: el primer CAMPO si lo hay y, si no, el
    // primer enfocable (el botón de cerrar). Con el «×» siempre primero, cada
    // diálogo se abría con ese botón resaltado y había que pulsar Tab para
    // empezar a escribir el título del evento o a buscar personas; como en el
    // sistema, el diálogo arranca listo para teclear.
    //
    // Con el DEDO, no: enfocar un campo saca el teclado en Android en cuanto se abre
    // la hoja y le tapa la mitad, antes de que nadie haya decidido escribir (en iOS no
    // sale, así que además cada teléfono hacía una cosa). El foco va al panel, que el
    // lector de pantalla anuncia igual, y el teclado sale al tocar el campo.
    const first = conElDedo()
      ? null
      : (panel?.querySelector<HTMLElement>(CAMPO) ?? panel?.querySelector<HTMLElement>(FOCUSABLE));
    (first ?? panel)?.focus();

    const onKeyDown = (e: KeyboardEvent) => {
      // Dropdown y Popover enfocan su propia capa: Escape/Tab se resuelven allí primero.
      if (tecladoEnOtraSuperposicion(panel)) return;
      if (e.key === 'Escape') {
        // UN DESPLEGABLE ABIERTO SE CIERRA ANTES QUE EL DIÁLOGO, que es lo que
        // espera cualquiera al pulsar Escape: primero lo de encima.
        //
        // Esta escucha va en `document` y en fase de CAPTURA —para que ningún
        // descendiente se coma la tecla y deje el diálogo sin salida—, y el
        // efecto secundario era que los Escape internos NUNCA llegaban: el
        // autocompletado de direcciones de Sytue Mail y el panel de enlace del
        // editor tenían su propio manejador y era código muerto. Escribir tres
        // letras de un contacto y pulsar Escape cerraba la redacción entera.
        //
        // Un descendiente no puede adelantarse a una captura en `document`, así
        // que la excepción se decide aquí: si el foco está en un combobox
        // desplegado, se deja pasar y que lo cierre quien lo abrió. El resto
        // sigue igual.
        const activo = document.activeElement as HTMLElement | null;
        const enComboAbierto =
          activo?.getAttribute('role') === 'combobox' &&
          activo.getAttribute('aria-expanded') === 'true';
        if (enComboAbierto) return;
        e.preventDefault();
        e.stopImmediatePropagation();
        onCloseRef.current();
        return;
      }
      if (e.key === 'Tab' && panel) {
        const nodes = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE));
        if (nodes.length === 0) {
          e.preventDefault();
          return;
        }
        const firstEl = nodes[0]!;
        const lastEl = nodes[nodes.length - 1]!;
        if (e.shiftKey && document.activeElement === firstEl) {
          e.preventDefault();
          lastEl.focus();
        } else if (!e.shiftKey && document.activeElement === lastEl) {
          e.preventDefault();
          firstEl.focus();
        }
      }
    };

    document.addEventListener('keydown', onKeyDown, true);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    return () => {
      document.removeEventListener('keydown', onKeyDown, true);
      document.body.style.overflow = prevOverflow;
      previouslyFocusedRef.current?.focus();
    };
  }, [open, onCloseRef]);

  // ¿Se desplaza el cuerpo? Se vuelve a mirar en cada render y cuando cambia de alto el
  // cuerpo o algo de lo que lleva dentro (una lista que termina de cargar sin que el
  // diálogo se repinte). `setDesborda` con el mismo valor no repinta.
  useLayoutEffect(() => {
    const cuerpo = cuerpoRef.current;
    if (!open || !cuerpo) return;
    const medir = () => setDesborda(cuerpo.scrollHeight > cuerpo.clientHeight + 1);
    medir();
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(medir);
    ro.observe(cuerpo);
    for (const hijo of Array.from(cuerpo.children)) ro.observe(hijo);
    return () => ro.disconnect();
  });

  // El campo en el que se escribe, a la vista cuando sale el teclado. La capa ya mide el
  // viewport visual, así que la hoja se apoya encima del teclado y el cuerpo encoge;
  // pero el campo se quedaba donde estaba, debajo del nuevo borde, en Chrome de Android
  // (Safari lo desplaza él solo). Es `ui.MostrarFoco` de Fyne: lo justo, sin saltos.
  useLayoutEffect(() => {
    const vv = typeof window === 'undefined' ? undefined : window.visualViewport;
    if (!open || !vv) return;
    const alCambiar = () => {
      const cuerpo = cuerpoRef.current;
      const activo = document.activeElement as HTMLElement | null;
      if (!cuerpo || !activo || !cuerpo.contains(activo) || !activo.matches(CAMPO)) return;
      requestAnimationFrame(() => activo.scrollIntoView({ block: 'nearest' }));
    };
    vv.addEventListener('resize', alCambiar);
    return () => vv.removeEventListener('resize', alCambiar);
  }, [open]);

  if (!open || typeof document === 'undefined') return null;

  // Bajar la hoja con el dedo: solo desde el asa y la cabecera —el cuerpo es suyo para
  // desplazarse— y solo con el dedo o el lápiz; con el ratón, arrastrar la cabecera de
  // una tarjeta centrada no tiene que moverla. Un diálogo que no se cierra con un clic
  // fuera (`dismissable={false}`) tampoco se cierra así.
  const empezarArrastre = (e: PointerEvent) => {
    if (!dismissable || e.pointerType === 'mouse' || !matchMedia(HOJA).matches) return;
    if ((e.target as HTMLElement).closest('button, a, input, select, textarea')) return;
    arrastreRef.current = { y: e.clientY, dy: 0, t: e.timeStamp };
    (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
  };
  const arrastrar = (e: PointerEvent) => {
    const a = arrastreRef.current;
    const panel = panelRef.current;
    if (!a || !panel) return;
    // Solo hacia abajo: hacia arriba la hoja ya está en su sitio.
    a.dy = Math.max(0, e.clientY - a.y);
    panel.style.transition = 'none';
    panel.style.transform = `translateY(${a.dy}px)`;
  };
  const soltarArrastre = (e: PointerEvent) => {
    const a = arrastreRef.current;
    const panel = panelRef.current;
    arrastreRef.current = null;
    if (!a || !panel) return;
    panel.style.transition = 'transform 150ms var(--ease-standard)';
    panel.style.transform = '';
    const ms = e.timeStamp - a.t;
    const tiron = a.dy > TIRON_MINIMO && ms > 0 && a.dy / ms > VELOCIDAD_CIERRE;
    if (a.dy > CIERRE_ARRASTRE || tiron) onCloseRef.current();
  };
  // Un gesto cancelado (el sistema se lo ha quedado) devuelve la hoja y no cierra.
  const cancelarArrastre = () => {
    const panel = panelRef.current;
    arrastreRef.current = null;
    if (!panel) return;
    panel.style.transition = 'transform 150ms var(--ease-standard)';
    panel.style.transform = '';
  };
  const zonaArrastre = {
    onPointerDown: empezarArrastre,
    onPointerMove: arrastrar,
    onPointerUp: soltarArrastre,
    onPointerCancel: cancelarArrastre,
  };

  // Filete transparente cuando no hace falta, y no ausente: así aparecer o desaparecer
  // no mueve nada de sitio.
  const filete = desborda ? 'border-border-subtle' : 'border-transparent';
  // La última banda se lleva la zona segura de abajo (la barra de inicio de un iPhone):
  // en la hoja, las acciones no pueden quedar debajo de ella.
  const bandaFinal = 'max-md:pb-[max(1.25rem,calc(env(safe-area-inset-bottom)_+_0.75rem))]';

  return createPortal(
    <div
      // Alto y origen = viewport VISUAL (los publica la shell), no `inset-0`:
      // Safari puede desplazar ese viewport al enfocar un campo. Sin el origen,
      // el alto sería correcto pero el diálogo seguiría anclado arriba y dejaría
      // una franja entre su pie y el teclado. Sin shell, `100dvh` desde 0.
      class={cx(
        'sytue-safe-overlay fixed inset-x-0 top-[var(--sytue-vvy,0px)] flex h-[var(--sytue-vvh,100dvh)]',
        // Teléfono: la hoja apoyada abajo, con aire arriba para que se lea como una capa
        // encima de la pantalla y no como una pantalla nueva.
        'max-md:items-end max-md:pt-[max(2.5rem,calc(env(safe-area-inset-top)_+_1rem))]',
        placement === 'right'
          ? 'md:items-stretch md:justify-end md:p-4'
          : 'md:items-center md:justify-center md:p-4',
      )}
      style={{ zIndex: 'var(--z-modal)' }}
    >
      <div
        class="sytue-fade-in absolute inset-0 bg-[var(--sytue-overlay)]"
        onClick={dismissable ? onClose : undefined}
        aria-hidden="true"
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        class={cx(
          'sytue-glass-surface sytue-hoja-movil relative flex max-h-full w-full flex-col',
          'border-border border bg-[var(--sytue-panel-surface,var(--sytue-surface))] focus-visible:outline-none',
          // Escritorio: el panel recorta y es el cuerpo el que se desplaza. En el
          // teléfono el panel ENTERO se puede desplazar, pero solo como último recurso:
          // el cuerpo no baja de 2,75rem (ver abajo), así que solo pasa cuando cabecera y
          // pie ya no caben juntos —horizontal con el teclado fuera—, y entonces se llega
          // a todo en vez de que se monten.
          'overflow-hidden max-md:overflow-y-auto max-md:overscroll-contain',
          // `dvh` y no `vh` en el contenedor, y aquí un tope en %: en Chrome Android `vh`
          // es la altura CON la barra de direcciones retraída, y el 90% de esa altura
          // dejaba la fila de botones por debajo del borde (regla 4 de lint-design.sh).
          placement === 'right' ? 'sytue-panel-in' : 'sytue-modal-in md:max-h-[90%]',
          'md:rounded-card md:shadow-[var(--shadow-xl)]',
          sizes[size],
          // `rounded-b-none` explícito: con el dedo el foco entra en el panel, y el
          // `:focus-visible` de theme.css le ponía 4 px en las esquinas de abajo. El aro
          // que le ponía también lo quita `.sytue-hoja-movil` en styles.css.
          'max-md:rounded-t-card max-md:max-w-none max-md:rounded-b-none max-md:border-x-0 max-md:border-b-0',
        )}
      >
        {dismissable ? (
          <div
            class="flex shrink-0 touch-none justify-center pt-2 md:hidden"
            aria-hidden="true"
            {...zonaArrastre}
          >
            <span class="bg-border-strong h-1 w-9 rounded-full" />
          </div>
        ) : null}
        <div
          class={cx(
            'flex shrink-0 items-center justify-between gap-3 border-b px-6 pt-5 pb-2',
            dismissable && 'max-md:touch-none max-md:pt-2',
            filete,
          )}
          {...zonaArrastre}
        >
          <h2 class="sytue-h4 text-text min-w-0">{title}</h2>
          <Button
            variant="ghost"
            size="sm"
            iconOnly
            aria-label={t('ui.close')}
            onClick={onCloseButtonClick ?? onClose}
            // El cierre usa el azul de las acciones, independiente del fondo del panel.
            style={{ color: 'var(--sytue-primary-text)' }}
            class="hover:bg-chrome-hover active:bg-chrome-active shrink-0"
          >
            <Icon name="close" size={18} />
          </Button>
        </div>
        <div
          ref={avisosRef}
          data-sytue-dialog-toasts
          class="max-h-[min(25dvh,10rem)] shrink-0 overflow-y-auto overscroll-contain empty:hidden"
        />
        <div
          ref={cuerpoRef}
          data-sytue-dialog-body
          class={cx(
            'text-text min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pt-3 text-base max-md:min-h-11',
            footer ? 'pb-3' : ['pb-5', bandaFinal],
          )}
        >
          {children}
        </div>
        {footer ? (
          <div
            class={cx(
              'flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-6 pt-2 pb-5',
              PIE_TELEFONO,
              filete,
              bandaFinal,
            )}
          >
            {footer}
          </div>
        ) : null}
      </div>
    </div>,
    document.body,
  );
}
