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
import { AppShell } from '@sytue/ui';
import { esCuentaPersonal, type SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { getAccessToken } from '@sytue/auth/client';
import PerfilCard from './PerfilCard.js';
import SesionesCard from './SesionesCard.js';
import DispositivosCard from './DispositivosCard.js';
import SeguridadCard from './SeguridadCard.js';
import RecuperacionCard from './RecuperacionCard.js';
import AppPasswordsCard from './AppPasswordsCard.js';
import PlanCard from './PlanCard.js';
import BorrarCuentaCard from './BorrarCuentaCard.js';

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
        {/* DOS COLUMNAS EXPLÍCITAS, no el flujo `columns` de CSS.

            Con `columns` reparte el navegador, por altura, y nadie decide qué
            cae a cada lado: Seguridad —con diferencia la sección más alta— se
            quedaba al final de la izquierda y la derecha era un hueco vacío
            debajo de «Contraseñas de aplicación».

            La razón por la que se eligió `columns` era evitar que una rejilla
            alineara las tarjetas por la más alta. Aquí no pasa: cada columna es
            su propio contenedor, así que las tarjetas no se alinean entre
            columnas y siguen apiladas sin huecos.

            El `mb-6` que llevan las tarjetas es quien las separa; por eso aquí
            NO hay `gap` vertical, que las separaría el doble. El
            `break-inside-avoid` que también llevan ya no hace nada —era del
            flujo multicolumna— y queda inerte.

            `grid-cols-1`: sin plantilla, la columna única era `auto` y medía como
            poco lo que midiera sin partir lo más ancho de dentro. Un texto con
            `truncate` —el nombre de un dispositivo, un correo— no parte, así que en
            vez de recortarse ensanchaba la columna entera: 350 px en una pantalla de
            320. Con `minmax(0, 1fr)` la columna mide lo que hay y el texto se recorta. */}
        <div class="mx-auto grid w-full max-w-5xl grid-cols-1 gap-x-6 p-4 sm:p-6 lg:grid-cols-2 lg:items-start">
          <div>
            {/* El plan, ARRIBA y solo en cuentas personales: es lo único comercial
                que una persona sola tiene que poder ver y tocar. Una organización
                factura por asientos desde Sytue Admin y aquí no pinta nada. */}
            {esCuentaPersonal(user) ? <PlanCard t={t} locale={locale} /> : null}
            <PerfilCard t={t} nombreInicial={nombreInicial} email={email} idBaseUrl={idBaseUrl} />
            <SesionesCard t={t} />
          </div>
          <div>
            <SeguridadCard t={t} mfaRequired={mfaRequired} />
            <RecuperacionCard t={t} />
            <DispositivosCard t={t} locale={locale} />
            <AppPasswordsCard t={t} locale={locale} />
            <BorrarCuentaCard t={t} idBaseUrl={idBaseUrl} />
          </div>
        </div>
      </AppShell>
    </SytueShell>
  );
}

export default AccountApp;
