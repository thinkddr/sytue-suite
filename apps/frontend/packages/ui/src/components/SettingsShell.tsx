/**
 * SettingsShell / SettingsSection — marco de una página de AJUSTES.
 *
 * Evita el «muro de formulario»: ancho de lectura acotado, título y descripción en
 * lenguaje llano, y el contenido troceado en secciones que se explican solas. Cada
 * sección tiene su propia acción a la derecha (guardar por sección, no un botón
 * gigante al final).
 *
 *   <SettingsShell title="Integraciones"
 *     description="Conecta las aplicaciones que ya usa tu equipo…"
 *     actions={<Button>Conectar una aplicación</Button>}>
 *     <SettingsSection title="Aplicaciones conectadas" description="…">
 *       …
 *     </SettingsSection>
 *   </SettingsShell>
 *
 * El título es un `h1` por defecto (la página lo titula); si la página ya tiene su
 * h1 (p. ej. un SectionHeader `level={1}`), pasa `level={2}`.
 *
 * ÍNDICE DE SECCIONES (`indice`, opcional). Una página larga de ajustes —Mi cuenta
 * tiene nueve secciones— necesita lo que tienen Google Account o el Apple ID: un
 * índice que diga dónde estás y anclas que se puedan enlazar («/account#seguridad»).
 * Con `indice`, cada `{id, rotulo}` apunta al elemento hijo con ese `id`:
 *
 *   - en ≥lg es una columna fija a la izquierda; en el teléfono, una fila que se
 *     desliza de lado sobre el contenido (sin desbordar la página a 320 px);
 *   - marca con `aria-current="location"` la sección que se está leyendo;
 *   - al saltar —desde el índice, al abrir con `#id` o con `seccionInicial`— lleva
 *     el FOCO al encabezado de la sección, no solo la vista: quien navega con
 *     teclado o lector de pantalla sigue desde ahí y no desde lo alto de la página.
 *
 * Sin `indice` la página sale exactamente como antes: es la de quince pantallas.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { MASCARA_SCROLL_X, useScrollRestante } from '../lib/scrollX.js';
import { Card, CardBody } from './Card.js';

/** Una entrada del índice: el `id` del elemento al que lleva y su rótulo. */
export interface SettingsIndexEntry {
  id: string;
  rotulo: string;
  /**
   * Otros fragmentos que llevan aquí: los nombres viejos de un ancla que alguien ya
   * enlazó (p. ej. `#app-passwords` desde Sytue Mail) siguen abriendo la sección.
   */
  alias?: string[];
}

export interface SettingsShellProps {
  title: string;
  /** Una o dos frases llanas: qué se configura aquí y para qué sirve. */
  description?: string;
  /** Acción principal de la página (a la derecha del título). */
  actions?: ComponentChildren;
  /** Aviso permanente sobre el contenido (p. ej. servicio no disponible). */
  banner?: ComponentChildren;
  level?: 1 | 2;
  /**
   * Índice de secciones. Cada `id` es el de un elemento del contenido (la tarjeta de
   * la sección); su primer encabezado recibe el foco al saltar.
   */
  indice?: SettingsIndexEntry[];
  /**
   * Sección a la que abrir si la URL no trae ya un `#id` del índice (p. ej. Mi cuenta
   * con `?mfa=required` abre en «Seguridad»). Solo con `indice`.
   */
  seccionInicial?: string | undefined;
  children: ComponentChildren;
  class?: string;
}

export function SettingsShell(props: SettingsShellProps): JSX.Element {
  if (props.indice && props.indice.length > 0) return <SettingsShellConIndice {...props} />;
  const { title, description, actions, banner, level = 1, children, class: cls } = props;
  return (
    <div class={cx('mx-auto w-full max-w-5xl px-4 py-6 sm:px-6', cls)}>
      <CabeceraAjustes title={title} description={description} actions={actions} level={level} />
      {banner ? <div class="mt-4">{banner}</div> : null}
      <div class="mt-6 flex flex-col gap-6">{children}</div>
    </div>
  );
}

/**
 * Cabecera de la página (título, descripción y acción): la misma con y sin índice.
 */
function CabeceraAjustes({
  title,
  description,
  actions,
  level = 1,
}: {
  title: string;
  description?: string | undefined;
  actions?: ComponentChildren;
  level?: 1 | 2;
}): JSX.Element {
  const Heading = `h${level}` as 'h1';
  return (
    <div class="flex flex-wrap items-start gap-4">
      <div class="min-w-0 flex-1">
        <Heading class="sytue-h2 text-text">{title}</Heading>
        {description ? (
          <p class="text-text-muted mt-1.5 max-w-2xl text-base leading-relaxed">{description}</p>
        ) : null}
      </div>
      {actions ? <div class="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
}

/** La sección del índice a la que lleva el fragmento de la URL (por `id` o por alias). */
export function seccionDelFragmento(
  indice: readonly SettingsIndexEntry[],
  hash: string,
): string | null {
  let frag: string;
  try {
    frag = decodeURIComponent(hash.replace(/^#/, ''));
  } catch {
    return null;
  }
  if (!frag) return null;
  return indice.find((e) => e.id === frag || e.alias?.includes(frag))?.id ?? null;
}

/**
 * Lleva la vista a la sección `id` y el foco a su encabezado.
 *
 * El foco va con `preventScroll`: el desplazamiento ya lo hace `scrollIntoView`, que
 * respeta `scroll-margin` y el `prefers-reduced-motion` (salto seco en vez de suave).
 * El encabezado no es enfocable por sí mismo: `tabindex="-1"` lo hace enfocable por
 * programa sin meterlo en el orden del tabulador.
 */
export function irASeccion(id: string): boolean {
  const destino = document.getElementById(id);
  if (!destino) return false;
  const quieto = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
  destino.scrollIntoView?.({ behavior: quieto ? 'auto' : 'smooth', block: 'start' });
  const encabezado = destino.querySelector<HTMLElement>('h1, h2, h3') ?? destino;
  if (!encabezado.hasAttribute('tabindex')) encabezado.setAttribute('tabindex', '-1');
  encabezado.focus({ preventScroll: true });
  return true;
}

/**
 * Tras un salto, el observador no manda durante este tiempo: el desplazamiento
 * suave cruza las secciones de en medio y las marcaría una tras otra, y las últimas
 * —cortas, al fondo— no llegan nunca arriba, así que se marcaría la de antes.
 */
const TREGUA_TRAS_SALTO_MS = 1000;

function SettingsShellConIndice({
  title,
  description,
  actions,
  banner,
  level = 1,
  indice = [],
  seccionInicial,
  children,
  class: cls,
}: SettingsShellProps): JSX.Element {
  const t = useT();
  const ids = indice.map((e) => e.id);
  const clave = ids.join(' ');
  const [activa, setActiva] = useState<string>(ids[0] ?? '');
  const saltoEn = useRef(0);
  // El fragmento que ya se atendió: `hashchange` solo salta cuando de verdad cambia
  // (algunos entornos lo disparan también tras `replaceState`).
  const fragmentoVisto = useRef('');
  const { ref: filaRef, queda } = useScrollRestante<HTMLUListElement>();

  const saltar = (id: string): void => {
    if (!irASeccion(id)) return;
    saltoEn.current = Date.now();
    setActiva(id);
  };

  // Al abrir: el `#id` de la URL manda; si no hay, la sección inicial. Tras pintar
  // (rAF): las tarjetas tienen que existir y haber ocupado su sitio.
  useEffect(() => {
    fragmentoVisto.current = window.location.hash;
    const inicial =
      seccionDelFragmento(indice, window.location.hash) ??
      (seccionInicial && ids.includes(seccionInicial) ? seccionInicial : null);
    if (!inicial) return;
    const raf = requestAnimationFrame(() => saltar(inicial));
    return () => cancelAnimationFrame(raf);
    // Solo al montar: luego manda el usuario.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Atrás/adelante entre anclas, o un enlace `#id` desde dentro de la página.
  useEffect(() => {
    const alCambiar = (): void => {
      if (window.location.hash === fragmentoVisto.current) return;
      fragmentoVisto.current = window.location.hash;
      const id = seccionDelFragmento(indice, window.location.hash);
      if (id) saltar(id);
    };
    window.addEventListener('hashchange', alCambiar);
    return () => window.removeEventListener('hashchange', alCambiar);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clave]);

  // La sección que se lee es la PRIMERA (en orden de página) que asoma por la franja
  // de arriba de la vista. Sin IntersectionObserver el índice sigue llevando a cada
  // sección; solo deja de seguir la lectura.
  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') return;
    const visibles = new Set<string>();
    const io = new IntersectionObserver(
      (entradas) => {
        for (const e of entradas) {
          if (e.isIntersecting) visibles.add(e.target.id);
          else visibles.delete(e.target.id);
        }
        if (Date.now() - saltoEn.current < TREGUA_TRAS_SALTO_MS) return;
        const primera = ids.find((id) => visibles.has(id));
        if (primera) setActiva(primera);
      },
      { rootMargin: '0px 0px -60% 0px' },
    );
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el) io.observe(el);
    }
    return () => io.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clave]);

  // En el teléfono, la entrada activa no se queda escondida al otro lado de la fila.
  useEffect(() => {
    const fila = filaRef.current;
    const enlace = fila?.querySelector<HTMLElement>('[aria-current]');
    if (!fila || !enlace || fila.scrollWidth <= fila.clientWidth) return;
    fila.scrollLeft = enlace.offsetLeft - (fila.clientWidth - enlace.offsetWidth) / 2;
  }, [activa, filaRef]);

  const alPulsar = (e: MouseEvent, id: string): void => {
    // Con modificadores (nueva pestaña, copiar enlace…) el navegador hace lo suyo.
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)
      return;
    e.preventDefault();
    // `pushState` y no `location.hash`: el hash dispararía `hashchange` y un segundo
    // salto. Queda en el historial, como un ancla normal: «Atrás» vuelve a la de antes.
    if (window.location.hash !== `#${id}`) window.history.pushState(null, '', `#${id}`);
    fragmentoVisto.current = window.location.hash;
    saltar(id);
  };

  // Los enlaces `#id` DENTRO del contenido (los avisos de la revisión de seguridad)
  // saltan igual que el índice. Sin esto, si la URL ya trae ese mismo `#id` —p. ej.
  // tras pulsar la entrada del índice—, el navegador no emite `hashchange`: la vista
  // se movía, pero el foco se quedaba en el botón y el índice no se enteraba.
  const alPulsarDentro = (e: MouseEvent): void => {
    const enlace = (e.target as Element | null)?.closest?.('a[href^="#"]');
    const id = enlace && seccionDelFragmento(indice, enlace.getAttribute('href') ?? '');
    if (id) alPulsar(e, id);
  };

  return (
    <div class={cx('mx-auto w-full max-w-5xl px-4 py-6 sm:px-6', cls)}>
      <CabeceraAjustes title={title} description={description} actions={actions} level={level} />
      {banner ? <div class="mt-4">{banner}</div> : null}
      <div class="mt-6 flex flex-col gap-4 lg:grid lg:grid-cols-[12rem_minmax(0,1fr)] lg:items-start lg:gap-8">
        {/* `top-6`: la columna se queda a la vista mientras desplaza el contenido. */}
        <nav aria-label={t('ui.settingsIndex')} class="min-w-0 lg:sticky lg:top-6">
          <ul
            ref={filaRef}
            data-scroll-x
            class={cx(
              'border-border relative flex gap-1 overflow-x-auto border-b',
              'lg:flex-col lg:gap-0.5 lg:overflow-visible lg:border-b-0',
              queda && cx(MASCARA_SCROLL_X, 'lg:[mask-image:none]'),
            )}
          >
            {indice.map(({ id, rotulo }) => {
              const actual = id === activa;
              return (
                <li key={id} class="shrink-0">
                  <a
                    href={`#${id}`}
                    aria-current={actual ? 'location' : undefined}
                    onClick={(e) => alPulsar(e, id)}
                    class={cx(
                      // Como las pestañas (`Tabs`): en el teléfono, subrayado abajo;
                      // en la columna, la raya de «estás aquí» pasa al borde de inicio.
                      'inline-flex items-center border-b-2 px-3 py-2.5 text-sm whitespace-nowrap pointer-coarse:min-h-[max(44px,2.75rem)]',
                      'lg:flex lg:w-full lg:border-s-2 lg:border-b-0 lg:py-1.5 lg:whitespace-normal',
                      'sytue-focus-ring-inner transition-colors duration-150',
                      actual
                        ? 'text-text border-indicator font-medium'
                        : 'text-text-muted hover:border-border-strong hover:text-text border-transparent',
                    )}
                  >
                    {rotulo}
                  </a>
                </li>
              );
            })}
          </ul>
        </nav>
        {/* `scroll-mt`: el salto deja aire sobre la tarjeta en vez de pegarla al borde. */}
        {/* El `onClick` delega en los enlaces internos; no es un control: el
            teclado llega a cada enlace por sí mismo. */}
        {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
        <div
          class="flex min-w-0 flex-col gap-6 *:scroll-mt-4 lg:*:scroll-mt-6"
          onClick={alPulsarDentro}
        >
          {children}
        </div>
      </div>
    </div>
  );
}

export interface SettingsSectionProps {
  title: string;
  /** Qué hace esta sección, en una frase. */
  description?: string;
  /** Acciones de la sección (a la derecha del título). */
  actions?: ComponentChildren;
  /** Sin tarjeta: la sección es solo un encabezado sobre contenido libre. */
  plain?: boolean;
  /** Ancla de la sección: el `id` al que apunta una entrada de `indice`. */
  id?: string;
  children: ComponentChildren;
  class?: string;
}

export function SettingsSection({
  title,
  description,
  actions,
  plain,
  id,
  children,
  class: cls,
}: SettingsSectionProps): JSX.Element {
  const header = (
    <div class="flex flex-wrap items-start gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="sytue-h4 text-text">{title}</h2>
        {description ? (
          <p class="text-text-muted mt-1 max-w-2xl text-sm leading-relaxed">{description}</p>
        ) : null}
      </div>
      {actions ? <div class="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
  if (plain) {
    return (
      <section id={id} class={cx('flex flex-col gap-3', cls)}>
        {header}
        {children}
      </section>
    );
  }
  return (
    <Card id={id} sectionLabel={title} class={cls}>
      <CardBody class="flex flex-col gap-4">
        {header}
        {children}
      </CardBody>
    </Card>
  );
}
