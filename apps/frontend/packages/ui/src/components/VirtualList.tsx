/**
 * VirtualList — lista vertical virtualizada sobre @tanstack/virtual-core (MIT).
 * Pinta solo la ventana visible + overscan; obligatorio para listas grandes
 * (mensajes de chat, ficheros de drive, docs) a escala de decenas de miles de
 * filas. Alturas estimadas + medición real por fila (filas variables).
 *
 * El contenedor que desplaza DEBE tener altura acotada (p. ej. `flex-1 min-h-0`
 * o `h-full`). Por defecto es la propia lista; `scrollElementRef` permite usar
 * un padre real para que la cabecera y la lista compartan el mismo scroll.
 *
 * A11y: contenedor role="list" y filas role="listitem" por defecto, con
 * aria-setsize/aria-posinset (el DOM solo contiene una ventana de filas);
 * ambos roles son sobreescribibles y el resto de atributos pasan al contenedor.
 *
 * Chat: `stickToBottom` ancla el scroll al final — arranca abajo, sigue los
 * mensajes nuevos solo si el usuario ya estaba abajo y mantiene la posición
 * estable al medir filas o cargar historial (con `getItemKey` estable).
 */
import type { ComponentChildren, JSX, RefObject } from 'preact';
import {
  useCallback,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'preact/hooks';
import {
  Virtualizer,
  elementScroll,
  observeElementOffset,
  observeElementRect,
} from '@tanstack/virtual-core';
import type {
  VirtualizerOptions,
  ScrollToOptions as VirtualScrollToOptions,
} from '@tanstack/virtual-core';
import { cx } from '../lib/cx.js';

/** Umbral «cerca del fondo» (px) para seguir mensajes nuevos (paridad con el chat actual). */
const NEAR_BOTTOM_PX = 160;

/** Handle imperativo: scroll-to-index (búsqueda, hilos) y fondo (chat). */
export interface VirtualListHandle {
  scrollToIndex: (index: number, opts?: VirtualScrollToOptions) => void;
  scrollToBottom: (behavior?: 'auto' | 'smooth' | 'instant') => void;
  /** true si el scroll está a ≤ threshold px del final (por defecto el umbral interno). */
  isAtBottom: (threshold?: number) => boolean;
}

export interface VirtualListProps<T> extends Omit<
  JSX.HTMLAttributes<HTMLDivElement>,
  'children' | 'role'
> {
  items: readonly T[];
  renderItem: (item: T, index: number) => ComponentChildren;
  /** Altura estimada de fila en px, o por índice; la real se mide del DOM. */
  estimateSize: number | ((index: number) => number);
  /** Filas extra renderizadas fuera del viewport por cada lado. */
  overscan?: number;
  /**
   * Clave estable por índice (id del item). Imprescindible para insertar por
   * arriba sin saltos (historial de chat). Memoizar: es clave de caché.
   */
  getItemKey?: (index: number) => string | number;
  /** Chat: ancla al final; arranca abajo y sigue los items nuevos si ya estaba abajo. */
  stickToBottom?: boolean;
  /** role del contenedor (por defecto "list"). */
  role?: JSX.AriaRole;
  /** role de cada fila (por defecto "listitem"). */
  itemRole?: JSX.AriaRole;
  /** Recibe el handle imperativo (scrollToIndex / scrollToBottom / isAtBottom). */
  handleRef?: RefObject<VirtualListHandle>;
  /** Padre que ya desplaza: la lista reserva su alto y no crea otro scroll. */
  scrollElementRef?: RefObject<HTMLDivElement>;
}

export function VirtualList<T>({
  items,
  renderItem,
  estimateSize,
  overscan = 6,
  getItemKey,
  stickToBottom = false,
  role = 'list',
  itemRole = 'listitem',
  handleRef,
  scrollElementRef,
  class: cls,
  ...rest
}: VirtualListProps<T>): JSX.Element {
  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [scrollMargin, setScrollMargin] = useState(0);
  const [, setTick] = useState(0);

  const estimate = useMemo(
    () => (typeof estimateSize === 'function' ? estimateSize : () => estimateSize),
    [estimateSize],
  );

  // Estable y FUERA del render: `setOptions` corre en cada render (patrón adapter
  // oficial) y una closure nueva que lee la ref se le pasaría en mitad del render.
  const getScrollElement = useCallback(
    () => (scrollElementRef ? scrollElementRef.current : scrollRef.current),
    [scrollElementRef],
  );

  const measureScrollMargin = useCallback(() => {
    const parent = scrollElementRef?.current;
    const content = contentRef.current;
    // El rectángulo es de ventana; scrollTop lo devuelve a coordenadas del
    // contenido del padre. clientTop descuenta su borde, que no desplaza.
    const next =
      parent && content
        ? content.getBoundingClientRect().top -
          parent.getBoundingClientRect().top -
          parent.clientTop +
          parent.scrollTop
        : 0;
    setScrollMargin((previous) => (previous === next ? previous : next));
  }, [scrollElementRef]);

  // También tras cambiar las filas/columnas o las props del contenedor. El scroll
  // del padre no cambia el margen: ambos rectángulos se miden en el mismo sistema.
  useLayoutEffect(measureScrollMargin, [measureScrollMargin, items, cls, rest.style]);
  useLayoutEffect(() => {
    const parent = scrollElementRef?.current;
    const content = contentRef.current;
    if (!parent || !content) return;
    const view = parent.ownerDocument.defaultView;
    const observer = view?.ResizeObserver ? new view.ResizeObserver(measureScrollMargin) : null;
    observer?.observe(parent);
    // Observar solo la lista no ve que una cabecera de FUERA crezca. Se observan
    // las ramas anteriores de cada ancestro hasta el padre que desplaza: incluye
    // galerías, avisos y filtros, sin recorrer las filas virtualizadas.
    for (
      let branch: Element | null = content;
      branch && branch !== parent;
      branch = branch.parentElement
    ) {
      observer?.observe(branch);
      for (
        let previous = branch.previousElementSibling;
        previous;
        previous = previous.previousElementSibling
      ) {
        observer?.observe(previous);
      }
    }
    view?.addEventListener('resize', measureScrollMargin);
    return () => {
      observer?.disconnect();
      view?.removeEventListener('resize', measureScrollMargin);
    };
  }, [scrollElementRef, measureScrollMargin]);

  const options: VirtualizerOptions<HTMLDivElement, HTMLElement> = {
    count: items.length,
    getScrollElement,
    initialOffset: () => getScrollElement()?.scrollTop ?? 0,
    scrollMargin,
    estimateSize: estimate,
    overscan,
    scrollToFn: elementScroll,
    observeElementRect,
    observeElementOffset,
    onChange: () => setTick((t) => t + 1),
  };
  if (getItemKey) options.getItemKey = getItemKey;
  if (stickToBottom) {
    // anchorTo end + followOnAppend: el core sigue los appends solo si el
    // usuario estaba a ≤ scrollEndThreshold del fondo, y mantiene el fondo
    // estable mientras las filas se miden o llega historial por arriba.
    options.anchorTo = 'end';
    options.followOnAppend = true;
    options.scrollEndThreshold = NEAR_BOTTOM_PX;
  }

  // Instancia única; las opciones se refrescan en cada render (patrón adapter oficial).
  const [virtualizer] = useState(() => new Virtualizer<HTMLDivElement, HTMLElement>(options));
  virtualizer.setOptions(options);

  useLayoutEffect(() => virtualizer._didMount(), [virtualizer]);
  useLayoutEffect(() => {
    virtualizer._willUpdate();
  });

  // Chat: posicionarse al fondo en el primer pintado.
  const didInitialStickRef = useRef(false);
  useLayoutEffect(() => {
    if (stickToBottom && !didInitialStickRef.current && items.length > 0) {
      didInitialStickRef.current = true;
      virtualizer.scrollToEnd({ behavior: 'instant' });
    }
  }, [stickToBottom, items.length, virtualizer]);

  const handle = useMemo<VirtualListHandle>(
    () => ({
      scrollToIndex: (index, opts) => virtualizer.scrollToIndex(index, opts),
      scrollToBottom: (behavior = 'auto') => virtualizer.scrollToEnd({ behavior }),
      isAtBottom: (threshold) => virtualizer.isAtEnd(threshold),
    }),
    [virtualizer],
  );
  useImperativeHandle(handleRef ?? null, () => handle, [handle]);

  const virtualItems = virtualizer.getVirtualItems();
  // TanStack ya resta scrollMargin del alto total. Volver a restarlo quitaría
  // filas al fondo; sumarlo repetiría aquí el alto de la cabecera del padre.
  const totalSize = virtualizer.getTotalSize();

  return (
    <div
      ref={scrollRef}
      role={role}
      class={cx(!scrollElementRef && 'overflow-y-auto overscroll-contain', cls)}
      {...rest}
    >
      <div ref={contentRef} class="relative w-full" style={{ height: `${totalSize}px` }}>
        {virtualItems.map((vi) => {
          const item = items[vi.index];
          if (item === undefined) return null;
          return (
            <div
              key={vi.key}
              data-index={vi.index}
              ref={virtualizer.measureElement}
              role={itemRole}
              aria-setsize={items.length}
              aria-posinset={vi.index + 1}
              class="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${vi.start - virtualizer.options.scrollMargin}px)` }}
            >
              {renderItem(item, vi.index)}
            </div>
          );
        })}
      </div>
    </div>
  );
}
