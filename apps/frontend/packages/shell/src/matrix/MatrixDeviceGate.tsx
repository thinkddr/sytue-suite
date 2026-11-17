/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren, JSX } from 'preact';
import type { StorageRootEnrollment, StorageRootService } from '@sytue/config/matrix';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, Checkbox, Icon, IconButton, Modal, Spinner, ToastProvider } from '@sytue/ui';
import { registerLogoutCleanup } from '@sytue/auth/client';
import { I18nProvider } from '@sytue/i18n/preact';
import { createTranslate, type Locale } from '@sytue/i18n';
import {
  CryptoEvent,
  VerificationPhase,
  VerificationRequestEvent,
  VerifierEvent,
  type ShowSasCallbacks,
  type VerificationRequest,
} from 'matrix-js-sdk/lib/crypto-api';
import {
  SuiteMatrix,
  MatrixCustodyError,
  MatrixSecurityError,
  type MatrixKeyCustody,
  type MatrixBootstrap,
  type MatrixDevice,
  type MatrixRecoveryUI,
  type MatrixSessionAdapter,
} from '@sytue/offline/matrix';

/** Like `closing.finally(() => client.closeAndDrain())`: the drain always runs and is
 * awaited, and its rejection wins over the close outcome, exactly as `finally` does. */
async function closeThenDrain(
  closing: Promise<void> | undefined,
  client: SuiteMatrix,
): Promise<void> {
  try {
    await closing;
  } finally {
    await client.closeAndDrain();
  }
}

export type MatrixGateMessage =
  | 'action.cancel'
  | 'action.retry'
  | 'matrix.allowStorageTitle'
  | 'matrix.allowStorageBody'
  | 'matrix.allowStorageAction'
  | 'matrix.auth'
  | 'matrix.authContinue'
  | 'matrix.compare'
  | 'matrix.connect'
  | 'matrix.devices'
  | 'matrix.failed'
  | 'matrix.incoming'
  | 'matrix.match'
  | 'matrix.mismatch'
  | 'matrix.reset'
  | 'matrix.resetAction'
  | 'matrix.resetBody'
  | 'matrix.resetUnderstood'
  | 'matrix.revoke'
  | 'matrix.signIn'
  | 'matrix.signInBody'
  | 'matrix.subtitle'
  | 'matrix.tab'
  | 'matrix.title'
  | 'matrix.trusted'
  | 'matrix.unknownKey'
  | 'matrix.unlocking'
  | 'matrix.verify'
  | 'matrix.verifyPeopleFirst'
  | 'matrix.verifyPerson'
  | 'matrix.waiting'
  | 'state.loading';

export interface MatrixGateHost {
  translate?: (key: MatrixGateMessage) => string;
  bootstrap: (signal?: AbortSignal) => Promise<MatrixBootstrap>;
  begin: (bootstrap: MatrixBootstrap) => Promise<void>;
  session: MatrixSessionAdapter;
  /**
   * Where the account's recovery key is kept, wrapped with the unlock key of the
   * person's Sytue ID sign-in (ADR 0084). Defaults to `session.custody`. Without one
   * the gate creates nothing and offers only verification from another device.
   */
  custody?: MatrixKeyCustody;
  allowPeers?: boolean;
  /** Explicit root consent for hosts that receive encrypted external inputs. */
  authority?: StorageRootService;
  /** Called only on a verified device, before its application children mount. */
  prepare?: (client: SuiteMatrix, signal: AbortSignal) => Promise<void>;
  activate?: (client: SuiteMatrix) => void;
  /** Detach private presenters synchronously; close drains their jobs before SDK teardown. */
  detach?: (client: SuiteMatrix) => void;
  close?: (client: SuiteMatrix) => Promise<void>;
  open?: (
    bootstrap: MatrixBootstrap,
    ui: MatrixRecoveryUI,
    session: MatrixSessionAdapter,
  ) => Promise<SuiteMatrix>;
}

const openMatrix: NonNullable<MatrixGateHost['open']> = (bootstrap, ui, session) =>
  SuiteMatrix.open(bootstrap, ui, session);

const isRecovery = (failure: unknown): boolean =>
  (failure instanceof MatrixSecurityError && failure.reason === 'recovery') ||
  (failure instanceof MatrixCustodyError && failure.reason === 'wrong');
const isSignIn = (failure: unknown): boolean =>
  failure instanceof MatrixCustodyError && failure.reason === 'signin';

/**
 * ADR 0085: a device verified before ADR 0084 (or one that opened an ADR 0082 wrapper
 * from its own owner-key cache) already holds the account's recovery key. It wraps it
 * for the Sytue ID sign-in itself, without asking anything; if this browser has no unlock
 * key yet, it simply tries again the next time. The device already works either way.
 */
async function upgradeQuietly(
  instance: SuiteMatrix,
  custody: MatrixKeyCustody,
  data: MatrixBootstrap,
  signal: AbortSignal,
): Promise<void> {
  try {
    const current = await instance.client.secretStorage.getDefaultKeyId();
    if (!current || signal.aborted) return;
    const state = await custody.state([current], signal);
    if (state.methods.includes('sytue-id') || signal.aborted) return;
    const held = await instance.heldRecoveryKey(current);
    if (!held || signal.aborted) return;
    await custody.wrap(
      held,
      { account: data.matrix_user_id, subject: data.actor, keyID: current },
      signal,
    );
  } catch {
    // Quietly: nothing here is needed for the device to work now.
  }
}

/**
 * ADR 0085: once the account's `sytue-id` wrapper is checked to open here, its ADR 0082
 * wrappers (password, passkey) are retired, in the same transaction that stores it again.
 * Quietly, like the upgrade above: nothing here is needed for the device to work.
 */
async function retireQuietly(
  instance: SuiteMatrix,
  custody: MatrixKeyCustody,
  data: MatrixBootstrap,
  signal: AbortSignal,
): Promise<void> {
  if (!custody.retireOlder) return;
  try {
    const current = await instance.client.secretStorage.getDefaultKeyId();
    if (!current || signal.aborted) return;
    const state = await custody.state([current], signal);
    if (!state.methods.includes('sytue-id') || state.methods.length < 2 || signal.aborted) return;
    await custody.retireOlder(
      { account: data.matrix_user_id, subject: data.actor, keyID: current },
      signal,
    );
  } catch {
    // Quietly: the old wrappers stay until the next time.
  }
}

export function MatrixDeviceGate({
  actor,
  tenant,
  locale,
  host,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  host: MatrixGateHost;
  children: ComponentChildren | ((client: SuiteMatrix) => ComponentChildren);
}): JSX.Element {
  const t = host.translate ?? createTranslate(locale);
  const [bootstrap, setBootstrap] = useState<MatrixBootstrap | null>(null);
  const [client, setClient] = useState<SuiteMatrix | null>(null);
  const [verified, setVerified] = useState(false);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState<'matrix.failed' | 'matrix.tab' | null>(null);
  const [tick, setTick] = useState(0);
  const [show, setShow] = useState(false);
  const [recipients, setRecipients] = useState<{ user: string; device: string; name: string }[]>(
    [],
  );
  const [devices, setDevices] = useState<MatrixDevice[]>([]);
  // ADR 0084: nothing is ever typed here. The unlock key comes from the person's Sytue
  // ID sign-in in this browser; without it, the only way out is signing in there again
  // (`older`: the public parameters of the account's ADR 0082 wrappers, if any).
  const [signIn, setSignIn] = useState<{ older: string } | null>(null);
  // The account already had a key this service does not hold: verification only.
  const [manual, setManual] = useState(false);
  const [storageConsent, setStorageConsent] = useState<StorageRootEnrollment | null>(null);
  const pendingRootRef = useRef<{
    resolve: () => void;
    reject: (error: Error) => void;
    request: StorageRootEnrollment;
  } | null>(null);
  const deniedRootRef = useRef<SuiteMatrix | null>(null);
  const [authentication, setAuthentication] = useState<string | null>(null);
  const [authOpened, setAuthOpened] = useState(false);
  // «He olvidado la clave»: explicit confirmation before resetting the account's encryption.
  const [resetting, setResetting] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [request, setRequest] = useState<VerificationRequest | null>(null);
  const [sas, setSas] = useState<ShowSasCallbacks | null>(null);
  const [phase, setPhase] = useState<VerificationPhase | null>(null);
  // The open device's recovery key, kept only while it is open. Never rendered.
  const keyRef = useRef<string | null>(null);
  const uiRef = useRef<MatrixRecoveryUI | null>(null);
  const pendingAuthRef = useRef<{ resolve: () => void; reject: (error: Error) => void } | null>(
    null,
  );
  const liveClientRef = useRef<SuiteMatrix | null>(null);
  const loadingRef = useRef<Promise<void> | null>(null);
  const lifetimeRef = useRef<AbortController | null>(null);
  const preparedRef = useRef<{ client: SuiteMatrix; job: Promise<void> } | null>(null);

  const fail = useCallback((failure: unknown): void => {
    // A device that needs the key later (an owner key it lost) and has no unlock key
    // here: the way out is the Sytue ID sign-in, not an error.
    if (isSignIn(failure)) {
      setSignIn({ older: '' });
      return;
    }
    setError(
      failure instanceof MatrixSecurityError && failure.reason === 'locked'
        ? 'matrix.tab'
        : 'matrix.failed',
    );
  }, []);
  const refresh = useCallback(
    async (instance: SuiteMatrix): Promise<boolean> => {
      const lifetime = lifetimeRef.current;
      const current = () =>
        lifetime !== null &&
        !lifetime.signal.aborted &&
        lifetimeRef.current === lifetime &&
        liveClientRef.current === instance &&
        !instance.isClosed;
      if (!lifetime || !current()) return false;
      const [trusted, list, peers] = await Promise.all([
        instance.trusted(),
        instance.devices(),
        host.allowPeers ? instance.unverifiedRecipients() : Promise.resolve([]),
      ]);
      if (!current()) return false;
      setVerified(trusted);
      setDevices(list);
      setRecipients(peers);
      if (!trusted) {
        setReady(false);
        return false;
      }
      if (preparedRef.current?.client !== instance) {
        const job = (async () => {
          if (host.authority) {
            if (deniedRootRef.current === instance) throw new MatrixSecurityError('unverified');
            await instance.ensureStorageAuthority(host.authority, lifetime.signal);
          }
          await host.prepare?.(instance, lifetime.signal);
        })();
        const prepared = { client: instance, job };
        preparedRef.current = prepared;
        void job.catch(() => {
          if (preparedRef.current === prepared) preparedRef.current = null;
        });
      }
      await preparedRef.current.job;
      if (!current()) return false;
      host.activate?.(instance);
      setReady(true);
      setManual(false);
      return true;
    },
    [host],
  );
  /**
   * No manual step (ADR 0084). An account without encryption gets a new key, wrapped
   * with the unlock key of the person's Sytue ID sign-in before it is published. An
   * account whose key is wrapped opens with it, and the device verifies itself through
   * cross-signing. A key still wrapped the ADR 0082 way (or held by the service, ADR
   * 0081) opens once and is wrapped the new way. Without an unlock key in this browser
   * the person is sent to sign in to Sytue ID; an account whose key nobody here holds
   * can only be verified from another device.
   */
  const automatic = useCallback(
    async (instance: SuiteMatrix, signal: AbortSignal): Promise<void> => {
      const custody = host.custody ?? host.session.custody;
      setManual(false);
      setSignIn(null);
      let older = '';
      try {
        if (!custody) throw new MatrixSecurityError('recovery');
        if (!(await instance.client.secretStorage.hasKey())) {
          await instance.setupRecovery();
          return;
        }
        const current = await instance.client.secretStorage.getDefaultKeyId();
        const state = await custody.state(current ? [current] : undefined, signal);
        older = state.older;
        if (state.methods.length > 0) {
          await instance.recover();
          // Opened with what the Sytue ID sign-in page computed for an ADR 0082
          // wrapper: from now on it opens with the unlock key. Opened from this
          // device's own cache instead: the same, quietly.
          if (!state.methods.includes('sytue-id') && current) {
            if (keyRef.current) await uiRef.current?.saveRecoveryKey(keyRef.current, current);
            else await upgradeQuietly(instance, custody, instance.bootstrap, signal);
          }
          await retireQuietly(instance, custody, instance.bootstrap, signal);
          return;
        }
        const legacy = state.legacy ? await custody.legacyRecoveryKey(signal) : null;
        if (!legacy || !current) throw new MatrixSecurityError('recovery');
        keyRef.current = legacy;
        await instance.recover();
        // Wrapping it also deletes the copy the service held.
        await uiRef.current?.saveRecoveryKey(legacy, current);
      } catch (failure) {
        if (isSignIn(failure)) {
          setSignIn({ older });
          return;
        }
        if (isRecovery(failure)) {
          keyRef.current = null;
          setManual(true);
          return;
        }
        throw failure;
      }
    },
    [host],
  );
  useEffect(() => {
    let disposed = false;
    const controller = new AbortController();
    lifetimeRef.current = controller;
    let unregisterLogout: (() => void) | undefined;
    const job = async (): Promise<void> => {
      if (disposed) return;
      setBusy(true);
      setReady(false);
      setVerified(false);
      setClient(null);
      setSignIn(null);
      setManual(false);
      keyRef.current = null;
      setAuthentication(null);
      setRequest(null);
      setSas(null);
      preparedRef.current = null;
      setError(null);
      try {
        const data = await host.bootstrap(controller.signal);
        if (data.actor !== actor || (tenant !== undefined && data.tenant !== tenant))
          throw new MatrixSecurityError('identity');
        if (disposed) return;
        setBootstrap(data);
        const lifecycle: { closed?: () => void } = {};
        const custody = host.custody ?? host.session.custody;
        const ui: MatrixRecoveryUI = {
          cancelPending: () => {
            pendingAuthRef.current?.reject(new MatrixSecurityError('device'));
            pendingRootRef.current?.reject(new MatrixSecurityError('device'));
            pendingRootRef.current = null;
            setStorageConsent(null);
            pendingAuthRef.current = null;
            keyRef.current = null;
            setAuthentication(null);
            lifecycle.closed?.();
          },
          confirmStorageRoot: (value) =>
            new Promise<void>((resolve, reject) => {
              if (disposed || controller.signal.aborted || pendingRootRef.current) {
                reject(new MatrixSecurityError('device'));
                return;
              }
              pendingRootRef.current = { resolve, reject, request: value };
              setStorageConsent(value);
            }),
          // A new key: wrapped with the unlock key and stored BEFORE the SDK publishes
          // it. A failure publishes nothing; the retry uses the same flow.
          saveRecoveryKey: async (value, keyID) => {
            if (!custody) throw new MatrixSecurityError('recovery');
            await custody.wrap(
              value,
              { account: data.matrix_user_id, subject: data.actor, keyID },
              controller.signal,
            );
            keyRef.current = value;
          },
          // Opened with the unlock key of the Sytue ID sign-in; never asked for.
          readRecoveryKey: async (keyIDs) => {
            const known = keyRef.current;
            if (known) return known;
            if (!custody) throw new MatrixSecurityError('recovery');
            const opened = await custody.unwrap(
              { account: data.matrix_user_id, subject: data.actor, keyIDs },
              controller.signal,
            );
            keyRef.current = opened;
            return opened;
          },
          authenticate: (url) =>
            new Promise<void>((resolve, reject) => {
              if (
                disposed ||
                controller.signal.aborted ||
                new URL(url).origin !== data.homeserver_url
              ) {
                reject(new MatrixSecurityError('identity'));
                return;
              }
              pendingAuthRef.current = { resolve, reject };
              setAuthOpened(false);
              setAuthentication(url);
            }),
        };
        uiRef.current = ui;
        const instance = await (host.open ?? openMatrix)(data, ui, host.session);
        if (disposed) {
          instance.close();
          return;
        }
        liveClientRef.current = instance;
        lifecycle.closed = () => {
          if (disposed || liveClientRef.current !== instance) return;
          controller.abort();
          liveClientRef.current = null;
          preparedRef.current = null;
          setReady(false);
          setClient(null);
          setBootstrap(null);
          setRequest(null);
          setSas(null);
          setShow(false);
          try {
            host.detach?.(instance);
          } finally {
            const closing = closeThenDrain(
              Promise.resolve().then(() => host.close?.(instance)),
              instance,
            );
            loadingRef.current = Promise.allSettled([loadingRef.current, closing]).then(() => {});
          }
        };
        unregisterLogout = registerLogoutCleanup(async () => {
          controller.abort();
          setReady(false);
          setBootstrap(null);
          setClient(null);
          liveClientRef.current = null;
          preparedRef.current = null;
          pendingAuthRef.current?.reject(new MatrixSecurityError('device'));
          pendingRootRef.current?.reject(new MatrixSecurityError('device'));
          pendingRootRef.current = null;
          setStorageConsent(null);
          pendingAuthRef.current = null;
          setSignIn(null);
          keyRef.current = null;
          setAuthentication(null);
          host.detach?.(instance);
          try {
            await host.close?.(instance);
          } finally {
            await instance.forget();
          }
        });
        setClient(instance);
        instance.client.on(CryptoEvent.VerificationRequestReceived, (incoming) => {
          if (
            disposed ||
            liveClientRef.current !== instance ||
            (!host.allowPeers && incoming.otherUserId !== instance.bootstrap.matrix_user_id) ||
            !instance.canVerifyUser(incoming.otherUserId)
          ) {
            void incoming.cancel().catch(() => undefined);
            return;
          }
          setRequest(incoming);
          setShow(true);
        });
        if (!(await refresh(instance)) && !disposed && liveClientRef.current === instance) {
          await automatic(instance, controller.signal);
          if (!disposed && liveClientRef.current === instance) await refresh(instance);
        } else if (custody && !disposed && liveClientRef.current === instance) {
          void upgradeQuietly(instance, custody, data, controller.signal).then(() =>
            retireQuietly(instance, custody, data, controller.signal),
          );
        }
      } catch (failure) {
        if (!disposed && !(failure instanceof MatrixSecurityError && failure.reason === 'login'))
          fail(failure);
      } finally {
        if (!disposed) setBusy(false);
      }
    };
    // A single persistent crypto store cannot be opened by two concurrent mounts.
    const previous = loadingRef.current ?? Promise.resolve();
    const jobPromise = previous.catch(() => undefined).then(job);
    loadingRef.current = jobPromise;
    return () => {
      unregisterLogout?.();
      disposed = true;
      controller.abort();
      setReady(false);
      setBootstrap(null);
      pendingAuthRef.current?.reject(new MatrixSecurityError('device'));
      pendingRootRef.current?.reject(new MatrixSecurityError('device'));
      pendingRootRef.current = null;
      setStorageConsent(null);
      pendingAuthRef.current = null;
      setSignIn(null);
      keyRef.current = null;
      setAuthentication(null);
      setRequest(null);
      setSas(null);
      setShow(false);
      const instance = liveClientRef.current;
      liveClientRef.current = null;
      preparedRef.current = null;
      if (instance) host.detach?.(instance);
      const closing = instance
        ? closeThenDrain(host.close?.(instance), instance)
        : Promise.resolve();
      // Do not open the next account's SDK store before the previous app has drained.
      loadingRef.current = Promise.allSettled([loadingRef.current, jobPromise, closing]).then(
        () => {},
      );
    };
  }, [actor, tenant, tick, fail, refresh, automatic, host]);
  useEffect(() => {
    // A confirmation given for one device never carries over to the next one.
    setResetting(false);
    setUnderstood(false);
  }, [client]);
  useEffect(() => {
    if (!client) return;
    const verify = (): void => {
      setShow(true);
      void refresh(client).catch((error) => {
        if (!client.isClosed) fail(error);
      });
    };
    window.addEventListener('sytue-matrix-verification-required', verify);
    return () => window.removeEventListener('sytue-matrix-verification-required', verify);
  }, [client, refresh, fail]);
  useEffect(() => {
    if (!request) return;
    let detached = false;
    let started = false;
    let activeVerifier: VerificationRequest['verifier'];
    const showSas = (value: ShowSasCallbacks): void => {
      if (!detached) setSas(value);
    };
    const run = async (): Promise<void> => {
      setPhase(request.phase);
      if (request.phase === VerificationPhase.Done) {
        if (client) await refresh(client);
        setRequest(null);
        setSas(null);
        return;
      }
      if (request.phase === VerificationPhase.Cancelled) {
        setRequest(null);
        setSas(null);
        return;
      }
      if (
        started ||
        (request.phase !== VerificationPhase.Ready && request.phase !== VerificationPhase.Started)
      )
        return;
      started = true;
      const verifier = request.verifier ?? (await request.startVerification('m.sas.v1'));
      activeVerifier = verifier;
      verifier.on(VerifierEvent.ShowSas, showSas);
      const current = verifier.getShowSasCallbacks();
      if (current) showSas(current);
      await verifier.verify();
      if (!detached && client) {
        await refresh(client);
        setRequest(null);
        setSas(null);
      }
    };
    const change = (): void => {
      void run().catch((error) => {
        if (!detached) fail(error);
      });
    };
    request.on(VerificationRequestEvent.Change, change);
    change();
    return () => {
      detached = true;
      request.off(VerificationRequestEvent.Change, change);
      activeVerifier?.off(VerifierEvent.ShowSas, showSas);
    };
  }, [request, client, fail, refresh]);
  async function operation(action: () => Promise<void>): Promise<void> {
    const lifetime = lifetimeRef.current;
    const instance = liveClientRef.current;
    if (busy || !lifetime || lifetime.signal.aborted) return;
    const current = () =>
      lifetimeRef.current === lifetime &&
      !lifetime.signal.aborted &&
      liveClientRef.current === instance;
    setBusy(true);
    setError(null);
    try {
      await action();
      if (current() && instance) await refresh(instance);
    } catch (failure) {
      if (current()) fail(failure);
    } finally {
      if (current()) setBusy(false);
    }
  }
  /** Retry: the automatic flow again on an unverified device. */
  function resume(instance: SuiteMatrix): Promise<void> {
    return operation(async () => {
      const lifetime = lifetimeRef.current;
      if (!lifetime) return;
      // A verified device just prepares again: if it needs the key, it opens it with
      // the unlock key itself (readRecoveryKey).
      if (!verified) await automatic(instance, lifetime.signal);
    });
  }
  const bound = bootstrap?.actor === actor && (tenant === undefined || bootstrap.tenant === tenant);
  const contents = bound ? (
    <div class="flex flex-col gap-4">
      <h1 class="sytue-h3">{t('matrix.title')}</h1>
      <p class="text-text-muted text-sm">{t('matrix.subtitle')}</p>
      <a
        class="text-text-muted text-xs underline underline-offset-2"
        href={
          bootstrap?.source_url &&
          /^https:\/\/github\.com\/element-hq\/synapse\/tree\/v\d+\.\d+\.\d+$/.test(
            bootstrap.source_url,
          )
            ? bootstrap.source_url
            : 'https://github.com/element-hq/synapse/tree/v1.163.0'
        }
        target="_blank"
        rel="noopener noreferrer"
      >
        Synapse · AGPL-3.0
      </a>
      {error ? (
        <p class="text-danger-text text-sm" role="alert">
          {t(error)}
        </p>
      ) : null}
      {busy ? (
        <p class="flex items-center gap-2" role="status">
          <Spinner size={18} />
          {t(client && !verified ? 'matrix.unlocking' : 'state.loading')}
        </p>
      ) : null}
      {client && !busy && signIn ? (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm">{t('matrix.signInBody')}</p>
          <Button
            onClick={() => {
              const custody = host.custody ?? host.session.custody;
              if (!custody) return;
              window.location.assign(
                custody.signInURL(window.location.pathname + window.location.search, signIn.older),
              );
            }}
          >
            {t('matrix.signIn')}
          </Button>
        </div>
      ) : null}
      {!client && bootstrap ? (
        <Button disabled={busy} onClick={() => void operation(() => host.begin(bootstrap))}>
          {t('matrix.connect')}
        </Button>
      ) : null}
      {client ? (
        <>
          {verified ? <p class="text-success-text">{t('matrix.trusted')}</p> : null}
          {!verified && manual ? (
            <p class="text-text-muted text-sm">{t('matrix.unknownKey')}</p>
          ) : null}
          {!verified && manual ? (
            <div class="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() =>
                  void operation(async () => setRequest(await client.requestVerification()))
                }
              >
                {t('matrix.verify')}
              </Button>
              {/* Private storage keeps its vault inside secret storage: the core refuses
                  to reset it (`unsupported`), so the way out is not offered there. */}
              {!client.bootstrap.storage_product ? (
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={() => {
                    setUnderstood(false);
                    setResetting(true);
                  }}
                >
                  {t('matrix.reset')}
                </Button>
              ) : null}
            </div>
          ) : null}
          {request ? (
            <div class="flex flex-col gap-3">
              <p>{t(sas ? 'matrix.compare' : 'matrix.waiting')}</p>
              {sas ? (
                <>
                  <div class="flex flex-wrap gap-3 text-3xl" aria-label={t('matrix.compare')}>
                    {sas.sas.emoji?.map(([emoji, name]) => (
                      <span key={name} title={name} role="img" aria-label={name}>
                        {emoji}
                      </span>
                    )) ?? <span>{sas.sas.decimal?.join(' · ')}</span>}
                  </div>
                  <div class="flex flex-wrap gap-2">
                    <Button onClick={() => void sas.confirm().catch(fail)}>
                      {t('matrix.match')}
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => {
                        sas.mismatch();
                        setSas(null);
                      }}
                    >
                      {t('matrix.mismatch')}
                    </Button>
                  </div>
                </>
              ) : phase === VerificationPhase.Requested && !request.initiatedByMe ? (
                <Button onClick={() => void request.accept().catch(fail)}>
                  {t('matrix.incoming')}
                </Button>
              ) : null}
              <Button variant="ghost" onClick={() => void request.cancel().catch(fail)}>
                {t('action.cancel')}
              </Button>
            </div>
          ) : null}
          {recipients.length > 0 ? (
            <p class="text-text-muted text-sm">{t('matrix.verifyPeopleFirst')}</p>
          ) : null}
          {recipients.map((peer) => (
            <div
              key={peer.user + peer.device}
              class="flex flex-wrap items-center justify-between gap-2"
            >
              <span class="text-sm break-all">
                {peer.user} · {peer.name}
              </span>
              <Button
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() =>
                  void operation(async () =>
                    setRequest(await client.verifyRecipient(peer.user, peer.device)),
                  )
                }
              >
                {t('matrix.verifyPerson')}
              </Button>
            </div>
          ))}
          {verified ? (
            <div class="flex flex-col gap-2">
              <h2 class="sytue-label">{t('matrix.devices')}</h2>
              {devices.map((device) => (
                <div key={device.id} class="flex flex-wrap items-center justify-between gap-2">
                  <span class="text-sm break-all">
                    {device.name} · {device.verified ? t('matrix.trusted') : t('matrix.verify')}
                  </span>
                  {!device.current ? (
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void operation(() => client.revokeDevice(device.id))}
                    >
                      {t('matrix.revoke')}
                    </Button>
                  ) : null}
                </div>
              ))}
            </div>
          ) : null}
        </>
      ) : null}
      {client && !ready && !busy && (error || (!verified && !manual && !signIn)) ? (
        <Button variant="secondary" disabled={busy} onClick={() => void resume(client)}>
          {t('action.retry')}
        </Button>
      ) : null}
      {!busy && !client ? (
        <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      ) : null}
    </div>
  ) : (
    <div class="flex flex-col gap-4">
      {error ? (
        <p class="text-danger-text text-sm" role="alert">
          {t(error)}
        </p>
      ) : (
        <p class="flex items-center gap-2" role="status">
          <Spinner size={18} />
          {t('state.loading')}
        </p>
      )}
      {error && !busy ? (
        <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      ) : null}
    </div>
  );
  return (
    <I18nProvider locale={locale}>
      {bound &&
      ready &&
      client?.bootstrap.actor === actor &&
      !client.isClosed &&
      (tenant === undefined || client.bootstrap.tenant === tenant) ? (
        <>
          <ToastProvider isolated>
            {typeof children === 'function' ? children(client) : children}
          </ToastProvider>
          {/* Flota sobre la app: solo icono —el nombre va en la ayuda y en aria-label—
              para no tapar el contenido de debajo (en un teléfono, el rótulo cubría el
              último elemento de una lista a todo el ancho), y fuera de la zona segura. */}
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
      <Modal
        open={bound && resetting && client !== null}
        onClose={() => setResetting(false)}
        title={t('matrix.resetAction')}
      >
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-sm">{t('matrix.resetBody')}</p>
          <Checkbox
            checked={understood}
            onChange={(event) => setUnderstood(event.currentTarget.checked)}
            label={t('matrix.resetUnderstood')}
          />
          <Button
            variant="danger"
            disabled={!understood || busy || !client}
            onClick={() => {
              if (!understood || !client) return;
              // Closed first: the new key is wrapped with the unlock key on its own, and
              // the reset only opens the homeserver's re-authentication. The new key
              // replaces the wrapped one.
              setResetting(false);
              setUnderstood(false);
              void operation(() => client.resetRecovery());
            }}
          >
            {t('matrix.resetAction')}
          </Button>
        </div>
      </Modal>
      <Modal
        open={bound && authentication !== null}
        onClose={() => {
          pendingAuthRef.current?.reject(new MatrixSecurityError('identity'));
          pendingAuthRef.current = null;
          setAuthentication(null);
        }}
        title={t('matrix.auth')}
      >
        <div class="flex flex-col gap-4">
          <Button
            onClick={() => {
              if (authentication) {
                window.open(authentication, '_blank', 'noopener,noreferrer');
                setAuthOpened(true);
              }
            }}
          >
            {t('matrix.auth')}
          </Button>
          <Button
            variant="secondary"
            disabled={!authOpened}
            onClick={() => {
              pendingAuthRef.current?.resolve();
              pendingAuthRef.current = null;
              setAuthentication(null);
            }}
          >
            {t('matrix.authContinue')}
          </Button>
        </div>
      </Modal>
      <Modal
        open={storageConsent !== null}
        title={t('matrix.allowStorageTitle')}
        onClose={() => {
          if (!storageConsent || pendingRootRef.current?.request !== storageConsent) return;
          deniedRootRef.current = liveClientRef.current;
          pendingRootRef.current?.reject(new MatrixSecurityError('unverified'));
          pendingRootRef.current = null;
          setStorageConsent(null);
        }}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                if (!storageConsent || pendingRootRef.current?.request !== storageConsent) return;
                deniedRootRef.current = liveClientRef.current;
                pendingRootRef.current?.reject(new MatrixSecurityError('unverified'));
                pendingRootRef.current = null;
                setStorageConsent(null);
              }}
            >
              {t('action.cancel')}
            </Button>
            <Button
              onClick={() => {
                if (!storageConsent || pendingRootRef.current?.request !== storageConsent) return;
                pendingRootRef.current.resolve();
                pendingRootRef.current = null;
                setStorageConsent(null);
              }}
            >
              {t('matrix.allowStorageAction')}
            </Button>
          </>
        }
      >
        <p>{t('matrix.allowStorageBody')}</p>
      </Modal>
    </I18nProvider>
  );
}
