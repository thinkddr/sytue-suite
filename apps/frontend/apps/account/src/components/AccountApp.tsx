/*
 * Mi cuenta — isla raíz. Monta la Sytue Shell (que trae el selector de
 * aplicaciones y la salida al resto de la suite) y, debajo, las secciones de la
 * cuenta.
 *
 * Esta pantalla se servía como HTML desde Sytue ID y por eso no tenía Shell: no
 * había bundler que montara una isla Preact. Replicar el selector en Go obligaba
 * a duplicar el catálogo de productos —nombres, iconos, colores— en un segundo
 * lenguaje, así que se movió la página aquí en vez de duplicar aquello.
 */
import type { JSX } from 'preact';
import { SytueShell } from '@sytue/shell';
import { AppShell, SettingsShell, type SettingsIndexEntry } from '@sytue/ui';
import { esCuentaPersonal, type SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { getAccessToken } from '@sytue/auth/client';
import PerfilCard from './PerfilCard.js';
import SesionesCard from './SesionesCard.js';
import DispositivosCard from './DispositivosCard.js';
import SeguridadCard from './SeguridadCard.js';
import RecuperacionCard from './RecuperacionCard.js';
import AppPasswordsCard from './AppPasswordsCard.js';
import PlanCard from './PlanCard.js';
import BorrarCuentaCard from './BorrarCuentaCard.js';
import RubricaCard from './RubricaCard.js';
import RevisionSeguridad from './RevisionSeguridad.js';
import ActividadCard from './ActividadCard.js';

/**
 * Las secciones de Mi cuenta, en el orden en que se pintan. Los `id` son anclas
 * PÚBLICAS (`/account#seguridad`): enlazan aquí el portal, la revisión de seguridad
 * (`revisionDeCuenta` de `@sytue/shell`), los avisos de Sytue ID y Sytue Mail; no se
 * renombran. Los alias son anclas que ya circulan con otro nombre: `#app-passwords`
 * (Sytue Mail, web y escritorio) y `#claves` (Sytue ID tras dar de alta una llave).
 */
export function indiceDeCuenta(t: T, personal: boolean): SettingsIndexEntry[] {
  return [
    { id: 'perfil', rotulo: t('profile.title') },
    { id: 'seguridad', rotulo: t('security.title'), alias: ['claves'] },
    { id: 'recuperacion', rotulo: t('recovery.title') },
    { id: 'sesiones', rotulo: t('sessions.title') },
    { id: 'dispositivos', rotulo: t('devices.title') },
    { id: 'actividad', rotulo: t('activity.title') },
    { id: 'contrasenas-app', rotulo: t('app_passwords.title'), alias: ['app-passwords'] },
    { id: 'firma', rotulo: t('signature.title') },
    ...(personal ? [{ id: 'plan', rotulo: t('plan.title') }] : []),
    { id: 'borrar', rotulo: t('delete.title') },
  ];
}

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /** Raíz pública de Sytue ID: la usa el enlace al perfil de configuración de Apple. */
  idBaseUrl: string;
  locale: Locale;
  /** Nombre visible actual. Llega del servidor para no parpadear al hidratar. */
  nombreInicial: string;
  /** Correo verificado de la cuenta. Solo lectura: se cambia en otro flujo. */
  email: string;
  /** Una operación protegida ha solicitado configurar el segundo factor. */
  mfaRequired?: boolean;
}

export function AccountApp({
  user,
  hubBaseUrl,
  idBaseUrl,
  locale,
  nombreInicial,
  email,
  mfaRequired = false,
}: Props): JSX.Element {
  const t = useT(locale);
  const personal = esCuentaPersonal(user);
  const indice = indiceDeCuenta(t, personal);
  return (
    // `currentApp="id"` y no una clave propia: esta pantalla ES Sytue ID, solo que
    // servida desde aquí. Inventarle una clave la sacaría del selector como un
    // producto aparte y habría que darle nombre, icono y color propios.
    <SytueShell
      currentApp="id"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* La topbar global la pinta la Sytue Shell: el AppShell ocupa el resto. */}
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        title={t('app.title')}
      >
        {/* UNA COLUMNA CON ÍNDICE, como Google Account o el Apple ID. Eran ocho
            tarjetas en dos columnas, sin título ni forma de enlazar a una sección:
            «/account#seguridad» abría arriba del todo. El índice de `SettingsShell`
            lleva a cada tarjeta por su `id` —que es el ancla pública: la usan la
            revisión de abajo, el portal, los avisos de Sytue ID y Sytue Mail— y
            deja el foco en su título.

            La revisión va PRIMERO y fuera del índice: es el resumen de lo que viene
            debajo y cada aviso lleva a su sección. */}
        <SettingsShell
          title={t('app.title')}
          description={t('app.lead')}
          indice={indice}
          seccionInicial={mfaRequired ? 'seguridad' : undefined}
        >
          <RevisionSeguridad t={t} />
          <PerfilCard t={t} nombreInicial={nombreInicial} email={email} idBaseUrl={idBaseUrl} />
          <SeguridadCard t={t} locale={locale} mfaRequired={mfaRequired} />
          <RecuperacionCard t={t} />
          <SesionesCard t={t} locale={locale} />
          <DispositivosCard t={t} locale={locale} />
          {/* Detrás de sesiones y dispositivos: es lo que se mira cuando algo de
              ahí arriba no cuadra, y lleva de vuelta a ellos. */}
          <ActividadCard t={t} locale={locale} />
          <AppPasswordsCard t={t} locale={locale} />
          <RubricaCard t={t} locale={locale} />
          {/* El plan, solo en cuentas personales: es lo único comercial que una
              persona sola tiene que poder ver y tocar. Una organización factura por
              asientos desde Sytue Admin y aquí no pinta nada. */}
          {personal ? <PlanCard t={t} locale={locale} /> : null}
          <BorrarCuentaCard t={t} idBaseUrl={idBaseUrl} />
        </SettingsShell>
      </AppShell>
    </SytueShell>
  );
}

export default AccountApp;
