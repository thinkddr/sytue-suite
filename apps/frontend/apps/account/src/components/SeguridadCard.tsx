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
 *     que cerrar esto sin copiarlos es perderlos: se pueden copiar o bajar en un
 *     `.txt` (CodigosRecuperacion) y no se cierran sin marcar «Los he guardado».
 *
 * Activarla es un asistente con orden (DosPasosWizard); desactivarla, un botón y un
 * diálogo que pide la prueba de identidad, en vez de un formulario siempre a la vista.
 *
 * Y dos más que se confirman ANTES, porque no tienen vuelta atrás: quitar una
 * passkey (ya no se entra con ella) y pedir códigos nuevos (los anteriores dejan
 * de valer en el acto, también los que estén impresos en un cajón).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import {
  Button,
  Card,
  ConfirmDialog,
  copiarAlPortapapeles,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  useConfirm,
  withBase,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import { altaPasskey, soportaPasskeys, type FalloPasskey } from '../lib/passkeys.js';
import { sobreDeContrasena } from '../lib/desbloqueo.js';
import {
  leerPolitica,
  requisitosPassword,
  type PoliticaPassword,
} from '../lib/requisitosPassword.js';
import { RequisitosPassword } from './RequisitosPassword.js';
import { CodigosRecuperacion } from './CodigosRecuperacion.js';
import { DosPasosWizard } from './DosPasosWizard.js';
import { FilaPasskey, type Passkey } from './FilaPasskey.js';
import { sugerirContrasena } from '../lib/contrasenaSegura.js';

type Estado = 'quieto' | 'enviando' | 'error';

export function SeguridadCard({
  t,
  locale = 'en',
  mfaRequired = false,
  sobreDeSeguridad = sobreDeContrasena,
}: {
  t: T;
  /** Para las fechas de las passkeys. */
  locale?: Locale;
  mfaRequired?: boolean;
  /** El sobre del desbloqueo para la contraseña nueva (ADR 0085); el puente de Sytue ID. */
  sobreDeSeguridad?: typeof sobreDeContrasena;
}): JSX.Element {
  const [mfa, setMfa] = useState<boolean | null>(null);
  // El `sub` de Sytue ID: de quién es la clave de desbloqueo que rehace el puente.
  const [sujeto, setSujeto] = useState('');
  const [error, setError] = useState('');
  // Separado de `error` para que `cargar` no dependa de `t`: con la traducción
  // dentro, el efecto de montaje la tenía como dependencia y se reejecutaba al
  // cambiar de idioma. El texto lo pone el render, que es donde se pinta.
  const [fallo, setFallo] = useState(false);

  // Contraseña
  const [actual, setActual] = useState('');
  const [nueva, setNueva] = useState('');
  const [estadoPwd, setEstadoPwd] = useState<Estado>('quieto');
  // La contraseña que se ha sugerido, mientras siga siendo la del campo: entonces se
  // enseña en claro (hay que poder leerla para guardarla) y se ofrece copiarla. En
  // cuanto se teclea encima deja de ser la sugerida y el campo vuelve a ocultarse.
  const [sugerida, setSugerida] = useState<string | null>(null);
  const [copiaSugerida, setCopiaSugerida] = useState<'nada' | 'hecha' | 'fallo'>('nada');
  const [cerrada, setCerrada] = useState(false);
  // La política efectiva que sirve `/me`. Sin ella (servicio antiguo o `/me`
  // caído) no se pinta la lista ni se bloquea nada: el servidor sigue validando.
  const [politica, setPolitica] = useState<PoliticaPassword | null>(null);

  // Segundo factor. El alta va en su asistente (DosPasosWizard), que lleva su
  // propio estado: aquí solo se sabe si está abierto.
  const [asistente, setAsistente] = useState(false);
  const [cuenta, setCuenta] = useState('');
  const [respaldo, setRespaldo] = useState<string[] | null>(null);
  const [respaldoGuardado, setRespaldoGuardado] = useState(false);
  const [estadoMfa, setEstadoMfa] = useState<Estado>('quieto');
  // Desactivar: un diálogo con la prueba de identidad, no un formulario siempre a
  // la vista que invitaba a rellenarlo sin querer.
  const [apagando, setApagando] = useState(false);
  const [codigo, setCodigo] = useState('');
  const [pwdApagar, setPwdApagar] = useState('');
  const [errorApagar, setErrorApagar] = useState('');
  const [aLlave, setALlave] = useState(false);

  // Passkeys
  const [llaves, setLlaves] = useState<Passkey[] | null>(null);
  const [nombreLlave, setNombreLlave] = useState('');
  const [altaEnCurso, setAltaEnCurso] = useState(false);
  // El soporte se resuelve tras hidratar: en el servidor no hay `navigator`, y
  // pintar el botón para luego quitarlo es peor que enseñarlo un instante tarde.
  const [hayWebAuthn, setHayWebAuthn] = useState(false);
  const { confirmar, dialogo } = useConfirm();
  // La página resuelve `?mfa=required` en SSR. El aviso ya aparece en el HTML,
  // incluso mientras /api/me carga o si falla, sin esperar a la hidratación.

  async function cargar(inicial = false): Promise<void> {
    try {
      const res = await (inicial ? primeraLectura('/api/me') : fetch('/api/me'));
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { mfa_enabled?: boolean; email?: string; id?: string };
      setMfa(Boolean(datos.mfa_enabled));
      setSujeto(typeof datos.id === 'string' ? datos.id : '');
      setCuenta(typeof datos.email === 'string' ? datos.email : '');
      setPolitica(leerPolitica(datos));
      setFallo(false);
    } catch {
      setMfa(null);
      setFallo(true);
    }
  }

  async function cargarLlaves(inicial = false): Promise<void> {
    try {
      const res = await (inicial ? primeraLectura('/api/passkeys') : fetch('/api/passkeys'));
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { credentials?: Passkey[] };
      setLlaves(datos.credentials ?? []);
    } catch {
      setLlaves([]);
    }
  }

  useEffect(() => {
    void cargar(true);
    void cargarLlaves(true);
    setHayWebAuthn(soportaPasskeys());
  }, []);

  // «Añadir una llave de acceso» desde el final del asistente: el campo existe
  // cuando el asistente ya se ha desmontado, así que el foco va tras el render.
  // Por id y no por `ref`: `Input` es un componente de función y no la reenvía.
  useEffect(() => {
    if (!aLlave || asistente) return;
    document.getElementById('pk-nombre')?.focus();
    setALlave(false);
  }, [aLlave, asistente]);

  /** Lee el mensaje del servicio: dice por qué falló mejor que un texto genérico. */
  // `null` es que no hubo respuesta (sin red): el texto genérico.
  async function motivo(res: Response | null): Promise<string> {
    if (!res) return t('security.failed');
    try {
      const cuerpo = (await res.json()) as { message?: string; error?: string };
      return cuerpo.message || cuerpo.error || t('security.failed');
    } catch {
      return t('security.failed');
    }
  }

  const verSugerida = sugerida !== null && nueva === sugerida;

  function sugerir(): void {
    const nuevaSugerida = sugerirContrasena(politica);
    setSugerida(nuevaSugerida);
    setNueva(nuevaSugerida);
    setCopiaSugerida('nada');
  }

  async function copiarSugerida(): Promise<void> {
    if (sugerida === null) return;
    setCopiaSugerida((await copiarAlPortapapeles(sugerida)) ? 'hecha' : 'fallo');
  }

  const requisitos = politica ? requisitosPassword(politica, nueva, actual) : [];
  const faltanRequisitos = requisitos.some((r) => !r.cumplido);

  async function cambiarPassword(e: Event): Promise<void> {
    e.preventDefault();
    setEstadoPwd('enviando');
    setError('');
    // ADR 0085: el sobre del desbloqueo con la contraseña nueva viaja con el cambio,
    // y el servicio lo guarda solo si el cambio sale bien. Sin él, el cambio sigue.
    const sobre = await sobreDeSeguridad(sujeto, actual, nueva);
    const res = await fetch('/api/password', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        current_password: actual,
        new_password: nueva,
        ...(sobre ? { unlock_envelope: sobre } : {}),
      }),
    });
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoPwd('error');
      return;
    }
    // Sin volver a pedir nada: esta sesión ya no existe del lado del servicio.
    setActual('');
    setNueva('');
    setSugerida(null);
    setEstadoPwd('quieto');
    setCerrada(true);
  }

  function abrirApagar(): void {
    setCodigo('');
    setPwdApagar('');
    setErrorApagar('');
    setApagando(true);
  }

  async function apagar(): Promise<void> {
    setEstadoMfa('enviando');
    setErrorApagar('');
    let res: Response | null;
    try {
      res = await fetch('/api/mfa/disable', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ code: codigo, password: pwdApagar }),
      });
    } catch {
      res = null;
    }
    if (!res?.ok) {
      // El error se dice DENTRO del diálogo, que sigue abierto con lo escrito.
      setErrorApagar(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    setCodigo('');
    setPwdApagar('');
    setApagando(false);
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

  async function quitarLlave(k: Passkey): Promise<void> {
    const ok = await confirmar({
      mensaje: t('security.passkeys.remove_confirm', {
        name: k.name || t('security.passkeys.unnamed'),
      }),
      confirmLabel: t('security.passkeys.remove'),
      tone: 'danger',
    });
    if (!ok) return;
    setError('');
    try {
      const res = await fetch(`/api/passkeys/${k.id}`, { method: 'DELETE' });
      // El servicio rechaza la última si es el único factor con el que cumplir la
      // verificación; su mensaje explica cómo salir del paso mejor que uno nuestro.
      if (!res.ok) setError(await motivo(res));
    } catch {
      // Sin red el `fetch` rechaza: callar dejaría creer que la llave se ha quitado.
      setError(await motivo(null));
    }
    await cargarLlaves();
  }

  async function regenerar(): Promise<void> {
    const ok = await confirmar({
      mensaje: t('security.codes.regenerate_confirm'),
      confirmLabel: t('security.mfa.new_codes'),
      tone: 'danger',
    });
    if (!ok) return;
    setEstadoMfa('enviando');
    setError('');
    // Sin red el `fetch` rechaza: sin este `try` el estado se quedaba en
    // «enviando», los botones deshabilitados y la tarjeta callada.
    let res: Response;
    try {
      res = await fetch('/api/mfa/recovery-codes', { method: 'POST' });
    } catch {
      setError(await motivo(null));
      setEstadoMfa('error');
      return;
    }
    if (!res.ok) {
      setError(await motivo(res));
      setEstadoMfa('error');
      return;
    }
    const datos = (await res.json()) as { recovery_codes?: string[] };
    setRespaldoGuardado(false);
    setRespaldo(datos.recovery_codes ?? []);
    setEstadoMfa('quieto');
  }

  if (cerrada) {
    return (
      <Card id="seguridad">
        <div class="flex flex-col gap-4 p-4 sm:p-5">
          <h2 class="text-lg font-semibold">{t('security.title')}</h2>
          {/* `alert` y no `status`: esta tarjeta sustituye entera a la del
              formulario, y una región `status` que nace ya con su texto muchos
              lectores de pantalla no la anuncian. Además el aviso importa: las
              sesiones se han cerrado y hay que volver a entrar. */}
          <p role="alert" class="text-sm">
            {t('security.password.done')}
          </p>
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
    <Card id="seguridad">
      <div class="flex flex-col gap-5 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('security.title')}</h2>
        {(error || fallo) && (
          <p role="alert" class="text-danger-text text-sm">
            {error || t('security.failed')}
          </p>
        )}

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
              type={verSugerida ? 'text' : 'password'}
              autocomplete="new-password"
              value={nueva}
              onInput={(e: Event) => setNueva((e.target as HTMLInputElement).value)}
              aria-describedby={politica ? 'pwd-requisitos' : undefined}
              required
            />
          </FieldGroup>
          {/* Una contraseña larga y aleatoria, que cumple la política de la
              organización. Se genera en el navegador (`crypto.getRandomValues`) y
              no viaja a ningún sitio hasta que se pulsa «Cambiar». */}
          <div class="-mt-2 flex flex-wrap items-center gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={sugerir}>
              {t('security.password.suggest')}
            </Button>
            {verSugerida && (
              <Button type="button" variant="ghost" size="sm" onClick={() => void copiarSugerida()}>
                {t('common.copy')}
              </Button>
            )}
            {/* Siempre montada y dentro de la fila: una región `status` que nace con
                el texto no la anuncian todos los lectores, y suelta en el
                formulario sumaría un hueco vacío entre campos. */}
            <span
              role="status"
              class={
                copiaSugerida === 'fallo' ? 'text-danger-text text-xs' : 'text-success-text text-xs'
              }
            >
              {verSugerida && copiaSugerida === 'hecha'
                ? t('common.copied')
                : verSugerida && copiaSugerida === 'fallo'
                  ? t('common.copy_failed')
                  : ''}
            </span>
          </div>
          {verSugerida && (
            <p class="text-text-muted -mt-2 text-xs">{t('security.password.suggested')}</p>
          )}
          {politica && (
            <RequisitosPassword
              t={t}
              id="pwd-requisitos"
              requisitos={requisitos}
              minimo={politica.min_length}
            />
          )}
          <div>
            <Button
              type="submit"
              disabled={estadoPwd === 'enviando' || !actual || !nueva || faltanRequisitos}
            >
              {t('security.password.submit')}
            </Button>
          </div>
        </form>

        <hr class="border-border" />

        {/* --- Verificación en dos pasos --- */}
        <div class="flex flex-col gap-4">
          <h3 class="text-sm font-medium">{t('security.mfa.title')}</h3>

          {mfa === null && <p class="text-text-muted text-sm">{t('common.loading')}</p>}

          {mfaRequired && mfa !== true && (
            // La organización lo exige: bloquea, así que se anuncia como alerta.
            <InlineHelp tone="warning" live="alert">
              {t('security.mfa.required')}
            </InlineHelp>
          )}

          {mfa === false && !asistente && (
            <>
              <p class="text-text-muted text-xs">{t('security.mfa.off')}</p>
              <div>
                <Button onClick={() => setAsistente(true)}>{t('security.mfa.enable')}</Button>
              </div>
            </>
          )}

          {asistente && (
            <DosPasosWizard
              t={t}
              cuenta={cuenta}
              hayWebAuthn={hayWebAuthn}
              onActivada={() => void cargar()}
              onCerrar={(llave) => {
                setAsistente(false);
                setALlave(llave);
              }}
            />
          )}

          {mfa === true && !asistente && (
            <>
              <p class="text-success-text text-xs">{t('security.mfa.on')}</p>
              <div class="flex flex-wrap gap-2">
                <Button
                  variant="ghost"
                  onClick={() => void regenerar()}
                  disabled={estadoMfa === 'enviando'}
                >
                  {t('security.mfa.new_codes')}
                </Button>
                <Button variant="ghost" onClick={abrirApagar} disabled={estadoMfa === 'enviando'}>
                  {t('security.mfa.disable')}
                </Button>
              </div>
            </>
          )}

          {respaldo && (
            <div class="border-border flex flex-col gap-3 rounded border p-3">
              <p class="text-sm font-medium">{t('security.codes.title')}</p>
              <p class="text-text-muted text-xs">{t('security.codes.once')}</p>
              <CodigosRecuperacion
                t={t}
                codigos={respaldo}
                cuenta={cuenta}
                guardados={respaldoGuardado}
                onGuardados={setRespaldoGuardado}
              />
              <div>
                <Button disabled={!respaldoGuardado} onClick={() => setRespaldo(null)}>
                  {t('security.mfa.close')}
                </Button>
              </div>
            </div>
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
                  <FilaPasskey
                    key={k.id}
                    t={t}
                    locale={locale}
                    llave={k}
                    onQuitar={(x) => void quitarLlave(x)}
                    onRenombrada={() => void cargarLlaves()}
                  />
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
        </div>
      </div>
      {dialogo}
      <ConfirmDialog
        open={apagando}
        onClose={() => setApagando(false)}
        onConfirm={() => void apagar()}
        title={t('security.mfa.disable_title')}
        confirmLabel={t('security.mfa.disable')}
        tone="danger"
        busy={estadoMfa === 'enviando'}
        confirmDisabled={!codigo.trim() && !pwdApagar}
      >
        <div class="flex flex-col gap-3">
          <p class="text-text text-sm leading-relaxed">{t('security.mfa.disable_warning')}</p>
          <p class="text-text-muted text-sm">{t('security.mfa.disable_hint')}</p>
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
              value={pwdApagar}
              onInput={(e: Event) => setPwdApagar((e.target as HTMLInputElement).value)}
            />
          </FieldGroup>
          {errorApagar && <FieldHint tone="danger">{errorApagar}</FieldHint>}
        </div>
      </ConfirmDialog>
    </Card>
  );
}

export default SeguridadCard;
