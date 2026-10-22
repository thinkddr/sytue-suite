/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El marco de Sytue Cast.
 *
 * Con sesión: la Sytue Shell (barra superior con el conmutador de apps, la búsqueda
 * y la cuenta) y, debajo, el AppShell con la navegación lateral y el botón flotante
 * de «Subir» en el móvil.
 *
 * Sin sesión (la portada, un canal o un vídeo público abiertos por un enlace): la
 * misma barra del marco, pero con la marca de Cast y «Entrar» en vez de la cuenta; sin
 * navegación lateral, porque lo único que hay para quien no ha entrado es mirar.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  AppShell,
  Button,
  Icon,
  MobileFab,
  ProductIcon,
  SkipLink,
  ThemeProvider,
  ToastProvider,
  TopBar,
  withBase,
} from '@sytue/ui';
import { SytueShell } from '@sytue/shell';
import type { SytueUser } from '@sytue/auth';
import { fetchSession, getAccessToken } from '@sytue/auth/client';
import type { Locale } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import { useT } from '../i18n/index.js';
import { Nav, type Seccion } from './Nav.js';
import { ruta } from '../lib/rutas.js';

export interface MarcoProps {
  user: SytueUser | null;
  hubBaseUrl: string;
  locale: Locale;
  activa: Seccion | null;
  children: ComponentChildren;
  /**
   * Con el botón flotante de «Subir» en el móvil. Solo en las listas (Inicio,
   * Explorar): en la página de un vídeo taparía el chat o los comentarios, y en
   * Subir sería un botón que lleva a donde ya se está.
   */
  fab?: boolean;
}

export function Marco({
  user,
  hubBaseUrl,
  locale,
  activa,
  children,
  fab = false,
}: MarcoProps): JSX.Element {
  const t = useT(locale);
  const [usuario, setUsuario] = useState(user);
  useEffect(() => {
    setUsuario(user);
    if (user) return;
    // Una portada abierta antes del login puede llegar sin usuario en el SSR.
    // El BFF confirma la sesión actual; los tokens siguen bajo su control.
    let vigente = true;
    void fetchSession()
      .then((actual) => {
        if (vigente) setUsuario(actual);
      })
      .catch(() => {
        // Un fallo de red conserva la cabecera pública y su acceso al login.
      });
    return () => {
      vigente = false;
    };
  }, [user]);
  // `relative`: el que desplaza es ESTE panel y no el `main` del AppShell, así que el
  // bloque contenedor de lo que lleva dentro tiene que ser él. Sin eso, los `sr-only` de
  // «Subir» se anclaban al `main` y le daban un segundo desplazamiento sin nada debajo
  // (y antes de que el `main` fuera `relative`, al documento: 192 px en la v3.11.1).
  const contenido = <div class="relative h-full min-h-0 overflow-y-auto">{children}</div>;

  if (!usuario) {
    return (
      <I18nProvider locale={locale}>
        <ThemeProvider>
          <ToastProvider>
            <SkipLink />
            <AppShell topbar={<BarraPublica locale={locale} />} title={t('app.title')}>
              {contenido}
            </AppShell>
          </ToastProvider>
        </ThemeProvider>
      </I18nProvider>
    );
  }

  return (
    <SytueShell
      currentApp="cast"
      user={usuario}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={
          <Nav
            t={t}
            activa={activa}
            administra={usuario.roles.some((r) => r === 'owner' || r === 'admin')}
          />
        }
        title={t('app.title')}
      >
        {contenido}
      </AppShell>
      {!fab ? null : (
        <MobileFab
          label={t('nav.upload')}
          icon={<Icon name="upload" size={24} />}
          onClick={() => window.location.assign(ruta('/upload'))}
        />
      )}
    </SytueShell>
  );
}

/** La barra de quien no ha entrado: la marca a la izquierda, «Entrar» a la derecha. */
function BarraPublica({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  const entrar = (): void => {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
  };
  return (
    <TopBar
      class="sytue-topbar-carril"
      brand={
        <a
          href={withBase(import.meta.env.BASE_URL, '/explore')}
          class="sytue-focus-ring-chrome text-topbar-fg rounded-control inline-flex min-h-[max(44px,2.75rem)] items-center gap-2 px-1 text-sm font-semibold"
        >
          <ProductIcon appKey="cast" size={24} />
          {t('app.title')}
        </a>
      }
      actions={
        <Button size="sm" variant="primary" onClick={entrar}>
          {t('nav.signIn')}
        </Button>
      }
    />
  );
}
