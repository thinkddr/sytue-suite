// SPDX-License-Identifier: Apache-2.0
// Banco sin servidor de la BARRA ÚNICA del teléfono en las diecinueve superficies de la
// suite, con sus marcos REALES (la Sytue Shell y AppShell/AdminShell/Marco de cada app).
// Lo compila y lo conduce test/e2e/barra-unica.mjs, que contesta la API desde Playwright.
// La superficie sale de `?app=` y el idioma de `?l=`.
//
// Rooms va con las mismas piezas que ChatApp pasa a BarraMovil: su app real se queda
// esperando al cifrado de Matrix sin servidor.
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { isLocale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { SytueShell } from '@sytue/shell';
import { BarraMovil, Button, Glifo, Icon, Sidebar, SidebarItem } from '@sytue/ui';
import { CalendarApp } from '../../../apps/calendar/src/components/CalendarApp.js';
import { MailApp } from '../../../apps/mail/src/components/MailApp.js';
import { ContactsApp } from '../../../apps/contacts/src/components/ContactsApp.js';
import { Marco as MarcoBases } from '../../../apps/bases/src/components/Marco.js';
import { Marco as MarcoPages } from '../../../apps/pages/src/components/Marco.js';
import { Marco as MarcoCast } from '../../../apps/cast/src/components/Marco.js';
import { Marco as MarcoPhotos } from '../../../apps/photos/src/components/Marco.js';
import ConsoleShell from '../../../apps/console/src/components/ConsoleShell.js';
import CloudShell from '../../../apps/cloud-dashboard/src/components/CloudShell.js';
import { Shell as ShellDrive } from '../../../apps/drive/src/components/Shell.js';
import { Shell as ShellPortal } from '../../../apps/portal/src/components/Shell.js';
import { AccountApp } from '../../../apps/account/src/components/AccountApp.js';
import { NotesApp } from '../../../apps/notes/src/components/NotesApp.js';
import { ErpApp } from '../../../apps/erp/src/components/ErpApp.js';
import { Shell as ShellOffice } from '../../office/src/components/Shell.js';
import { DocList } from '../../office/src/components/DocList.js';

const q = new URLSearchParams(location.search);
const pedido = q.get('l');
const locale = isLocale(pedido) ? pedido : 'es';
const app = q.get('app') ?? 'calendar';
const user: SytueUser = {
  id: 'u1',
  email: 'ana@sytue.test',
  emailVerified: true,
  name: 'Ana García',
  preferredUsername: 'ana',
  roles: ['owner'],
  groups: [],
  tenant: 'acme',
};
const hub = '/hub';
const relleno = <div class="text-text p-4 text-sm">·</div>;

/** Rooms: la misma composición que ChatApp (BarraMovil con el ☰ de canales). */
function Rooms() {
  const [abierto, setAbierto] = useState(false);
  return (
    <SytueShell currentApp="rooms" user={user} hubBaseUrl={hub} accessToken="x" locale={locale}>
      <div class="sytue-frame sytue-safe-workspace flex h-[calc(100dvh-var(--spacing-topbar))]">
        <BarraMovil
          soloEnLaTopbar
          menu={{ id: 'm', controla: 'c', abierto, alPulsar: () => setAbierto(!abierto) }}
          titulo="general"
          acciones={
            <>
              <Button variant="ghost" size="sm" iconOnly aria-label="i">
                <Icon name="info" size={18} />
              </Button>
              <Button variant="ghost" size="sm" iconOnly aria-label="s">
                <Icon name="search" size={18} />
              </Button>
              <Button variant="ghost" size="sm" iconOnly aria-label="v">
                <Glifo size={18}>
                  <path d="M23 7 16 12l7 5V7Z" />
                  <rect x="1" y="5" width="15" height="14" rx="2" />
                </Glifo>
              </Button>
            </>
          }
        />
        <aside id="c" class={abierto ? 'sytue-nav-drawer max-md:fixed' : 'max-md:hidden'}>
          <Sidebar label="canales">
            <SidebarItem label="general" icon={<Icon name="home" size={18} />} active />
          </Sidebar>
        </aside>
        <main class="sytue-sheet flex-1">{relleno}</main>
      </div>
    </SytueShell>
  );
}

function vista() {
  switch (app) {
    case 'mail':
      return <MailApp locale={locale} hubBaseUrl={hub} idBaseUrl="/id" user={user} />;
    case 'contacts':
      return <ContactsApp locale={locale} hubBaseUrl={hub} user={user} />;
    case 'bases':
      return (
        <MarcoBases user={user} hubBaseUrl={hub} locale={locale} activa={null}>
          {relleno}
        </MarcoBases>
      );
    case 'pages':
      return (
        <MarcoPages user={user} hubBaseUrl={hub} locale={locale} activa={null}>
          {relleno}
        </MarcoPages>
      );
    case 'cast':
      return (
        <MarcoCast user={user} hubBaseUrl={hub} locale={locale} activa={null}>
          {relleno}
        </MarcoCast>
      );
    case 'photos':
      return (
        <MarcoPhotos user={user} hubBaseUrl={hub} locale={locale} title="Fotos">
          {relleno}
        </MarcoPhotos>
      );
    case 'console':
      return (
        <ConsoleShell user={user} hubBaseUrl={hub} locale={locale} basePath="/admin">
          {relleno}
        </ConsoleShell>
      );
    case 'cloud':
      return (
        <CloudShell user={user} hubBaseUrl={hub} locale={locale}>
          {relleno}
        </CloudShell>
      );
    case 'drive':
      return <ShellDrive user={user} hubBaseUrl={hub} locale={locale} />;
    case 'portal':
      return (
        <ShellPortal user={user} hubBaseUrl={hub} locale={locale}>
          {relleno}
        </ShellPortal>
      );
    case 'account':
      return (
        <AccountApp
          user={user}
          hubBaseUrl={hub}
          idBaseUrl="/id"
          locale={locale}
          nombreInicial="Ana García"
          email="ana@sytue.test"
        />
      );
    case 'notes':
      return <NotesApp user={user} hubBaseUrl={hub} locale={locale} publicBase="/n" />;
    case 'erp':
      return <ErpApp user={user} hubBaseUrl={hub} locale={locale} />;
    case 'rooms':
      return <Rooms />;
    case 'docs':
    case 'sheets':
    case 'slides':
    case 'pdf': {
      const kind =
        app === 'docs'
          ? 'document'
          : app === 'sheets'
            ? 'spreadsheet'
            : app === 'slides'
              ? 'presentation'
              : '';
      return (
        <ShellOffice user={user} hubBaseUrl={hub} locale={locale} producto={app}>
          <DocList locale={locale} userId="u1" kind={kind} />
        </ShellOffice>
      );
    }
    default:
      return <CalendarApp locale={locale} hubBaseUrl={hub} user={user} />;
  }
}

const raiz = document.createElement('div');
raiz.setAttribute('data-banco-app', '');
document.body.appendChild(raiz);
render(vista(), raiz);
