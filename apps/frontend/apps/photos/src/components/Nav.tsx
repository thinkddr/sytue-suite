/**
 * La navegación de Sytue Fotos: fotos, favoritas, álbumes, compartido conmigo y papelera. Enlaces RELATIVOS, que se
 * resuelven bajo el prefijo de la app sin ayudante.
 */
import type { JSX } from 'preact';
import { Glifo, Icon } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import type { useT } from '../i18n/index.js';

export function Nav({
  t,
  locale,
  activa,
}: {
  t: ReturnType<typeof useT>;
  /** Para el rótulo común de «Compartido conmigo» (@sytue/i18n). */
  locale: Locale;
  activa: 'photos' | 'favorites' | 'albums' | 'shared' | 'duplicates' | 'trash';
}): JSX.Element {
  const clase = (k: 'photos' | 'favorites' | 'albums' | 'shared' | 'duplicates' | 'trash') =>
    // `h-8` en vez de `py-1`: con el relleno, la pestaña medía 23px de alto, por
    // debajo de los 24×24 que pide WCAG 2.5.8 (AA) para lo que se toca con el
    // dedo. Son las cinco pestañas de la galería, o sea la navegación principal
    // de la app en un teléfono.
    // `shrink-0 whitespace-nowrap`: en la fila que se desplaza de un teléfono, la
    // pestaña es un hijo flex y encogía partiendo su rótulo —«Compartido / conmigo»
    // en dos líneas, medido a 390—. Y con el dedo, 44 de alto: es la navegación
    // principal de la app.
    `sytue-focus-ring inline-flex h-8 shrink-0 items-center gap-1.5 rounded-sm px-2 text-sm whitespace-nowrap pointer-coarse:min-h-[max(44px,2.75rem)] ${activa === k ? 'bg-bg-subtle font-medium' : 'text-text-muted hover:text-text'}`;
  return (
    // `min-w-0` NO es decoración. Esta nav es un hijo flex de la fila de
    // herramientas, y un hijo flex trae `min-width: auto`: se niega a encoger
    // por debajo de su contenido, así que en vez de dejar que su propio
    // `flex-wrap` reparta las cinco pestañas, las empuja fuera. Medido en un
    // iPhone 14 Pro Max: la fila llega hasta 416 y «Papelera» acababa en 428, a
    // 2px del cristal, en las cinco rutas de la app.
    // En móvil, UNA fila que se desplaza: con `flex-wrap` las cinco pestañas
    // partían en dos filas y, con el buscador y los botones debajo, el cromo
    // ocupaba un tercio de la pantalla antes de la primera foto.
    // `data-scroll-x` DECLARA que esta fila se desplaza de lado a propósito. No es
    // decoración tampoco: el barrido de desbordamiento trata cualquier scroll
    // horizontal como defecto salvo que se diga, y sin esto marcaba las cinco
    // rutas de Fotos en todos los teléfonos (+118px en iPhone SE, +48 en iPhone
    // 12) por la fila que nosotros mismos decidimos que se desplazara.
    <nav
      data-scroll-x
      class="mb-4 flex min-w-0 [scrollbar-width:none] flex-nowrap gap-1 overflow-x-auto sm:flex-wrap sm:overflow-visible"
      aria-label="Sytue Fotos"
    >
      <a href="./" class={clase('photos')} aria-current={activa === 'photos' ? 'page' : undefined}>
        <Icon name="image" size={16} />
        {t('nav.photos')}
      </a>
      <a
        href="favorites"
        class={clase('favorites')}
        aria-current={activa === 'favorites' ? 'page' : undefined}
      >
        <Icon name="corazon" size={16} />
        {t('nav.favorites')}
      </a>
      <a
        href="albums"
        class={clase('albums')}
        aria-current={activa === 'albums' ? 'page' : undefined}
      >
        <Icon name="album" size={16} />
        {t('nav.albums')}
      </a>
      {/* «Compartido conmigo»: los álbumes que otras personas me han dado. */}
      <a
        href="shared"
        class={clase('shared')}
        aria-current={activa === 'shared' ? 'page' : undefined}
      >
        <Icon name="users" size={16} />
        {translate(locale, 'sharedWithMe.title')}
      </a>
      <a
        href="duplicates"
        class={clase('duplicates')}
        aria-current={activa === 'duplicates' ? 'page' : undefined}
      >
        <GlifoRepetidas />
        {t('nav.duplicates')}
      </a>
      <a href="trash" class={clase('trash')} aria-current={activa === 'trash' ? 'page' : undefined}>
        <Icon name="trash" size={16} />
        {t('nav.trash')}
      </a>
    </nav>
  );
}

/** Dos marcos superpuestos: «repetidas» no puede llevar el mismo icono que «álbumes». */
function GlifoRepetidas(): JSX.Element {
  return <Glifo d="M8 8h12v12H8zM4 16V4h12" size={16} />;
}
