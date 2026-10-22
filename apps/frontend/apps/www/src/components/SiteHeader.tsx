/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SiteHeader — cabecera pública de Sytue One. Usa la TopBar canónica de
 * @sytue/ui con la marca de la suite, navegación de marketing, selector de
 * idioma y CTA de acceso (a one.sytue.eu) + toggle de tema. Isla Preact (menú
 * móvil, tema y menú de idioma). docs/design.md §2.
 */
import { useState } from 'preact/hooks';
import { LOCALES, LOCALE_NAMES, persistLocale, type Locale } from '@sytue/i18n';
import {
  TopBar,
  Button,
  Dropdown,
  Icon,
  MenuItem,
  ThemeProvider,
  useTheme,
  logos,
  Glifo,
} from '@sytue/ui';
import { appDomains } from '@sytue/config';
import { localePath } from '../i18n/paths';

function ThemeToggle({ labels }: { labels: HeaderLabels }) {
  const { resolved, toggle } = useTheme();
  const isDark = resolved === 'dark';
  return (
    <Button
      variant="ghost"
      size="sm"
      iconOnly
      aria-label={isDark ? labels.toLight : labels.toDark}
      onClick={toggle}
    >
      {isDark ? (
        // Sol (modo claro disponible)
        <Glifo size={18}>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </Glifo>
      ) : (
        // Luna (modo oscuro disponible)
        <Glifo d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" size={18} />
      )}
    </Button>
  );
}

/**
 * Selector de idioma. Dos reglas que no son negociables:
 *
 *  1. Cambiar de idioma NO devuelve a la portada: son enlaces a ESTA misma
 *     página en el otro idioma (`/es/privacy` → `/fr/privacy`). Perder la página
 *     al traducirla es la forma más rápida de que alguien abandone.
 *  2. Los nombres van SIEMPRE en su propio idioma («Deutsch», no «Alemán»): es lo
 *     único que permite encontrar el suyo a quien no entiende la página actual.
 *     Por eso no pasan por el catálogo, igual que en `LanguageSelect` de la shell.
 *
 * Son <a> de verdad y no un `location.assign`, así que se abren en pestaña nueva
 * y se copian como cualquier enlace. Solo existen con el menú desplegado, que es
 * el motivo de que el descubrimiento por rastreador NO dependa de ellos: eso lo
 * cubren los `hreflang` recíprocos que el layout emite en el <head>.
 *
 * Elegir uno guarda la cookie `sytue_locale`, que es lo que la raíz consulta
 * ANTES que `Accept-Language` (static-serve.mjs): sin ella, quien eligiera
 * English con el navegador en alemán volvería a `/de` al pulsar el logo.
 *
 * Son 26: el panel se acota y se desplaza, y el teclado (flechas, Inicio/Fin,
 * escribir para saltar) lo pone el Dropdown de @sytue/ui.
 */
function LanguageMenu({ locale, path, label }: { locale: Locale; path: string; label: string }) {
  return (
    <Dropdown
      label={label}
      panelClass="max-h-[min(28rem,calc(100dvh-80px))]"
      trigger={(props) => (
        <Button variant="ghost" size="sm" aria-label={label} {...props}>
          <Icon name="globe" size={16} />
          <span class="hidden sm:inline">{LOCALE_NAMES[locale]}</span>
        </Button>
      )}
    >
      {LOCALES.map((code) => (
        <MenuItem
          key={code}
          href={localePath(code, path)}
          aria-current={code === locale ? 'true' : undefined}
          lang={code}
          class="aria-[current=true]:font-semibold"
          onClick={() => persistLocale(code)}
        >
          {LOCALE_NAMES[code]}
        </MenuItem>
      ))}
    </Dropdown>
  );
}

/**
 * Textos de la cabecera, ya traducidos por el SERVIDOR. La isla no lleva `t`: si
 * importara el catálogo, los 26 idiomas completos —las páginas legales
 * incluidas— viajarían al navegador solo para pintar una topbar.
 */
export interface HeaderLabels {
  products: string;
  sovereignty: string;
  standards: string;
  pricing: string;
  downloads: string;
  nav: string;
  navMobile: string;
  home: string;
  openMenu: string;
  closeMenu: string;
  toLight: string;
  toDark: string;
  language: string;
  signIn: string;
  start: string;
}

interface HeaderProps {
  locale: Locale;
  /** Ruta BASE de la página actual (sin prefijo de idioma): '/', '/downloads'… */
  path: string;
  labels: HeaderLabels;
}

function HeaderInner({ locale, path, labels }: HeaderProps) {
  const [open, setOpen] = useState(false);
  const home = localePath(locale, '/');
  // Las anclas de sección son las del inglés en todos los idiomas, igual que los
  // slugs: el id de un <section> es un identificador, no texto de escaparate.
  const nav = [
    { href: `${home}#products`, label: labels.products },
    { href: `${home}#sovereignty`, label: labels.sovereignty },
    { href: `${home}#standards`, label: labels.standards },
    { href: `${home}#pricing`, label: labels.pricing },
    { href: localePath(locale, '/downloads'), label: labels.downloads },
  ];

  return (
    <>
      <TopBar
        brand={
          <a href={home} aria-label={labels.home} class="inline-flex items-center">
            <img src={logos.suite} alt="Sytue One" height={26} />
          </a>
        }
        center={
          <nav aria-label={labels.nav} class="hidden items-center gap-1 lg:flex">
            {nav.map((n) => (
              <a
                key={n.href}
                href={n.href}
                // `h-8` y no `py-2`: con el relleno, el enlace medía 33,3px de
                // alto mientras los botones de al lado —el selector de idioma, el
                // de tema, «Entrar», «Empieza gratis»— median 28. Al pasar el
                // ratón, el velo de los enlaces era un dedo más alto que el de los
                // botones de su misma fila. La altura del control la fija la
                // escala de botones, no el texto que le toque dentro.
                class="text-text-muted hover:bg-bg-subtle hover:text-text active:bg-surface-sunken rounded-control inline-flex h-8 items-center px-2.5 text-sm font-medium whitespace-nowrap transition-colors xl:px-3 xl:text-base"
              >
                {n.label}
              </a>
            ))}
          </nav>
        }
        actions={
          <>
            <LanguageMenu locale={locale} path={path} label={labels.language} />
            <ThemeToggle labels={labels} />
            <a href={appDomains.one} class="hidden md:inline-flex">
              <Button variant="ghost" size="sm">
                {labels.signIn}
              </Button>
            </a>
            <a href={`${appDomains.id}/signup`} class="hidden md:inline-flex">
              <Button variant="primary" size="sm">
                {labels.start}
              </Button>
            </a>
            <Button
              variant="ghost"
              size="sm"
              iconOnly
              class="lg:hidden"
              aria-label={open ? labels.closeMenu : labels.openMenu}
              aria-expanded={open}
              aria-controls="mobile-menu"
              onClick={() => setOpen((v) => !v)}
            >
              <Icon name={open ? 'close' : 'menu'} size={20} />
            </Button>
          </>
        }
      />
      {open ? (
        <div id="mobile-menu" class="sytue-menu-surface border-border border-b px-4 py-3 lg:hidden">
          <nav aria-label={labels.navMobile} class="flex flex-col gap-1">
            {nav.map((n) => (
              <a
                key={n.href}
                href={n.href}
                onClick={() => setOpen(false)}
                class="text-text hover:bg-bg-subtle active:bg-surface-sunken rounded-control px-3 py-2 text-base font-medium transition-colors"
              >
                {n.label}
              </a>
            ))}
            <a href={`${appDomains.id}/signup`} class="mt-1">
              <Button variant="primary" fullWidth>
                {labels.start}
              </Button>
            </a>
          </nav>
        </div>
      ) : null}
    </>
  );
}

export default function SiteHeader(props: HeaderProps) {
  return (
    <ThemeProvider>
      <HeaderInner {...props} />
    </ThemeProvider>
  );
}
