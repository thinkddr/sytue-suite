/**
 * SytueShell — la Sytue Shell montable en CUALQUIER app Astro+Preact. Compone
 * la TopBar global (logo de la suite + busqueda global + app switcher de 9
 * puntos + notificaciones + presencia + menu de cuenta) y provee el contexto de
 * shell (Hub, usuario, app actual). Toda app de la suite la monta para lograr
 * experiencia unica. docs/CONVENTIONS.md §11, docs/design.md §2.
 *
 * Uso (isla Preact en Astro):
 *   <SytueShell client:load currentApp="mail" user={user} hubBaseUrl="https://hub.sytue.eu" accessToken={token} />
 *
 * El token de acceso lo entrega el BFF (nunca se persiste en el cliente fuera
 * de memoria); para refresco transparente, pasar un getter.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';
import { ThemeProvider, ToastProvider, TopBar, SkipLink } from '@sytue/ui';
import { HubClient, bearer } from '@sytue/api-clients';
import { activateNativeOffline } from '@sytue/offline';
import { useConnectivity } from '@sytue/offline/preact';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { I18nProvider, useT } from '@sytue/i18n/preact';
import { appDomains, type ProductKey, appUrl } from '@sytue/config';
import { listenForDeepLinks, inNativeShell } from './openInApp.js';
import { publicarViewportVisual } from './viewportVisual.js';
import { fotoConVersion } from './avisos.js';
import { ShellContext, useShell } from './context.js';
import { tokenTieneOrganizacion } from './tenantClaim.js';
import { GlobalSearch } from './GlobalSearch.js';
import { AppSwitcher } from './AppSwitcher.js';
import { NotificationCenter } from './NotificationCenter.js';
import { AccountMenu } from './AccountMenu.js';

export interface SytueShellProps {
  /** App actual (resalta su icono en el app switcher). */
  currentApp: ProductKey;
  /** Usuario autenticado (de /auth/session). */
  user: SytueUser;
  /** URL base del Hub (por defecto el Hub del apex actual: dev localhost / prod .eu). */
  hubBaseUrl?: string;
  /** Token de acceso o getter (lo provee el BFF). */
  accessToken: string | (() => string | Promise<string>);
  /** Si se muestra la barra de busqueda global (por defecto true). */
  showSearch?: boolean;
  /**
   * Idioma ya negociado en el servidor (`negotiateLocale` de @sytue/i18n con las
   * cabeceras de la peticion). Si se omite, la shell lo deduce del `<html lang>`
   * al hidratar. docs/CONVENTIONS.md §13.1.
   */
  locale?: Locale;
  /** Contenido renderizado bajo la topbar (la propia app). */
  children?: ComponentChildren;
}

const OFFLINE_PRODUCTS = new Set<ProductKey>([
  'mail',
  'drive',
  'calendar',
  'contacts',
  'docs',
  'sheets',
  'slides',
  'rooms',
]);

const ONBOARDING_URL = appUrl('one', 'bienvenida');

/**
 * Aviso de organización sin derecho de uso.
 *
 * Cuando Sytue ID emite el token en modo bootstrap —sesión SIN organización,
 * porque la suscripción de esa organización no está activa— los diez servicios
 * responden 403 `no_active_organization` a TODA petición de datos. Cada app lo
 * enseñaba como lo que parecía desde dentro: «no se pudieron cargar las
 * carpetas», «no se pudieron cargar los canales». Diez mensajes distintos, la
 * misma causa, y ninguno la nombraba ni decía adónde ir.
 *
 * Va en la shell porque es lo único que está en las diez superficies, y se lee
 * del propio token: no hace ninguna petición, así que aparece incluso cuando
 * todas las demás están fallando.
 */
function SinOrganizacion(): JSX.Element | null {
  const { accessToken } = useShell();
  const t = useT();
  const [token, setToken] = useState<string | undefined>(
    typeof accessToken === 'string' ? accessToken : undefined,
  );
  useEffect(() => {
    if (typeof accessToken === 'string') return;
    let vivo = true;
    void Promise.resolve(accessToken()).then((v) => {
      if (vivo) setToken(v);
    });
    return () => {
      vivo = false;
    };
  }, [accessToken]);

  if (token === undefined || tokenTieneOrganizacion(token)) return null;
  return (
    <div
      class="border-warning/40 bg-warning/10 text-text flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-2.5 text-sm"
      role="status"
    >
      <span class="font-semibold">{t('noOrg.title')}</span>
      <span class="text-text-muted">{t('noOrg.description')}</span>
      <a class="font-semibold underline underline-offset-2" href={ONBOARDING_URL}>
        {t('noOrg.action')}
      </a>
    </div>
  );
}

function OfflineStatus(): JSX.Element | null {
  const connectivity = useConnectivity();
  const t = useT();
  if (connectivity === 'online') return null;
  return (
    <span
      // Más apretado en <640px: con el aviso puesto, la fila de acciones de la
      // topbar se salía 3px de un iPhone SE. Se recorta el aire, no el rótulo:
      // «sin conexión» es justo lo que hay que poder leer yendo por la calle.
      class="border-chrome-border bg-chrome-hover text-header-fg inline-flex h-8 items-center gap-2 rounded-md border px-2.5 text-sm font-semibold max-sm:gap-1.5 max-sm:px-1.5"
      role="status"
      aria-live="polite"
      title={t('offline.description')}
    >
      <span class="bg-warning h-2 w-2 rounded-full" aria-hidden="true" />
      {t('offline.status')}
    </span>
  );
}

// La marca de la suite ya no es un componente aparte: el logo ES el disparador
// del selector de aplicaciones y vive en AppSwitcher.tsx, que la topbar monta
// como `brand`, sobre la cabecera del carril (`rail`): la barra es clara y el
// color de marca vive en esa esquina y en el menú lateral. marker-a1

export function SytueShell({
  currentApp,
  user,
  hubBaseUrl = appDomains.hub,
  accessToken,
  showSearch = true,
  locale,
  children,
}: SytueShellProps): JSX.Element {
  const hub = useMemo(
    () => new HubClient({ baseUrl: hubBaseUrl, auth: bearer(accessToken) }),
    [hubBaseUrl, accessToken],
  );

  // La foto de la barra se cambia EN VIVO: al llegar `event: avatar` por el flujo de
  // la campana (Sytue ID lo manda al cambiarla), con la versión nueva en la URL para
  // que el navegador no pinte la de su caché. Sin esto, solo se veía al entrar.
  const [foto, setFoto] = useState(user.picture);
  useEffect(() => setFoto(user.picture), [user.picture]);
  const cambiarFoto = useCallback(
    (version: string) =>
      setFoto((antes) =>
        fotoConVersion(antes, version, new URL(appUrl(currentApp, 'auth/avatar')).pathname),
      ),
    [currentApp],
  );
  const usuario = useMemo(() => ({ ...user, picture: foto }), [user, foto]);

  // Paleta por app (estilo Mattermost). Fijamos en un envoltorio display:contents
  // los tokens que las UTILIDADES Tailwind leen DE VERDAD (--color-*): el puente
  // --color-*: var(--sytue-*) de theme.css se resuelve UNA vez en :root y no se
  // reevalúa en descendientes, así que el override por-app debe declararse aquí.
  // Con el producto actual conocido, derivamos la rampa ARMONIZADA con color-mix
  // desde 2 bases: el acento (topbar/activo) y la base del menú lateral.
  // Deep links entrantes en las apps móviles: el webview arranca en la raíz de la suite y
  // la URL pedida llega aparte, por el bridge de Capacitor. Aquí, en la shell, porque la
  // monta cada app: la que esté cargada atiende el enlace. No-op en navegador y escritorio.
  useEffect(() => listenForDeepLinks(), []);

  // Teclado virtual de iOS: altura visual en `--sytue-vvh` y vuelta al origen al
  // retirarse (ver viewportVisual.ts). Aquí por lo mismo que los deep links: una vez.
  useEffect(() => publicarViewportVisual(), []);

  // Un solo punto de activación para todas las apps capaces de trabajar sin
  // conexión. El service worker y su caché quedan aislados por usuario/producto.
  useEffect(() => {
    if (!OFFLINE_PRODUCTS.has(currentApp)) return;
    return activateNativeOffline({ ownerId: user.id, product: currentApp });
  }, [currentApp, user.id]);

  // Aquí vivía una rampa de color POR APP: un objeto de 20 custom properties que
  // derivaba topbar, cabecera, menú lateral, selección, primario y anillo de foco de
  // `var(--sytue-header-<app>)`. Se ha borrado entera, y esto es lo que había pasado.
  //
  // Cuando la suite migró a cromo grafito único, los 14 `--sytue-header-*` pasaron a
  // resolver TODOS a `var(--sytue-chrome)` (colors.css:158-171, y su propio comentario lo
  // dice). Los overrides se quedaron, pero ya no derivaban de un acento por producto sino
  // de un gris. Sus comentarios seguían hablando de «acento sólido del producto» y de
  // «granate, índigo…»: la huella exacta de una migración a medias (docs/design.md §4.0).
  //
  // Lo que rompía, medido:
  //   · `--sytue-focus-ring` = grafito al 55 % sobre superficie oscura ⇒ 1,06:1. El anillo
  //     de foco era INVISIBLE en tema oscuro en toda la suite (WCAG 2.4.11).
  //   · `--color-sidebar-hover` = mezcla de dos grises casi iguales ⇒ 1,03:1 contra el
  //     propio rail: el hover del menú lateral no existía en ninguna app.
  //   · `--sytue-primary` = grafito, así que el primario de la suite dejaba de ser latón.
  //   · `--color-header` = blanco, contradiciendo `theme.css:120`, mientras los controles
  //     de Drive y Rooms seguían pintando velos claros (`bg-chrome-hover`) encima.
  //
  // No hace falta sustituirlo por nada: `:root` ya define la rampa completa y calibrada
  // —topbar, sidebar con sus escalones, selección y anillo de foco, en claro y oscuro
  // (colors.css)—, que es justo el sitio donde el puente de theme.css la puede leer.

  // EL TOPBAR ES LO ÚNICO QUE NO EXISTE EN LAS APPS NATIVAS. En el móvil cada
  // producto es una app aparte (Sytue Mail, Sytue Drive…): el conmutador de la
  // suite no tiene a dónde conmutar, y búsqueda global, notificaciones y cuenta
  // son cromo de NAVEGADOR de escritorio. Todo lo demás —sidebars, listas,
  // editores, avisos— es la misma web y sigue apareciendo. En escritorio nativo
  // (la suite en Gio) esta shell ni se carga; la detección cubre el bridge y a las
  // futuras ventanas web que vuelvan a inyectarlo.
  const nativo = inNativeShell();

  return (
    <I18nProvider locale={locale}>
      <ThemeProvider>
        <ToastProvider>
          <ShellContext.Provider
            value={{ currentApp, hub, hubBaseUrl, user: usuario, accessToken, cambiarFoto }}
          >
            {/* Sin barra de la suite se reserva solo la zona segura superior. El
                contenido resta ese mismo alto, y no una segunda banda de 56 px. */}
            <div
              class="sytue-shell contents"
              style={nativo ? { '--spacing-topbar': 'var(--sytue-safe-top, 0px)' } : undefined}
            >
              <SkipLink />
              <SinOrganizacion />
              {nativo ? (
                <div class="sytue-native-safe-top sytue-topbar-carril" aria-hidden="true" />
              ) : (
                <TopBar
                  rail
                  brand={<AppSwitcher />}
                  center={showSearch ? <GlobalSearch /> : undefined}
                  actions={
                    <>
                      <OfflineStatus />
                      <NotificationCenter />
                      <span
                        class="bg-chrome-divider marco:invisible mx-1 h-6 w-px"
                        aria-hidden="true"
                      />
                      <AccountMenu />
                    </>
                  }
                />
              )}
              {children}
            </div>
          </ShellContext.Provider>
        </ToastProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
