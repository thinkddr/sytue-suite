/*
 * Seguridad: contraseña y verificación en dos pasos.
 *
 * Las dos operaciones tienen una consecuencia que hay que CONTAR antes de que
 * ocurra, no después:
 *
 *   - Cambiar la contraseña cierra todas las sesiones, incluida esta. La
 *     pantalla lo dice y, al terminar, manda a entrar de nuevo en vez de quedarse
 *     mostrando datos que ya no se pueden refrescar.
 *   - Los códigos de recuperación se enseñan UNA vez. Solo se guarda su hash, así
 *     que cerrar esto sin copiarlos es perderlos: la pantalla obliga a confirmar
 *     que se han guardado antes de quitarlos de en medio.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, FieldGroup, Input, withBase } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { altaPasskey, soportaPasskeys, type FalloPasskey } from '../lib/passkeys.js';

type Estado = 'quieto' | 'enviando' | 'error';

interface Passkey {
  id: string;
  name: string;
  last_used?: string | null;
}

interface Enrolamiento {
  otpauth_uri: string;
  secret: string;
  /** `data:image/png;base64,…` ya montado por el servicio. Ausente si no cupo. */
  qr_png?: string;
}

export function SeguridadCard({ t }: { t: T }): JSX.Element {
  const [mfa, setMfa] = useState<boolean | null>(null);
  const [error, setError] = useState('');
  // Separado de `error` para que `cargar` no dependa de `t`: con la traducción
  // dentro, el efecto de montaje la tenía como dependencia y se reejecutaba al
  // cambiar de idioma. El texto lo pone el render, que es donde se pinta.
  const [fallo, setFallo] = useState(false);

  // Contraseña
  const [actual, setActual] = useState('');
  const [nueva, setNueva] = useState('');
  const [estadoPwd, setEstadoPwd] = useState<Estado>('quieto');
  const [cerrada, setCerrada] = useState(false);

  // Segundo factor
  const [alta, setAlta] = useState<Enrolamiento | null>(null);
  const [codigo, setCodigo] = useState('');
  const [respaldo, setRespaldo] = useState<string[] | null>(null);
  const [estadoMfa, setEstadoMfa] = useState<Estado>('quieto');

  // Passkeys
  const [llaves, setLlaves] = useState<Passkey[] | null>(null);
  const [nombreLlave, setNombreLlave] = useState('');
  const [altaEnCurso, setAltaEnCurso] = useState(false);
  // El soporte se resuelve tras hidratar: en el servidor no hay `navigator`, y
  // pintar el botón para luego quitarlo es peor que enseñarlo un instante tarde.
  const [hayWebAuthn, setHayWebAuthn] = useState(false);
  // `?mfa=required`: Sytue ID manda aquí a quien intentó abrir algo que exige el
  // segundo factor sin tenerlo (authorize.go, `desviarAStepUp`). Sin el aviso, la
  // vuelta a la cuenta parecía un rebote sin motivo. Se lee al hidratar: en el
  // servidor no hay `location`.
  const [exigida, setExigida] = useState(false);

  async function cargar(): Promise<void> {
    try {
      const res = await fetch('/api/me');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { mfa_enabled?: boolean };
      setMfa(Boolean(datos.mfa_enabled));
      setFallo(false);
    } catch {
      setMfa(null);
      setFallo(true);
    }
  }

  async function cargarLlaves(): Promise<void> {
    try {
      const res = await fetch('/api/passkeys');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { credentials?: Passkey[] };
      setLlaves(datos.credentials ?? []);
    } catch {
      setLlaves([]);
    }
  }

  useEffect(() => {
    void cargar();
    void cargarLlaves();
    setHayWebAuthn(soportaPasskeys());
    setExigida(new URLSearchParams(window.location.search).get('mfa') === 'required');
  }, []);

  /** Lee el mensaje del servicio: dice por qué falló mejor que un texto genérico. */
  async function motivo(res: Response): Promise<string> {
    try {
      const cuerpo = (await res.json()) as { message?: string; error?: string };
      return cuerpo.message || cuerpo.error || t('security.failed');
    } catch {
      return t('security.failed');
    }
  }

  async function cambiarPassword(e: Event): Promise<void> {
    e.preventDefault();
    setEstadoPwd('enviando');
    setError('');
    const res = await fetch('/api/password', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ current_password: actual, new_password: nueva }),
    });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoPwd('error');
      return;
    }
    // Sin volver a pedir nada: esta sesión ya no existe del lado del servicio.
    setActual('');
    setNueva('');
    setEstadoPwd('quieto');
    setCerrada(true);
  }

  async function empezarAlta(): Promise<void> {
    setEstadoMfa('enviando');
    setError('');
    const res = await fetch('/api/mfa/enroll', { method: 'POST' });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    setAlta((await res.json()) as Enrolamiento);
    setEstadoMfa('quieto');
  }

  async function confirmarAlta(e: Event): Promise<void> {
    e.preventDefault();
    setEstadoMfa('enviando');
    setError('');
    const res = await fetch('/api/mfa/confirm', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ code: codigo }),
    });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    const datos = (await res.json()) as { recovery_codes?: string[] };
    setRespaldo(datos.recovery_codes ?? []);
    setAlta(null);
    setCodigo('');
    setEstadoMfa('quieto');
    await cargar();
  }

  async function apagar(e: Event): Promise<void> {
    e.preventDefault();
    setEstadoMfa('enviando');
    setError('');
    const res = await fetch('/api/mfa/disable', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ code: codigo, password: actual }),
    });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    setCodigo('');
    setActual('');
    setEstadoMfa('quieto');
    await cargar();
  }

  async function anadirLlave(e: Event): Promise<void> {
    e.preventDefault();
    setAltaEnCurso(true);
    setError('');
    const fallo: FalloPasskey | null = await altaPasskey(nombreLlave.trim());
    setAltaEnCurso(false);
    // Cancelar el diálogo del sistema NO es un error del que informar: quien lo
    // ha cerrado sabe lo que ha hecho, y decírselo suena a que algo se ha roto.
    if (fallo && fallo !== 'cancelado') setError(t(`security.passkeys.${fallo}`));
    if (!fallo) setNombreLlave('');
    await cargarLlaves();
  }

  async function quitarLlave(id: string): Promise<void> {
    setError('');
    const res = await fetch(`/api/passkeys/${id}`, { method: 'DELETE' });
    // El servicio rechaza la última si es el único factor con el que cumplir la
    // verificación; su mensaje explica cómo salir del paso mejor que uno nuestro.
    if (!res.ok) setError(await motivo(res));
    await cargarLlaves();
  }

  async function regenerar(): Promise<void> {
    setEstadoMfa('enviando');
    setError('');
    const res = await fetch('/api/mfa/recovery-codes', { method: 'POST' });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    const datos = (await res.json()) as { recovery_codes?: string[] };
    setRespaldo(datos.recovery_codes ?? []);
    setEstadoMfa('quieto');
  }

  if (cerrada) {
    return (
      <Card class="mb-6 break-inside-avoid">
        <div class="flex flex-col gap-4 p-4 sm:p-5">
          <h2 class="text-lg font-semibold">{t('security.title')}</h2>
          <p class="text-sm">{t('security.password.done')}</p>
          {/* `withBase`: la app se sirve bajo `/account`, así que `/auth/login` a
              secas es la raíz del host, donde no hay nada que enrutar. Lo vigila
              `rutaDeLogin.test.ts`, que fue quien lo cazó aquí. */}
          <a
            href={withBase(import.meta.env.BASE_URL, '/auth/login')}
            class="text-primary text-sm font-medium underline"
          >
            {t('security.password.signin_again')}
          </a>
        </div>
      </Card>
    );
  }

  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-5 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('security.title')}</h2>
        {(error || fallo) && <p class="text-danger text-sm">{error || t('security.failed')}</p>}

        {/* --- Contraseña --- */}
        {/* `(e) => void f(e)` y no `f` a secas: un manejador que devuelve una
            promesa en un atributo que espera `void` deja los rechazos sin
            recoger —el navegador los tira como «unhandled rejection» y el
            formulario se queda como si nada hubiera pasado—. Es el patrón que
            usa el resto de la suite. */}
        <form onSubmit={(e) => void cambiarPassword(e)} class="flex flex-col gap-4">
          <h3 class="text-sm font-medium">{t('security.password.title')}</h3>
          <p class="text-text-muted text-xs">{t('security.password.warning')}</p>
          <FieldGroup label={t('security.password.current')} htmlFor="pwd-actual" required>
            <Input
              id="pwd-actual"
              type="password"
              autocomplete="current-password"
              value={actual}
              onInput={(e: Event) => setActual((e.target as HTMLInputElement).value)}
              required
            />
          </FieldGroup>
          <FieldGroup label={t('security.password.new')} htmlFor="pwd-nueva" required>
            <Input
              id="pwd-nueva"
              type="password"
              autocomplete="new-password"
              value={nueva}
              onInput={(e: Event) => setNueva((e.target as HTMLInputElement).value)}
              required
            />
          </FieldGroup>
          <div>
            <Button type="submit" disabled={estadoPwd === 'enviando' || !actual || !nueva}>
              {t('security.password.submit')}
            </Button>
          </div>
        </form>

        <hr class="border-border" />

        {/* --- Verificación en dos pasos --- */}
        <div class="flex flex-col gap-4">
          <h3 class="text-sm font-medium">{t('security.mfa.title')}</h3>

          {mfa === null && <p class="text-text-muted text-sm">{t('common.loading')}</p>}

          {mfa === false && exigida && (
            <p role="alert" class="bg-warning-bg text-warning-text rounded-md p-3 text-sm">
              {t('security.mfa.required')}
            </p>
          )}

          {mfa === false && !alta && (
            <>
              <p class="text-text-muted text-xs">{t('security.mfa.off')}</p>
              <div>
                <Button onClick={() => void empezarAlta()} disabled={estadoMfa === 'enviando'}>
                  {t('security.mfa.enable')}
                </Button>
              </div>
            </>
          )}

          {alta && (
            <form onSubmit={(e) => void confirmarAlta(e)} class="flex flex-col gap-3">
              <p class="text-text-muted text-xs">{t('security.mfa.scan')}</p>
              {/* PNG y no SVG: varios gestores de TOTP dejan «subir una imagen
                  del código» y filtran el selector por PNG/JPEG, donde un SVG ni
                  aparece. Lo pinta el servicio (totp/qr.go) y aquí solo se
                  enseña, sin inyectar marcado. */}
              {alta.qr_png && (
                <img
                  src={alta.qr_png}
                  alt={t('security.mfa.qr_alt')}
                  width={180}
                  height={180}
                  class="self-start rounded bg-white p-2"
                />
              )}
              {/* La clave en texto SIEMPRE, no solo cuando falta el QR: es la única
                  vía si se enrola desde el mismo dispositivo que muestra la pantalla. */}
              <p class="text-text-muted text-xs">{t('security.mfa.manual')}</p>
              <code class="bg-surface-raised rounded px-2 py-1 font-mono text-sm break-all">
                {alta.secret}
              </code>
              <FieldGroup label={t('security.mfa.code')} htmlFor="mfa-codigo" required>
                <Input
                  id="mfa-codigo"
                  inputMode="numeric"
                  autocomplete="one-time-code"
                  value={codigo}
                  onInput={(e: Event) => setCodigo((e.target as HTMLInputElement).value)}
                  required
                />
              </FieldGroup>
              <div class="flex gap-2">
                <Button type="submit" disabled={estadoMfa === 'enviando' || !codigo}>
                  {t('security.mfa.confirm')}
                </Button>
                <Button variant="ghost" onClick={() => setAlta(null)}>
                  {t('common.cancel')}
                </Button>
              </div>
            </form>
          )}

          {mfa === true && (
            <>
              <p class="text-success text-xs">{t('security.mfa.on')}</p>
              <div class="flex flex-wrap gap-2">
                <Button
                  variant="ghost"
                  onClick={() => void regenerar()}
                  disabled={estadoMfa === 'enviando'}
                >
                  {t('security.mfa.new_codes')}
                </Button>
              </div>
              <form onSubmit={(e) => void apagar(e)} class="flex flex-col gap-3">
                <p class="text-text-muted text-xs">{t('security.mfa.disable_hint')}</p>
                <FieldGroup label={t('security.mfa.code_or_password')} htmlFor="mfa-prueba">
                  <Input
                    id="mfa-prueba"
                    inputMode="numeric"
                    autocomplete="one-time-code"
                    value={codigo}
                    onInput={(e: Event) => setCodigo((e.target as HTMLInputElement).value)}
                  />
                </FieldGroup>
                <FieldGroup label={t('security.password.current')} htmlFor="mfa-pwd">
                  <Input
                    id="mfa-pwd"
                    type="password"
                    autocomplete="current-password"
                    value={actual}
                    onInput={(e: Event) => setActual((e.target as HTMLInputElement).value)}
                  />
                </FieldGroup>
                <div>
                  <Button
                    type="submit"
                    variant="ghost"
                    disabled={estadoMfa === 'enviando' || (!codigo && !actual)}
                  >
                    {t('security.mfa.disable')}
                  </Button>
                </div>
              </form>
            </>
          )}

          {/* Las passkeys, aquí y no en una tarjeta aparte: son el otro factor
              con el que se cumple la verificación de arriba, y separarlas las
              alejaría de aquello para lo que sirven. El ALTA no está: WebAuthn
              exige un diálogo del navegador y va en su propia tanda. */}
          {llaves !== null && (
            <div class="flex flex-col gap-2">
              <h3 class="text-sm font-medium">{t('security.passkeys.title')}</h3>
              <p class="text-text-muted text-xs">{t('security.passkeys.hint')}</p>
              <ul class="flex flex-col gap-2">
                {llaves.map((k) => (
                  <li key={k.id} class="flex items-center justify-between gap-3">
                    <span class="truncate text-sm">{k.name || t('security.passkeys.unnamed')}</span>
                    <Button variant="ghost" onClick={() => void quitarLlave(k.id)}>
                      {t('security.passkeys.remove')}
                    </Button>
                  </li>
                ))}
              </ul>

              {hayWebAuthn ? (
                <form onSubmit={(e) => void anadirLlave(e)} class="flex flex-col gap-3">
                  <FieldGroup
                    label={t('security.passkeys.name')}
                    htmlFor="pk-nombre"
                    help={t('security.passkeys.name.hint')}
                  >
                    <Input
                      id="pk-nombre"
                      value={nombreLlave}
                      onInput={(e: Event) => setNombreLlave((e.target as HTMLInputElement).value)}
                      maxLength={60}
                    />
                  </FieldGroup>
                  <div>
                    <Button type="submit" disabled={altaEnCurso}>
                      {altaEnCurso ? t('security.passkeys.adding') : t('security.passkeys.add')}
                    </Button>
                  </div>
                </form>
              ) : (
                <p class="text-text-muted text-xs">{t('security.passkeys.no_soportado')}</p>
              )}
            </div>
          )}

          {respaldo && (
            <div class="border-border flex flex-col gap-2 rounded border p-3">
              <p class="text-sm font-medium">{t('security.codes.title')}</p>
              <p class="text-text-muted text-xs">{t('security.codes.once')}</p>
              <ul class="grid grid-cols-2 gap-1 font-mono text-sm">
                {respaldo.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
              <div>
                <Button variant="ghost" onClick={() => setRespaldo(null)}>
                  {t('security.codes.saved')}
                </Button>
              </div>
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

export default SeguridadCard;
