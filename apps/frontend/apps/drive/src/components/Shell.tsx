/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Shell — isla raiz de Sytue Drive. Envuelve <SytueShell> (topbar global + app
 * switcher + cuenta + ToastProvider) e integra DENTRO, en el mismo arbol de
 * isla, el layout de dos paneles del explorador (sidebar de ubicaciones +
 * navegador). Componer el navegador aqui —y no como isla suelta— es lo que hace
 * que sus toasts (subida, errores) compartan el ToastProvider de la shell y que
 * todo quede montado bajo la Sytue Shell. Recibe solo props serializables
 * (usuario, Hub, idioma) desde el layout SSR.
 *
 * El idioma se negocia UNA vez en el servidor (`src/middleware.ts`) y entra aqui
 * por props: <SytueShell> lo publica en el contexto de i18n, de donde lo leen las
 * piezas de dentro (sidebar, unidades de equipo, historial de versiones). Ninguna
 * pieza lo detecta por su cuenta (docs/CONVENTIONS.md §13.1).
 */
import type { ComponentChildren } from 'preact';
import { useState } from 'preact/hooks';
import { SytueShell } from '@sytue/shell';
import { AdminShell } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import { DriveSidebar, type Section } from './DriveSidebar';
import type { Cloud } from './PersonalClouds';
import { PrivateDriveGate } from './PrivateDriveGate.js';
import { PrivateDriveProvider } from '../client/private-browser.js';
import { PrivateFileBrowser } from './PrivateFileBrowser.js';

interface ShellProps {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma ya negociado en el servidor. */
  locale: Locale;
  children?: ComponentChildren;
}

const SECCIONES: readonly Section[] = [
  'my',
  'documents',
  'team',
  'shared',
  'recent',
  'favorites',
  'trash',
];

/**
 * La sección con la que se entra: `?section=` la elige. Es la llegada de un aviso de
 * «compartido contigo» del Hub (`hubnotify.EnlaceCompartidoConmigo`), que no puede
 * mandar a la carpeta del dueño —quien recibe no ve su ruta—; hasta el 2026-09-24
 * mandaba a `/shared`, que no es ninguna página.
 */
function seccionDeLaUrl(): Section {
  if (typeof window === 'undefined') return 'my';
  const s = new URLSearchParams(window.location.search).get('section') ?? '';
  return (SECCIONES as readonly string[]).includes(s) ? (s as Section) : 'my';
}

export function Shell({ user, hubBaseUrl, locale }: ShellProps) {
  const [section, setSection] = useState<Section>(seccionDeLaUrl);
  const [cloud, setCloud] = useState<Cloud | null>(null);
  const [navigation, setNavigation] = useState(0);

  return (
    <SytueShell
      currentApp="drive"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      <PrivateDriveGate actor={user.id} tenant={user.tenant} locale={locale}>
        {(session, baseURL) => (
          <PrivateDriveProvider session={session} baseURL={baseURL}>
            <AdminShell
              sidebar={
                <DriveSidebar
                  active={section}
                  onSelect={(s) => {
                    const url = new URL(window.location.href);
                    for (const key of ['request', 'path', 'preview', 'file', 'folder'])
                      url.searchParams.delete(key);
                    url.searchParams.set('section', s);
                    window.history.replaceState(null, '', url);
                    setCloud(null);
                    setSection(s);
                    setNavigation((value) => value + 1);
                  }}
                  locale={locale}
                  cloud={cloud?.instance_id ?? null}
                  onCloud={setCloud}
                />
              }
              // Sin `title`: cada vista trae su propio cabecero con el nombre de la sección
              // justo debajo de la barra móvil, y «Mi unidad» salía dos veces seguidas.
            >
              <div class="bg-surface flex min-h-full flex-col">
                <PrivateFileBrowser
                  key={navigation}
                  section={cloud ? 'external' : section}
                  onOpenFolder={() => {
                    setCloud(null);
                    setSection('my');
                  }}
                />
              </div>
            </AdminShell>
          </PrivateDriveProvider>
        )}
      </PrivateDriveGate>
    </SytueShell>
  );
}
