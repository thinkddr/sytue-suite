import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useT as useCommonT } from '@sytue/i18n/preact';
import {
  Button,
  Badge,
  ChoiceCard,
  DataFlow,
  FieldGroup,
  Icon,
  InlineHelp,
  ReviewSummary,
  Select,
  Spinner,
  Wizard,
  withBase,
  type WizardStep,
} from '@sytue/ui';
import type { Channel } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

interface Provider {
  key: string;
  name: string;
  scopes: string[];
  connected: boolean;
  available: boolean;
  instance_id?: string;
}

interface Connections {
  available: boolean;
  providers: Provider[];
  items: { id: string; template_key: string; name: string; connected: boolean }[];
}

interface Destination {
  id: string;
  name: string;
}

interface Props {
  initialProvider?: string | undefined;
  channels: Channel[];
  activeChannelId?: string | undefined;
  footerTarget: Element | null;
  onClose: () => void;
  onConnected: (channelId: string) => void;
  t: T;
}

const EMPTY: Connections = { available: false, providers: [], items: [] };
const API = withBase(import.meta.env.BASE_URL, '/api/app-connections');

/**
 * Consentimiento de cuenta y puente de canal por el Integration Gateway. Solo
 * Slack, Teams y Google Chat: WhatsApp tiene su propio asistente y Google Meet es
 * personal (Canales externos), no una conexión de la organización.
 */
export function AppConnectionsWizard({
  initialProvider,
  channels,
  activeChannelId,
  footerTarget,
  onClose,
  onConnected,
  t,
}: Props): JSX.Element {
  const commonT = useCommonT();
  const [data, setData] = useState<Connections>(EMPTY);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const [providerKey, setProviderKey] = useState(initialProvider ?? '');
  const [instanceId, setInstanceId] = useState('');
  const [step, setStep] = useState(initialProvider ? 1 : 0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<
    'load' | 'unavailable' | 'provider' | 'denied' | 'action' | null
  >(null);
  const [channelId, setChannelId] = useState(
    channels.some((channel) => channel.id === activeChannelId) ? activeChannelId! : '',
  );
  const [destinations, setDestinations] = useState<Destination[]>([]);
  const [remoteId, setRemoteId] = useState('');
  const [loadingDestinations, setLoadingDestinations] = useState(false);
  const [destinationError, setDestinationError] = useState(false);
  const [retryDestinations, setRetryDestinations] = useState(0);
  const [confirmDisconnect, setConfirmDisconnect] = useState<Provider | null>(null);
  const [disconnected, setDisconnected] = useState(false);
  const pendingRef = useRef<AbortController | null>(null);
  const wizardRef = useRef<HTMLDivElement>(null);
  const returnedHandledRef = useRef(false);
  const [returned] = useState(() => {
    const query =
      typeof window === 'undefined' ? null : new URLSearchParams(window.location.search);
    return { instance: query?.get('instance') ?? '', error: query?.get('oauth_error') ?? '' };
  });
  const provider: Provider | undefined = data.providers.find((item) => item.key === providerKey);
  const providerAvailable = provider?.available === true;
  const accounts = data.items.filter((item) => item.template_key === providerKey && item.connected);
  const selectedAccount = accounts.find((item) => item.id === instanceId);
  const awaitingReturnedAccount = Boolean(
    returned.instance && returned.instance === instanceId && !returned.error && !selectedAccount,
  );
  const channelName = channels.find((channel) => channel.id === channelId)?.name ?? '';
  const destinationName =
    destinations.find((destination) => destination.id === remoteId)?.name ?? '';
  const choosingDestinations = step >= 2;
  const hasChannel = channels.some((channel) => channel.id === channelId);
  const hasDestination = destinations.some((destination) => destination.id === remoteId);

  useEffect(() => {
    const url = new URL(window.location.href);
    if (url.searchParams.get('connect-app') !== '1') return;
    for (const key of ['connect-app', 'instance', 'oauth_error']) url.searchParams.delete(key);
    window.history.replaceState(
      window.history.state,
      '',
      `${url.pathname}${url.search}${url.hash}`,
    );
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void fetch(API, { credentials: 'include', signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error(response.status === 403 ? 'unavailable' : 'load');
        const connections = (await response.json()) as Connections;
        if (!connections.available) throw new Error('unavailable');
        setData(connections);
        if (initialProvider && !returned.instance) {
          const initial = connections.providers.find((item) => item.key === initialProvider);
          if (initial?.connected && initial.instance_id) {
            setInstanceId(initial.instance_id);
            setStep(2);
          } else if (!initial) {
            setProviderKey('');
            setStep(0);
          }
        }
        if (returned.instance && !returnedHandledRef.current) {
          const instance = connections.items.find((item) => item.id === returned.instance);
          const selected = connections.providers.find(
            (item) => item.instance_id === returned.instance || item.key === instance?.template_key,
          );
          if (selected) {
            setProviderKey(selected.key);
            setInstanceId(returned.instance);
            setStep(returned.error ? 1 : instance?.connected ? 2 : 1);
            if (returned.error || instance?.connected) returnedHandledRef.current = true;
          }
        }
        if (returned.error) setError(returned.error === 'access_denied' ? 'denied' : 'action');
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          failure instanceof Error && failure.message === 'unavailable' ? 'unavailable' : 'load',
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [initialProvider, reload, returned]);

  useEffect(() => {
    if (!instanceId || !choosingDestinations || !providerAvailable) {
      setDestinations([]);
      setRemoteId('');
      setLoadingDestinations(false);
      return;
    }
    const controller = new AbortController();
    setLoadingDestinations(true);
    setDestinationError(false);
    setDestinations([]);
    setRemoteId('');
    void fetch(`${API}?instance=${encodeURIComponent(instanceId)}`, {
      credentials: 'include',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) throw new Error('destinations');
        const { items } = (await response.json()) as { items: Destination[] };
        setDestinations(items);
        if (items.length === 1) setRemoteId(items[0]!.id);
      })
      .catch(() => {
        if (!controller.signal.aborted) setDestinationError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingDestinations(false);
      });
    return () => controller.abort();
  }, [instanceId, choosingDestinations, retryDestinations, providerAvailable]);

  useEffect(() => () => pendingRef.current?.abort(), []);

  // The person explicitly opened this wizard; announce its first loaded step.
  useEffect(() => {
    if (!loading) wizardRef.current?.querySelector<HTMLElement>('h2')?.focus();
  }, [loading]);

  async function authorize(newAccount = !instanceId): Promise<void> {
    if (!provider || !providerAvailable || busy) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(API, {
        method: 'POST',
        credentials: 'include',
        signal: controller.signal,
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'consent',
          provider: provider.key,
          ...(newAccount ? { new_account: true } : instanceId ? { instance_id: instanceId } : {}),
        }),
      });
      if (!response.ok) {
        const result = (await response.json()) as { error?: string };
        if (result.error === 'oauth_provider_unavailable') {
          setError('provider');
          return;
        }
        throw new Error('consent');
      }
      const result = (await response.json()) as { authorize_url: string; instance_id: string };
      const url = new URL(result.authorize_url);
      if (url.protocol !== 'https:' || url.username || url.password) throw new Error('consent');
      if (!controller.signal.aborted) window.location.assign(url.href);
    } catch {
      if (!controller.signal.aborted) setError('action');
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  async function connect(): Promise<void> {
    if (busy || !providerAvailable || !instanceId || !hasChannel || !hasDestination) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(API, {
        method: 'POST',
        credentials: 'include',
        signal: controller.signal,
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'connect-bridge',
          instance_id: instanceId,
          channel_id: channelId,
          remote_id: remoteId,
        }),
      });
      if (!response.ok) throw new Error('connection');
      if (controller.signal.aborted) return;
      setStep(4);
      onConnected(channelId);
    } catch {
      if (!controller.signal.aborted) setError('action');
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  async function disconnectAccount(): Promise<void> {
    const account = confirmDisconnect;
    if (busy || !account?.instance_id) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(API, {
        method: 'POST',
        credentials: 'include',
        signal: controller.signal,
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: 'disconnect', instance_id: account.instance_id }),
      });
      if (!response.ok) throw new Error('disconnect');
      if (controller.signal.aborted) return;
      setData((previous) => ({
        ...previous,
        providers: previous.providers.map((item) =>
          item.key === account.key ? { ...item, connected: false } : item,
        ),
        items: previous.items.map((item) =>
          item.id === account.instance_id ? { ...item, connected: false } : item,
        ),
      }));
      setInstanceId('');
      setConfirmDisconnect(null);
      setDisconnected(true);
      returnedHandledRef.current = true;
      setStep(0);
      setReload((value) => value + 1);
      if (hasChannel) onConnected(channelId);
    } catch {
      if (!controller.signal.aborted) setError('action');
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  if (loading) {
    return (
      <div class="flex items-center justify-center gap-3 py-12" role="status">
        <Spinner size={22} />
        <span class="text-text-muted text-sm">{t('state.loading')}</span>
      </div>
    );
  }
  if (error === 'load' || error === 'unavailable') {
    return (
      <InlineHelp
        tone="neutral"
        title={t(error === 'unavailable' ? 'appConnection.unavailable' : 'appConnection.failed')}
      >
        <Button variant="secondary" size="sm" onClick={() => setReload((value) => value + 1)}>
          {t('action.retry')}
        </Button>
      </InlineHelp>
    );
  }

  const channelPicker = (
    <FieldGroup label={t('appConnection.roomsChannel')} htmlFor="app-rooms-channel" required>
      <Select
        id="app-rooms-channel"
        value={channelId}
        disabled={busy || !providerAvailable}
        onChange={(event) => setChannelId(event.currentTarget.value)}
      >
        <option value="">
          {t(channels.length ? 'appConnection.chooseChannel' : 'integrations.noChannels')}
        </option>
        {channels.map((channel) => (
          <option key={channel.id} value={channel.id}>
            #{channel.name}
          </option>
        ))}
      </Select>
    </FieldGroup>
  );

  const steps: WizardStep[] = [
    {
      id: 'choose',
      title: t('appConnection.choose'),
      canContinue: Boolean(provider && (provider.available || provider.connected)),
      onNext: () => {
        if (provider?.connected && provider.instance_id) {
          setInstanceId(provider.instance_id);
          setStep(2);
        } else setStep(1);
      },
      content: (
        <div class="grid gap-3 sm:grid-cols-2">
          {data.providers.map((item) => (
            <ChoiceCard
              key={item.key}
              name="rooms-app-provider"
              value={item.key}
              title={item.name}
              {...(!item.available
                ? { description: t('external.unavailable', { provider: item.name }) }
                : {})}
              icon="link"
              badge={item.connected ? <Badge tone="success">{t('meet.connected')}</Badge> : null}
              disabled={busy || (!item.available && !item.connected)}
              selected={providerKey === item.key}
              onSelect={() => {
                returnedHandledRef.current = true;
                setProviderKey(item.key);
                setInstanceId('');
                setError(null);
                setConfirmDisconnect(null);
                setDisconnected(false);
              }}
            />
          ))}
        </div>
      ),
    },
    {
      id: 'authorize',
      title: t('appConnection.authorize'),
      description: t('appConnection.authorizeBody', { provider: provider?.name ?? '' }),
      canContinue: awaitingReturnedAccount || providerAvailable,
      nextLabel: awaitingReturnedAccount
        ? t('action.retry')
        : t('appConnection.continue', { provider: provider?.name ?? '' }),
      onNext: () => (awaitingReturnedAccount ? setReload((value) => value + 1) : authorize()),
      content: (
        <div class="border-border bg-bg-subtle flex items-center gap-3 rounded-lg border p-4">
          <Icon name="lock" size={24} class="text-text-muted shrink-0" />
          <span class="text-text font-medium">{provider?.name}</span>
        </div>
      ),
    },
    {
      id: 'channels',
      title: t('appConnection.channels'),
      canContinue: providerAvailable && Boolean(selectedAccount) && hasChannel && hasDestination,
      content: (
        <div class="flex flex-col gap-5">
          {accounts.length ? (
            <FieldGroup htmlFor="app-connected-account" label={commonT('account.menu')} required>
              <Select
                id="app-connected-account"
                name="instance_id"
                value={instanceId}
                required
                disabled={busy}
                onChange={(event) => {
                  setInstanceId(event.currentTarget.value);
                  setError(null);
                  setConfirmDisconnect(null);
                }}
              >
                {accounts.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          ) : null}
          <Button
            variant="secondary"
            disabled={busy || !providerAvailable}
            onClick={() => {
              returnedHandledRef.current = true;
              setInstanceId('');
              setConfirmDisconnect(null);
              setStep(1);
            }}
          >
            {t('external.addAccount')}
          </Button>
          {channelPicker}
          <FieldGroup
            label={t('appConnection.destination', { provider: provider?.name ?? '' })}
            htmlFor="app-remote-channel"
            required
          >
            {loadingDestinations ? (
              <div class="flex items-center gap-2 py-2" role="status">
                <Spinner size={18} />
                <span class="text-text-muted text-sm">{t('state.loading')}</span>
              </div>
            ) : destinationError ? (
              <InlineHelp
                tone="danger"
                title={t('appConnection.failed')}
                action={
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => setRetryDestinations((value) => value + 1)}
                  >
                    {t('action.retry')}
                  </Button>
                }
              />
            ) : (
              <Select
                id="app-remote-channel"
                value={remoteId}
                onChange={(event) => setRemoteId(event.currentTarget.value)}
              >
                <option value="">
                  {t(
                    destinations.length ? 'appConnection.chooseChannel' : 'integrations.noChannels',
                  )}
                </option>
                {destinations.map((destination) => (
                  <option key={destination.id} value={destination.id}>
                    {destination.name}
                  </option>
                ))}
              </Select>
            )}
          </FieldGroup>
        </div>
      ),
    },
    {
      id: 'review',
      title: t('appConnection.review'),
      description: t('appConnection.shareSummary', {
        channel: channelName,
        destination: destinationName,
      }),
      nextLabel: t('appConnection.connect'),
      canContinue: providerAvailable && Boolean(instanceId) && hasChannel && hasDestination,
      onNext: connect,
      content: (
        <div class="flex flex-col gap-4">
          <ReviewSummary
            items={[
              { label: t('appConnection.choose'), value: provider?.name ?? '' },
              { label: t('appConnection.roomsChannel'), value: `#${channelName}` },
              {
                label: t('appConnection.destination', { provider: provider?.name ?? '' }),
                value: destinationName,
              },
            ]}
          />
          <DataFlow compact outgoing={['messages']} />
          <p class="text-text-muted text-sm">{t('appConnection.testHint')}</p>
        </div>
      ),
    },
    {
      id: 'done',
      title: t('appConnection.done'),
      done: true,
      content: (
        <div class="flex flex-col items-start gap-4">
          <Icon name="check" size={32} class="text-success-text" />
          <p class="text-text text-sm">
            {t('appConnection.doneBody', { channel: channelName, provider: provider?.name ?? '' })}
          </p>
          <Button onClick={onClose}>{t('action.done')}</Button>
        </div>
      ),
    },
  ];

  return (
    <div ref={wizardRef} class="flex flex-col gap-4">
      {provider && selectedAccount && step !== 4 ? (
        <div class="border-border-subtle flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-text text-sm font-medium">{selectedAccount.name}</span>
            <Badge tone="success">{t('meet.connected')}</Badge>
          </div>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            onClick={() => setConfirmDisconnect({ ...provider, instance_id: selectedAccount.id })}
          >
            {t('integrations.bridgeDisconnect')}
          </Button>
        </div>
      ) : null}
      {confirmDisconnect ? (
        <InlineHelp
          tone="warning"
          title={t('appConnection.disconnectBody', { provider: confirmDisconnect.name })}
        >
          <div class="mt-3 flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="danger"
              loading={busy}
              onClick={() => void disconnectAccount()}
            >
              {t('integrations.bridgeDisconnect')}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={busy}
              onClick={() => setConfirmDisconnect(null)}
            >
              {t('action.cancel')}
            </Button>
          </div>
        </InlineHelp>
      ) : null}
      {disconnected ? (
        <p class="text-text-muted text-sm" role="status">
          {t('appConnection.disconnected')}
        </p>
      ) : null}
      {error ? (
        <p class="text-danger-text text-sm" role="alert">
          {t(
            error === 'denied'
              ? 'appConnection.denied'
              : error === 'provider'
                ? 'external.unavailable'
                : 'appConnection.failed',
            { provider: provider?.name ?? '' },
          )}
        </p>
      ) : null}
      {provider && !providerAvailable ? (
        <p class="text-text-muted text-sm" role="status">
          {t('external.unavailable', { provider: provider.name })}
        </p>
      ) : null}
      <Wizard
        steps={steps}
        current={step}
        onCurrentChange={(value) => {
          setError(null);
          setConfirmDisconnect(null);
          setStep(value);
        }}
        busy={busy}
        onCancel={onClose}
        footerTarget={footerTarget}
      />
    </div>
  );
}
