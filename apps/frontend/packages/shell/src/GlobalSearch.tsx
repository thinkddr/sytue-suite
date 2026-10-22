/**
 * Búsqueda global de la shell — consume Sytue Hub (búsqueda federada). Combobox
 * accesible: input + popover con resultados, estado vacío y error con reintento;
 * navegación con flechas y Esc. docs/CONVENTIONS.md §11.
 */
import { Fragment, type JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  appForKey,
  Button,
  cx,
  EmptyState,
  formatBytes,
  Icon,
  Illustration,
  Input,
  Spinner,
  ProductIcon,
  useAtajo,
} from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { Interpretado, SearchResult } from '@sytue/api-clients';
import type { ProductKey } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import { useShell } from './context.js';
import { fichasDe, mereceInterpretarse } from './interpretacion.js';

/** Agrupa los resultados por producto preservando el orden de aparición (ya
 * vienen rankeados globalmente por el Hub: el mejor producto encabeza). */
function groupBySource(results: SearchResult[]): { key: ProductKey; items: SearchResult[] }[] {
  const order: ProductKey[] = [];
  const buckets = new Map<ProductKey, SearchResult[]>();
  for (const r of results) {
    let bucket = buckets.get(r.source);
    if (!bucket) {
      bucket = [];
      buckets.set(r.source, bucket);
      order.push(r.source);
    }
    bucket.push(r);
  }
  return order.map((key) => ({ key, items: buckets.get(key) ?? [] }));
}

/**
 * «Sytue Mail» y no `mail`: la clave interna no le dice nada a quien busca.
 *
 * Y no se enseña el motivo técnico que manda el Hub (`timeout`, `502`…): no le
 * sirve a nadie desde aquí, y es la clase de detalle que no debe salir a una
 * pantalla.
 */
/**
 * La segunda línea de un resultado del Hub —búsqueda o «Reciente»—: el tamaño de un
 * fichero en el idioma de quien lee, o el fragmento. El Hub escribe el tamaño del
 * fragmento siempre igual («1.0 KiB»).
 */
export function detalleDe(locale: Locale, r: SearchResult): string | undefined {
  return r.sizeBytes !== undefined ? formatBytes(locale, r.sizeBytes) : r.snippet;
}

function nombresDe(fallidos: { source: string }[]): string {
  return fallidos.map((f) => appForKey(f.source)?.name ?? f.source).join(', ');
}

export function GlobalSearch(): JSX.Element {
  const { hub } = useShell();
  const { locale, t } = useI18n();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  // Pista del atajo, visible mientras el campo no tiene foco ni texto. Se
  // calcula al montar y no en el servidor: el SSR no sabe si hay un Mac
  // delante, y pintar «Ctrl K» para luego cambiarlo parpadea.
  const [enfocado, setEnfocado] = useState(false);
  /** La segunda vuelta, la que pregunta al modelo. Tarda más y se dice. */
  const [entendiendo, setEntendiendo] = useState(false);
  const atajo = useAtajo('Ctrl K');
  // Orígenes que NO contestaron. `error` es que se cayera el Hub entero; esto es
  // que se cayera UNO de los productos, que es lo normal y lo que hasta ahora se
  // tragaba: la búsqueda es federada, así que sin este aviso «no hay resultados»
  // afirma algo falso cuando el único origen que los tenía no respondió.
  const [fallidos, setFallidos] = useState<{ source: string; error: string }[]>([]);
  /**
   * Lo que la IA entendió de la frase, cuando ha hecho falta preguntárselo.
   *
   * `null` = se buscó literalmente, que es lo normal y lo de siempre.
   */
  const [interpretado, setInterpretado] = useState<Interpretado | null>(null);
  /**
   * «Buscar las palabras exactas»: quien lo pulsa quiere lo literal aunque no
   * haya resultados. Se guarda LA CONSULTA y no un booleano porque al cambiar
   * el texto la preferencia deja de aplicar: es sobre esta búsqueda, no un modo.
   */
  const [literalDe, setLiteralDe] = useState('');
  /**
   * Resultado marcado, por índice.
   *
   * Antes se navegaba moviendo el FOCO del DOM, y eso dejaba dos cosas mal: el
   * `aria-selected` estaba en «false» siempre —mentía a los lectores de pantalla—
   * y al escribir no había nada marcado, aunque Enter abriera el primero. En
   * Spotlight, que es la vara de medir, el primero está resaltado desde el primer
   * carácter: ver dónde vas a caer ANTES de pulsar Enter es la mitad de por qué
   * ese buscador se siente rápido.
   *
   * El foco se queda en el campo (patrón combobox de WAI-ARIA) y quién manda lo
   * dice `aria-activedescendant`. Así seguir escribiendo no exige volver atrás.
   */
  const [activo, setActivo] = useState(0);
  const [attempt, setAttempt] = useState(0);
  const boxRef = useRef<HTMLDivElement>(null);

  // Búsqueda con debounce de 250ms; abre el popover también en vacío/error
  // para dar siempre respuesta visible.
  useEffect(() => {
    if (query.trim().length < 2) {
      setResults([]);
      setError(false);
      setLoading(false);
      setOpen(false);
      setInterpretado(null);
      return;
    }
    setLoading(true);
    const handle = setTimeout(() => {
      void (async () => {
        try {
          const q = query.trim();
          let res = await hub.search(q);
          let entendido: Interpretado | null = null;
          // LA IA ENTRA SOLO CUANDO LA BÚSQUEDA DE SIEMPRE NO ENCUENTRA NADA, y
          // esto es lo que hace que la función salga gratis: mientras buscar
          // funciona, no se gasta ni una llamada ni un milisegundo. Cuando falla
          // —que es cuando alguien ha escrito una frase en vez de una palabra—
          // no hay nada que perder: peor que cero resultados no se puede quedar.
          if (res.results.length === 0 && q !== literalDe && mereceInterpretarse(q)) {
            setEntendiendo(true);
            try {
              const conIa = await hub.search(q, undefined, { ia: true });
              if (conIa.results.length > 0) {
                res = conIa;
                entendido = conIa.interpretado ?? null;
              }
            } finally {
              setEntendiendo(false);
            }
          }
          setResults(res.results);
          setFallidos(res.fallidos);
          setInterpretado(entendido);
          setActivo(0);
          setError(false);
        } catch {
          setResults([]);
          setFallidos([]);
          setInterpretado(null);
          setError(true);
        } finally {
          setLoading(false);
          setOpen(true);
        }
      })();
    }, 250);
    return () => clearTimeout(handle);
  }, [query, hub, attempt, literalDe]);

  /**
   * ⌘K / Ctrl+K abre el buscador desde cualquier app.
   *
   * No lo había, y es el atajo que la gente prueba sin pensarlo — Spotlight,
   * Linear, GitHub, todos—. Sin él, buscar exigía ir con el ratón a la barra
   * superior, que es justo lo que un buscador global viene a evitar.
   *
   * NO se roba el atajo si hay un campo enfocado ni un diálogo abierto: dentro de
   * un modal, ⌘K puede ser el «insertar enlace» del editor, y pisárselo sería
   * cambiar un hueco por un fallo peor.
   */
  useEffect(() => {
    const alPulsar = (e: KeyboardEvent): void => {
      if (e.key.toLowerCase() !== 'k' || !(e.metaKey || e.ctrlKey) || e.altKey) return;
      // Una app que se lo ha quedado antes (Notepads lleva ⌘K a su captura) manda.
      if (e.defaultPrevented) return;
      if (document.querySelector('dialog[open], [role="dialog"]')) return;
      e.preventDefault();
      const campo = boxRef.current?.querySelector('input');
      campo?.focus();
      campo?.select();
    };
    document.addEventListener('keydown', alPulsar);
    return () => document.removeEventListener('keydown', alPulsar);
  }, []);

  useEffect(() => {
    // `pointerdown` y no `mousedown`: el mismo manejador vale para ratón, dedo y
    // lápiz. Con `mousedown`, en táctil el evento llega sintetizado y tarde
    // —después del `touchend`—, así que cerrar al tocar fuera iba con retraso.
    // Es el mismo patrón que dejó inservible el selector de emoji de Rooms, aquí
    // en la pieza que usan las doce apps.
    const alPulsarFuera = (e: PointerEvent) => {
      if (!boxRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('pointerdown', alPulsarFuera);
    return () => document.removeEventListener('pointerdown', alPulsarFuera);
  }, []);

  /**
   * Los resultados en el MISMO orden en que se pintan.
   *
   * Se agrupan por producto al mostrarlos, así que el orden visual no es el que
   * devuelve el servidor. Navegar con el índice del array crudo haría que la
   * flecha abajo saltara de sitio: hay que recorrer lo que se ve.
   */
  const ordenados = groupBySource(results).flatMap((g) => g.items);

  // Teclado: Esc cierra y devuelve el foco al input; flechas recorren resultados;
  // Enter desde el input (sin haber bajado con flechas) abre el primer resultado.
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && open) {
      e.preventDefault();
      setOpen(false);
      boxRef.current?.querySelector('input')?.focus();
      return;
    }
    if (e.key === 'Enter' && open && ordenados.length > 0) {
      e.preventDefault();
      // Se navega al marcado, sin depender de dónde esté el foco.
      const destino = ordenados[activo]?.url;
      if (destino) window.location.assign(destino);
      return;
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    if (ordenados.length === 0) return;
    e.preventDefault();
    // Da la vuelta por los dos extremos: bajar desde el último lleva al primero,
    // como en cualquier lista de este tipo.
    setActivo((i) => (i + (e.key === 'ArrowDown' ? 1 : -1) + ordenados.length) % ordenados.length);
  };

  /**
   * La banda de «entendido como», con una ficha por filtro aplicado y la salida
   * a la búsqueda literal.
   *
   * ES OBLIGATORIA, no un adorno. Una búsqueda que ha acotado a un producto o a
   * un rango de fechas que nadie ha visto devuelve pocos resultados —o ninguno—
   * sin explicar por qué, y quien busca concluye que lo que busca no existe.
   * Enseñarlo Y poder deshacerlo en un clic es lo que convierte una deducción en
   * una ayuda en vez de en una jugarreta.
   */
  const fichas = interpretado
    ? fichasDe(
        interpretado,
        (k) => appForKey(k)?.name.replace('Sytue ', '') ?? k,
        (kind) => t(`search.kind.${kind}` as Parameters<typeof t>[0]),
        (ms) => new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' }),
        {
          fuente: (nombre) => t('search.ai.source', { name: nombre }),
          tipo: (kind) => t('search.ai.kind', { kind }),
          rango: (desde, hasta) => t('search.ai.range', { from: desde, to: hasta }),
          desde: (d) => t('search.ai.since', { from: d }),
          hasta: (h) => t('search.ai.until', { to: h }),
        },
      )
    : [];

  const banda =
    interpretado && fichas.length > 0 ? (
      <div
        role="status"
        class="border-border flex flex-wrap items-center gap-1.5 border-b px-3 py-2 text-xs"
      >
        {/* Sin `label`: es decorativo, el texto de al lado ya lo dice. */}
        <Icon name="ai" size={14} class="text-text-muted shrink-0" />
        <span class="text-text-muted">{t('search.ai.applied')}</span>
        {fichas.map((f) => (
          <span key={f.clave} class="bg-bg-subtle text-text rounded px-1.5 py-0.5">
            {f.texto}
          </span>
        ))}
        <Button
          size="sm"
          variant="ghost"
          class="ml-auto"
          onClick={() => {
            // Se recuerda PARA ESTA CONSULTA: al cambiar el texto vuelve a
            // ofrecerse, porque la preferencia era sobre esta búsqueda.
            setLiteralDe(query.trim());
            setInterpretado(null);
          }}
        >
          {t('search.ai.literal')}
        </Button>
      </div>
    ) : null;

  return (
    <div
      ref={boxRef}
      role="combobox"
      aria-expanded={open}
      aria-haspopup="listbox"
      aria-controls="sytue-search-results"
      aria-owns="sytue-search-results"
      aria-activedescendant={open && ordenados.length > 0 ? `sytue-opt-${activo}` : undefined}
      tabIndex={-1}
      class="relative w-full max-w-[480px]"
      onKeyDown={onKeyDown}
    >
      <Input
        // `id`/`name`: sin ellos Chrome lo marca como campo sin identificar y el
        // autocompletado del navegador no sabe qué es.
        id="sytue-busqueda-global"
        name="q"
        type="search"
        value={query}
        onInput={(e) => setQuery(e.currentTarget.value)}
        onFocus={() => {
          setEnfocado(true);
          if (results.length > 0 || error) setOpen(true);
        }}
        onBlur={() => setEnfocado(false)}
        placeholder={t('search.placeholder')}
        aria-label={t('search.label')}
        // Píldora PERFILADA sobre la barra de tinta —velo y borde blancos—: un campo
        // blanco macizo en mitad de la barra oscura pesaría más que el título de la
        // página.
        //
        // `rounded-pill!` con `!`: el radio base del campo (`rounded-md`) es otra
        // utilidad de la misma propiedad, y entre dos así manda el orden de la hoja
        // generada, no el de la cadena.
        class="border-chrome-border bg-chrome-hover text-on-chrome placeholder:text-on-chrome-muted hover:border-on-chrome-muted rounded-pill!"
        leadingIcon={
          loading || entendiendo ? (
            <Spinner
              size={16}
              label={entendiendo ? t('search.ai.busy') : t('search.busy')}
              class="text-on-chrome-muted"
            />
          ) : (
            <Icon name="search" size={16} class="text-on-chrome-muted" />
          )
        }
      />
      {/* El atajo a la vista, como en Spotlight: quien lo ve una vez ya no lo
          busca en ningún manual. Solo en escritorio —en el móvil no hay
          teclado— y solo mientras el campo está en reposo, que es cuando el
          hueco de la derecha está libre (con texto, `type="search"` pinta ahí
          su aspa de borrar). Decorativo: el atajo real lo escucha `document`. */}
      {atajo && !enfocado && !query ? (
        <kbd
          aria-hidden="true"
          class="border-chrome-border text-on-chrome-muted pointer-events-none absolute top-1/2 right-3 hidden -translate-y-1/2 items-center rounded-full border bg-transparent px-1.5 py-0.5 font-sans text-[11px] leading-none font-medium md:inline-flex"
        >
          {atajo}
        </kbd>
      ) : null}
      {/* Anuncio del resultado para lectores de pantalla. */}
      <div class="sr-only" role="status" aria-live="polite">
        {open && !loading
          ? error
            ? t('search.failedShort')
            : t('search.resultCount', { count: results.length })
          : ''}
      </div>
      {open ? (
        <div
          id="sytue-search-results"
          // En el teléfono, los resultados a todo el ancho bajo la barra (`top-14`, sus
          // 56 px), sin cantos ni filetes laterales: colgados del campo medían lo que él
          // (≈150 px entre la marca y las acciones) y los títulos no se leían.
          class="sytue-pop-in sytue-menu-surface border-border absolute top-[calc(100%+6px)] right-0 left-0 overflow-hidden rounded-lg border shadow-[var(--shadow-md)] max-md:fixed max-md:inset-x-0 max-md:top-14 max-md:rounded-none max-md:border-x-0"
          style={{ zIndex: 'var(--z-popover)' }}
        >
          {error ? (
            <EmptyState
              title={t('search.failed')}
              description={t('error.connection')}
              illustration={<Icon name="danger" size={28} />}
              action={
                <Button variant="secondary" size="sm" onClick={() => setAttempt((n) => n + 1)}>
                  {t('action.retry')}
                </Button>
              }
            />
          ) : results.length === 0 ? (
            <EmptyState
              title={t('search.emptyTitle', { query: query.trim() })}
              // Cuando falta un origen, el «no hay nada» deja de ser una
              // afirmación y pasa a ser lo que de verdad ocurrió.
              description={
                fallidos.length > 0
                  ? t('search.partial', { sources: nombresDe(fallidos) })
                  : t('search.emptyDescription')
              }
              // La misma ilustración de «sin resultados» que el resto de la suite; el
              // icono gris de 28px hacía del vacío una pantalla muerta.
              illustration={<Illustration name="emptySearch" size={96} />}
            />
          ) : (
            <>
              {banda}
              {fallidos.length > 0 ? (
                <p role="status" class="border-border text-text-muted border-b px-3 py-2 text-xs">
                  {t('search.partial', { sources: nombresDe(fallidos) })}
                </p>
              ) : null}
              <ul
                role="listbox"
                aria-label={t('search.results')}
                class="max-h-96 overflow-y-auto p-1 max-md:max-h-[calc(100dvh-8rem)]"
              >
                {groupBySource(results).map((group) => {
                  const app = appForKey(group.key);
                  const name = app ? app.name.replace('Sytue ', '') : group.key;
                  return (
                    <Fragment key={group.key}>
                      <li
                        role="presentation"
                        class="flex items-center gap-1.5 px-2.5 pt-2.5 pb-1 first:pt-1.5"
                      >
                        {app ? <ProductIcon appKey={app.key} size={15} /> : null}
                        {/* El color de producto vive SOLO en la tesela.
                          Pintarlo como TEXTO incumplía WCAG 1.4.3 en tema oscuro: 12 px
                          semibold exige 4,5:1 y los 13 caían a 1,4-3,1:1 sobre el panel. */}
                        <span class="sytue-overline text-text-muted">{name}</span>
                      </li>
                      {group.items.map((r) => {
                        // El índice es el del orden VISUAL, que es por el que se
                        // mueven las flechas.
                        const i = ordenados.indexOf(r);
                        const marcado = i === activo;
                        return (
                          <li
                            key={`${r.source}:${r.id}`}
                            id={`sytue-opt-${i}`}
                            role="option"
                            aria-selected={marcado}
                          >
                            <a
                              href={r.url}
                              // El resalte del marcado NO es un `:hover` ni un foco:
                              // el foco se queda en el campo para poder seguir
                              // escribiendo, así que hace falta un estado propio.
                              class={cx(
                                'active:bg-surface-sunken block rounded-md px-2.5 py-1.5 transition-colors duration-150 focus-visible:outline-none',
                                marcado ? 'bg-selected' : 'hover:bg-bg-subtle',
                              )}
                              onMouseEnter={() => setActivo(i)}
                            >
                              <span
                                class="text-text block truncate text-base font-medium"
                                title={r.title}
                              >
                                {r.title}
                              </span>
                              {detalleDe(locale, r) ? (
                                <span
                                  class="text-text-muted block truncate text-sm"
                                  title={detalleDe(locale, r)}
                                >
                                  {detalleDe(locale, r)}
                                </span>
                              ) : null}
                            </a>
                          </li>
                        );
                      })}
                    </Fragment>
                  );
                })}
              </ul>
            </>
          )}
        </div>
      ) : null}
    </div>
  );
}
