/**
 * CloudShell — armazón de Sytue DevX. Monta la Sytue Shell global (topbar + app
 * switcher de 9 puntos + búsqueda + notificaciones + cuenta) desde @sytue/shell
 * y, debajo, el patrón sidebar + contenido de @sytue/ui. Project-centric:
 * cuando hay proyecto activo, el sidebar muestra sus secciones (estilo Vercel);
 * si no, muestra la navegación global. docs/CONVENTIONS.md §5, §11, §14.
 */
import type { ComponentChildren } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { SytueShell } from '@sytue/shell';
import { getAccessToken } from '@sytue/auth/client';
import { AdminShell, Sidebar, SidebarSection, SidebarItem, Icon, Badge, useAtajo } from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { MARCA_ORG, RE_SLUG_ORG, rutaConOrg } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import CommandPalette from './CommandPalette.js';
import type { PaletteCommand } from '../lib/palette.js';
import { useT, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';

interface CloudShellProps {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma ya negociado en el servidor; baja a la shell y a la paleta. */
  locale: Locale;
  /** Seccion activa dentro del proyecto / global. */
  section?: string | undefined;
  /** Slug del proyecto activo (project-centric) o undefined para vista global. */
  project?: string | undefined;
  /**
   * El proyecto activo tiene un servicio Postgres gestionado (PostgREST activo).
   * Deriva del estado real de servicios (`project.services`), nunca hardcodeado:
   * sin él la sección «Conectado» del sidebar no se muestra.
   */
  hasDb?: boolean | undefined;
  children: ComponentChildren;
}

interface NavEntry {
  id: string;
  label: string;
  icon: IconName;
  href: string;
}

function globalNav(t: T): NavEntry[] {
  return [
    // `r('/')` y no `'/'`: Cloud se construye con `base: '/cloud'`, así que una
    // barra suelta pide la RAÍZ DEL HOST —el portal, que redirige a la app por
    // defecto— y quien pulsaba «Proyectos» acababa en el calendario.
    { id: 'projects', label: t('nav.projects'), icon: 'grid', href: r('/') },
    { id: 'templates', label: t('nav.templates'), icon: 'rocket', href: r('/templates') },
    { id: 'usage', label: t('nav.usage'), icon: 'chart', href: r('/usage') },
    { id: 'billing', label: t('nav.billing'), icon: 'receipt', href: r('/billing') },
    { id: 'team', label: t('nav.team'), icon: 'users', href: r('/team') },
    { id: 'tokens', label: t('nav.tokens'), icon: 'lock', href: r('/tokens') },
    { id: 'audit', label: t('nav.audit'), icon: 'audit', href: r('/audit') },
    { id: 'settings', label: t('nav.settings'), icon: 'settings', href: r('/settings') },
  ];
}

/** Convierte entradas de navegación en comandos para la paleta ⌘K. */
function toNavCommands(entries: NavEntry[], group: string): PaletteCommand[] {
  return entries.map((e): PaletteCommand => ({
    id: `nav:${e.href}`,
    group,
    label: e.label,
    icon: e.icon,
    keywords: e.id,
    run: () => {
      window.location.href = e.href;
    },
  }));
}

function projectNav(t: T, slug: string): NavEntry[] {
  // Con `r()`: Cloud vive bajo `/cloud`, así que sin prefijo TODA la barra
  // lateral del proyecto —doce entradas— apuntaba a la raíz del host, o sea
  // fuera de la aplicación. Basta ponerlo aquí: el resto se compone sobre esta.
  const base = r(`/projects/${slug}`);
  return [
    { id: 'overview', label: t('nav.overview'), icon: 'grid', href: base },
    { id: 'deployments', label: t('nav.deployments'), icon: 'rocket', href: `${base}/deployments` },
    {
      id: 'database',
      label: t('nav.database'),
      icon: 'database',
      href: `${base}/database`,
    },
    { id: 'storage', label: t('nav.storage'), icon: 'bucket', href: `${base}/storage` },
    { id: 'functions', label: t('nav.functions'), icon: 'code', href: `${base}/functions` },
    { id: 'tasks', label: t('nav.tasks'), icon: 'clock', href: `${base}/tasks` },
    { id: 'apps', label: t('nav.apps'), icon: 'grid', href: `${base}/apps` },
    { id: 'domains', label: t('nav.domains'), icon: 'globe', href: `${base}/domains` },
    { id: 'monitoring', label: t('nav.monitoring'), icon: 'bell', href: `${base}/monitoring` },
    { id: 'analytics', label: t('nav.analytics'), icon: 'chart', href: `${base}/analytics` },
    { id: 'env', label: t('nav.env'), icon: 'lock', href: `${base}/env` },
    { id: 'logs', label: t('nav.logs'), icon: 'audit', href: `${base}/logs` },
    // Terminal en el contenedor de producción: el control-plane solo la abre a
    // admin/owner, así que a un miembro le responderá 403 (la vista lo explica).
    { id: 'terminal', label: t('nav.terminal'), icon: 'terminal', href: `${base}/terminal` },
    { id: 'settings', label: t('nav.settings'), icon: 'settings', href: `${base}/settings` },
  ];
}

export default function CloudShell({
  user,
  hubBaseUrl,
  locale,
  section,
  project,
  hasDb,
  children,
}: CloudShellProps) {
  const t = useT(locale);
  const entries = project ? projectNav(t, project) : globalNav(t);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const metaHint = useAtajo('Ctrl K');

  // --- La barra de direcciones converge a la canónica (ADR 0056) -----------
  // La vuelta del login aterriza en la forma que el middleware sirve POR DENTRO
  // (`/cloud/projects/x?__org=acme`): sirve, pero la marca `__org` es plomería
  // del middleware y no debe quedarse en la barra —ni copiarse dentro de un
  // enlace—. Aquí se reescribe UNA vez a la forma con org
  // (`/cloud/acme/projects/x`), conservando query y hash.
  useEffect(() => {
    const marca = new URLSearchParams(window.location.search).get(MARCA_ORG);
    if (!marca || !RE_SLUG_ORG.test(marca)) return;
    const base = import.meta.env.BASE_URL.replace(/\/+$/, '');
    if (!window.location.pathname.startsWith(`${base}/`)) return;
    const query = new URLSearchParams(window.location.search);
    query.delete(MARCA_ORG);
    const qs = query.toString();
    window.history.replaceState(
      null,
      '',
      rutaConOrg(base, marca, window.location.pathname.slice(base.length) || '/') +
        (qs ? `?${qs}` : '') +
        window.location.hash,
    );
  }, []);

  // La paleta ofrece las secciones del proyecto activo Y la navegación global.
  const navCommands = toNavCommands(
    project ? [...entries, ...globalNav(t)] : entries,
    t('palette.group.navigation'),
  );

  const sidebar = (
    <Sidebar
      // Sin cabecera con «Sytue DevX»: el selector de apps de la barra superior
      // ya lo dice; la barra lateral lo repetía justo debajo (2026-09-09).
      footer={
        <div class="text-sidebar-fg-muted flex items-center gap-2 px-2 py-1 text-xs">
          <span
            // Con aro: un punto verde sobre un carril verde no se ve.
            class="ring-sidebar-fg h-2 w-2 shrink-0 rounded-full bg-[var(--sytue-presence-online)] ring-[1.5px]"
            aria-hidden="true"
          />
          <span class="truncate">{t('shell.region')}</span>
        </div>
      }
    >
      {/* Disparador de la paleta de comandos ⌘K (estilo Linear). El atajo global
          funciona en toda la app; este botón lo hace descubrible. */}
      <button
        type="button"
        onClick={() => setPaletteOpen(true)}
        aria-keyshortcuts="Meta+K Control+K"
        class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg border-chrome-border bg-chrome-hover rounded-control mb-3 flex w-full items-center gap-2 border px-2.5 py-2 text-sm transition-colors duration-150"
      >
        <Icon name="search" size={16} />
        <span class="min-w-0 flex-1 truncate text-left">{t('shell.searchTrigger')}</span>
        <kbd class="border-chrome-divider rounded border px-1.5 py-0.5 font-sans text-xs leading-none tabular-nums">
          {metaHint}
        </kbd>
      </button>
      {project ? (
        <SidebarSection title={t('shell.sectionProject')}>
          <SidebarItem
            icon={<Icon name="chevronLeft" size={16} />}
            label={t('shell.allProjects')}
            href={r('/')}
          />
        </SidebarSection>
      ) : null}
      <SidebarSection title={project ?? t('shell.organization')}>
        {entries.map((e) => (
          <SidebarItem
            key={e.id}
            href={e.href}
            label={e.label}
            active={section === e.id}
            icon={<Icon name={e.icon} size={20} />}
          />
        ))}
      </SidebarSection>
      {project && hasDb ? (
        <SidebarSection title={t('shell.connected')}>
          <li class="px-2.5 py-1">
            <Badge tone="accent">{t('shell.postgrestActive')}</Badge>
          </li>
        </SidebarSection>
      ) : null}
    </Sidebar>
  );

  return (
    <SytueShell
      currentApp="cloud"
      user={user}
      accessToken={getAccessToken}
      hubBaseUrl={hubBaseUrl}
      locale={locale}
    >
      {/* Sidebar responsive compartida (@sytue/ui): en ≥768px en flujo, en
          <768px drawer con overlay, foco gestionado y Esc — mismo main#contenido. */}
      <AdminShell sidebar={sidebar} title={project ?? 'Sytue DevX'}>
        {children}
      </AdminShell>
      <CommandPalette
        locale={locale}
        nav={navCommands}
        project={project}
        metaHint={metaHint}
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
      />
    </SytueShell>
  );
}
