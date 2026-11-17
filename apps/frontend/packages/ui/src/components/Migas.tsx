/**
 * Migas — la ruta de carpetas de un explorador: la raíz, los niveles intermedios y el
 * actual (`aria-current="page"`), separados por galones. docs/design.md §2.7.
 *
 * CUANDO NO CABE, PLIEGA. Con una ruta de ocho niveles en un teléfono, el `truncate`
 * de cada segmento los dejaba TODOS en «Pro…», «Doc…», «Con…»: ocho botones de los que
 * no se leía ninguno. Ahora, si la ruta entera no entra en su hueco, se queda en
 * «raíz › … › padre › actual», y el «…» abre un menú con los niveles que esconde. Lo
 * que se lee es justo lo que hace falta: dónde empieza, dónde estás y a dónde subes.
 * Las rutas que caben —casi todas en el escritorio— no cambian al píxel.
 *
 * Se mide con `ResizeObserver` contra una COPIA invisible de la ruta entera, y no con el
 * desbordamiento del propio `<nav>`: los segmentos llevan `min-w-0` y `truncate`, así
 * que la ruta real nunca desborda —se encoge— y el `scrollWidth` no avisa de nada. La
 * copia va recortada dentro de una caja del tamaño del `<nav>`, para que su ancho no
 * cree desplazamiento lateral en la página.
 *
 * Antes cada vista escribía su ruta a mano (el explorador de Drive, las unidades de
 * equipo); esta es la pieza común. Su par nativo es `ui.Migas` (apps/fyne/ui/migas.go).
 */
import type { JSX } from 'preact';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Dropdown, MenuItem } from './Dropdown.js';
import { Icon } from './Icon.js';

/** Un nivel de la ruta. */
export interface Miga {
  /** Identidad estable del nivel (su ruta o su id): la `key` del segmento. */
  clave: string;
  /** Nombre que se pinta, ya decodificado. */
  nombre: string;
  /** Ir a ese nivel. */
  onIr: () => void;
}

export interface MigasProps {
  /** `aria-label` del `<nav>`, ya traducido. */
  label: string;
  /** Los niveles, de la raíz al actual. El último es el actual. */
  migas: Miga[];
  /** Clases del `<nav>`: su sitio en la cabecera de quien lo usa. */
  class?: string;
  /**
   * Tope de ancho de CADA segmento, raíz incluida (`max-w-[40vw]`). Sin él la raíz
   * mide lo que su rótulo y solo se recortan los demás, que es la ruta del explorador.
   */
  acotar?: string;
  /** Un galón también delante de la raíz: la ruta va detrás de un «Volver». */
  galonInicial?: boolean;
}

/** Con menos de cuatro niveles no hay nada que plegar entre la raíz y el padre. */
const MINIMO_PARA_PLEGAR = 4;

/**
 * Qué se ve de una ruta de `n` niveles: los índices visibles y los que esconde el «…».
 * Plegada es siempre raíz, «…», padre y actual; sin plegar, todos.
 */
export function pliegueDeMigas(
  n: number,
  plegada: boolean,
): { visibles: number[]; ocultos: number[] } {
  const todos = Array.from({ length: n }, (_, i) => i);
  if (!plegada || n < MINIMO_PARA_PLEGAR) return { visibles: todos, ocultos: [] };
  return { visibles: [0, n - 2, n - 1], ocultos: todos.slice(1, n - 2) };
}

const BASE =
  'hover:bg-surface-sunken active:bg-surface-sunken sytue-focus-ring rounded-control px-2 py-1 transition-colors duration-150';
const tinta = (actual: boolean): string =>
  actual ? 'text-text font-semibold' : 'text-text-muted hover:text-text font-medium';

function Galon(): JSX.Element {
  return <Icon name="chevronRight" size={14} class="text-text/50 shrink-0" />;
}

export function Migas({ label, migas, class: cls, acotar, galonInicial }: MigasProps): JSX.Element {
  const t = useT();
  const navRef = useRef<HTMLElement>(null);
  const medidaRef = useRef<HTMLDivElement>(null);
  const [plegada, setPlegada] = useState(false);
  // Lo que mide la ruta entera, para la base del `<nav>` mientras está plegada.
  const [natural, setNatural] = useState(0);
  const plegable = migas.length >= MINIMO_PARA_PLEGAR;
  // La firma de la ruta: si cambia (se entra en otra carpeta), se vuelve a medir.
  const firma = migas.map((m) => m.nombre).join('\u0000');

  useLayoutEffect(() => {
    const nav = navRef.current;
    const medida = medidaRef.current;
    if (!plegable || !nav || !medida) {
      setPlegada(false);
      return;
    }
    const medir = (): void => {
      // Medio píxel de holgura: los anchos fraccionarios de la caja de texto.
      const natural = medida.getBoundingClientRect().width;
      const hueco = nav.getBoundingClientRect().width;
      setNatural(natural);
      setPlegada(natural - hueco > 0.5);
    };
    medir();
    if (typeof ResizeObserver === 'undefined') return;
    // La copia también: cambia de ancho cuando cargan las fuentes.
    const ro = new ResizeObserver(medir);
    ro.observe(nav);
    ro.observe(medida);
    return () => ro.disconnect();
  }, [plegable, firma]);

  const ultimo = migas.length - 1;
  const raizClase = acotar ? cx('truncate', acotar) : 'whitespace-nowrap';
  const segmento = (m: Miga, i: number): JSX.Element => {
    const actual = i === ultimo;
    const boton = (
      <button
        type="button"
        onClick={m.onIr}
        aria-current={actual ? 'page' : undefined}
        title={i === 0 && !acotar ? undefined : m.nombre}
        class={cx(BASE, i === 0 ? raizClase : cx('truncate', acotar), tinta(actual))}
      >
        {m.nombre}
      </button>
    );
    if (i === 0) return boton;
    return (
      <span key={m.clave} class="flex min-w-0 items-center gap-0.5">
        <Galon />
        {boton}
      </span>
    );
  };

  const { visibles, ocultos } = pliegueDeMigas(migas.length, plegada);
  const raiz = migas[0];
  const nombreOcultos = t('ui.migas.ocultas');

  return (
    <nav
      ref={navRef}
      aria-label={label}
      class={cx('flex min-w-0 items-center gap-0.5 text-base', plegable && 'relative', cls)}
      // Plegada, la base `auto` sería la de la ruta corta: en una cabecera `flex-wrap`
      // el `<nav>` subiría a compartir línea, el hueco medido menguaría y la ruta ya no
      // se desplegaría aunque entera cupiese en su línea. Con la base de la ruta entera
      // ocupa el mismo sitio plegada o no, y la medida no se engaña a sí misma.
      style={plegada && natural > 0 ? { flexBasis: `${natural}px` } : undefined}
    >
      {galonInicial ? <Galon /> : null}
      {raiz ? segmento(raiz, 0) : null}
      {ocultos.length > 0 ? (
        <span class="flex shrink-0 items-center gap-0.5">
          <Galon />
          <Dropdown
            align="start"
            label={nombreOcultos}
            trigger={(props) => (
              <button
                type="button"
                {...props}
                aria-label={nombreOcultos}
                title={nombreOcultos}
                class={cx(BASE, 'whitespace-nowrap', tinta(false))}
              >
                …
              </button>
            )}
          >
            {(cerrar) =>
              ocultos.map((i) => {
                const m = migas[i]!;
                return (
                  <MenuItem
                    key={m.clave}
                    onClick={() => {
                      cerrar();
                      m.onIr();
                    }}
                  >
                    <span class="min-w-0 truncate">{m.nombre}</span>
                  </MenuItem>
                );
              })
            }
          </Dropdown>
        </span>
      ) : null}
      {visibles.filter((i) => i > 0).map((i) => segmento(migas[i]!, i))}
      {plegable ? (
        // La ruta entera, invisible y sin encoger: su ancho es lo que la ruta pediría.
        // Fuera del árbol accesible y de la interacción; recortada al `<nav>`.
        <div
          aria-hidden="true"
          class="pointer-events-none invisible absolute inset-0 overflow-hidden"
        >
          <div ref={medidaRef} class="flex w-max items-center gap-0.5">
            {galonInicial ? <Galon /> : null}
            {migas.map((m, i) => (
              <span key={m.clave} class="flex shrink-0 items-center gap-0.5">
                {i > 0 ? <Galon /> : null}
                <span
                  class={cx(
                    BASE,
                    'whitespace-nowrap',
                    acotar && cx('truncate', acotar),
                    tinta(i === ultimo),
                  )}
                >
                  {m.nombre}
                </span>
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </nav>
  );
}
