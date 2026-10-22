/**
 * Menú de cuenta de la shell — avatar con presencia, datos del usuario,
 * selector de presencia, conmutador de tema claro/oscuro y cierre de sesión
 * (logout SSO vía BFF). docs/CONVENTIONS.md §11, docs/design.md §8.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Avatar,
  Dropdown,
  Icon,
  LanguageSelect,
  MenuItem,
  MenuSeparator,
  THEME_COLORS,
  THEME_FORMAS,
  themeColorSwatch,
  useTheme,
  cx,
  useToast,
} from '@sytue/ui';
import { login, logout } from '@sytue/auth/client';
import { appDomains } from '@sytue/config';
import { useT } from '@sytue/i18n/preact';
import type { PresenceState } from '@sytue/api-clients';
import { useShell } from './context.js';

/** Orden de presentacion; la etiqueta sale del catalogo (`presence.<estado>`). */
const PRESENCE_OPTIONS: PresenceState[] = ['online', 'away', 'dnd', 'offline'];

/** Una de MIS organizaciones, tal y como la sirve `/auth/organizations`. */
interface MyOrg {
  id: string;
  slug: string;
  name: string;
  active: boolean;
}

/** Sistema primero: es el valor por defecto y el que la mayoría no toca. */
const APARIENCIAS = ['light', 'translucent', 'dark', 'system'] as const;

/** Un grupo de opciones excluyentes en línea: la apariencia, el color y la forma. */
function Segmentado<T extends string>({
  etiqueta,
  opciones,
  valor,
  elegir,
  rotulo,
  muestra,
}: {
  etiqueta: string;
  opciones: readonly T[];
  valor: T;
  elegir: (opcion: T) => void;
  rotulo: (opcion: T) => string;
  /** Color CSS del círculo que acompaña al rótulo, si la opción es un color. */
  muestra?: (opcion: T) => string;
}): JSX.Element {
  return (
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
      <span class="text-text text-sm">{etiqueta}</span>
      <div
        role="radiogroup"
        aria-label={etiqueta}
        class="bg-bg-subtle flex flex-wrap rounded-md p-0.5"
      >
        {opciones.map((o) => (
          <button
            key={o}
            type="button"
            role="radio"
            aria-checked={valor === o}
            onClick={() => elegir(o)}
            class={cx(
              'sytue-focus-ring rounded-control flex items-center gap-1.5 px-2 py-1 text-xs font-medium transition-colors duration-150',
              valor === o
                ? 'bg-surface text-text shadow-[var(--shadow-xs)]'
                : 'text-text-muted hover:text-text',
            )}
          >
            {muestra && (
              <span
                aria-hidden="true"
                class="size-2.5 rounded-full"
                style={{ background: muestra(o) }}
              />
            )}
            {rotulo(o)}
          </button>
        ))}
      </div>
    </div>
  );
}

/** La opción de un eje que no elige nada: seguir a la organización. */
const SEGUIR = 'org';

/**
 * El selector de un eje que la organización puede fijar por defecto: sus valores y,
 * delante, «la organización», que es NO elegir. Sin ella, quien eligió una vez ya no
 * podía volver a seguirla. Una cuenta personal no tiene quien fije nada (`conOrg`).
 */
function SegmentadoDeEje<T extends string>({
  etiqueta,
  valores,
  elegido,
  deLaOrg,
  elegir,
  rotulo,
  rotuloOrg,
  muestra,
  conOrg,
}: {
  etiqueta: string;
  valores: readonly T[];
  /** Lo que la persona eligió; `null` si sigue a su organización. */
  elegido: T | null;
  deLaOrg: T;
  elegir: (valor: T | null) => void;
  rotulo: (valor: T) => string;
  rotuloOrg: string;
  muestra?: (valor: T) => string;
  conOrg: boolean;
}): JSX.Element {
  const opciones: readonly (T | typeof SEGUIR)[] = conOrg ? [SEGUIR, ...valores] : valores;
  return (
    <Segmentado
      etiqueta={etiqueta}
      opciones={opciones}
      valor={elegido ?? (conOrg ? SEGUIR : deLaOrg)}
      elegir={(o) => elegir(o === SEGUIR ? null : o)}
      rotulo={(o) => (o === SEGUIR ? rotuloOrg : rotulo(o))}
      {...(muestra
        ? { muestra: (o: T | typeof SEGUIR) => muestra(o === SEGUIR ? deLaOrg : o) }
        : {})}
    />
  );
}

export function AccountMenu(): JSX.Element {
  const { user, hub } = useShell();
  const { mode, setMode, colorChoice, orgColor, setColor, formaChoice, orgForma, setForma } =
    useTheme();
  // Una cuenta personal no tiene organización que fije nada: no se le ofrece seguirla.
  const conOrg = user.orgKind !== 'personal';
  const { notify } = useToast();
  const t = useT();
  const [presence, setPresence] = useState<PresenceState>('online');
  // Las organizaciones de esta persona. `null` = todavía no se sabe; con una
  // sola o con ninguna no se pinta selector, porque un desplegable con una
  // opción es una pregunta sin respuesta alternativa.
  const [orgs, setOrgs] = useState<MyOrg[] | null>(null);
  const [cambiandoOrg, setCambiandoOrg] = useState('');

  // Publica presencia "online" al montar la shell.
  useEffect(() => {
    void hub.setPresence('online').catch(() => undefined);
  }, [hub]);

  // Desconexión al cerrar la pestaña o el navegador. `pagehide` y no
  // `beforeunload`: este último no dispara en iOS Safari al cerrar pestaña (solo
  // al navegar dentro de la misma), y `pagehide` cubre los dos casos. Mejor
  // esfuerzo con `keepalive` (ver el SDK): perderlo no rompe nada, la presencia
  // sigue expirando sola por TTL como hasta ahora.
  useEffect(() => {
    const onPageHide = () => void hub.offline().catch(() => undefined);
    window.addEventListener('pagehide', onPageHide);
    return () => window.removeEventListener('pagehide', onPageHide);
  }, [hub]);

  // Se piden una vez al montar. Un fallo deja la lista vacía y el menú
  // exactamente como estaba: esto no puede romper la shell de diez apps.
  useEffect(() => {
    let vivo = true;
    void (async () => {
      try {
        const res = await fetch('/auth/organizations');
        if (!res.ok) throw new Error('http');
        const body = (await res.json()) as { organizations?: MyOrg[] };
        if (vivo) setOrgs(Array.isArray(body.organizations) ? body.organizations : []);
      } catch {
        if (vivo) setOrgs([]);
      }
    })();
    return () => {
      vivo = false;
    };
  }, []);

  // Cambiar de organización y rehacer el login SSO: el `tenant` va firmado
  // dentro del token que este navegador tiene, así que sin ese viaje la
  // aplicación seguiría trabajando con la organización anterior. Con la sesión
  // de Sytue ID viva es una redirección, no volver a escribir la contraseña.
  const cambiarOrg = async (org: MyOrg) => {
    if (org.active || cambiandoOrg) return;
    setCambiandoOrg(org.id);
    try {
      const res = await fetch('/auth/organizations', {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ org_id: org.id }),
      });
      if (!res.ok) throw new Error('http');
      // `login()` y no `/auth/login` a pelo: la shell vive en TODAS las apps y en
      // el dominio de organización cada una va bajo su prefijo (`/mail/auth/login`),
      // así que la ruta desnuda es la raíz del HOST —un 404 de Traefik tras el PUT,
      // medido: las cinco apps lo daban—. El helper compone con el base publicado
      // por el servidor (`window.__sytueBase`), igual que hace `logout()` abajo.
      login(window.location.pathname);
    } catch {
      setCambiandoOrg('');
      notify({
        tone: 'danger',
        title: t('account.switchOrgFailed'),
        description: t('error.connection'),
      });
    }
  };

  // Cambio optimista; si el Hub falla, revierte y avisa.
  const changePresence = async (state: PresenceState) => {
    const previous = presence;
    setPresence(state);
    try {
      await hub.setPresence(state);
    } catch {
      setPresence(previous);
      notify({
        tone: 'danger',
        title: t('account.presenceFailed'),
        description: t('error.connection'),
      });
    }
  };

  return (
    <Dropdown
      label={t('account.menu')}
      trigger={({ onClick, ...aria }) => (
        <button
          type="button"
          onClick={onClick}
          aria-label={t('account.menuFor', { name: user.name })}
          class={`hover:bg-chrome-hover active:bg-chrome-active rounded-control ml-0.5 inline-flex h-9 w-9 items-center justify-center transition-colors duration-150 focus-visible:outline-none pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)] ${
            aria['aria-expanded'] ? 'bg-chrome-hover' : ''
          }`}
          {...aria}
        >
          {/* `xs` (24px) y no `sm` (32): el botón mide 31,5px, así que el avatar
              grande LO DESBORDABA medio píxel y su borde quedaba pegado al del
              control. Con 24 queda el mismo aire dentro de la caja que tiene el
              logo en el otro extremo de la barra —medido: 24,5px del cristal a la
              izquierda contra 21 a la derecha, y ahora los dos a 24,x—. */}
          {/* `sytue-aro-carril`: cuando la barra es de color (en «Marco» siempre; en
              «Carril», en <768px), el disco de la persona va directo sobre él y puede ser
              de su mismo tono. El aro lo pone tokens/appearance.css, que es quien sabe
              cuándo la barra es de color. */}
          <Avatar
            name={user.name}
            src={user.picture}
            size="xs"
            presence={presence}
            class="sytue-aro-carril"
          />
        </button>
      )}
    >
      {/* 272 px y no 256: con 16 más, las cuatro opciones de apariencia caben en una
          línea y el menú mide 24 px menos de alto. Junto con el idioma en una línea y
          las filas de preferencias a 8 px, deja ver «Cerrar sesión» a 1280×800 también
          con dos organizaciones (medido: de 780 px a los que da el arnés de capturas). */}
      <div class="w-68">
        <div class="flex items-center gap-3 px-3 pt-3 pb-2.5">
          <Avatar name={user.name} src={user.picture} size="md" presence={presence} />
          <div class="min-w-0">
            <p class="text-text truncate text-base font-semibold" title={user.name}>
              {user.name}
            </p>
            <p class="text-text-muted truncate text-sm" title={user.email}>
              {user.email}
            </p>
            {user.tenant ? (
              <p
                class="sytue-meta truncate"
                title={t('account.organization', { tenant: user.tenant })}
              >
                {t('account.organization', { tenant: user.tenant })}
              </p>
            ) : null}
          </div>
        </div>
        {/* Con dos o más organizaciones, la línea de arriba dice cuál está
            activa y aquí se cambia. Antes no había forma: la activa era la
            membresía más antigua y quien era `member` en ella no podía
            administrar la otra aunque fuese su propietario. Con una sola
            organización —el caso normal— esto no aparece. */}
        {orgs && orgs.length > 1 ? (
          <>
            <MenuSeparator />
            <p class="sytue-overline text-text-subtle px-3 pt-2 pb-1">{t('account.switchOrg')}</p>
            {orgs.map((o) => (
              <MenuItem
                key={o.id}
                onClick={() => void cambiarOrg(o)}
                icon={<Icon name="building" size={18} />}
              >
                <span class="flex-1 truncate">{o.name}</span>
                {o.active ? <Icon name="check" size={16} /> : null}
              </MenuItem>
            ))}
          </>
        ) : null}
        <MenuSeparator />
        <p class="sytue-overline text-text-subtle px-3 pt-2 pb-1">{t('account.presence')}</p>
        {PRESENCE_OPTIONS.map((state) => (
          <MenuItem
            key={state}
            onClick={() => void changePresence(state)}
            icon={
              // Ancho fijo de 18px para alinear el punto con los iconos del menú.
              <span class="inline-flex w-[18px] justify-center">
                <span
                  class="inline-block h-2.5 w-2.5 rounded-full"
                  style={{ backgroundColor: `var(--sytue-presence-${state})` }}
                  aria-hidden="true"
                />
              </span>
            }
          >
            <span class="flex-1">
              {t(`presence.${state}`)}
              {presence === state ? (
                <span class="sr-only">{t('account.presenceActive')}</span>
              ) : null}
            </span>
            {presence === state ? <Icon name="check" size={16} /> : null}
          </MenuItem>
        ))}
        <MenuSeparator />
        {/* Preferencias de la sesión: apariencia e idioma juntos, que es donde el
            usuario los busca. El idioma se guarda para toda la suite. */}
        <div class="flex flex-col gap-2 px-2.5 py-2">
          {/* Tres opciones y no un interruptor: el interruptor solo sabía claro u
              oscuro, y en cuanto se tocaba se perdía «como el sistema», que es lo
              que espera quien cambia el tema del Mac o del iPhone por la noche. */}
          {/* ENVUELVE, PORQUE EL MENÚ TIENE ANCHO FIJO (entonces 256px, hoy 272) Y EL
              TEXTO NO ES EL MISMO EN TODOS LOS IDIOMAS. En alemán, «Erscheinungsbild» junto a
              «System · Hell · Dunkel» pide unos 267px: se salía 15px y sacaba
              barra de scroll horizontal a TODA la página —el barrido de
              escritorio lo cazó en 79 combinaciones de 123, en las cuatro
              resoluciones, porque el ancho del menú es fijo y no depende de la
              ventana—.

              Con `flex-wrap` los botones bajan a su propia línea solo cuando no
              caben: en español y en inglés se quedan al lado, como estaban. */}
          <Segmentado
            etiqueta={t('account.appearance')}
            opciones={APARIENCIAS}
            valor={mode}
            elegir={setMode}
            rotulo={(m) => t(`account.appearance.${m}`)}
          />
          {/* El color es un eje aparte del modo (ADR 0071). La muestra sale de la
              hoja y no del primario en vigor: tiene que enseñar también el color
              que NO está pintado. */}
          <SegmentadoDeEje
            etiqueta={t('account.color')}
            valores={THEME_COLORS}
            elegido={colorChoice}
            deLaOrg={orgColor}
            elegir={setColor}
            rotulo={(c) => t(`account.color.${c}`)}
            rotuloOrg={t('account.followOrg')}
            muestra={themeColorSwatch}
            conOrg={conOrg}
          />
          {/* Y la forma, el tercer eje: «Marco», la de la casa, o «Carril»
              (docs/design.md §1.0.1, «forma-marco»). */}
          <SegmentadoDeEje
            etiqueta={t('account.forma')}
            valores={THEME_FORMAS}
            elegido={formaChoice}
            deLaOrg={orgForma}
            elegir={setForma}
            rotulo={(f) => t(`account.forma.${f}`)}
            rotuloOrg={t('account.followOrg')}
            conOrg={conOrg}
          />
          <LanguageSelect />
        </div>
        <MenuItem
          icon={<Icon name="user" size={18} />}
          onClick={() => window.location.assign(`${appDomains.id}/account`)}
        >
          {t('account.myAccount')}
        </MenuItem>
        <MenuSeparator />
        {/* Navega al BFF, que cierra la sesión SSO en Sytue ID; el IdP se encarga
            de que las demás superficies cierren la suya. La presencia se marca
            desconectada ANTES de navegar (mejor esfuerzo, sin esperar: `logout`
            navega de inmediato) — si no, queda «en línea» hasta que expire el
            TTL del último latido, pese a que aquí SÍ se sabe que la sesión
            termina. */}
        <MenuItem
          danger
          icon={<Icon name="logout" size={18} />}
          onClick={() => {
            void hub.offline().catch(() => undefined);
            logout();
          }}
        >
          {t('account.signOut')}
        </MenuItem>
      </div>
    </Dropdown>
  );
}
