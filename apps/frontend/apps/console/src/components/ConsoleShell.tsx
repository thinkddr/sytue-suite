/**
 * ConsoleShell — armazón de Sytue Admin. Monta la Sytue Shell global y, debajo,
 * el AdminShell de @sytue/ui (sidebar en flujo en ≥768px, drawer accesible en
 * móvil). Navegación de administración: panel, organizaciones, usuarios, grupos
 * y auditoría. docs/CONVENTIONS.md §5, §11.
 */
import type { ComponentChildren } from 'preact';
import { useEffect } from 'preact/hooks';
import { SytueShell } from '@sytue/shell';
import { getAccessToken } from '@sytue/auth/client';
import { AdminShell, Sidebar, SidebarSection, SidebarItem, Icon } from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { MARCA_ORG, orgActual, RE_SLUG_ORG, rutaConOrg } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey } from '../i18n/index.js';
import OrgSwitcher from './OrgSwitcher.js';

interface ConsoleShellProps {
  user: SytueUser;
  hubBaseUrl: string;
  section?: string | undefined;
  /** Ids de NAV visibles (permiso del usuario). Ausente = todas (owner/admin). */
  allowed?: readonly string[] | undefined;
  /** Idioma ya negociado en el servidor; llega por props, no se detecta aquí. */
  locale: Locale;
  /**
   * Prefijo REAL bajo el que se sirve esta app en este host, que no siempre es
   * el `base` del build. La misma imagen sirve dos superficies: `admin.<dominio>`
   * bajo `/admin` y `platform.<dominio>` en la RAÍZ. Como `r()` resuelve con
   * `import.meta.env.BASE_URL` —sustituido al construir, o sea `/admin` siempre—,
   * en plataforma el menú apuntaba a `/admin/platform`, que ahí es un 404 medido.
   * No se notaba porque esa superficie tenía una sola entrada y nadie pulsa la
   * pestaña en la que ya está; con la de conexiones al lado, se nota a la primera.
   * Lo sabe el servidor (`SYTUE_BASE_PATH`) y por eso baja por props: así el HTML
   * inicial ya sale bien, sin esperar a la hidratación.
   */
  basePath: string;
  children: ComponentChildren;
}

// La etiqueta NO va aquí: va al catálogo. Se guarda la CLAVE para que el orden del
// menú (que es una decisión de producto) viva separado de su traducción.
const NAV: { id: string; labelKey: ConsoleMessageKey; icon: IconName; href: string }[] = [
  { id: 'dashboard', labelKey: 'nav.dashboard', icon: 'grid', href: '/' },
  { id: 'tenants', labelKey: 'nav.tenants', icon: 'building', href: '/tenants' },
  { id: 'users', labelKey: 'nav.users', icon: 'user', href: '/users' },
  { id: 'groups', labelKey: 'nav.groups', icon: 'users', href: '/groups' },
  { id: 'roles', labelKey: 'nav.roles', icon: 'userCheck', href: '/roles' },
  { id: 'pim', labelKey: 'nav.pim', icon: 'lock', href: '/privileged-access' },
  { id: 'sso', labelKey: 'nav.sso', icon: 'key', href: '/sso' },
  { id: 'access', labelKey: 'nav.access', icon: 'settings', href: '/access' },
  { id: 'devices', labelKey: 'nav.devices', icon: 'device', href: '/devices' },
  { id: 'apps', labelKey: 'nav.apps', icon: 'globe', href: '/apps' },
  { id: 'migrations', labelKey: 'nav.migrations', icon: 'database', href: '/migrations' },
  // Con qué editor se abren los documentos de la organización. Era una variable de
  // entorno de Drive: había que editar el despliegue y todas las organizaciones de una
  // instalación compartían editor.
  {
    id: 'document-editor',
    labelKey: 'nav.documentEditor',
    icon: 'table',
    href: '/editor-de-documentos',
  },
  // Qué hace la organización con las peticiones de firma que le llegan de otras
  // organizaciones (ADR 0059): abiertas, cerradas o solo de las de confianza.
  {
    id: 'external-signatures',
    labelKey: 'nav.externalSignatures',
    icon: 'pen',
    href: '/firmas-de-fuera',
  },
  // Canales de Rooms. Era el único producto de la suite sin administración: un
  // canal privado archivado por error no lo podía deshacer nadie de fuera.
  { id: 'rooms', labelKey: 'nav.rooms', icon: 'comment', href: '/rooms' },
  // Quién ve y quién toca cada módulo de Sytue ERP, al estilo de los permisos de
  // aplicación de Odoo: el ERP es de toda la organización, no de quien lo abrió.
  { id: 'erp', labelKey: 'nav.erp', icon: 'receipt', href: '/erp' },
  { id: 'governance', labelKey: 'nav.governance', icon: 'shield', href: '/governance' },
  { id: 'compliance', labelKey: 'nav.compliance', icon: 'check', href: '/compliance' },
  {
    id: 'sensitivity-labels',
    labelKey: 'nav.sensitivityLabels',
    icon: 'tag',
    href: '/sensitivity-labels',
  },
  { id: 'audit', labelKey: 'nav.audit', icon: 'audit', href: '/audit' },
  // Administración GLOBAL, al final y separada: no es administración de esta
  // organización sino de toda la plataforma. Solo la ve quien está en
  // `platform_admins`; para el resto, la página responde que no puede entrar.
  { id: 'platform', labelKey: 'nav.platform', icon: 'globe', href: '/platform' },
  // Con qué credenciales se presenta Sytue ante Google, Microsoft o Airtable. Es
  // de la plataforma (sirve a todos los clientes) y por eso vive en esta
  // superficie; sección propia porque estaba al final de la de arriba y llegar
  // exigía saber que había que bajar.
  { id: 'connections', labelKey: 'nav.connections', icon: 'cloud', href: '/conexiones' },
  // Las denuncias de Cast y de Rooms de todas las organizaciones: la cola del
  // operador (ADR 0073 y 0074). Las mismas filas que la cola de cada organización.
  { id: 'moderation', labelKey: 'nav.moderation', icon: 'shield', href: '/moderacion' },
];

export default function ConsoleShell({
  user,
  hubBaseUrl,
  section,
  allowed,
  locale,
  basePath,
  children,
}: ConsoleShellProps) {
  const t = useT(locale);

  // --- La barra de direcciones converge a la canónica (ADR 0056) -----------
  // La vuelta del login aterriza en la forma que el middleware sirve POR DENTRO
  // (`/admin/users?__org=acme`): sirve, pero la marca `__org` es plomería del
  // middleware y no debe quedarse en la barra —ni copiarse dentro de un
  // enlace—. Aquí se reescribe UNA vez a la forma con org
  // (`/admin/acme/users`), conservando query y hash. La consola de plataforma no
  // cuelga de `/admin`: ahí no hay marca que limpiar ni org que poner.
  useEffect(() => {
    const marca = new URLSearchParams(window.location.search).get(MARCA_ORG);
    if (!marca || !RE_SLUG_ORG.test(marca)) return;
    const base = basePath.replace(/\/+$/, '');
    if (!base || !window.location.pathname.startsWith(`${base}/`)) return;
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
  }, [basePath]);

  // El prefijo del host manda sobre el del build (ver `basePath` arriba). `r()`
  // se queda para el resto de la app, que solo se sirve bajo `/admin`. Con
  // prefijo real, la navegación conserva la org de la URL vista (ADR 0056): la
  // canónica es `/admin/<org>/users`. En plataforma —sin prefijo— los enlaces
  // quedan como están: ahí NUNCA hay org.
  const ruta = (p: string) => {
    const base = basePath.replace(/\/+$/, '');
    if (!base) return p;
    return rutaConOrg(base, orgActual(base), p);
  };
  const sidebar = (
    <Sidebar
      // Sin cabecera con «Sytue Admin»: el selector de apps de la barra superior
      // ya lo dice; la barra lateral lo repetía justo debajo (2026-09-09).
      footer={
        <div class="flex flex-col gap-1">
          {/* Con qué organización se está administrando, y con cuál se puede.
              Va en el pie del sidebar y no en el menú de cuenta porque aquí es
              donde importa: cada página de esta consola habla de UNA
              organización, y equivocarse de cuál es el error caro. */}
          <OrgSwitcher t={t} />
          <div class="text-sidebar-fg-muted flex items-center gap-2 px-2 py-1 text-xs">
            <span
              // Con aro: un punto verde sobre un carril verde no se ve.
              class="ring-sidebar-fg h-2 w-2 shrink-0 rounded-full bg-[var(--sytue-presence-online)] ring-[1.5px]"
              aria-hidden="true"
            />
            <span class="truncate">Sytue ID · id.sytue.eu</span>
          </div>
        </div>
      }
    >
      <SidebarSection title={t('shell.sectionAdmin')}>
        {NAV.filter((n) => allowed === undefined || allowed.includes(n.id)).map((n) => (
          <SidebarItem
            key={n.id}
            href={ruta(n.href)}
            label={t(n.labelKey)}
            active={section === n.id}
            icon={<Icon name={n.icon} size={18} />}
          />
        ))}
      </SidebarSection>
      <SidebarSection title={t('shell.sectionAccount')}>
        <SidebarItem
          href={ruta('/account')}
          label={t('nav.account')}
          active={section === 'account'}
          icon={<Icon name="user" size={18} />}
        />
      </SidebarSection>
      {/*
        Aquí había una sección «Identidad» con un distintivo fijo «SCIM activo»,
        escrito en el JSX y sin consultar nada. En producción SCIM está apagado
        (/scim/v2/ServiceProviderConfig responde 503), así que anunciaba que el
        aprovisionamiento —y sobre todo el DESaprovisionamiento— del IdP
        corporativo funcionaba mientras nadie revocaba ninguna cuenta al salir de
        la empresa. Un control de seguridad fingido es peor que ninguno: se deja
        de mirar. Vuelve cuando Sytue ID exponga su estado real (activo / no
        configurado / error) y solo para quien administre la organización.
      */}
    </Sidebar>
  );

  return (
    <SytueShell
      currentApp="console"
      user={user}
      accessToken={getAccessToken}
      hubBaseUrl={hubBaseUrl}
      locale={locale}
    >
      <AdminShell sidebar={sidebar} title={t('shell.brand')}>
        {children}
      </AdminShell>
    </SytueShell>
  );
}
