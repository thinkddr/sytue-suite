/**
 * App switcher de la shell — la MARCA de la suite es el disparador: pulsar el
 * logo de Sytue One abre el AppGrid (mail/drive/calendar/contacts/docs/rooms/
 * cloud/console/hub/id .sytue.eu). Vive en el extremo IZQUIERDO de la topbar,
 * donde el ojo busca la identidad y la navegación de producto, no entre las
 * acciones de cuenta de la derecha. docs/design.md §9.
 *
 * Integrarlo en el logo cuesta el enlace directo a one.sytue.eu, pero no el
 * acceso: «Sytue One» es la PRIMERA entrada del grid (APP_SWITCHER_ORDER), así
 * que Inicio sigue a un clic de distancia y el logo gana una función.
 *
 * El AppGrid ya es la superficie del popover (borde + sombra + radio propios y
 * gestión de teclado). Por eso aquí lo posicionamos directamente en lugar de
 * anidarlo en un Dropdown, evitando el doble marco. Patrón de popover propio,
 * como GlobalSearch.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  AppGrid,
  CLAVE_ADMIN,
  OrgMark,
  appsPresentables,
  hrefDe,
  puedeAdministrar,
  suiteApps,
  Glifo,
} from '@sytue/ui';
import { permsFromAccessToken } from '@sytue/auth/client';
import { sinAdminPorPlan, soloOrganizacion } from '@sytue/auth';
import { appDomains, orgActual, prefijoDeApp } from '@sytue/config';
import { useT } from '@sytue/i18n/preact';
import { useShell } from './context.js';
import { useProductMap } from './useProductMap.js';
import { useAppsDesplegadas } from './useAppsDesplegadas.js';

export function AppSwitcher(): JSX.Element {
  const { currentApp, accessToken, hubBaseUrl, user } = useShell();
  const t = useT();
  // El nombre del producto va junto a la marca (el <span> de abajo): en Mail,
  // Calendar, Contacts y Notepads la barra lateral no lo lleva y nada en
  // pantalla decía en qué aplicación estabas. Sin «Sytue» delante, que ya lo
  // dice la marca de al lado.
  const producto = suiteApps.find((a) => a.key === currentApp)?.name.replace('Sytue ', '');
  const [open, setOpen] = useState(false);
  const [falloLogo, setFalloLogo] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);
  // Quién sirve cada producto en ESTA organización. Se pide al abrir por primera vez;
  // vacío ⇒ todo apunta al producto nativo, que es la degradación correcta.
  const { map, load } = useProductMap(accessToken);
  // Y qué productos levanta esta instalación: no toda despliega la suite entera
  // —el perfil piloto se queda sin Sytue DevX— y una tesela que lleva a un
  // dominio inexistente parece una avería. Se pide al abrir, como el mapa.
  const { desplegadas, load: cargarDesplegadas } = useAppsDesplegadas(hubBaseUrl, accessToken);

  // SYTUE ADMIN SOLO PARA QUIEN PUEDE ENTRAR. Ofrecer una tesela que va a dar un
  // 403 no es un agujero —el gate está en el servidor, `apps/console` exige
  // owner/admin— pero sí mala interfaz: la suite entera enseñaba «Sytue Admin» a
  // cualquier miembro.
  //
  // Los permisos se leen del access token porque los ADMINISTRADORES DELEGADOS
  // no tienen el rol y entran igual; con `user.roles` a secas les quitaríamos la
  // puerta a lo único que administran. Se resuelve el token como hacen los dos
  // hooks de arriba, que admite getter asíncrono.
  const [perms, setPerms] = useState<readonly string[]>([]);
  useEffect(() => {
    let vivo = true;
    void (async () => {
      try {
        const tok = typeof accessToken === 'function' ? await accessToken() : accessToken;
        if (vivo) setPerms(permsFromAccessToken(tok));
      } catch {
        // Sin permisos legibles se cae al lado seguro: manda `user.roles`, y un
        // owner/admin sigue viendo su tesela.
      }
    })();
    return () => {
      vivo = false;
    };
  }, [accessToken]);

  // La organización que llevan las teselas (ADR 0056) es la de la URL ACTUAL: el
  // conmutador vive en la app desde la que se clica, así que el base que mira
  // `orgActual` es el prefijo de ESTA app (`currentApp`), no el del destino de
  // cada tesela. Sin prefijo que mirar —la cuenta de Sytue ID, cuyo host no
  // escopia org— o sin org en la URL (SSR, forma transicional) las teselas salen
  // como siempre: el middleware del destino manda a la canónica con su 302.
  const prefijoActual = prefijoDeApp(currentApp);
  const org = prefijoActual === null ? null : orgActual(prefijoActual);

  // Las teselas sustituidas apuntan a la aplicación elegida y llevan su nombre entre
  // paréntesis: quien pulsa «Hojas de cálculo» debe saber ANTES del clic que va a salir
  // a Google, no descubrirlo al aterrizar en otro dominio.
  // La descripción sale del catálogo común, no de `suiteApps`: ahí está fijada en
  // español y este conmutador aparece en las once aplicaciones, así que la suite
  // entera enseñaba «Correo electrónico» a un usuario alemán. Las claves ya
  // existían traducidas en `@sytue/i18n`; solo faltaba usarlas.
  const apps = appsPresentables(suiteApps, desplegadas)
    // Y no a la cuenta personal gratuita, que es `owner` de la suya y por rol
    // la vería: Sytue Admin es solo con suscripción.
    .filter(
      (app) =>
        app.key !== CLAVE_ADMIN || (puedeAdministrar(user.roles, perms) && !sinAdminPorPlan(user)),
    )
    .map((app) => {
      const description = t(`app.${app.key}.description`);
      const sub = map[app.key];
      const disabled = soloOrganizacion(app.key, user);
      // La sustituida manda —es otra aplicación y ahí no hay org de Sytue—; la
      // nativa sale con la org del navegador y aterriza sin el 302 (ADR 0056).
      return sub
        ? { ...app, href: sub.href, description: `${description} · ${sub.name}`, disabled }
        : { ...app, href: hrefDe(app.key, org), description, disabled };
    });

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (e: MouseEvent) => {
      if (!boxRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  // Cierra y devuelve el foco al disparador (Esc lo gestiona el propio AppGrid).
  const close = () => {
    setOpen(false);
    boxRef.current?.querySelector<HTMLButtonElement>('button')?.focus();
  };

  return (
    <div ref={boxRef} class="relative inline-flex">
      <button
        type="button"
        // El nombre accesible CONTIENE el texto visible (WCAG 2.5.3): «Mail · Aplicaciones».
        aria-label={producto ? `${producto} · ${t('shell.appSwitcher')}` : t('shell.appSwitcher')}
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => {
          load();
          cargarDesplegadas();
          setOpen((v) => !v);
        }}
        // ALTURA FIJA `h-9`, la misma que el buscador, las notificaciones y la
        // cuenta. Sin ella la caja la marcaba el texto del producto y salía a 35px
        // mientras sus vecinos median 31,5: cuatro controles en una fila y el de
        // la izquierda más alto que los otros tres. El relleno lateral se queda
        // —es lo que separa el logo del velo del hover— y ahora cuadra con el aire
        // que deja el avatar del otro extremo. `-ml-1` se come ese `pl-1`: la marca
        // VISIBLE queda a un canalón del cristal, como el avatar (medido en drive, one
        // y mail: 24,5 a la izquierda contra 21 a la derecha); el velo se sale.
        class={`hover:bg-chrome-hover active:bg-chrome-active sytue-focus-ring-chrome rounded-control -ml-1 inline-flex h-9 items-center gap-1.5 pr-1.5 pl-1 transition-colors duration-150 focus-visible:outline-none pointer-coarse:min-h-[max(44px,2.75rem)] ${
          open ? 'bg-chrome-hover' : ''
        }`}
      >
        {/* El logo de la organización si lo ha subido, y si no la marca de la casa
            teñida por su slug. Excluyentes: NUNCA se enmarca ni se tiñe el logo de
            un cliente con nuestro acento.

            `max-w-[160px]` y `object-contain` no son decoración: la anchura real
            solo se sabe al descargar la imagen —depende del aspecto—, así que sin
            tope un wordmark apaisado empujaría el resto de la barra. Si la imagen
            no carga (logo recién borrado con el token todavía vivo), se cae a la
            marca de la casa en vez de dejar el hueco roto. */}
        {user.orgLogo && user.tenant && !falloLogo ? (
          <img
            src={`${appDomains.id}/branding/${encodeURIComponent(user.tenant)}/logo.png?v=${user.orgLogo}`}
            alt=""
            height={28}
            // Placa blanca redondeada, SIEMPRE, como la marca de la casa: el logo de un
            // cliente está pensado para papel blanco y aquí va sobre el carril de color.
            class="h-7 w-auto max-w-[160px] rounded-md bg-white object-contain p-0.5"
            onError={() => setFalloLogo(true)}
          />
        ) : (
          <OrgMark slug={user.tenant} size={28} />
        )}
        {producto ? (
          // Oculto en <640px: ahí la barra no tiene sitio y la app se lee en su
          // propia barra móvil.
          <span class="text-topbar-fg text-sm font-semibold max-sm:hidden">{producto}</span>
        ) : null}
        {/* Chevron discreto: sin él, nada delata que la marca es pulsable. */}
        <Glifo
          d="m6 9 6 6 6-6"
          size={14}
          class={`text-topbar-fg shrink-0 opacity-70 transition-transform duration-150 ${
            open ? 'rotate-180' : ''
          }`}
        />
      </button>
      {open ? (
        <div
          class="sytue-pop-in absolute top-[calc(100%+6px)] left-0"
          style={{ zIndex: 'var(--z-popover)' }}
        >
          <AppGrid apps={apps} current={currentApp} onClose={close} />
        </div>
      ) : null}
    </div>
  );
}
