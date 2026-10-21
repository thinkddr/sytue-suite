/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren, JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, Checkbox, Input, Modal, Spinner } from '@sytue/ui';
import { registerLogoutCleanup } from '@sytue/auth/client';
import { clearDraftMemory } from '../lib/drafts.js';
import { clearMatrixMessages } from '../client/matrix/messages.js';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import {
  CryptoEvent,
  VerificationPhase,
  VerificationRequestEvent,
  VerifierEvent,
  type ShowSasCallbacks,
  type VerificationRequest,
} from 'matrix-js-sdk/lib/crypto-api';
import { matrixBootstrap, beginMatrixSso } from '../client/matrix/auth.js';
import { RoomsMatrix, setRoomsMatrix, type MatrixDevice } from '../client/matrix/client.js';
import { MatrixSecurityError, type MatrixBootstrap } from '../client/matrix/types.js';
import { roomsT } from '../i18n/index.js';

export function MatrixRoomsGate({
  actor,
  locale,
  children,
}: {
  actor: string;
  locale: Locale;
  children: ComponentChildren;
}): JSX.Element {
  const t = roomsT(locale);
  const [bootstrap, setBootstrap] = useState<MatrixBootstrap | null>(null);
  const [client, setClient] = useState<RoomsMatrix | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState<'matrix.failed' | 'matrix.tab' | null>(null);
  const [tick, setTick] = useState(0);
  const [show, setShow] = useState(false);
  const [recipients, setRecipients] = useState<{ user: string; device: string; name: string }[]>(
    [],
  );
  const [devices, setDevices] = useState<MatrixDevice[]>([]);
  const [secret, setSecret] = useState<{ kind: 'save' | 'read'; value: string } | null>(null);
  const [saved, setSaved] = useState(false);
  const [authentication, setAuthentication] = useState<string | null>(null);
  const [authOpened, setAuthOpened] = useState(false);
  const [request, setRequest] = useState<VerificationRequest | null>(null);
  const [sas, setSas] = useState<ShowSasCallbacks | null>(null);
  const [phase, setPhase] = useState<VerificationPhase | null>(null);
  const pendingSecretRef = useRef<{
    resolve: (value: string) => void;
    reject: (error: Error) => void;
  } | null>(null);
  const pendingAuthRef = useRef<{ resolve: () => void; reject: (error: Error) => void } | null>(
    null,
  );
  const liveClientRef = useRef<RoomsMatrix | null>(null);
  const loadingRef = useRef<Promise<void> | null>(null);

  const fail = useCallback((failure: unknown): void => {
    setError(
      failure instanceof MatrixSecurityError && failure.reason === 'locked'
        ? 'matrix.tab'
        : 'matrix.failed',
    );
  }, []);
  const refresh = useCallback(async (instance: RoomsMatrix): Promise<void> => {
    const [verified, list, peers] = await Promise.all([
      instance.trusted(),
      instance.devices(),
      instance.unverifiedRecipients(),
    ]);
    setReady(verified);
    setDevices(list);
    setRecipients(peers);
    if (verified) {
      setRoomsMatrix(instance);
      window.dispatchEvent(new Event('sytue-matrix-verified'));
    }
  }, []);
  useEffect(() => {
    let disposed = false;
    const controller = new AbortController();
    let unregisterLogout: (() => void) | undefined;
    const job = async (): Promise<void> => {
      setBusy(true);
      setReady(false);
      setClient(null);
      setError(null);
      try {
        const data = await matrixBootstrap(controller.signal);
        if (data.actor !== actor) throw new MatrixSecurityError('identity');
        if (disposed) return;
        setBootstrap(data);
        const instance = await RoomsMatrix.open(data, {
          saveRecoveryKey: (value) =>
            new Promise<void>((resolve, reject) => {
              if (disposed) {
                reject(new MatrixSecurityError('device'));
                return;
              }
              pendingSecretRef.current = { resolve: () => resolve(), reject };
              setSaved(false);
              setSecret({ kind: 'save', value });
            }),
          readRecoveryKey: () =>
            new Promise<string>((resolve, reject) => {
              if (disposed) {
                reject(new MatrixSecurityError('device'));
                return;
              }
              pendingSecretRef.current = { resolve, reject };
              setSecret({ kind: 'read', value: '' });
            }),
          authenticate: (url) =>
            new Promise<void>((resolve, reject) => {
              if (disposed || new URL(url).origin !== data.homeserver_url) {
                reject(new MatrixSecurityError('identity'));
                return;
              }
              pendingAuthRef.current = { resolve, reject };
              setAuthOpened(false);
              setAuthentication(url);
            }),
        });
        if (disposed) {
          instance.close();
          return;
        }
        liveClientRef.current = instance;
        unregisterLogout = registerLogoutCleanup(async () => {
          await instance.forget();
          clearMatrixMessages();
          clearDraftMemory();
        });
        setClient(instance);
        instance.client.on(CryptoEvent.VerificationRequestReceived, (incoming) => {
          if (!instance.canVerifyUser(incoming.otherUserId)) {
            void incoming.cancel().catch(() => undefined);
            return;
          }
          setRequest(incoming);
          setShow(true);
        });
        await refresh(instance);
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
      clearMatrixMessages();
      clearDraftMemory();
      disposed = true;
      controller.abort();
      setRoomsMatrix(null);
      pendingSecretRef.current?.reject(new MatrixSecurityError('device'));
      pendingAuthRef.current?.reject(new MatrixSecurityError('device'));
      liveClientRef.current?.close();
      liveClientRef.current = null;
    };
  }, [actor, tick, fail, refresh]);
  useEffect(() => {
    if (!client) return;
    const verify = (): void => {
      setShow(true);
      void refresh(client).catch(fail);
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
      void run().catch(fail);
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
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await action();
      if (client) await refresh(client);
    } catch (failure) {
      fail(failure);
    } finally {
      setBusy(false);
    }
  }
  const contents = (
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
          {t('state.loading')}
        </p>
      ) : null}
      {!client && bootstrap ? (
        <Button disabled={busy} onClick={() => void operation(() => beginMatrixSso(bootstrap))}>
          {t('matrix.connect')}
        </Button>
      ) : null}
      {client ? (
        <>
          {ready ? <p class="text-success-text">{t('matrix.trusted')}</p> : null}
          {!ready ? (
            <div class="flex flex-wrap gap-2">
              <Button disabled={busy} onClick={() => void operation(() => client.setupRecovery())}>
                {t('matrix.setup')}
              </Button>
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => void operation(() => client.recover())}
              >
                {t('matrix.recover')}
              </Button>
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() =>
                  void operation(async () => setRequest(await client.requestVerification()))
                }
              >
                {t('matrix.verify')}
              </Button>
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
          {ready ? (
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
      {!busy && !client ? (
        <Button variant="secondary" onClick={() => setTick((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      ) : null}
    </div>
  );
  return (
    <I18nProvider locale={locale}>
      {ready && client?.bootstrap.actor === actor ? (
        <>
          {children}
          <Button
            variant="ghost"
            size="sm"
            class="fixed right-3 bottom-3 z-40"
            onClick={() => setShow(true)}
          >
            {t('matrix.title')}
          </Button>
          <Modal open={show} onClose={() => setShow(false)} title={t('matrix.title')}>
            {contents}
          </Modal>
        </>
      ) : (
        <main class="mx-auto flex w-full max-w-xl flex-col p-6">{contents}</main>
      )}
      <Modal
        open={secret !== null}
        onClose={() => {
          pendingSecretRef.current?.reject(new MatrixSecurityError('recovery'));
          pendingSecretRef.current = null;
          setSecret(null);
        }}
        title={t(secret?.kind === 'save' ? 'matrix.saveTitle' : 'matrix.recover')}
      >
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-sm">
            {t(secret?.kind === 'save' ? 'matrix.saveBody' : 'matrix.recoverBody')}
          </p>
          {secret?.kind === 'save' ? (
            <>
              <code class="border-border rounded-lg border p-4 text-sm break-all select-all">
                {secret.value}
              </code>
              <Checkbox
                checked={saved}
                onChange={(event) => setSaved(event.currentTarget.checked)}
                label={t('matrix.saved')}
              />
            </>
          ) : (
            <Input
              type="password"
              autoComplete="off"
              value={secret?.value ?? ''}
              aria-label={t('matrix.recoveryKey')}
              onInput={(event) => setSecret({ kind: 'read', value: event.currentTarget.value })}
            />
          )}
          <Button
            disabled={secret?.kind === 'save' ? !saved : !secret?.value.trim()}
            onClick={() => {
              if (!secret) return;
              pendingSecretRef.current?.resolve(secret.value);
              pendingSecretRef.current = null;
              setSecret(null);
            }}
          >
            {t('action.done')}
          </Button>
        </div>
      </Modal>
      <Modal
        open={authentication !== null}
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
    </I18nProvider>
  );
}
