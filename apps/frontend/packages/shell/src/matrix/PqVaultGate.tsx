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
  PqVault,
  PqVaultDisabledError,
  PqVaultError,
  pqVaultProfile,
  type PqVaultOpenOptions,
} from '@sytue/offline/vault';
import type { MatrixGateMessage } from './MatrixDeviceGate.js';
import { MatrixStorageFixtureContext } from './storage-fixture-context.js';

function normalizeBase(value: string | undefined): string {
  if (!value || value === '/') return '';
  return value.replace(/\/+$/, '');
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
  children: ComponentChildren | ((vault: PqVault) => ComponentChildren);
}

const defaultOpenVault = (options: PqVaultOpenOptions): Promise<PqVault> => PqVault.open(options);
const defaultResetVault = (options: PqVaultOpenOptions): Promise<PqVault> => PqVault.reset(options);

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
  children,
}: PqVaultGateProps): JSX.Element {
  const fixture = useContext(MatrixStorageFixtureContext);
  const resolvedBase = useMemo(
    () => normalizeBase(basePath ?? prefijoDeApp(product === 'hub' ? 'one' : product) ?? ''),
    [basePath, product],
  );
  const translate = useMemo(() => createTranslate(locale), [locale]);
  const t = (key: MatrixGateMessage): string => translate(key);

  const [vault, setVault] = useState<PqVault | null>(null);
  const [ready, setReady] = useState(false);
  const [disabled, setDisabled] = useState(false);
  const [busy, setBusy] = useState(!fixture);
  const [needSignIn, setNeedSignIn] = useState(false);
  const [wrongKey, setWrongKey] = useState(false);
  const [error, setError] = useState<MatrixGateMessage | null>(null);
  const [show, setShow] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [tick, setTick] = useState(0);

  const liveVaultRef = useRef<PqVault | null>(null);
  const resetRequestedRef = useRef(false);
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
    setError(null);

    const shouldReset = resetRequestedRef.current;
    resetRequestedRef.current = false;
    const openFn = shouldReset ? resetVault : openVault;
    void (async () => {
      try {
        const instance = await openFn({
          actor,
          tenant,
          product,
          basePath: resolvedBase,
          signal: controller.signal,
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
            setWrongKey(true);
            return;
          }
        }
        setError('matrix.failed');
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
      {!busy && wrongKey ? (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm">{t('matrix.unknownKey')}</p>
          <div class="flex flex-wrap gap-2">
            <Button
              onClick={() => {
                setUnderstood(false);
                setResetting(true);
              }}
            >
              {t('matrix.reset')}
            </Button>
            <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
              {t('action.retry')}
            </Button>
          </div>
        </div>
      ) : null}
      {bound ? <p class="text-success-text">{t('matrix.trusted')}</p> : null}
      {!busy && !bound && !needSignIn && !wrongKey ? (
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
      <Modal open={resetting} onClose={() => setResetting(false)} title={t('matrix.resetAction')}>
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-sm">{t('matrix.resetBody')}</p>
          <Checkbox
            checked={understood}
            onChange={(event) => setUnderstood(event.currentTarget.checked)}
            label={t('matrix.resetUnderstood')}
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
            {t('matrix.resetAction')}
          </Button>
        </div>
      </Modal>
    </I18nProvider>
  );
}
