/**
 * Dropdown / Menu — menus de accion y de cuenta. role=menu con el patrón de
 * teclado de WAI-ARIA: flechas, Inicio/Fin, escribir para saltar al item que
 * empieza así, Enter activa (nativo de <a>/<button>), Esc cierra y devuelve el
 * foco al disparador, Tab cierra y sigue desde él. Foco itinerante: los items
 * no están en el orden de Tab. Cierre al click fuera. docs/design.md §9.
 *
 * En el TELÉFONO (por debajo de 768, `useEnHoja`) el menú no cuelga de su botón: es una
 * HOJA inferior, como los diálogos —a todo el ancho, apoyada abajo, redonda arriba, con
 * el velo detrás y la zona segura—, y cada acción mide 44 con el dedo. Un menú colgado de
 * un botón de 32 en una pantalla de 390 tapaba lo que se estaba mirando y dejaba las
 * acciones a la altura del pulgar solo por casualidad (barrido de diálogos y menús,
 * 2026-10-16). El teclado, el foco y el cierre son los mismos.
 *
 * MODO CONTROLADO (`abiertoEn` + `onCerrar`): el menú contextual. Sin disparador, se abre
 * en un PUNTO de la ventana (donde se hizo clic derecho, o la celda activa con Mayús+F10)
 * y lo abre y lo cierra quien lo usa. Es el mismo menú —encaje en la ventana, teclado,
 * hoja en el teléfono—, no un segundo componente: al cerrar con Escape, Tab o una acción,
 * el foco vuelve a lo que lo tenía al abrirse (la rejilla de Calcus).
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { createPortal } from 'preact/compat';
import { cx } from '../lib/cx.js';
import { plegar } from '@sytue/i18n';
import { tecladoEnOtraSuperposicion } from '../lib/atajos.js';
import { limitesViewport, observarViewport } from '../lib/viewport.js';
import { useMobileBack } from '../lib/useMobileBack.js';
import { useEnHoja } from '../lib/useEnHoja.js';
import { useUltimo } from '../lib/useUltimo.js';
import { Icon } from './Icon.js';

/** Aire mínimo entre el panel y el borde de la ventana (px). */
const MARGEN_VENTANA = 8;
/** Por debajo de esto un panel ya no se puede usar: antes tapa el disparador. */
const ALTO_MINIMO = 120;

type Lados = { top: number; bottom: number; left: number; right: number };

/** Si a un panel que se desplaza le queda contenido por debajo de lo que enseña. */
const quedaMasAbajo = (el: HTMLElement): boolean =>
  el.scrollHeight - el.scrollTop - el.clientHeight > 1;

/**
 * Dónde va el panel y cuánto puede medir de alto.
 *
 * EL PANEL NO TAPA SU DISPARADOR. Se queda en el hueco de un lado —abajo, salvo que no
 * quepa y arriba haya más sitio— y lo que no entra ahí se desplaza por dentro. Antes
 * el alto máximo era el de la ventana y el panel se subía para caber: el menú de
 * cuenta, al crecer, se plantaba encima de su propio botón (1280×800, 2026-10-05) y ya
 * no se podía cerrar pulsando donde se abrió.
 *
 * `panel` es la caja tal como se pinta SIN este tope (con los de `panelClass`).
 */
export function encajarPanel(
  d: Lados,
  panel: { ancho: number; alto: number },
  ventana: { ancho: number; alto: number },
  side: 'top' | 'bottom',
  align: 'start' | 'end',
): { top: number; left: number; altoMaximo: number } {
  // Los 6 px entre el disparador y el panel van como literal, y `d.bottom + 6` tal cual:
  // es el contrato que lee el menú de las nativas (apps/fyne/ui/desplegable_test.go).
  const sitioAbajo = ventana.alto - d.bottom - 6 - MARGEN_VENTANA;
  const sitioArriba = d.top - 6 - MARGEN_VENTANA;
  // ARRIBA SI NO CABE ABAJO, sin que el llamante tenga que pedirlo. `side` sigue
  // existiendo para quien SABE que quiere abrir hacia arriba, pero el caso corriente es
  // este: un menú cerca del pie de la ventana —el de «más opciones» de una
  // videollamada de Rooms— se desplegaba hacia abajo y quedaba medio fuera.
  const arriba = side === 'top' || (panel.alto > sitioAbajo && sitioArriba > sitioAbajo);
  // Con menos de ALTO_MINIMO en el lado elegido no hay hueco que respetar: el panel
  // se queda dentro de la ventana, que es lo que resuelve el tope de más abajo.
  const altoMaximo = Math.min(
    Math.max(0, ventana.alto - 2 * MARGEN_VENTANA),
    Math.max(arriba ? sitioArriba : sitioAbajo, ALTO_MINIMO),
  );
  const alto = Math.min(panel.alto, altoMaximo);
  const tope = (v: number, max: number): number =>
    Math.max(MARGEN_VENTANA, Math.min(v, max - MARGEN_VENTANA));
  return {
    top: tope(arriba ? d.top - 6 - alto : d.bottom + 6, ventana.alto - alto),
    left: tope(align === 'end' ? d.right - panel.ancho : d.left, ventana.ancho - panel.ancho),
    altoMaximo,
  };
}

/**
 * Dónde se abre el menú controlado, en px de la ventana (`clientX`/`clientY`). Un punto
 * (el puntero) o, con `ancho` y `alto`, una caja (la celda activa con Mayús+F10), de la
 * que el panel cuelga como de un botón, sin taparla.
 */
export interface PuntoDeMenu {
  x: number;
  y: number;
  ancho?: number | undefined;
  alto?: number | undefined;
}

/**
 * La caja de un PUNTO para `encajarPanel`: el panel sale con la esquina en el punto
 * (abajo y a la derecha, o arriba si no cabe), sin los 6 px que separan el panel de un
 * botón. Un menú contextual se abre donde está el puntero, no al lado.
 */
export const cajaDePunto = (p: PuntoDeMenu): Lados =>
  p.alto === undefined
    ? { top: p.y + 6, bottom: p.y - 6, left: p.x, right: p.x }
    : { top: p.y, bottom: p.y + p.alto, left: p.x, right: p.x + (p.ancho ?? 0) };

export interface DropdownProps {
  /**
   * Disparador: recibe props para accesibilidad (aria-expanded, onClick). Opcional solo
   * en el modo controlado (`abiertoEn`), donde lo abre quien lo usa.
   */
  trigger?: (props: {
    onClick: () => void;
    'aria-expanded': boolean;
    'aria-haspopup': 'menu';
  }) => ComponentChildren;
  /**
   * Contenido del menu. Como render-prop recibe `close` para cerrarse desde dentro
   * (mismo contrato que Popover): lo necesita el item que abre un modal o un dialogo
   * del sistema y debe dejar el menu recogido. Los hijos como nodo siguen igual: el
   * menu solo se cierra con Esc o al hacer click fuera.
   */
  children: ComponentChildren | ((close: () => void) => ComponentChildren);
  align?: 'start' | 'end';
  /**
   * Lado del disparador donde se despliega el menu. 'bottom' (por defecto) es el caso
   * normal; 'top' lo necesita el cromo anclado al pie de la pantalla (p.ej. el menu
   * «+» del compositor de Rooms), donde abrir hacia abajo lo dejaria fuera de vista.
   */
  side?: 'top' | 'bottom';
  /** Etiqueta accesible del menu. */
  label?: string;
  /** Clases del envoltorio (p. ej. ocultar el menú entero en un breakpoint). */
  class?: string;
  /**
   * Clases del PANEL. Para acotar un menú largo más que la ventana (el de 26
   * idiomas de www): la altura máxima por defecto es la de la ventana y el resto
   * se desplaza.
   */
  panelClass?: string;
  /**
   * MODO CONTROLADO, el menú contextual: con un punto, abierto ahí; con `null`, cerrado.
   * Sin la prop (undefined), el menú es el de siempre y lo abre su disparador.
   */
  abiertoEn?: PuntoDeMenu | null;
  /** Lo llama el menú controlado para cerrarse: Escape, Tab, clic fuera o una acción. */
  onCerrar?: () => void;
}

/**
 * El panel como HOJA del teléfono: a todo el ancho, apoyado en el borde de abajo del
 * viewport VISUAL (el teclado no lo tapa), redondo arriba y sin filetes a los lados ni
 * abajo, con la zona segura y acciones de 44. Con `!` porque las clases del menú colgado
 * van en el mismo elemento y dos utilidades del mismo tipo se resuelven por la hoja
 * generada, no por el orden de la cadena.
 */
const HOJA_MENU = cx(
  'sytue-hoja-movil inset-x-0! top-auto! bottom-[calc(100dvh_-_var(--sytue-vvy,0px)_-_var(--sytue-vvh,100dvh))]! max-h-[calc(var(--sytue-vvh,100dvh)*0.8)]! max-w-none! min-w-0!',
  'rounded-t-card! rounded-b-none! border-x-0! border-b-0! p-2! pb-[max(0.5rem,env(safe-area-inset-bottom))]!',
  '[&_[role=menuitem]]:min-h-11',
);

/**
 * Los ítems del menú que mueven las flechas: las acciones y también las opciones
 * marcables (`menuitemradio`, `menuitemcheckbox`), que es como WAI-ARIA llama a una
 * paleta o a una lista de casillas DENTRO de un menú (los colores de la pestaña de
 * hoja). Sin ellas, las muestras quedaban fuera del recorrido del teclado.
 */
const ITEMS = ['menuitem', 'menuitemradio', 'menuitemcheckbox'].map((r) => `[role="${r}"]`);
const SELECTOR_ITEMS = ITEMS.join(',');
const SELECTOR_VIVOS = ITEMS.map((s) => `${s}:not([disabled])`).join(',');

/** Texto comparable para «escribir para saltar»: sin mayúsculas ni tildes («e» encuentra «Español»). */
const plano = (s: string): string => plegar(s).trim();

export function Dropdown({
  trigger,
  children,
  align = 'end',
  side = 'bottom',
  label,
  class: cls,
  panelClass,
  abiertoEn,
  onCerrar,
}: DropdownProps): JSX.Element {
  const controlado = abiertoEn !== undefined;
  const [abiertoPropio, setOpen] = useState(false);
  const open = controlado ? abiertoEn !== null : abiertoPropio;
  /** Cierra el menú; en el controlado, avisando a quien lo abrió. */
  const cerrar = (): void => {
    if (controlado) onCerrar?.();
    else setOpen(false);
  };
  const cerrarRef = useUltimo(cerrar);
  useMobileBack(open, () => cerrarRef.current(), { nivel: 2 });
  const enHoja = useEnHoja();
  const menuRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLDivElement>(null);
  const tecleado = useRef({ texto: '', hasta: 0 });
  // EL FOCO DEL MENÚ CONTROLADO, DE VUELTA A LA CELDA. Lo que tenía el foco al abrirse se
  // apunta ANTES de que el efecto de abajo lo mueva al primer ítem (uno de layout corre
  // antes que los normales), y al cerrarse —ya fuera el panel— recupera el foco si se ha
  // quedado en el body: es lo que pasa tras una acción, que cierra y quita el ítem
  // enfocado. Si la acción abrió un diálogo, el foco ya es suyo y no se le quita. Va en
  // la limpieza de un efecto de layout, que Preact corre al confirmar el cierre, y no en
  // el `close` que reciben los hijos: ese se crea durante el render.
  useLayoutEffect(() => {
    if (!controlado || !open) return;
    const activo = document.activeElement;
    const previo = activo instanceof HTMLElement ? activo : null;
    return () => {
      const ahora = document.activeElement;
      if (previo?.isConnected && (ahora === null || ahora === document.body)) {
        previo.focus({ preventScroll: true });
      }
    };
  }, [controlado, open]);
  const esMenuPuro = (): boolean =>
    !menuRef.current?.querySelector('input,select,textarea,[contenteditable="true"]');

  // El encaje en la ventana lo hace `colocar()` unas líneas más abajo, no
  // `useFitInViewport`: ese empuja con `translate` sobre una caja anclada al
  // disparador, y aquí la caja ya se calcula en coordenadas de ventana, así que
  // sumaría el desplazamiento dos veces. Sigue haciendo falta acotar por lo
  // mismo de siempre: un menú ancho (el centro de notificaciones mide 330px)
  // colgado de un disparador que no está en el borde arranca en coordenada
  // negativa, y lo recortado NO se alcanza desplazando.

  useEffect(() => {
    if (!open) return;
    // Lo que tenía el foco al abrirse (aún no se ha movido al primer ítem): adonde vuelve
    // con Escape o Tab en el modo controlado. En el otro, el disparador.
    const activo = document.activeElement;
    const previo = controlado && activo instanceof HTMLElement ? activo : null;
    const volver = (): void => {
      const destino = controlado
        ? previo
        : triggerRef.current?.querySelector<HTMLElement>('button,a,[tabindex]');
      if (destino?.isConnected) destino.focus({ preventScroll: true });
    };
    // Dentro del menú, el clic derecho (o Mayús+F10 sobre un ítem) no abre el del navegador
    // encima del nuestro.
    const sinMenuNativo = (e: MouseEvent): void => {
      if (menuRef.current?.contains(e.target as Node)) e.preventDefault();
    };
    const onClickOutside = (e: MouseEvent) => {
      const t = e.target as Node;
      if (!menuRef.current?.contains(t) && !triggerRef.current?.contains(t)) {
        cerrarRef.current();
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (tecladoEnOtraSuperposicion(menuRef.current)) return;
      if (e.key === 'Escape') {
        // Solo cierra el menú: sin esto, dentro de un <dialog> modal el mismo
        // Escape cerraba también el diálogo (el visor de Fotos entero).
        e.preventDefault();
        e.stopImmediatePropagation();
        cerrarRef.current();
        volver();
        return;
      }
      if (e.key === 'Tab' && esMenuPuro()) {
        // SIN preventDefault: el foco vuelve al disparador y el Tab del navegador
        // sigue desde ahí. El panel está en un portal al final del body, así que
        // sin esto Tab saltaba del menú a la barra de direcciones. Un panel con
        // controles de formulario (el <select> de idioma del menú de cuenta)
        // conserva el Tab nativo: es la única forma de llegar a ellos.
        cerrarRef.current();
        volver();
        return;
      }
      // Con el foco dentro de un control de formulario del menu (el <select> de
      // idioma, el switch de tema), las flechas son SUYAS: mover el foco al
      // siguiente item impediria elegir una opcion con el teclado.
      const focused = document.activeElement;
      if (
        focused instanceof HTMLSelectElement ||
        focused instanceof HTMLInputElement ||
        focused instanceof HTMLTextAreaElement
      ) {
        return;
      }
      const items = Array.from(
        menuRef.current?.querySelectorAll<HTMLElement>(SELECTOR_VIVOS) ?? [],
      );
      if (items.length === 0) return;
      const idx = items.indexOf(document.activeElement as HTMLElement);
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        (items[(idx + 1) % items.length] ?? items[0]!).focus();
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        (items[(idx - 1 + items.length) % items.length] ?? items[0]!).focus();
      } else if (e.key === 'Home' || e.key === 'PageUp') {
        e.preventDefault();
        items[0]!.focus();
      } else if (e.key === 'End' || e.key === 'PageDown') {
        e.preventDefault();
        items[items.length - 1]!.focus();
      } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
        // Escribir para saltar: las teclas seguidas en menos de medio segundo se
        // acumulan («ne» → Nederlands, no la siguiente con «e»). Una sola letra
        // repetida recorre las que empiezan por ella.
        const ahora = Date.now();
        const t = tecleado.current;
        t.texto = ahora < t.hasta ? t.texto + e.key : e.key;
        t.hasta = ahora + 500;
        const buscado = plano(t.texto);
        if (buscado === '') return;
        e.preventDefault();
        const repetida = /^(.)\1*$/u.exec(buscado);
        const clave = repetida ? repetida[1]! : buscado;
        const desde = repetida ? idx + 1 : Math.max(idx, 0);
        const orden = [...items.slice(desde), ...items.slice(0, desde)];
        orden.find((el) => plano(el.textContent ?? '').startsWith(clave))?.focus();
      }
    };
    document.addEventListener('mousedown', onClickOutside);
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('contextmenu', sinMenuNativo);
    // Foco itinerante en un menú puro: los items salen del orden de Tab y el foco
    // lo mueven las flechas. Se hace aquí y no en MenuItem porque MenuItem
    // también se usa suelto (la cinta de Docs, el menú de mensaje de Rooms), con
    // su propio Tab.
    if (esMenuPuro()) {
      menuRef.current
        ?.querySelectorAll<HTMLElement>(SELECTOR_ITEMS)
        .forEach((el) => (el.tabIndex = -1));
    }
    // Foco al item marcado como actual (el idioma en uso) o, si no hay, al
    // primero. `focus()` además lo desplaza a la vista dentro del panel.
    //
    // AL PRIMERO VIVO, NO AL PRIMERO. Un botón inerte no acepta el foco y
    // `focus()` no hace nada: el foco se quedaba donde estaba y el menú abierto
    // no recibía el teclado. En el «Más» del editor de Notes, cuando «Deshacer»
    // cabe en la fila, la primera opción es «Rehacer», inerte sin nada que
    // rehacer: el foco seguía en el texto, y el Escape que debía cerrar el menú
    // lo recibía el editor y terminaba la edición (640×320, 2026-11-24).
    (
      menuRef.current?.querySelector<HTMLElement>(
        '[role="menuitem"][aria-current="true"]:not([disabled])',
      ) ?? menuRef.current?.querySelector<HTMLElement>(SELECTOR_VIVOS)
    )?.focus();
    return () => {
      document.removeEventListener('mousedown', onClickOutside);
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('contextmenu', sinMenuNativo);
    };
  }, [open, controlado, cerrarRef]);

  // EL PANEL VA A UN PORTAL, Y NO ES UN ADORNO.
  //
  // Anclado dentro de la fila, `z-index: 1000` no sirve de nada en cuanto un
  // ancestro crea un contexto de apilamiento: el z-index solo compite DENTRO de
  // ese contexto, y las filas siguientes —que van después en el DOM— se pintan
  // encima del menú abierto. Medido en producción el 2026-09-13 sobre Drive: el
  // panel salía opaco y con z-index 1000, y aun así `elementFromPoint` en su
  // centro devolvía `SPAN.text-text-muted` (la columna «modificado») de una fila
  // de más abajo. El culpable era el contenedor de la lista virtualizada
  // —`DIV.absolute.top-0.left-0.w-full` con un `transform` para colocar las
  // filas—, y bastan un `transform`, un `filter`, un `opacity` menor que 1 o un
  // `will-change` para reproducirlo. Es un bug que reaparece cada vez que
  // alguien envuelve una lista en algo que se anima.
  //
  // En el body no hay ancestro que atrape nada. Y `position: fixed` SOLO no
  // valdría: un `transform` en un ancestro también le crea bloque contenedor,
  // así que el panel seguiría preso. Hace falta sacarlo del árbol.
  //
  // SALVO DENTRO DE UN <dialog> MODAL, donde va al propio diálogo: `showModal()`
  // lo sube a la capa superior y deja inerte todo lo demás, así que un menú en el
  // body quedaba DEBAJO y no se podía tocar. Medido el 2026-09-15 en el visor de
  // Sytue Fotos: «Añadir a álbum», «Descargar» y «Fondo» abrían un panel de 220px
  // y `elementFromPoint` en su centro devolvía el diálogo.
  const [caja, setCaja] = useState<{ top: number; left: number } | null>(null);
  // Si queda contenido por debajo de lo que se ve. Un panel que se desplaza por dentro
  // lo tiene que ENSEÑAR: en macOS y en el móvil la barra no se pinta hasta que se
  // mueve, y el menú de cuenta se quedaba con «Cerrar sesión» bajo el pliegue sin que
  // nada dijera que había más.
  const [masAbajo, setMasAbajo] = useState(false);
  // El <dialog> que contiene al disparador, si lo hay. Se busca al montar —un nodo no
  // cambia de antepasados en toda su vida— y no leyendo la ref en el render, donde en
  // el primer pase aún está vacía.
  const [dialogo, setDialogo] = useState<HTMLDialogElement | null>(null);
  useLayoutEffect(() => {
    setDialogo(triggerRef.current?.closest('dialog') ?? null);
  }, []);

  const puntoX = abiertoEn?.x;
  const puntoY = abiertoEn?.y;
  const puntoAncho = abiertoEn?.ancho;
  const puntoAlto = abiertoEn?.alto;
  useLayoutEffect(() => {
    if (!open || enHoja) return;
    const colocar = (e?: Event): void => {
      // En el controlado, el «disparador» es el punto: no se mueve con el desplazamiento
      // (el menú contextual se queda donde se pidió, como en el escritorio).
      const d =
        puntoX !== undefined && puntoY !== undefined
          ? cajaDePunto({ x: puntoX, y: puntoY, ancho: puntoAncho, alto: puntoAlto })
          : triggerRef.current?.getBoundingClientRect();
      const el = menuRef.current;
      if (!d || !el) return;
      // El desplazamiento DEL PROPIO PANEL no mueve a su disparador, y recolocar aquí lo
      // rompía: para medir se quita el tope de alto un instante, el panel crece, y el
      // navegador le recorta el `scrollTop` a lo que da ese alto. A 1280×800 el menú de
      // cuenta volvía arriba con cada evento y «Cerrar sesión» no se alcanzaba nunca
      // (medido el 2026-10-05: 0 px desplazados de 40).
      if (e?.type === 'scroll' && e.target instanceof Node && el.contains(e.target)) {
        setMasAbajo(quedaMasAbajo(el));
        return;
      }
      // Se mide SIN nuestro tope, para saber lo que el panel quiere medir (con los
      // topes de `panelClass`, que son de quien lo pidió), y solo entonces se pone. Y
      // con `offset*`, no con la caja pintada: la animación de entrada arranca al 98 %
      // de escala, y un panel que se pasaba del hueco por menos de eso no se acotaba.
      const desplazado = el.scrollTop;
      const v = limitesViewport();
      const anchoMaximo = Math.max(0, v.width - 2 * MARGEN_VENTANA);
      el.style.maxWidth = `${anchoMaximo}px`;
      el.style.minWidth = `${Math.min(220, anchoMaximo)}px`;
      el.style.maxHeight = '';
      const alto = el.offsetHeight;
      const { top, left, altoMaximo } = encajarPanel(
        {
          top: d.top - v.top,
          bottom: d.bottom - v.top,
          left: d.left - v.left,
          right: d.right - v.left,
        },
        { ancho: el.offsetWidth, alto },
        { ancho: v.width, alto: v.height },
        side,
        // Desde un punto, el panel crece hacia la derecha (y vuelve si no cabe).
        controlado ? 'start' : align,
      );
      if (alto > altoMaximo) el.style.maxHeight = `${altoMaximo}px`;
      // Lo mismo al recolocar por un cambio de fuera (la ventana, la página): lo ya
      // desplazado dentro del panel se conserva.
      el.scrollTop = desplazado;
      setCaja({ top: top + v.top, left: left + v.left });
      setMasAbajo(quedaMasAbajo(el));
    };
    colocar();
    // En CAPTURA: la lista de Drive y la de Mail no scrollean la ventana, sino
    // un div interno. Sin `true` el panel se quedaba clavado mientras la fila a
    // la que pertenece se iba hacia arriba.
    return observarViewport(colocar);
  }, [open, side, align, enHoja, controlado, puntoX, puntoY, puntoAncho, puntoAlto]);

  const panel = open ? (
    <div
      ref={menuRef}
      role="menu"
      aria-label={label}
      class={cx(
        enHoja ? HOJA_MENU : null,
        // Elevación nivel 2 (dropdown/popover): shadow-md, como AppGrid.
        // El max-w es condición del clamp: sin él, apartar del borde izquierdo
        // un panel más ancho que la ventana lo sacaría por el derecho.
        'sytue-pop-in border-border fixed max-w-[calc(100vw-16px)] min-w-[220px] rounded-lg border',
        // La altura, por lo mismo: un menú más alto que su hueco se desplaza dentro,
        // en vez de salirse por abajo sin forma de alcanzarlo. Este es el tope hasta
        // que `colocar()` mide; el del hueco lo pone él (ver `encajarPanel`).
        'max-h-[calc(var(--sytue-vvh,100dvh)-16px)] overflow-y-auto overscroll-contain',
        'sytue-menu-surface p-1 shadow-[var(--shadow-md)]',
        panelClass,
      )}
      // Fuera de la ventana hasta que se mide: `useLayoutEffect` coloca el panel
      // ANTES del pintado, así que este valor no llega a verse; sin él, el primer
      // fotograma saldría pegado a la esquina superior izquierda de la página.
      style={
        enHoja
          ? { zIndex: 'var(--z-modal)' }
          : {
              zIndex: 'var(--z-dropdown)',
              top: caja ? `${caja.top}px` : '-9999px',
              left: caja ? `${caja.left}px` : '-9999px',
            }
      }
    >
      {typeof children === 'function' ? children(cerrar) : children}
      {masAbajo ? (
        // El aviso: el borde inferior se funde con la superficie y lleva una flecha. Va
        // pegado al pie de lo que se ve (`sticky`) y no ocupa sitio (`-mt-8`); al llegar
        // al final desaparece, así que no tapa el último elemento.
        <div
          aria-hidden="true"
          class="text-text-muted pointer-events-none sticky bottom-0 -mt-8 flex h-8 items-end justify-center bg-linear-to-t from-[var(--sytue-menu-surface)] from-35% to-transparent"
        >
          <Icon name="chevronDown" size={14} />
        </div>
      ) : null}
    </div>
  ) : null;

  return (
    <div class={cx(trigger ? 'relative inline-flex' : 'contents', cls)}>
      {/* `min-w-0`: si quien lo usa deja encoger el desplegable (`class`), el
          disparador encoge con él y su rótulo se recorta dentro. Sin él, esta capa
          medía siempre el rótulo entero y el «Gespeichert» de la cabecera de Docus
          empujaba «Teilen» 12 px fuera de un teléfono de 320 (barrido móvil). Sin
          `class` que lo permita no cambia nada: la capa de fuera sigue midiendo el
          contenido. */}
      <div ref={triggerRef} class={trigger ? 'inline-flex min-w-0' : 'contents'}>
        {trigger?.({
          onClick: () => (controlado ? cerrar() : setOpen((v) => !v)),
          'aria-expanded': open,
          'aria-haspopup': 'menu',
        })}
      </div>
      {panel && typeof document !== 'undefined'
        ? createPortal(
            enHoja ? (
              <>
                {/* El velo de los diálogos; tocarlo cierra (es «fuera» del menú). */}
                <div
                  class="sytue-fade-in fixed inset-0 overscroll-contain bg-[var(--sytue-overlay)]"
                  style={{ zIndex: 'var(--z-modal)' }}
                  aria-hidden="true"
                />
                {panel}
              </>
            ) : (
              panel
            ),
            dialogo ?? document.body,
          )
        : null}
    </div>
  );
}

export interface MenuItemProps extends Omit<
  JSX.ButtonHTMLAttributes<HTMLButtonElement>,
  'icon' | 'size'
> {
  icon?: JSX.Element;
  /** Item destructivo (p.ej. cerrar sesion). */
  danger?: boolean;
  /** Densidad: 'md' (por defecto, app shell) o 'sm' (chrome denso de los editores). */
  size?: 'sm' | 'md';
  /** Si se indica, el item se renderiza como enlace <a href> en vez de <button>
   *  (mismos estilos/rol); útil para acciones de navegación dentro del menú. */
  href?: string;
  /** Descarga del enlace, conservando el nombre sugerido por el servidor. */
  download?: string | boolean;
}

export function MenuItem({
  icon,
  danger,
  size = 'md',
  href,
  download,
  class: cls,
  children,
  ...rest
}: MenuItemProps): JSX.Element {
  const klass = cx(
    'flex w-full items-center gap-2 rounded-md text-left transition-colors duration-150',
    size === 'sm' ? 'px-2.5 py-1.5 text-sm' : 'px-3 py-2 text-base',
    // Con el dedo, 44 de alto como un botón (medían 33 y 26).
    'pointer-coarse:min-h-[max(44px,2.75rem)]',
    'active:bg-border focus-visible:outline-none',
    'disabled:pointer-events-none disabled:opacity-50',
    // El ítem destructivo tiñe también su hover/foco (spec §Dropdown).
    danger
      ? 'text-danger-text hover:bg-danger-bg focus-visible:bg-danger-bg'
      : 'text-text hover:bg-bg-subtle focus-visible:bg-bg-subtle',
    cls,
  );
  const inner = (
    <>
      {icon ? (
        <span class={cx('shrink-0', danger ? 'text-danger-text' : 'text-text-muted')}>{icon}</span>
      ) : null}
      {children}
    </>
  );
  // Variante enlace: mismos estilos y role=menuitem (participa en la navegación por
  // teclado del Dropdown). Se comparten solo los handlers comunes (onClick).
  if (href !== undefined) {
    return (
      <a
        role="menuitem"
        href={href}
        download={download === true ? '' : download || undefined}
        class={klass}
        {...(rest as unknown as JSX.HTMLAttributes<HTMLAnchorElement>)}
      >
        {inner}
      </a>
    );
  }
  return (
    <button type="button" role="menuitem" class={klass} {...rest}>
      {inner}
    </button>
  );
}

export function MenuSeparator(): JSX.Element {
  return <div class="bg-border-subtle -mx-1 my-1 h-px" role="separator" />;
}
