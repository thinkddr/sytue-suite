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
import { FranjaPeticion, SytueShell } from '@sytue/shell';
import { AdminShell } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import { DriveSidebar, type Section } from './DriveSidebar';
import { FileBrowser, TrashView, ResourceListView, SharedWithMeView } from './FileBrowser';
import { TeamDrivesView } from './TeamDrives';
import { DocumentsView } from './DocumentsView';
import { CloudView, type Cloud } from './PersonalClouds';

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
  const [peticion] = useState(() =>
    typeof window === 'undefined'
      ? ''
      : (new URLSearchParams(window.location.search).get('request') ?? ''),
  );

  return (
    <SytueShell
      currentApp="drive"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* Cuerpo bajo la topbar (56px): sidebar de ubicaciones + navegador. El
          AdminShell aporta el `main`, el alto y —en <768px— la sidebar como
          drawer con su botón de menú. */}
      <AdminShell
        sidebar={
          <DriveSidebar
            active={section}
            onSelect={(s) => {
              setCloud(null);
              setSection(s);
            }}
            locale={locale}
            cloud={cloud?.instance_id ?? null}
            onCloud={setCloud}
          />
        }
        // Sin `title`: cada vista trae su propio cabecero con el nombre de la sección
        // justo debajo de la barra móvil, y «Mi unidad» salía dos veces seguidas.
      >
        {/* El aviso de una petición de edición (`hubnotify.EnlacePeticionDrive`): quien
            compartió ve aquí quién le pide editar y lo concede con un clic. */}
        {peticion ? (
          <FranjaPeticion
            endpoint={`/api/edit-requests?id=${encodeURIComponent(peticion)}`}
            soloLectura={false}
            puedeConceder
            mensaje=""
            locale={locale}
          />
        ) : null}
        {/* Sobre SUPERFICIE: las listas de Drive son filas con filetes, no tarjetas, y
            directamente sobre el lienzo gris se leían como contenido sin panel. */}
        <div class="bg-surface min-h-full">
          {cloud ? (
            <CloudView
              key={cloud.instance_id}
              cloud={cloud}
              onDisconnected={() => setCloud(null)}
            />
          ) : section === 'documents' ? (
            <DocumentsView />
          ) : section === 'trash' ? (
            <TrashView />
          ) : section === 'recent' ? (
            <ResourceListView mode="recent" />
          ) : section === 'favorites' ? (
            <ResourceListView mode="favorites" />
          ) : section === 'shared' ? (
            <SharedWithMeView />
          ) : section === 'team' ? (
            <TeamDrivesView />
          ) : (
            <FileBrowser />
          )}
        </div>
      </AdminShell>
    </SytueShell>
  );
}
