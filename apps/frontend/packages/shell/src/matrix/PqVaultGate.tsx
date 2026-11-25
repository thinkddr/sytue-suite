/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren, JSX } from 'preact';
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { MatrixStorageProduct } from '@sytue/config/matrix';
import { prefijoDeApp } from '@sytue/config/tokens';
import { registerLogoutCleanup } from '@sytue/auth/client';
import { createTranslate, type Locale } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import {
  Button,
  Checkbox,
  cuandoAppLista,
  Icon,
  IconButton,
  Modal,
  Spinner,
  ToastProvider,
  useUltimo,
} from '@sytue/ui';
import {
  bridgeHandoffKey,
  cancelPqVaultCopy,
  PqVault,
  PqVaultDisabledError,
  PqVaultError,
  pqVaultProfile,
  readPqVaultCopy,
  vaultFailureCode,
  type PqVaultCopyOptions,
  type PqVaultExpect,
  type PqVaultOpenOptions,
} from '@sytue/offline/vault';
import type { MatrixGateMessage } from './MatrixDeviceGate.js';
import {
  releaseBeforeEmpty,
  rememberPendingEmpty,
  sourceReleaseClient,
  takePendingEmpty,
  type SourceReleaser,
} from './source-release.js';
import { MatrixStorageFixtureContext } from './storage-fixture-context.js';
import {
  VaultStatusContext,
  VaultStatusSinkContext,
  type VaultIndicatorState,
  type VaultStatus,
} from './vault-status.js';

/**
 * Mensajes de la puerta: los generales de Matrix (título, desbloqueo, inicio de
 * sesión) más los propios de la bóveda directa. Los de «verifica desde otro
 * dispositivo» no aplican aquí: en ADR 0086 la clave vive en Sytue ID, no en
 * otro dispositivo, así que lo que se ofrece es volver a entrar o vaciar.
 */
export type PqVaultGateMessage =
  | MatrixGateMessage
  | 'vault.wrongKey'
  | 'vault.signInAgain'
  | 'vault.empty'
  | 'vault.emptyTitle'
  | 'vault.emptyBody'
  | 'vault.emptyUnderstood'
  | 'vault.failed'
  | 'vault.copyInProgress'
  | 'vault.copyCancel'
  | 'vault.copyCancelHint'
  | 'vault.moreInfo'
  | 'vault.attention';

function normalizeBase(value: string | undefined): string {
  if (!value || value === '/') return '';
  // Sin regex: `/\/+$/` es cuadrático con muchas barras seguidas.
  let end = value.length;
  while (end > 0 && value.charCodeAt(end - 1) === 47) end--;
  return value.slice(0, end);
}

function buildSignInUrl(basePath: string, returnTo: string): string {
  const params = new URLSearchParams({ unlock: '1', returnTo });
  const key = bridgeHandoffKey();
  if (key) params.set('key', key);
  return `${normalizeBase(basePath)}/auth/login?${params.toString()}`;
}

/**
 * «Vaciar» que pidió identificarse antes de tocar la bóveda (a este navegador le
 * faltaba la clave de desbloqueo con la que sellar la nueva). La marca, de esta
 * pestaña y esta cuenta, dice a la vuelta que la persona ya confirmó vaciar: sin
 * ella, la puerta abría la bóveda de antes, volvía a fallar y había que vaciar otra
 * vez. Solo se pone si la bóveda no se llegó a vaciar: nunca se vacía dos veces.
 */
const resetIntentKey = (product: string) => 'sytue.vault-reset.' + product;
const resetIntentLifetime = 10 * 60 * 1000;

function rememberResetIntent(product: string, actor: string, tenant: string | undefined): void {
  try {
    sessionStorage.setItem(
      resetIntentKey(product),
      JSON.stringify({ actor, tenant: tenant ?? '', at: Date.now() }),
    );
  } catch {
    // Sin almacenamiento de sesión, a la vuelta se pide vaciar otra vez.
  }
}

// La vuelta automática a Sytue ID (`autoSignIn`) se hace una vez por pestaña y
// producto en este rato: si a la vuelta la bóveda sigue sin abrir, ir otra vez sería
// un bucle, y la persona ve el botón y lo que pasa.
const AUTO_SIGN_IN_WINDOW = 3 * 60 * 1000;
function autoSignInKey(product: string): string {
  return `sytue.vault-auto-signin.${product}`;
}
function takeAutoSignIn(product: string, now: number): boolean {
  try {
    const raw = sessionStorage.getItem(autoSignInKey(product));
    const at = raw ? Number(raw) : NaN;
    if (Number.isFinite(at) && now >= at && now - at < AUTO_SIGN_IN_WINDOW) return false;
    sessionStorage.setItem(autoSignInKey(product), String(now));
    return true;
  } catch {
    // Sin almacenamiento no hay cómo evitar el bucle: mejor el botón.
    return false;
  }
}
function forgetAutoSignIn(product: string): void {
  try {
    sessionStorage.removeItem(autoSignInKey(product));
  } catch {
    // Nada que olvidar.
  }
}

function takeResetIntent(product: string, actor: string, tenant: string | undefined): boolean {
  try {
    const raw = sessionStorage.getItem(resetIntentKey(product));
    sessionStorage.removeItem(resetIntentKey(product));
    if (!raw) return false;
    const value = JSON.parse(raw) as { actor?: unknown; tenant?: unknown; at?: unknown };
    const age = typeof value.at === 'number' ? Date.now() - value.at : -1;
    return (
      value.actor === actor &&
      value.tenant === (tenant ?? '') &&
      age >= 0 &&
      age <= resetIntentLifetime
    );
  } catch {
    return false;
  }
}

async function closeThenDrain(closing: Promise<void> | undefined, vault: PqVault): Promise<void> {
  try {
    await closing;
  } finally {
    await vault.closeAndDrain();
  }
}

export interface PqVaultGateProps {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  product: MatrixStorageProduct;
  basePath?: string | undefined;
  prepare?: (vault: PqVault, signal: AbortSignal) => Promise<void>;
  activate?: (vault: PqVault) => void;
  detach?: (vault: PqVault) => void;
  close?: (vault: PqVault) => Promise<void>;
  /** Sustituible en pruebas unitarias de la puerta. */
  openVault?: (options: PqVaultOpenOptions) => Promise<PqVault>;
  resetVault?: (options: PqVaultOpenOptions) => Promise<PqVault>;
  /** Cancela la copia en curso de otro dispositivo (sustituible en pruebas). */
  cancelCopy?: (options: PqVaultCopyOptions) => Promise<void>;
  /**
   * La fuente del servidor que se suelta antes de vaciar (Drive). `null`, ninguna.
   * Por defecto, la de Drive: su bóveda nueva importa el Drive del servidor, y si la
   * fuente siguiera congelada no tendría de dónde (sustituible en pruebas).
   */
  sourceReleaser?: SourceReleaser | null;
  /** Navegación a Sytue ID (sustituible en pruebas). */
  navigate?: (url: string) => void;
  /**
   * Ir sola a Sytue ID cuando a este navegador le falta la clave (`signin`), tiene
   * una que no es la de la bóveda (`wrong`) o Sytue ID pide identificarse de nuevo
   * (`reauth`), en vez de enseñar «Iniciar sesión». Una vez por pestaña y producto
   * cada pocos minutos: si a la vuelta sigue igual, se enseña el botón. Por defecto,
   * sí.
   */
  autoSignIn?: boolean;
  /**
   * Arranca el worker del núcleo a la vez que el bootstrap de la bóveda; devuelve cómo
   * soltarlo si nadie llega a usarlo. Por defecto no hace falta: lo hace la propia
   * apertura (`PqVault.open`), también para quien abre la bóveda sin puerta.
   */
  prewarm?: (() => () => void) | undefined;
  /**
   * La app no guarda nada en la bóveda: sus hijos (estáticos) nunca la reciben. Se
   * pintan YA, piden sus datos del servidor como sin puerta, y la bóveda se abre en
   * segundo plano cuando la app está lista (`marcarAppLista`). Su estado va al
   * candado de la barra de la suite; los fallos no tapan la app. Solo con hijos
   * estáticos: unos hijos que reciben la bóveda siguen esperándola.
   */
  background?: boolean | undefined;
  /**
   * Con hijos que reciben la bóveda (Mail, Drive): el armazón y su esqueleto mientras
   * se abre, en vez de la pantalla de la puerta. No debe leer nada de la bóveda: se
   * pinta antes de abrirla. Como función recibe la franja «Desbloqueando…» para
   * ponerla donde quiera; si no, va encima.
   */
  skeleton?: ComponentChildren | ((status: JSX.Element) => ComponentChildren);
  children: ComponentChildren | ((vault: PqVault) => ComponentChildren);
}

const defaultOpenVault = (options: PqVaultOpenOptions): Promise<PqVault> => PqVault.open(options);
const defaultResetVault = (options: PqVaultOpenOptions): Promise<PqVault> => PqVault.reset(options);
/** Cancela la marca que haya ahora (la que la persona tiene delante). */
const defaultCancelCopy = async (options: PqVaultCopyOptions): Promise<void> => {
  const marker = await readPqVaultCopy(options);
  if (marker) await cancelPqVaultCopy(options, marker.id);
};

/**
 * Puerta de cifrado post-cuántico directo (ADR 0086).
 *
 * Abre la bóveda `age-MLKEM768-X25519-v1` del usuario mediante el puente de
 * Sytue ID (`id.<dominio>`) y `services/drive` (`/api/pq-vault/*`), sin pasar
 * por Synapse ni cargar `matrix-js-sdk`.
 */
export function PqVaultGate({
  actor,
  tenant,
  locale,
  product,
  basePath,
  prepare,
  activate,
  detach,
  close,
  openVault = defaultOpenVault,
  resetVault = defaultResetVault,
  cancelCopy = defaultCancelCopy,
  sourceReleaser,
  navigate = (url) => window.location.assign(url),
  autoSignIn = true,
  prewarm,
  background = false,
  skeleton,
  children,
}: PqVaultGateProps): JSX.Element {
  const fixture = useContext(MatrixStorageFixtureContext);
  // En segundo plano solo con hijos estáticos: los que reciben la bóveda la esperan.
  const enSegundoPlano = background && typeof children !== 'function';
  const resolvedBase = useMemo(
    () => normalizeBase(basePath ?? prefijoDeApp(product === 'hub' ? 'one' : product) ?? ''),
    [basePath, product],
  );
  // Solo Drive suelta su fuente al vaciar: es la app cuya bóveda importa del servidor.
  const releaseOwner = useMemo(
    () => (!fixture && tenant && product === 'drive' ? { actor, tenant, product } : null),
    [fixture, actor, tenant, product],
  );
  const releaser = useMemo<SourceReleaser | null>(
    () =>
      sourceReleaser !== undefined
        ? sourceReleaser
        : releaseOwner
          ? sourceReleaseClient({ ...releaseOwner, basePath: resolvedBase })
          : null,
    [sourceReleaser, releaseOwner, resolvedBase],
  );
  const releaserRef = useRef(releaser);
  useEffect(() => {
    releaserRef.current = releaser;
  });
  const translate = useMemo(() => createTranslate(locale), [locale]);
  const t = (key: PqVaultGateMessage): string => translate(key);

  const [vault, setVault] = useState<PqVault | null>(null);
  const [ready, setReady] = useState(false);
  const [disabled, setDisabled] = useState(false);
  const [busy, setBusy] = useState(!fixture);
  const [needSignIn, setNeedSignIn] = useState(false);
  // Sytue ID pide identificarse de nuevo (`reauth`): el inicio de sesión va con
  // `reauth=1` (`max_age=0`), que obliga a teclear la clave aunque haya sesión.
  const [needReauth, setNeedReauth] = useState(false);
  const [wrongKey, setWrongKey] = useState(false);
  // Otro dispositivo está pasando su Drive de Matrix a esta bóveda (ADR 0086).
  const [copying, setCopying] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<PqVaultGateMessage | null>(null);
  // El código corto del último fallo genérico (`prepare/drive.import.row_heads`): va
  // en «Más información» y en la consola, para saber qué comprobación lo rechazó.
  const [failureCode, setFailureCode] = useState<string | null>(null);
  const [show, setShow] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [tick, setTick] = useState(0);
  // En segundo plano, hasta que la app está lista la bóveda ni se empieza a abrir.
  const [aplazada, setAplazada] = useState(enSegundoPlano && !fixture);
  const esperaHechaRef = useRef(!enSegundoPlano);
  // Cuántas barras enseñan el candado: con alguna, no se pinta el flotante.
  const [reclamado, setReclamado] = useState(0);
  const calentar = prewarm;
  const calentarRef = useRef(calentar);

  const liveVaultRef = useRef<PqVault | null>(null);
  // La navegación por defecto es una función nueva en cada pintado: el efecto que
  // abre la bóveda no depende de ella. La última se apunta tras pintar (`useUltimo`),
  // no durante el pintado: escribir una ref ahí es lo que el lint rechaza.
  const navigateRef = useUltimo(navigate);
  const resetRequestedRef = useRef(false);
  // Lo que se vio de la bóveda cuando la clave no la abrió: vaciar solo vacía eso.
  const wrongExpectRef = useRef<PqVaultExpect | undefined>(undefined);
  const hasFunctionChildrenRef = useRef(typeof children === 'function');
  const prepareRef = useRef(prepare);
  const activateRef = useRef(activate);
  const detachRef = useRef(detach);
  const closeRef = useRef(close);
  useEffect(() => {
    hasFunctionChildrenRef.current = typeof children === 'function';
    prepareRef.current = prepare;
    activateRef.current = activate;
    detachRef.current = detach;
    closeRef.current = close;
    calentarRef.current = calentar;
  });

  const syntheticVault = useMemo<PqVault | null>(() => {
    if (!fixture) return null;
    return {
      scope: Object.freeze({ tenant: tenant ?? 'fixture', subject: actor, product }),
      profile: pqVaultProfile,
      idOrigin: 'https://id.fixture.invalid',
      vaultId: '!pq_fixture',
      keyId: 'fixture000000000000000000000000000000000000000000000000000000000',
      recipient: 'age1pq1fixture',
      isClosed: false,
      close: () => {},
      closeAndDrain: async () => {},
    } as unknown as PqVault;
  }, [fixture, actor, tenant, product]);

  useEffect(() => {
    if (syntheticVault) {
      setVault(syntheticVault);
      setReady(true);
      setBusy(false);
      setDisabled(false);
      setNeedSignIn(false);
      setWrongKey(false);
      setError(null);
      return;
    }

    const controller = new AbortController();
    let mounted = true;
    let opened: PqVault | null = null;
    // Si la bóveda no llegó a usar el worker precalentado, se termina.
    let soltarNucleo: (() => void) | undefined;
    let teardownStarted = false;

    const teardown = (instance: PqVault | null): Promise<void> => {
      if (!instance || teardownStarted) return Promise.resolve();
      teardownStarted = true;
      if (liveVaultRef.current === instance) liveVaultRef.current = null;
      detachRef.current?.(instance);
      instance.close();
      return closeThenDrain(closeRef.current?.(instance), instance);
    };

    const unregisterLogout = registerLogoutCleanup(async () => {
      controller.abort();
      await teardown(opened ?? liveVaultRef.current);
    });

    setVault(null);
    setReady(false);
    setDisabled(false);
    setBusy(true);
    setNeedSignIn(false);
    setNeedReauth(false);
    setWrongKey(false);
    setCopying(false);
    setError(null);
    setFailureCode(null);

    let shouldReset = resetRequestedRef.current;
    resetRequestedRef.current = false;
    // De vuelta de identificarse tras un «vaciar» que no llegó a vaciar.
    if (!shouldReset && takeResetIntent(product, actor, tenant)) shouldReset = true;
    const expect = shouldReset ? wrongExpectRef.current : undefined;
    wrongExpectRef.current = undefined;
    // Dónde iba la apertura cuando falló: vaciar o soltar la fuente, abrir la
    // bóveda, o preparar la app sobre ella (la importación de Drive).
    let stage: 'release' | 'open' | 'prepare' = 'open';
    void (async () => {
      try {
        // Una app que no guarda nada en la bóveda no reparte su primera carga con
        // el núcleo: se espera a que esté lista, una sola vez.
        if (!esperaHechaRef.current) {
          await cuandoAppLista(controller.signal);
          if (!mounted || controller.signal.aborted) return;
          esperaHechaRef.current = true;
          setAplazada(false);
        }
        // De vuelta de Sytue ID tras confirmar «vaciar»: se suelta la época que la
        // persona vio y se vacía, sin volver a preguntar.
        const pendingRelease = releaserRef.current;
        if (!shouldReset && pendingRelease && releaseOwner) {
          const confirmed = takePendingEmpty(releaseOwner);
          if (confirmed) {
            stage = 'release';
            const released = await releaseBeforeEmpty(pendingRelease, controller.signal, confirmed);
            if (released.outcome !== 'none' && released.outcome !== 'released')
              throw new PqVaultError('unavailable');
            shouldReset = true;
            stage = 'open';
          }
        }
        controller.signal.throwIfAborted();
        // El núcleo empieza a bajar YA, en paralelo con el bootstrap, las claves y el
        // puente: lo arranca la propia apertura (`PqVault.open`). Un `prewarm` propio
        // (las pruebas) se arranca aquí, igual de pronto.
        soltarNucleo = calentarRef.current?.();
        const openFn = shouldReset ? resetVault : openVault;
        const instance = await openFn({
          actor,
          tenant,
          product,
          basePath: resolvedBase,
          signal: controller.signal,
          ...(expect ? { expect } : {}),
        });
        opened = instance;
        if (!mounted || controller.signal.aborted) {
          await teardown(instance);
          return;
        }
        if (
          instance.scope.subject !== actor ||
          (tenant !== undefined && instance.scope.tenant !== tenant) ||
          instance.scope.product !== product
        ) {
          await teardown(instance);
          throw new PqVaultError('identity');
        }
        liveVaultRef.current = instance;
        if (prepareRef.current) {
          stage = 'prepare';
          await prepareRef.current(instance, controller.signal);
        }
        if (!mounted || controller.signal.aborted || instance.isClosed) {
          await teardown(instance);
          return;
        }
        forgetAutoSignIn(product);
        setVault(instance);
        setReady(true);
        setBusy(false);
        activateRef.current?.(instance);
      } catch (failure) {
        // La bóveda no llegó a abrirse (ni, con `shouldReset`, a vaciarse).
        const notOpened = opened === null;
        if (opened) {
          await teardown(opened).catch(() => {});
          opened = null;
        }
        if (!mounted || controller.signal.aborted) return;
        setBusy(false);
        if (failure instanceof PqVaultDisabledError) {
          if (!hasFunctionChildrenRef.current) {
            setDisabled(true);
            return;
          }
          setError('matrix.failed');
          return;
        }
        if (failure instanceof PqVaultError) {
          // La clave es de la cuenta y vive en Sytue ID: sin ella aquí, con una que
          // no es la de la bóveda o con Sytue ID pidiendo identificarse de nuevo, se
          // va a buscarla sin pedir nada a la persona (una vez; ver autoSignIn). En
          // segundo plano no: la app ya está en pantalla y no la necesita para
          // enseñar lo suyo; su candado lo dice.
          if (
            (failure.reason === 'signin' ||
              failure.reason === 'reauth' ||
              failure.reason === 'wrong') &&
            autoSignIn &&
            !enSegundoPlano &&
            takeAutoSignIn(product, Date.now())
          ) {
            if (failure.reason === 'signin' && shouldReset && notOpened)
              rememberResetIntent(product, actor, tenant);
            setBusy(true);
            navigateRef.current(
              buildSignInUrl(resolvedBase, window.location.pathname + window.location.search) +
                (failure.reason === 'reauth' ? '&reauth=1' : ''),
            );
            return;
          }
          if (failure.reason === 'signin') {
            if (shouldReset && notOpened) rememberResetIntent(product, actor, tenant);
            setNeedSignIn(true);
            return;
          }
          if (failure.reason === 'reauth') {
            setNeedReauth(true);
            setNeedSignIn(true);
            return;
          }
          if (failure.reason === 'wrong') {
            wrongExpectRef.current = failure.expect;
            setWrongKey(true);
            return;
          }
          if (failure.reason === 'copy') {
            setCopying(true);
            return;
          }
        }
        // Cualquier otro fallo (clave que no cuadra, bóveda corrupta, migración que
        // no entra) deja ofrecer reintentar, volver a entrar o vaciar. Con su código:
        // sin él, en producción no había forma de saber qué comprobación fue.
        const code = `${stage}/${vaultFailureCode(failure)}`;
        console.warn(`[sytue] la bóveda de ${product} no abrió: ${code}`);
        setFailureCode(code);
        setError('vault.failed');
      } finally {
        soltarNucleo?.();
      }
    })();

    return () => {
      mounted = false;
      unregisterLogout();
      soltarNucleo?.();
      controller.abort();
      void teardown(opened ?? liveVaultRef.current);
    };
  }, [
    actor,
    tenant,
    product,
    resolvedBase,
    syntheticVault,
    openVault,
    resetVault,
    releaseOwner,
    autoSignIn,
    enSegundoPlano,
    tick,
    // Estable (`useUltimo`): está para el lint, no vuelve a abrir la bóveda.
    navigateRef,
  ]);

  // «Vaciar»: con fuente en el servidor (Drive), primero se suelta la fuente
  // congelada; si para eso hay que identificarse otra vez, se vuelve aquí y se
  // termina (takePendingEmpty). Después, la bóveda se vacía y se vuelve a abrir.
  const emptyVault = async () => {
    const current = releaserRef.current;
    if (current && releaseOwner) {
      setBusy(true);
      setError(null);
      setWrongKey(false);
      const released = await releaseBeforeEmpty(current);
      if (released.outcome === 'reauth') {
        rememberPendingEmpty(releaseOwner, released.epoch);
        navigate(
          buildSignInUrl(resolvedBase, window.location.pathname + window.location.search) +
            '&reauth=1',
        );
        return;
      }
      if (released.outcome === 'failed') {
        setBusy(false);
        setFailureCode('release/failed');
        setError('vault.failed');
        return;
      }
    }
    resetRequestedRef.current = true;
    setTick((value) => value + 1);
  };

  const bound =
    vault !== null &&
    ready &&
    !vault.isClosed &&
    vault.scope.subject === actor &&
    (tenant === undefined || vault.scope.tenant === tenant) &&
    vault.scope.product === product;

  // El estado para el candado de la barra: nada mientras la bóveda ni se ha empezado
  // a abrir (segundo plano) o si no hay cifrado en este despliegue.
  const estado: VaultIndicatorState | null =
    aplazada || disabled ? null : bound ? 'abierta' : busy ? 'abriendo' : 'atencion';
  const abrirDetalles = useCallback(() => setShow(true), []);
  const reclamar = useCallback(() => {
    setReclamado((value) => value + 1);
    return () => setReclamado((value) => value - 1);
  }, []);
  const status = useMemo<VaultStatus | null>(
    () => (estado ? { estado, abrirDetalles, reclamar } : null),
    [estado, abrirDetalles, reclamar],
  );
  // Dentro de la shell (Drive, Docs): la barra de arriba enseña este estado.
  const sink = useContext(VaultStatusSinkContext);
  useEffect(() => {
    sink?.(status);
  }, [sink, status]);
  useEffect(() => () => sink?.(null), [sink]);
  const enLaBarra = sink !== null || reclamado > 0;

  if (disabled && typeof children !== 'function' && !enSegundoPlano) {
    return <>{children}</>;
  }

  // Con `reauth` (Sytue ID pide identificarse de nuevo), con `max_age=0`: si no,
  // el relevo entrega el código con la sesión de antes y vuelve el 403.
  const signIn = (): void => {
    navigate(
      buildSignInUrl(resolvedBase, window.location.pathname + window.location.search) +
        (needReauth && needSignIn ? '&reauth=1' : ''),
    );
  };

  // La jerga (algoritmos, perfil, huella de la clave) va en «Más información»: la
  // pantalla dice qué pasa y qué hacer. El barrido de pantallas reconoce la puerta
  // por `data-sytue-boveda`, no por este texto (`test/e2e/tests/puerta-cifrado.ts`).
  const contents = (
    <div class="flex flex-col gap-4">
      <h1 class="sytue-h3">{t('matrix.title')}</h1>
      <p class="text-text-muted text-sm">{t('matrix.subtitle')}</p>
      {error ? (
        <p class="text-danger-text text-sm" role="alert">
          {t(error)}
        </p>
      ) : null}
      {busy ? (
        <p class="flex items-center gap-2" role="status">
          <Spinner size={18} />
          {t('matrix.unlocking')}
        </p>
      ) : null}
      {!busy && needSignIn ? (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm">{t('matrix.signInBody')}</p>
          <Button onClick={signIn}>{t('matrix.signIn')}</Button>
        </div>
      ) : null}
      {!busy && (wrongKey || error === 'vault.failed') ? (
        <div class="flex flex-col gap-3">
          {wrongKey ? <p class="text-text-muted text-sm">{t('vault.wrongKey')}</p> : null}
          <div class="flex flex-wrap gap-2">
            <Button onClick={signIn}>{t('vault.signInAgain')}</Button>
            <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
              {t('action.retry')}
            </Button>
          </div>
          {/* Lo destructivo, aparte y sin relleno: no puede leerse como «Reintentar». */}
          <div class="border-border-subtle border-t pt-3">
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="trash" size={16} />}
              onClick={() => {
                setUnderstood(false);
                setResetting(true);
              }}
            >
              {t('vault.empty')}
            </Button>
          </div>
        </div>
      ) : null}
      {!busy && copying ? (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm" role="status">
            {t('vault.copyInProgress')}
          </p>
          <p class="text-text-muted text-sm">{t('vault.copyCancelHint')}</p>
          <div class="flex flex-wrap gap-2">
            <Button onClick={() => setTick((value) => value + 1)}>{t('action.retry')}</Button>
            <Button
              variant="secondary"
              disabled={cancelling}
              onClick={() => {
                setCancelling(true);
                void cancelCopy({ actor, tenant, product, basePath: resolvedBase })
                  .catch(() => {})
                  .finally(() => {
                    setCancelling(false);
                    setTick((value) => value + 1);
                  });
              }}
            >
              {t('vault.copyCancel')}
            </Button>
          </div>
        </div>
      ) : null}
      {bound ? <p class="text-success-text">{t('matrix.trusted')}</p> : null}
      {!busy && !bound && !needSignIn && !wrongKey && !copying && error !== 'vault.failed' ? (
        <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      ) : null}
      <details class="text-text-muted text-sm">
        <summary class="w-fit cursor-pointer select-none">{t('vault.moreInfo')}</summary>
        <p class="mt-2 font-mono text-xs break-all">
          ML-KEM-768 + X25519 · {pqVaultProfile}
          {bound && vault ? ` · ${vault.keyId.slice(0, 16)}` : ''}
        </p>
        {!bound && error === 'vault.failed' && failureCode ? (
          <p class="mt-1 font-mono text-xs break-all" data-sytue-boveda-fallo={failureCode}>
            {failureCode}
          </p>
        ) : null}
      </details>
    </div>
  );

  // El candado de reserva, solo si ninguna barra de la suite lo enseña (la shell
  // nativa no tiene barra). Abierta o pidiendo atención; nunca mientras abre. Sube
  // por encima del botón flotante de la app cuando lo hay, en vez de taparlo.
  const candado =
    status && !enLaBarra && status.estado !== 'abriendo' ? (
      <IconButton
        label={t(status.estado === 'atencion' ? 'vault.attention' : 'matrix.title')}
        size="lg"
        class="sytue-popover-surface border-border-subtle fixed right-[max(0.75rem,var(--sytue-safe-right,0px))] bottom-[max(0.75rem,var(--sytue-safe-bottom,0px))] z-40 border shadow-[var(--shadow-sm)] [body:has([data-sytue-fab])_&]:bottom-[calc(max(1rem,var(--sytue-safe-bottom,0px))+4.5rem)]"
        onClick={abrirDetalles}
      >
        <>
          <Icon name="lock" size={16} />
          {status.estado === 'atencion' ? (
            <span
              class="bg-danger absolute top-1.5 right-1.5 h-2 w-2 rounded-full"
              aria-hidden="true"
            />
          ) : null}
        </>
      </IconButton>
    ) : null;

  const detalles = (
    <Modal open={show} onClose={() => setShow(false)} title={t('matrix.title')}>
      {contents}
    </Modal>
  );

  const vaciar = (
    <Modal open={resetting} onClose={() => setResetting(false)} title={t('vault.emptyTitle')}>
      <div class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('vault.emptyBody')}</p>
        <Checkbox
          checked={understood}
          onChange={(event) => setUnderstood(event.currentTarget.checked)}
          label={t('vault.emptyUnderstood')}
        />
        <Button
          variant="danger"
          disabled={!understood || busy}
          onClick={() => {
            if (!understood) return;
            setResetting(false);
            setUnderstood(false);
            void emptyVault();
          }}
        >
          {t('vault.empty')}
        </Button>
      </div>
    </Modal>
  );

  // En segundo plano, el árbol de la app es SIEMPRE el mismo: que la bóveda abra,
  // falle o no exista no la vuelve a montar (perdería su estado y repetiría sus
  // peticiones). Lo que no esté abierto se enseña en el candado, no encima de la app.
  if (enSegundoPlano) {
    return (
      <I18nProvider locale={locale}>
        <VaultStatusContext.Provider value={status}>
          <ToastProvider isolated>{children}</ToastProvider>
        </VaultStatusContext.Provider>
        {candado}
        {detalles}
        {vaciar}
      </I18nProvider>
    );
  }

  // La franja discreta «Desbloqueando…» del esqueleto. Nada de la bóveda: solo dice
  // que se está abriendo.
  const franja = (
    <p
      class="text-text-muted border-border-subtle bg-surface flex items-center gap-2 border-b px-4 py-2 text-sm"
      role="status"
    >
      <Spinner size={14} />
      {t('matrix.unlocking')}
    </p>
  );

  let pantalla: ComponentChildren;
  if (bound && vault) {
    pantalla = (
      <>
        <VaultStatusContext.Provider value={status}>
          <ToastProvider isolated>
            {typeof children === 'function' ? children(vault) : children}
          </ToastProvider>
        </VaultStatusContext.Provider>
        {candado}
        {detalles}
      </>
    );
  } else if (skeleton !== undefined && busy) {
    // Abriendo, con esqueleto: el armazón de la app, sin nada descifrado. El barrido
    // de pantallas lo cuenta como puerta abriéndose.
    pantalla = (
      <div class="contents" data-sytue-boveda="abriendo" aria-busy="true">
        <VaultStatusContext.Provider value={status}>
          {typeof skeleton === 'function' ? (
            skeleton(franja)
          ) : (
            <>
              {franja}
              {skeleton}
            </>
          )}
        </VaultStatusContext.Provider>
      </div>
    );
  } else {
    pantalla = (
      <main
        class="mx-auto flex w-full max-w-xl flex-col p-6"
        data-sytue-boveda={busy ? 'abriendo' : 'cerrada'}
      >
        {contents}
      </main>
    );
  }

  return (
    <I18nProvider locale={locale}>
      {pantalla}
      {vaciar}
    </I18nProvider>
  );
}
