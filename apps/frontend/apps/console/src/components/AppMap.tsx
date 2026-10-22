/**
 * AppMap — el mapa de la suite: quién sirve cada producto.
 *
 * La información es un MAPEO BIPARTITO —aplicaciones conectadas a la izquierda,
 * productos de la suite a la derecha— y por eso se pinta como tal, con conectores entre
 * las dos columnas: es la misma forma que usan las herramientas de flujos no-code para
 * decir «esto alimenta a aquello», y se entiende sin leer nada.
 *
 * # POR QUÉ EL ARRASTRE NO ES LA ÚNICA FORMA DE HACERLO
 *
 * Arrastrar de un puerto a otro es el gesto natural del idioma, pero como ÚNICA vía
 * dejaría fuera el teclado, el lector de pantalla y buena parte del uso táctil. Aquí el
 * arrastre es un ATAJO encima de la interacción real: cada producto es un botón que abre
 * su lista de alternativas, y esa ruta funciona con Tab y Enter. El mapa se puede operar
 * entero sin tocar el ratón; quien lo tenga, además puede tirar del conector.
 *
 * # SOBRIEDAD
 *
 * Nada de lienzo infinito, zoom ni minimapa: hay doce productos y un puñado de
 * conexiones, así que cabe entero en pantalla y esas piezas solo añadirían cromo que
 * estorba. Los conectores son hairlines de un color de token, más gruesos y en latón
 * cuando están activos. Sin degradados ni sombras: docs/design.md §1-2.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { Icon, EmptyState, Spinner, cx, useMobileBack, useToast } from '@sytue/ui';
import { appForKey } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { ProductChoice, SubstitutionOption } from '../lib/gateway.js';

interface AppMapProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Productos con al menos una alternativa. Vacío ⇒ no hay nada que elegir todavía. */
  choices: ProductChoice[];
  /** false ⇒ la pasarela no contestó; se avisa en vez de fingir un mapa correcto. */
  live: boolean;
  /** Sin permiso de escritura el mapa es de solo lectura (sigue siendo informativo). */
  canEdit: boolean;
  onChanged: () => void;
}

/** Coordenadas de un puerto en el sistema de referencia del lienzo. */
interface Port {
  x: number;
  y: number;
}

/** Un tramo del arrastre en curso, para pintar el conector que sigue al cursor. */
interface Dragging {
  instanceId: string;
  from: Port;
  to: Port;
  /** Producto sobre el que se soltaría ahora mismo (resalta el destino válido). */
  over: string;
}

/** Nombre del producto nativo, para el nodo «Sytue» y los textos. */
function productName(key: string): string {
  return appForKey(key)?.name ?? key;
}

/**
 * curve dibuja el conector entre dos puertos. Bézier horizontal —tangentes en x— porque
 * las columnas son verticales: una recta cruzaría los nodos intermedios y una curva con
 * tangente vertical saldría del puerto en la dirección equivocada.
 */
function curve(from: Port, to: Port): string {
  const dx = Math.max(40, Math.abs(to.x - from.x) * 0.5);
  return `M ${from.x} ${from.y} C ${from.x + dx} ${from.y}, ${to.x - dx} ${to.y}, ${to.x} ${to.y}`;
}

export default function AppMap({ t, choices, live, canEdit, onChanged }: AppMapProps): JSX.Element {
  const toast = useToast();
  const canvasRef = useRef<HTMLDivElement>(null);
  const providerRefs = useRef(new Map<string, HTMLElement>());
  const productRefs = useRef(new Map<string, HTMLElement>());
  const [ports, setPorts] = useState<{ providers: Map<string, Port>; products: Map<string, Port> }>(
    { providers: new Map(), products: new Map() },
  );
  const [drag, setDrag] = useState<Dragging | null>(null);
  const [busy, setBusy] = useState('');
  const [openMenu, setOpenMenu] = useState('');

  const cerrarMenu = useCallback(() => {
    productRefs.current.get(openMenu)?.focus();
    setOpenMenu('');
  }, [openMenu]);
  useMobileBack(Boolean(openMenu), cerrarMenu);

  // Proveedores únicos: una app que sirve siete productos es UN nodo con siete
  // conectores, no siete nodos. Lo contrario convertiría Google Workspace en una
  // columna entera y el mapa dejaría de leerse de un vistazo.
  const providers = new Map<string, SubstitutionOption>();
  for (const c of choices) {
    for (const o of c.options) if (!providers.has(o.instanceId)) providers.set(o.instanceId, o);
  }

  /** Mide los puertos tras cada render que pueda moverlos (layout, no efecto: se lee
   *  geometría y pintar después evitaría un parpadeo del conector). */
  const measure = useCallback(() => {
    const canvas = canvasRef.current?.getBoundingClientRect();
    if (!canvas) return;
    const read = (map: Map<string, HTMLElement>, side: 'right' | 'left'): Map<string, Port> => {
      const out = new Map<string, Port>();
      for (const [key, el] of map) {
        const r = el.getBoundingClientRect();
        out.set(key, {
          x: (side === 'right' ? r.right : r.left) - canvas.left,
          y: r.top + r.height / 2 - canvas.top,
        });
      }
      return out;
    };
    setPorts({
      providers: read(providerRefs.current, 'right'),
      products: read(productRefs.current, 'left'),
    });
  }, []);

  useLayoutEffect(measure, [measure, choices]);
  useEffect(() => {
    // El mapa se re-mide al cambiar el tamaño de la ventana: los conectores son
    // coordenadas absolutas y sin esto quedarían colgando tras un resize.
    const onResize = (): void => measure();
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [measure]);

  /*
   * Cerrar el menú de un producto: con Escape y al tocar fuera.
   *
   * Sin esto solo se cerraba eligiendo una opción o volviendo a pulsar el mismo
   * botón, así que quien lo abría por error se quedaba con un desplegable pegado
   * encima del mapa —tapando justo la fila de abajo— hasta acertar con el botón
   * exacto. Con teclado no había ninguna salida.
   *
   * `pointerdown` y no `mousedown`: en táctil el segundo no llega hasta que el
   * navegador decide que no era un gesto de scroll, y el menú se quedaba abierto
   * medio segundo después del toque. El foco vuelve al botón que lo abrió: al
   * cerrar por Escape el elemento enfocado desaparece del árbol y el foco caería
   * al body, perdiendo el sitio en la página.
   */
  useEffect(() => {
    if (!openMenu) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') cerrarMenu();
    };
    const onDown = (e: PointerEvent): void => {
      const t = e.target;
      if (t instanceof Element && t.closest('[data-producto]')) return;
      setOpenMenu(''); // sin devolver el foco: el dedo/ratón ya está en otro sitio
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onDown);
    };
  }, [openMenu, cerrarMenu]);

  const apply = useCallback(
    async (product: string, instanceId: string): Promise<void> => {
      setBusy(product);
      try {
        const res = await fetch(
          `/api/admin/apps/substitutions?product=${encodeURIComponent(product)}`,
          {
            method: 'PUT',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ instance_id: instanceId }),
          },
        );
        if (!res.ok) throw new Error(String(res.status));
        toast.notify({
          tone: 'success',
          title: t(instanceId ? 'appmap.toast.switched' : 'appmap.toast.reverted', {
            product: productName(product),
          }),
        });
        onChanged();
      } catch {
        toast.notify({
          tone: 'danger',
          title: t('appmap.toast.failed'),
          description: t('appmap.toast.failedHint'),
        });
      } finally {
        setBusy('');
        setOpenMenu('');
      }
    },
    [onChanged, t, toast],
  );

  // ---- Arrastre (atajo; la ruta accesible es el menú de cada producto) ----

  const onPointerDown = (e: PointerEvent, instanceId: string): void => {
    if (!canEdit) return;
    const from = ports.providers.get(instanceId);
    if (!from) return;
    e.preventDefault();
    const canvas = canvasRef.current?.getBoundingClientRect();
    if (!canvas) return;

    const move = (ev: PointerEvent): void => {
      const to = { x: ev.clientX - canvas.left, y: ev.clientY - canvas.top };
      // Destino válido = producto bajo el cursor que ESTA aplicación puede servir. Se
      // resuelve durante el arrastre para poder resaltarlo: soltar sobre un producto
      // que el proveedor no sirve no debe parecer posible.
      let over = '';
      for (const c of choices) {
        if (!c.options.some((o) => o.instanceId === instanceId)) continue;
        const el = productRefs.current.get(c.product);
        if (!el) continue;
        const r = el.getBoundingClientRect();
        if (
          ev.clientX >= r.left &&
          ev.clientX <= r.right &&
          ev.clientY >= r.top &&
          ev.clientY <= r.bottom
        ) {
          over = c.product;
          break;
        }
      }
      setDrag({ instanceId, from, to, over });
    };
    const up = (ev: PointerEvent): void => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      setDrag((cur) => {
        if (cur?.over) void apply(cur.over, instanceId);
        return null;
      });
      ev.preventDefault();
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    setDrag({ instanceId, from, to: from, over: '' });
  };

  if (!live) {
    return (
      <EmptyState
        title={t('appmap.offline.title')}
        description={t('appmap.offline.description')}
        illustration={<Icon name="warning" size={28} class="text-danger-text" />}
      />
    );
  }
  if (choices.length === 0) {
    return (
      <EmptyState
        title={t('appmap.empty.title')}
        description={t('appmap.empty.description')}
        illustration={<Icon name="grid" size={28} />}
      />
    );
  }

  return (
    <div class="relative" ref={canvasRef}>
      {/* Conectores. aria-hidden: son la representación VISUAL de un estado que el
          botón de cada producto ya dice con palabras; anunciarlos dos veces sería ruido. */}
      <svg class="pointer-events-none absolute inset-0 h-full w-full" aria-hidden="true">
        {choices.map((c) => {
          if (!c.currentInstanceId) return null;
          const from = ports.providers.get(c.currentInstanceId);
          const to = ports.products.get(c.product);
          if (!from || !to) return null;
          return (
            <path
              key={c.product}
              d={curve(from, to)}
              fill="none"
              stroke="var(--sytue-primary-500)"
              stroke-width="1.5"
            />
          );
        })}
        {drag ? (
          <path
            d={curve(drag.from, drag.to)}
            fill="none"
            stroke="var(--sytue-primary-500)"
            stroke-width="1.5"
            stroke-dasharray="4 3"
          />
        ) : null}
      </svg>

      <div class="relative grid gap-8 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        {/* Columna de aplicaciones conectadas */}
        <section aria-labelledby="mapa-apps" class="flex flex-col gap-2">
          <h3 id="mapa-apps" class="text-text-muted sytue-overline">
            {t('appmap.column.apps')}
          </h3>
          {[...providers.values()].map((p) => (
            <div
              key={p.instanceId}
              ref={(el) => {
                if (el) providerRefs.current.set(p.instanceId, el);
                else providerRefs.current.delete(p.instanceId);
              }}
              class="border-border bg-surface flex items-center gap-3 rounded-md border px-3 py-2.5"
            >
              <span
                class="grid h-8 w-8 shrink-0 place-items-center"
                style={{ color: p.color }}
                aria-hidden="true"
              >
                {/* Como <img>: un SVG ahí no ejecuta nada, y el del manifiesto lo puede
                    escribir quien da de alta una integración, no solo la semilla. */}
                {p.iconUrl ? (
                  <img
                    src={p.iconUrl}
                    alt=""
                    width={24}
                    height={24}
                    class="h-6 w-6 object-contain"
                  />
                ) : null}
              </span>
              <span class="text-text min-w-0 flex-1 truncate text-sm font-medium">{p.name}</span>
              {canEdit ? (
                <button
                  type="button"
                  onPointerDown={(e) => onPointerDown(e, p.instanceId)}
                  aria-hidden="true"
                  tabIndex={-1}
                  title={t('appmap.dragHint')}
                  class="border-border-strong hover:border-primary hover:bg-primary/10 bg-surface -mr-1.5 h-3 w-3 shrink-0 cursor-grab rounded-full border transition-colors duration-150"
                />
              ) : null}
            </div>
          ))}
        </section>

        {/* Columna de productos de la suite */}
        <section aria-labelledby="mapa-productos" class="flex flex-col gap-2">
          <h3 id="mapa-productos" class="text-text-muted sytue-overline">
            {t('appmap.column.products')}
          </h3>
          {choices.map((c) => {
            const current = c.options.find((o) => o.instanceId === c.currentInstanceId);
            const isTarget = drag?.over === c.product;
            return (
              <div key={c.product} class="relative" data-producto={c.product}>
                <button
                  type="button"
                  ref={(el) => {
                    if (el) productRefs.current.set(c.product, el);
                    else productRefs.current.delete(c.product);
                  }}
                  disabled={!canEdit || busy === c.product}
                  aria-expanded={openMenu === c.product}
                  aria-haspopup="menu"
                  onClick={() => setOpenMenu(openMenu === c.product ? '' : c.product)}
                  class={cx(
                    'flex w-full items-center gap-3 rounded-md border px-3 py-2.5 text-left transition-colors duration-150',
                    'sytue-focus-ring',
                    isTarget ? 'border-primary bg-primary/10' : 'border-border bg-surface',
                    canEdit ? 'hover:border-border-strong' : 'cursor-default',
                  )}
                >
                  <span class="text-text min-w-0 flex-1 truncate text-sm font-medium">
                    {productName(c.product)}
                  </span>
                  <span class="text-text-muted shrink-0 text-xs">
                    {busy === c.product ? <Spinner size={14} /> : (current?.name ?? 'Sytue')}
                  </span>
                  {canEdit ? (
                    <Icon name="chevronDown" size={14} class="text-text-subtle shrink-0" />
                  ) : null}
                </button>

                {openMenu === c.product ? (
                  <div
                    role="menu"
                    aria-label={t('appmap.menuAria', { product: productName(c.product) })}
                    class="sytue-menu-surface border-border absolute top-full right-0 left-0 z-10 mt-1 rounded-md border py-1 shadow-[var(--shadow-sm)]"
                  >
                    <MenuOption
                      label={`Sytue ${productName(c.product).replace(/^Sytue /, '')}`}
                      hint={t('appmap.nativeHint')}
                      selected={!c.currentInstanceId}
                      onSelect={() => void apply(c.product, '')}
                    />
                    {c.options.map((o) => (
                      <MenuOption
                        key={o.instanceId}
                        label={o.name}
                        hint={t(
                          o.hasDocumentHandoff ? 'appmap.handoff.full' : 'appmap.handoff.appOnly',
                        )}
                        selected={o.instanceId === c.currentInstanceId}
                        onSelect={() => void apply(c.product, o.instanceId)}
                      />
                    ))}
                  </div>
                ) : null}
              </div>
            );
          })}
        </section>
      </div>

      {canEdit ? <p class="text-text-muted mt-4 text-xs">{t('appmap.footer')}</p> : null}
    </div>
  );
}

/** Una opción del menú de un producto. Botón, no <li role="menuitem"> a mano: el foco y
 *  el teclado los da el elemento nativo. */
function MenuOption({
  label,
  hint,
  selected,
  onSelect,
}: {
  label: string;
  hint: string;
  selected: boolean;
  onSelect: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={selected}
      onClick={onSelect}
      class="hover:bg-bg-subtle focus-visible:bg-bg-subtle flex w-full items-start gap-2.5 px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none"
    >
      <span class="mt-0.5 w-4 shrink-0">
        {selected ? <Icon name="check" size={14} class="text-primary-text" /> : null}
      </span>
      <span class="min-w-0">
        <span class="text-text block truncate text-sm font-medium">{label}</span>
        <span class="text-text-muted block text-xs">{hint}</span>
      </span>
    </button>
  );
}
