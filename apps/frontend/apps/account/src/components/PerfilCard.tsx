/*
 * Perfil — los siete campos que edita su dueño, más el @usuario y el correo en solo
 * lectura.
 *
 * El correo no se toca aquí a propósito: cambiarlo exige verificar la dirección
 * nueva, y ese flujo vive en Sytue ID.
 *
 * El idioma sí, y no es cosmético: es el que usa Sytue ID para escribirle a esta
 * persona, y la cookie que fija el BFF al guardarlo vale para toda la suite.
 *
 * Guarda contra `PATCH /api/v1/me` mandando SOLO lo que ha cambiado, que es lo
 * que hace que dos pestañas abiertas no se pisen campos que ninguna ha tocado.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  AvatarPicker,
  Button,
  Card,
  FieldGroup,
  Input,
  Select,
  withBase,
  type ImageErrorCode,
} from '@sytue/ui';
import { LOCALES, LOCALE_NAMES } from '@sytue/i18n';
import type { AccountMessageKey, T } from '../i18n/index.js';

/** Código de fallo de imagen -> clave de catálogo (mismo mapa que Contactos). */
const PHOTO_ERROR_KEY: Record<ImageErrorCode, AccountMessageKey> = {
  format: 'photo.errorFormat',
  process: 'photo.errorProcess',
  tooLarge: 'photo.errorTooLarge',
  read: 'photo.errorRead',
};

/** La foto la sirve `/auth/avatar`, la misma ruta que pinta la barra de la suite. */
function avatarUrl(version: string): string {
  return withBase(import.meta.env.BASE_URL, `/auth/avatar?v=${encodeURIComponent(version)}`);
}

interface Props {
  t: T;
  nombreInicial: string;
  email: string;
  /** Raíz pública de Sytue ID, para el perfil de configuración de Apple. */
  idBaseUrl: string;
}

/** Los campos de texto, en el orden en que se pintan. */
const CAMPOS = [
  { clave: 'name', etiqueta: 'profile.name', pista: 'profile.name.hint', max: 120 },
  { clave: 'given_name', etiqueta: 'profile.given_name', max: 120 },
  { clave: 'family_name', etiqueta: 'profile.family_name', max: 120 },
  { clave: 'title', etiqueta: 'profile.job_title', max: 120 },
  { clave: 'department', etiqueta: 'profile.department', max: 120 },
  { clave: 'location', etiqueta: 'profile.location', max: 120 },
] as const;

type Clave = (typeof CAMPOS)[number]['clave'] | 'locale';
type Ficha = Record<Clave, string>;
type Estado = 'quieto' | 'guardando' | 'guardado' | 'error';

const VACIA: Ficha = {
  name: '',
  given_name: '',
  family_name: '',
  title: '',
  department: '',
  location: '',
  locale: 'en',
};

export function PerfilCard({ t, nombreInicial, email, idBaseUrl }: Props): JSX.Element {
  // El nombre viene del servidor para que la tarjeta no parpadee al hidratar; el
  // resto llega con la primera lectura, que es lo único que hay que esperar.
  const [ficha, setFicha] = useState<Ficha>({ ...VACIA, name: nombreInicial });
  const [original, setOriginal] = useState<Ficha | null>(null);
  const [estado, setEstado] = useState<Estado>('quieto');
  // El @usuario (ADR 0040) es de solo lectura: en una cuenta personal ES su
  // dirección, y cambiarlo sería cambiar el correo.
  const [usuario, setUsuario] = useState('');
  const [hasAvatar, setHasAvatar] = useState(false);
  const [avatarVersion, setAvatarVersion] = useState('');
  const [avatarBusy, setAvatarBusy] = useState(false);

  // `nombreInicial` en las dependencias: es una prop y el efecto la usa de
  // respaldo si el servicio no devuelve nombre. En la práctica no cambia —la
  // pinta el SSR una vez—, pero declararlo cuesta una palabra y evita razonar
  // sobre si algún día cambiará.
  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch('/api/me');
        if (!res.ok) return;
        const d = (await res.json()) as Partial<Record<Clave | 'username', string>> & {
          has_avatar?: boolean;
          avatar_version?: string;
        };
        setUsuario(d.username ?? '');
        setHasAvatar(Boolean(d.has_avatar));
        setAvatarVersion(d.avatar_version ?? '');
        const leida: Ficha = {
          name: d.name ?? nombreInicial,
          given_name: d.given_name ?? '',
          family_name: d.family_name ?? '',
          title: d.title ?? '',
          department: d.department ?? '',
          location: d.location ?? '',
          locale: d.locale ?? 'en',
        };
        setFicha(leida);
        setOriginal(leida);
      } catch {
        /* la tarjeta sigue editable con lo que vino del servidor */
      }
    })();
  }, [nombreInicial]);

  function editar(clave: Clave, valor: string): void {
    setFicha((f) => ({ ...f, [clave]: valor }));
    if (estado !== 'quieto') setEstado('quieto');
  }

  async function pickAvatar(dataUri: string): Promise<void> {
    setAvatarBusy(true);
    try {
      const res = await fetch('/api/avatar', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ photo: dataUri }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setHasAvatar(true);
      setAvatarVersion(String(Date.now()));
    } finally {
      setAvatarBusy(false);
    }
  }

  async function removeAvatar(): Promise<void> {
    setAvatarBusy(true);
    try {
      const res = await fetch('/api/avatar', { method: 'DELETE' });
      if (!res.ok) throw new Error(String(res.status));
      setHasAvatar(false);
    } finally {
      setAvatarBusy(false);
    }
  }

  async function guardar(e: Event): Promise<void> {
    e.preventDefault();
    setEstado('guardando');
    // Solo lo que ha cambiado. Mandar la ficha entera convierte cada guardado en
    // una escritura de siete columnas y pisa lo que otra pestaña acabe de tocar.
    const parche: Partial<Ficha> = {};
    for (const clave of Object.keys(ficha) as Clave[]) {
      if (!original || ficha[clave] !== original[clave]) parche[clave] = ficha[clave];
    }
    if (Object.keys(parche).length === 0) {
      setEstado('guardado');
      return;
    }
    try {
      const res = await fetch('/api/me', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(parche),
      });
      if (!res.ok) throw new Error(String(res.status));
      setOriginal({ ...ficha });
      setEstado('guardado');
    } catch {
      setEstado('error');
    }
  }

  return (
    <Card class="mb-6 break-inside-avoid">
      <form onSubmit={(e) => void guardar(e)} class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('profile.title')}</h2>

        <AvatarPicker
          name={ficha.name.trim() || nombreInicial}
          src={hasAvatar ? avatarUrl(avatarVersion) : undefined}
          hasPhoto={hasAvatar}
          busy={avatarBusy}
          onPick={pickAvatar}
          onRemove={removeAvatar}
          labels={{
            change: t('profile.avatarChange'),
            add: t('profile.avatarAdd'),
            presets: t('profile.avatarPresets'),
            upload: t('profile.avatarUpload'),
            remove: t('profile.avatarRemove'),
            error: (code) => t(PHOTO_ERROR_KEY[code]),
          }}
        />

        {CAMPOS.map((campo) => (
          <FieldGroup
            key={campo.clave}
            label={t(campo.etiqueta)}
            htmlFor={`perfil-${campo.clave}`}
            {...('pista' in campo ? { help: t(campo.pista) } : {})}
            required={campo.clave === 'name'}
          >
            <Input
              id={`perfil-${campo.clave}`}
              value={ficha[campo.clave]}
              onInput={(e: Event) => editar(campo.clave, (e.target as HTMLInputElement).value)}
              maxLength={campo.max}
              required={campo.clave === 'name'}
            />
          </FieldGroup>
        ))}

        <FieldGroup
          label={t('profile.locale')}
          htmlFor="perfil-locale"
          help={t('profile.locale.hint')}
        >
          <Select
            id="perfil-locale"
            value={ficha.locale}
            onChange={(e: Event) => editar('locale', (e.target as HTMLSelectElement).value)}
          >
            {LOCALES.map((codigo) => (
              <option key={codigo} value={codigo}>
                {LOCALE_NAMES[codigo]}
              </option>
            ))}
          </Select>
        </FieldGroup>

        {usuario ? (
          <FieldGroup
            label={t('profile.username')}
            htmlFor="perfil-username"
            help={t('profile.username.hint')}
          >
            <Input id="perfil-username" value={`@${usuario}`} readOnly />
          </FieldGroup>
        ) : null}

        <FieldGroup
          label={t('profile.email')}
          htmlFor="perfil-email"
          help={t('profile.email.hint')}
        >
          <Input id="perfil-email" value={email} readOnly />
        </FieldGroup>

        {/* EL PERFIL DE APPLE ES UN ENLACE, no un proxy.
            Lo sirve Sytue ID exigiendo su COOKIE de sesión, no un bearer, así que
            el BFF no puede pedirlo en nombre de nadie. Pero quien está en esta
            pantalla tiene sesión SSO en el IdP por definición —entró por él—, y
            en una navegación top-level la cookie `SameSite=Lax` viaja: el enlace
            descarga. Proxyarlo habría obligado a duplicar el generador del plist
            o a inventar un endpoint con bearer para un fichero que ya existe. */}
        <a
          href={`${idBaseUrl}/account/apple.mobileconfig`}
          // `min-h-[24px]`: el enlace medía 16 de alto, que es lo que ocupa su
          // línea de texto, y por debajo del mínimo de WCAG 2.5.8 (AA). La
          // excepción de ese criterio es para enlaces DENTRO de una frase; este
          // va suelto, con su propia línea, así que cuenta como control. En px y no
          // `min-h-6`: eso es 1,5 rem, 21 px con la raíz de la suite a 14.
          class="text-primary inline-flex min-h-[24px] items-center self-start text-sm font-medium underline"
        >
          {t('profile.apple')}
        </a>
        <p class="text-text-muted -mt-2 text-xs">{t('profile.apple.hint')}</p>

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={estado === 'guardando' || !ficha.name.trim()}>
            {estado === 'guardando' ? t('profile.saving') : t('profile.save')}
          </Button>
          {/* `aria-live` y no un toast: el resultado de guardar es la única
              información nueva de la tarjeta, y un lector de pantalla debe
              anunciarla sin que haya que ir a buscarla. */}
          <span aria-live="polite" class="text-sm">
            {estado === 'guardado' && <span class="text-success">{t('profile.saved')}</span>}
            {estado === 'error' && <span class="text-danger">{t('profile.failed')}</span>}
          </span>
        </div>
      </form>
    </Card>
  );
}

export default PerfilCard;
