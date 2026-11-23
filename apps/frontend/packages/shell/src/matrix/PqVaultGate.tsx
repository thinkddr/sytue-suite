/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren, JSX } from 'preact';
import { useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { MatrixStorageProduct } from '@sytue/config/matrix';
import { prefijoDeApp } from '@sytue/config/tokens';
import { registerLogoutCleanup } from '@sytue/auth/client';
import { createTranslate, type Locale } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import { Button, Checkbox, Icon, IconButton, Modal, Spinner, ToastProvider } from '@sytue/ui';
import {
  bridgeHandoffKey,
  cancelPqVaultCopy,
  PqVault,
  PqVaultDisabledError,
  PqVaultError,
  pqVaultProfile,
  readPqVaultCopy,
  type PqVaultCopyOptions,
  type PqVaultExpect,
  type PqVaultOpenOptions,
} from '@sytue/offline/vault';
import type { MatrixGateMessage } from './MatrixDeviceGate.js';
import { MatrixStorageFixtureContext } from './storage-fixture-context.js';

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
  | 'vault.copyCancelHint';

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
  children,
}: PqVaultGateProps): JSX.Element {
  const fixture = useContext(MatrixStorageFixtureContext);
  const resolvedBase = useMemo(
    () => normalizeBase(basePath ?? prefijoDeApp(product === 'hub' ? 'one' : product) ?? ''),
    [basePath, product],
  );
  const translate = useMemo(() => createTranslate(locale), [locale]);
  const t = (key: PqVaultGateMessage): string => translate(key);

  const [vault, setVault] = useState<PqVault | null>(null);
  const [ready, setReady] = useState(false);
  const [disabled, setDisabled] = useState(false);
  const [busy, setBusy] = useState(!fixture);
  const [needSignIn, setNeedSignIn] = useState(false);
  const [wrongKey, setWrongKey] = useState(false);
  // Otro dispositivo está pasando su Drive de Matrix a esta bóveda (ADR 0086).
  const [copying, setCopying] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<PqVaultGateMessage | null>(null);
  const [show, setShow] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [tick, setTick] = useState(0);

  const liveVaultRef = useRef<PqVault | null>(null);
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
    setWrongKey(false);
    setCopying(false);
    setError(null);

    const shouldReset = resetRequestedRef.current;
    resetRequestedRef.current = false;
    const expect = shouldReset ? wrongExpectRef.current : undefined;
    wrongExpectRef.current = undefined;
    const openFn = shouldReset ? resetVault : openVault;
    void (async () => {
      try {
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
          await prepareRef.current(instance, controller.signal);
        }
        if (!mounted || controller.signal.aborted || instance.isClosed) {
          await teardown(instance);
          return;
        }
        setVault(instance);
        setReady(true);
        setBusy(false);
        activateRef.current?.(instance);
      } catch (failure) {
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
          if (failure.reason === 'signin') {
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
        // no entra) deja ofrecer reintentar, volver a entrar o vaciar.
        setError('vault.failed');
      }
    })();

    return () => {
      mounted = false;
      unregisterLogout();
      controller.abort();
      void teardown(opened ?? liveVaultRef.current);
    };
  }, [actor, tenant, product, resolvedBase, syntheticVault, openVault, resetVault, tick]);

  if (disabled && typeof children !== 'function') {
    return <>{children}</>;
  }

  const bound =
    vault !== null &&
    ready &&
    !vault.isClosed &&
    vault.scope.subject === actor &&
    (tenant === undefined || vault.scope.tenant === tenant) &&
    vault.scope.product === product;

  const contents = (
    <div class="flex flex-col gap-4">
      <h1 class="sytue-h3">{t('matrix.title')}</h1>
      <p class="text-text-muted text-sm">{t('matrix.subtitle')}</p>
      <p class="text-text-muted font-mono text-xs">
        ML-KEM-768 + X25519 · {pqVaultProfile}
        {bound && vault ? ` · ${vault.keyId.slice(0, 16)}` : ''}
      </p>
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
          <Button
            onClick={() => {
              window.location.assign(
                buildSignInUrl(resolvedBase, window.location.pathname + window.location.search),
              );
            }}
          >
            {t('matrix.signIn')}
          </Button>
        </div>
      ) : null}
      {!busy && (wrongKey || error === 'vault.failed') ? (
        <div class="flex flex-col gap-3">
          {wrongKey ? <p class="text-text-muted text-sm">{t('vault.wrongKey')}</p> : null}
          <div class="flex flex-wrap gap-2">
            <Button
              onClick={() => {
                window.location.assign(
                  buildSignInUrl(resolvedBase, window.location.pathname + window.location.search),
                );
              }}
            >
              {t('vault.signInAgain')}
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setUnderstood(false);
                setResetting(true);
              }}
            >
              {t('vault.empty')}
            </Button>
            <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
              {t('action.retry')}
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
    </div>
  );

  return (
    <I18nProvider locale={locale}>
      {bound && vault ? (
        <>
          <ToastProvider isolated>
            {typeof children === 'function' ? children(vault) : children}
          </ToastProvider>
          <IconButton
            label={t('matrix.title')}
            size="lg"
            class="sytue-popover-surface border-border-subtle fixed right-[max(0.75rem,var(--sytue-safe-right,0px))] bottom-[max(0.75rem,var(--sytue-safe-bottom,0px))] z-40 border shadow-[var(--shadow-sm)]"
            onClick={() => setShow(true)}
          >
            <Icon name="lock" size={16} />
          </IconButton>
          <Modal open={show} onClose={() => setShow(false)} title={t('matrix.title')}>
            {contents}
          </Modal>
        </>
      ) : (
        <main class="mx-auto flex w-full max-w-xl flex-col p-6">{contents}</main>
      )}
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
              resetRequestedRef.current = true;
              setTick((value) => value + 1);
            }}
          >
            {t('vault.empty')}
          </Button>
        </div>
      </Modal>
    </I18nProvider>
  );
}
