/**
 * Una tesela de la galería del lanzador: el acceso a un producto de la suite.
 *
 * Vive aparte de `Launcher.tsx` por la tesela DESACTIVADA. Antes era el mismo
 * enlace sin `href`, al 40 % de opacidad, con una píldora «Desactivado» y el
 * porqué solo en el `title`: quien no pasa el puntero por encima —en el móvil,
 * nadie; con el teclado, nadie, porque un `<a>` sin `href` ni siquiera recibe el
 * foco— veía un producto apagado sin saber qué hacer con él. Ahora el motivo se
 * lee en la propia tesela y lleva a la ficha del producto, que lo explica.
 *
 * La tesela desactivada NO es un enlace: dentro va otro enlace (la ayuda), y un
 * `<a>` dentro de otro no es HTML válido. Es un grupo con nombre cuyo único
 * elemento enfocable es la ayuda, y esa ayuda entra en la navegación con flechas
 * de la rejilla (`data-hub-product`) como cualquier otra tesela.
 *
 * Aquí vive también la tesela de una APP CONECTADA (un producto de terceros dado de alta
 * por el admin). Iba con su propia caja —`rounded-lg border-border bg-surface`, sin la
 * forma compacta de Marco y fuera de las flechas— y, al lado de las propias, se leía como
 * de otro sitio. Ahora comparte la caja y la navegación con la de producto.
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { ProductIcon, cx, type SuiteApp } from '@sytue/ui';
import { useT } from '../i18n/index.js';
import { ayudaSoloOrganizaciones } from '../lib/teselas.js';

/** App conectada (integración) tal como la sirve el gateway vía /api/my-apps: misma
 *  forma que un SuiteApp + estado. key es el id de instancia (no un ProductKey). */
export interface AppConectada {
  key: string;
  name: string;
  description: string;
  href: string;
  iconUrl: string;
  color: string;
  status: string;
}

export interface ProductoDeLaGaleria {
  key: SuiteApp['key'];
  name: string;
  description: string;
  href: string;
  color: string;
  /** Producto solo de organizaciones visto desde una cuenta personal. */
  disabled: boolean;
}

/** La caja común: misma geometría activa o no, para que la rejilla no baile. */
const CAJA = cx(
  // En «Marco», tesela compacta: el icono a la izquierda (42 px) y el
  // nombre y la descripción, en una línea cada uno, a la derecha.
  'hub-product-card group border-border-subtle rounded-card flex h-full flex-col items-start gap-3 border p-4 transition-colors',
  'marco:flex-row marco:items-center marco:px-3 marco:py-2.5 marco:[&>span:first-child]:scale-105',
);

/** Las teclas que mueven el foco por la galería. */
const TECLAS_GALERIA = 'ArrowLeft ArrowRight ArrowUp ArrowDown Home End';

/**
 * Las flechas, Inicio y Fin entre las teselas de la galería.
 *
 * El ámbito es el contenedor `[data-hub-galeria]`, que abarca la rejilla propia y la de
 * apps conectadas: buscando desde el `ul` de la tesela, las flechas no pasaban de una a
 * otra. Sin ese contenedor (una rejilla suelta) vale su `ul`.
 */
export function navegarProducto(e: KeyboardEvent): void {
  if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
  if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(e.key)) return;
  const actual = e.currentTarget as HTMLAnchorElement;
  const ambito = actual.closest('[data-hub-galeria]') ?? actual.closest('ul');
  const enlaces = Array.from(
    ambito?.querySelectorAll<HTMLAnchorElement>('a[data-hub-product][href]') ?? [],
  );
  const i = enlaces.indexOf(actual);
  if (i < 0) return;
  let destino: HTMLAnchorElement | undefined;
  if (e.key === 'Home') destino = enlaces[0];
  else if (e.key === 'End') destino = enlaces[enlaces.length - 1];
  else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight')
    destino = enlaces[i + (e.key === 'ArrowLeft' ? -1 : 1)];
  else {
    // La FILA la marca la tesela (su `li`), no el enlace: en la desactivada el
    // enfocable es su «Más información», que va más abajo que el borde de la tesela
    // y, medido por sí solo, contaría como de la fila siguiente.
    const caja = (el: HTMLElement): DOMRect => (el.closest('li') ?? el).getBoundingClientRect();
    const r = caja(actual);
    const dir = e.key === 'ArrowUp' ? -1 : 1;
    destino = enlaces
      .map((el) => ({ el, r: caja(el) }))
      .filter((c) => (c.r.top - r.top) * dir > 1)
      .sort(
        (a, b) =>
          Math.abs(a.r.top - r.top) - Math.abs(b.r.top - r.top) ||
          Math.abs(a.r.left - r.left) - Math.abs(b.r.left - r.left),
      )[0]?.el;
  }
  e.preventDefault();
  destino?.focus();
}

/** La caja de 40 de `ProductIcon size={40}` y su distintivo: de 4 a 36, radio 9. */
const MARCA = { width: '40px', height: '40px' };
const DISTINTIVO = { inset: '4px', borderRadius: '9px' };

/** La inicial de reserva de una app conectada sin icono (la primera letra, no el primer
 *  código: «Écija» da «É» entero y un emoji no se parte). */
export const inicialDe = (nombre: string): string =>
  (Array.from(nombre.trim())[0] ?? '?').toLocaleUpperCase();

export function TeselaProducto({
  app,
  locale,
  onKeyDown,
}: {
  app: ProductoDeLaGaleria;
  locale: Locale;
  /** La navegación con flechas de la rejilla (`navegarProducto` del lanzador). */
  onKeyDown: (e: KeyboardEvent) => void;
}): JSX.Element {
  const t = useT(locale);
  const nombre = app.name.replace('Sytue ', '');
  const estilo = { '--hub-product-color': app.color };

  if (app.disabled) {
    const idMotivo = `hub-producto-${app.key}-motivo`;
    return (
      <div
        role="group"
        aria-label={app.name}
        aria-describedby={idMotivo}
        data-hub-desactivado={app.key}
        style={estilo}
        class={CAJA}
      >
        {/* Lo apagado es la MARCA, no el texto: con toda la tarjeta al 40 % el motivo
        no pasaba el contraste AA, y es justo lo que hay que leer. */}
        <span class="opacity-50 grayscale">
          <ProductIcon key={app.key} appKey={app.key} size={40} />
        </span>
        <span class="flex w-full min-w-0 flex-col gap-0.5">
          <span class="flex items-center justify-between gap-1">
            <span class="text-text-muted truncate font-semibold">{nombre}</span>
            <span class="text-text-muted bg-bg-subtle border-border shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium tracking-wider uppercase">
              {t('launcher.disabled')}
            </span>
          </span>
          <span id={idMotivo} class="text-text-muted text-sm leading-snug">
            {t('launcher.orgOnlyReason')}
          </span>
          <a
            href={ayudaSoloOrganizaciones(app.key, locale)}
            data-hub-product={app.key}
            onKeyDown={onKeyDown}
            aria-keyshortcuts={TECLAS_GALERIA}
            aria-label={t('launcher.orgOnlyHelpAria', { name: app.name })}
            class="text-primary-text sytue-focus-ring inline-flex items-center self-start rounded-sm text-sm font-medium underline-offset-2 hover:underline pointer-coarse:min-h-[max(44px,2.75rem)]"
          >
            {t('launcher.orgOnlyHelp')}
          </a>
        </span>
      </div>
    );
  }

  return (
    <a
      data-hub-product={app.key}
      onKeyDown={onKeyDown}
      aria-keyshortcuts={TECLAS_GALERIA}
      href={app.href}
      aria-label={t('launcher.appAria', { name: app.name, description: app.description })}
      title={t('launcher.appAria', { name: app.name, description: app.description })}
      style={estilo}
      class={cx(CAJA, 'hover:border-border-strong')}
    >
      {/* El icono va SOLO. El SVG del producto ya ES la tesela —lleva su
      `rect` redondeado con el color de la app y el glifo en blanco— y
      ProductIcon le añade encima su anillo. Envolverlo en otra caja
      teñida del mismo tono apilaba halo sobre badge sobre tarjeta sobre
      página; y el selector de aplicaciones, a un clic de aquí, pinta
      estas mismas marcas desnudas (AppGrid). Dos tratamientos de lo
      mismo tan cerca se leen como un descuido. */}
      <ProductIcon key={app.key} appKey={app.key} size={40} />
      <span class="flex w-full min-w-0 flex-col">
        <span class="text-text truncate font-semibold">{nombre}</span>
        <span class="text-text-muted marco:line-clamp-1 line-clamp-2 text-sm leading-snug">
          {app.description}
        </span>
      </span>
    </a>
  );
}

/**
 * La tesela de una app conectada: la caja, el tamaño y la forma compacta de Marco de la
 * de producto, y su sitio en las flechas de la galería.
 *
 * El icono lo pone un tercero y su forma es impredecible, así que va dentro de un
 * distintivo NEUTRO con la geometría exacta de `ProductIcon` (de 4 a 36 sobre 40, radio
 * del 22,5 %): mide lo mismo que las marcas de al lado. Lo que no se hace es teñirlo con
 * nuestro acento —el logotipo de otro no es nuestro—. Sin icono, la inicial del nombre
 * sobre ese mismo neutro: antes quedaba una caja gris vacía.
 */
export function TeselaConectada({
  app,
  locale,
  onKeyDown,
}: {
  app: AppConectada;
  locale: Locale;
  onKeyDown: (e: KeyboardEvent) => void;
}): JSX.Element {
  const t = useT(locale);
  const rotulo = t('launcher.appAria', { name: app.name, description: app.description });
  return (
    <a
      data-hub-product={app.key}
      data-hub-conectada=""
      onKeyDown={onKeyDown}
      aria-keyshortcuts={TECLAS_GALERIA}
      href={app.href}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={rotulo}
      title={rotulo}
      class={cx(CAJA, 'hover:border-border-strong')}
    >
      <span class="relative inline-flex shrink-0" style={MARCA}>
        <span
          data-hub-distintivo=""
          class="bg-bg-subtle border-border-subtle text-text-muted absolute flex items-center justify-center border font-semibold"
          style={DISTINTIVO}
        >
          {app.iconUrl ? (
            <img src={app.iconUrl} alt="" width={20} height={20} aria-hidden="true" />
          ) : (
            <span aria-hidden="true">{inicialDe(app.name)}</span>
          )}
        </span>
      </span>
      <span class="flex w-full min-w-0 flex-col">
        <span class="text-text truncate font-semibold">{app.name}</span>
        <span class="text-text-muted marco:line-clamp-1 line-clamp-2 text-sm leading-snug">
          {app.description}
        </span>
      </span>
    </a>
  );
}
