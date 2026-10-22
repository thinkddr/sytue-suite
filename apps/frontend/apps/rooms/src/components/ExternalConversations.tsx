/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  EXTERNAL_CHAT_PROVIDERS,
  HttpError,
  type ExternalChatAccount,
  type ExternalChatAccounts,
  type ExternalChatChannel,
  type ExternalChatMessage,
  type ExternalChatProvider,
} from '@sytue/api-clients';
import { contiene } from '@sytue/i18n';
import {
  Badge,
  Button,
  ChoiceCard,
  ConfirmDialog,
  FieldGroup,
  Icon,
  InlineHelp,
  Input,
  Modal,
  ReviewSummary,
  Select,
  Spinner,
  Textarea,
  Wizard,
  type WizardStep,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { ExternalMeetings, type MeetingScope, type MeetingChannel } from './ExternalMeetings.js';
import { externalConversations as api } from '../client/externalConversations.js';

interface Conversation {
  account: ExternalChatAccount;
  channel: ExternalChatChannel;
}
const EMPTY: ExternalChatAccounts = { items: [], providers: [] };
function accountLabel(account: ExternalChatAccount): string {
  return [account.name, account.workspace_name || account.account_id].filter(Boolean).join(' · ');
}
function mergeMessages(
  previous: ExternalChatMessage[],
  next: ExternalChatMessage[],
): ExternalChatMessage[] {
  const all = new Map(previous.map((message) => [message.id, message]));
  for (const message of next) all.set(message.id, message);
  return [...all.values()].sort(
    (a, b) => Date.parse(a.created_at) - Date.parse(b.created_at) || a.id.localeCompare(b.id),
  );
}

/** Personal external channels have their own reader; native E2E messages never enter it. */
export function ExternalConversations({
  onClose,
  t,
  meetingChannels = [],
  activeChannelId,
  captureMeetingScope,
}: {
  onClose: () => void;
  t: T;
  meetingChannels?: MeetingChannel[];
  activeChannelId?: string | undefined;
  captureMeetingScope?: (() => MeetingScope) | undefined;
}): JSX.Element {
  const [data, setData] = useState<ExternalChatAccounts>(EMPTY);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const [reloadChannels, setReloadChannels] = useState(0);
  const returnedAppliedRef = useRef(false);
  const [step, setStep] = useState(0);
  const [provider, setProvider] = useState<ExternalChatProvider>('slack');
  const [meetingProvider, setMeetingProvider] = useState<'' | 'google-meet' | 'microsoft-teams'>(
    '',
  );
  const [instance, setInstance] = useState('');
  const [channels, setChannels] = useState<ExternalChatChannel[]>([]);
  const [cursor, setCursor] = useState('');
  const [remote, setRemote] = useState('');
  const [filter, setFilter] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [footer, setFooter] = useState<HTMLDivElement | null>(null);
  const [disconnecting, setDisconnecting] = useState<ExternalChatAccount | null>(null);
  const [returned] = useState(() => new URLSearchParams(window.location.search));
  const selected = data.items.find(
    (account) => account.instance_id === instance && account.provider === provider,
  );
  const channel = channels.find((item) => item.id === remote);
  const awaitingReturnedAccount = Boolean(
    returned.get('instance') === instance &&
    !returned.get('oauth_error') &&
    selected &&
    !selected.connected,
  );
  const name = EXTERNAL_CHAT_PROVIDERS[provider].name;
  const available = data.providers.some((item) => item.provider === provider && item.available);
  const connectionEpochRef = useRef(0);
  const aliveRef = useRef(true);
  useEffect(
    () => () => {
      aliveRef.current = false;
      connectionEpochRef.current++;
    },
    [],
  );

  useEffect(() => {
    const url = new URL(window.location.href);
    if (url.searchParams.get('connect-external') !== '1') return;
    for (const key of ['connect-external', 'instance', 'oauth_error', 'meeting'])
      url.searchParams.delete(key);
    window.history.replaceState(
      window.history.state,
      '',
      `${url.pathname}${url.search}${url.hash}`,
    );
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(false);
    void api
      .accounts(controller.signal)
      .then((result) => {
        setData(result);
        const account =
          !returnedAppliedRef.current &&
          result.items.find((item) => item.instance_id === returned.get('instance'));
        if (account) {
          setProvider(account.provider);
          setInstance(account.instance_id);
          setStep(account.connected ? 2 : 1);
        }
        const meeting =
          !returnedAppliedRef.current &&
          result.meetings?.find((item) => item.instance_id === returned.get('instance'));
        if (meeting && (!account || returned.get('meeting') === '1')) {
          setMeetingProvider(meeting.provider);
          setStep(1);
        }
        if (!returnedAppliedRef.current && returned.get('oauth_error')) setError(true);
        if (
          !returned.get('instance') ||
          (account && account.connected) ||
          (meeting && meeting.connected) ||
          returned.get('oauth_error')
        )
          returnedAppliedRef.current = true;
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [reload, returned]);
  useEffect(() => {
    const epoch = ++connectionEpochRef.current;
    setChannels([]);
    setCursor('');
    setRemote('');
    setFilter('');
    setBusy(false);
    if (!instance || !selected?.connected) return;
    const controller = new AbortController();
    setBusy(true);
    void api
      .channels(instance, '', controller.signal)
      .then((page) => {
        if (connectionEpochRef.current !== epoch) return;
        setChannels(page.items);
        setCursor(page.next_cursor ?? '');
        if (page.items.length === 1) setRemote(page.items[0]!.id);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted && connectionEpochRef.current === epoch) setBusy(false);
      });
    return () => controller.abort();
  }, [instance, selected?.connected, reloadChannels]);

  async function connect(account?: ExternalChatAccount): Promise<void> {
    setBusy(true);
    setError(false);
    try {
      const result = await api.connect(provider, account?.instance_id);
      if (aliveRef.current) window.location.assign(result.authorize_url);
    } catch {
      setError(true);
      setBusy(false);
    }
  }
  async function moreChannels(): Promise<void> {
    const epoch = connectionEpochRef.current;
    setBusy(true);
    setError(false);
    try {
      const page = await api.channels(instance, cursor);
      if (epoch !== connectionEpochRef.current) return;
      setChannels((previous) => [
        ...new Map([...previous, ...page.items].map((item) => [item.id, item])).values(),
      ]);
      setCursor(page.next_cursor ?? '');
    } catch {
      if (epoch === connectionEpochRef.current) setError(true);
    } finally {
      if (epoch === connectionEpochRef.current) setBusy(false);
    }
  }
  const accounts = data.items.filter((account) => account.provider === provider);
  const steps: WizardStep[] = [
    {
      id: 'provider',
      title: t('appConnection.choose'),
      content: (
        <div class="space-y-3">
          <p class="text-text-muted text-sm">{t('external.body')}</p>
          <fieldset class="grid gap-2 sm:grid-cols-3">
            <legend class="sr-only">{t('appConnection.choose')}</legend>
            {(Object.keys(EXTERNAL_CHAT_PROVIDERS) as ExternalChatProvider[]).map((key) => (
              <ChoiceCard
                disabled={busy}
                key={key}
                name="external-provider"
                title={EXTERNAL_CHAT_PROVIDERS[key].name}
                selected={!meetingProvider && provider === key}
                onSelect={() => {
                  setMeetingProvider('');
                  setProvider(key);
                  setInstance('');
                  setError(false);
                }}
              />
            ))}
          </fieldset>
          <h3 class="text-text text-sm font-semibold">{t('external.meet.title')}</h3>
          <fieldset class="grid gap-2 sm:grid-cols-2">
            <legend class="sr-only">{t('external.meet.title')}</legend>
            {(['google-meet', 'microsoft-teams'] as const)
              .filter((key) => data.meetings?.some((item) => item.provider === key))
              .map((key) => (
                <ChoiceCard
                  key={key}
                  name="external-provider"
                  title={t('link.joinMeeting', {
                    provider: key === 'google-meet' ? 'Google Meet' : 'Microsoft Teams',
                  })}
                  description={
                    !data.meetings?.some(
                      (item) =>
                        item.provider === key &&
                        !item.disabled_by_org &&
                        (item.available || item.connected),
                    )
                      ? t('external.unavailable', {
                          provider: key === 'google-meet' ? 'Google Meet' : 'Microsoft Teams',
                        })
                      : undefined
                  }
                  selected={meetingProvider === key}
                  disabled={
                    busy ||
                    !data.meetings?.some(
                      (item) =>
                        item.provider === key &&
                        !item.disabled_by_org &&
                        (item.available || item.connected),
                    )
                  }
                  onSelect={() => {
                    setMeetingProvider(key);
                    setError(false);
                  }}
                />
              ))}
          </fieldset>
        </div>
      ),
    },
    {
      id: 'account',
      title: t('appConnection.authorize'),
      description: t('appConnection.authorizeBody', { provider: name }),
      content: (
        <div class="space-y-3">
          <fieldset class="space-y-2">
            <legend class="sr-only">{t('appConnection.authorize')}</legend>
            {accounts.map((account) => (
              <div key={account.instance_id} class="flex items-center gap-2">
                <ChoiceCard
                  disabled={busy}
                  class="min-w-0 flex-1"
                  name="external-account"
                  title={accountLabel(account)}
                  selected={account.instance_id === instance}
                  onSelect={() => setInstance(account.instance_id)}
                />
                <Button
                  variant="ghost"
                  size="xs"
                  disabled={busy}
                  onClick={() => setDisconnecting(account)}
                >
                  {t('integrations.bridgeDisconnect')}
                </Button>
              </div>
            ))}
          </fieldset>
          <Button variant="secondary" disabled={busy || !available} onClick={() => void connect()}>
            {t('external.addAccount')}
          </Button>
          {!available ? (
            <InlineHelp>{t('external.unavailable', { provider: name })}</InlineHelp>
          ) : null}
        </div>
      ),
      canContinue: awaitingReturnedAccount || Boolean(selected?.connected) || available,
      ...(!selected?.connected
        ? {
            nextLabel: t(awaitingReturnedAccount ? 'action.retry' : 'appConnection.continue', {
              provider: name,
            }),
          }
        : {}),
      onNext: () => {
        if (awaitingReturnedAccount) setReload((value) => value + 1);
        else if (selected?.connected) setStep(2);
        else return connect(selected);
      },
    },
    {
      id: 'channel',
      title: t('appConnection.channels'),
      content: (
        <div class="space-y-3">
          <FieldGroup htmlFor="external-filter" label={t('browse.searchPlaceholder')}>
            <Input
              id="external-filter"
              value={filter}
              onInput={(event) => setFilter(event.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            htmlFor="external-channel"
            label={t('appConnection.destination', { provider: name })}
          >
            <Select
              id="external-channel"
              value={remote}
              onChange={(event) => setRemote(event.currentTarget.value)}
            >
              <option value="">{t('appConnection.chooseChannel')}</option>
              {channels
                .filter((item) => !filter || contiene(item.name, filter))
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
            </Select>
          </FieldGroup>
          {cursor ? (
            <Button variant="ghost" disabled={busy} onClick={() => void moreChannels()}>
              {t('action.loadMore')}
            </Button>
          ) : null}
          {!channels.length && !busy ? (
            <p class="text-text-muted text-sm">{t('sidebar.noChannelsYet')}</p>
          ) : null}
        </div>
      ),
      canContinue: Boolean(channel),
    },
    {
      id: 'review',
      title: t('appConnection.review'),
      content: (
        <div class="space-y-3">
          <ReviewSummary
            items={[
              { label: name, value: selected ? accountLabel(selected) : '' },
              {
                label: t('appConnection.destination', { provider: name }),
                value: channel?.name ?? '',
              },
            ]}
          />
          <InlineHelp>{t('external.protection', { provider: name })}</InlineHelp>
        </div>
      ),
      nextLabel: t('external.open'),
      canContinue: Boolean(selected?.connected && channel),
      onNext: () => {
        if (selected && channel) setConversation({ account: selected, channel });
      },
    },
  ];
  const visibleSteps: WizardStep[] = meetingProvider
    ? [
        steps[0]!,
        {
          id: 'meeting',
          title: t('external.meet.title'),
          content: (
            <ExternalMeetings
              chatInstances={data.items
                .filter((item) => item.connected)
                .map((item) => item.instance_id)}
              onlyProvider={meetingProvider}
              t={t}
              meetings={data.meetings ?? []}
              returnedInstance={returned.get('instance') ?? ''}
              returnedError={Boolean(returned.get('oauth_error'))}
              onReload={() => setReload((value) => value + 1)}
              channels={meetingChannels}
              activeChannelId={activeChannelId}
              captureScope={captureMeetingScope}
            />
          ),
          nextLabel: t('action.close'),
          onNext: onClose,
        },
      ]
    : steps;
  return (
    <>
      <Modal
        open
        onClose={onClose}
        title={conversation?.channel.name ?? t('external.title')}
        size="lg"
        footer={!conversation ? <div ref={setFooter} class="w-full min-w-0" /> : undefined}
      >
        {loading ? (
          <div class="grid min-h-48 place-items-center">
            <Spinner />
          </div>
        ) : conversation ? (
          <ExternalConversation
            key={`${conversation.account.instance_id}:${conversation.channel.id}`}
            conversation={conversation}
            t={t}
            onBack={() => setConversation(null)}
          />
        ) : (
          <>
            {error ? (
              <div role="alert" class="mb-3">
                <InlineHelp tone="danger">{t('appConnection.failed')}</InlineHelp>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setReload((value) => value + 1);
                    setReloadChannels((value) => value + 1);
                  }}
                >
                  {t('action.retry')}
                </Button>
              </div>
            ) : null}
            <Wizard
              current={step}
              onCurrentChange={setStep}
              steps={visibleSteps}
              busy={busy}
              onCancel={onClose}
              footerTarget={footer}
            />
          </>
        )}
      </Modal>
      <ConfirmDialog
        open={Boolean(disconnecting)}
        onClose={() => setDisconnecting(null)}
        title={t('integrations.bridgeDisconnect')}
        tone="danger"
        confirmLabel={t('integrations.bridgeDisconnect')}
        busy={busy}
        onConfirm={() => {
          if (!disconnecting) return;
          setBusy(true);
          void api
            .disconnect(disconnecting.instance_id)
            .then(() => {
              if (instance === disconnecting.instance_id) setInstance('');
              setDisconnecting(null);
              setReload((value) => value + 1);
            })
            .catch(() => setError(true))
            .finally(() => setBusy(false));
        }}
      >
        <p>{t('external.disconnectBody', { provider: disconnecting?.name ?? name })}</p>
      </ConfirmDialog>
    </>
  );
}

function ExternalConversation({
  conversation: { account, channel },
  t,
  onBack,
}: {
  conversation: Conversation;
  t: T;
  onBack: () => void;
}): JSX.Element {
  const [messages, setMessages] = useState<ExternalChatMessage[]>([]);
  const [capabilities, setCapabilities] = useState(channel.capabilities);
  const [cursor, setCursor] = useState('');
  const [thread, setThread] = useState('');
  const [draft, setDraft] = useState('');
  const [editing, setEditing] = useState<ExternalChatMessage | null>(null);
  const [deleting, setDeleting] = useState<ExternalChatMessage | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [reload, setReload] = useState(0);
  const generationRef = useRef(0);
  const name = EXTERNAL_CHAT_PROVIDERS[account.provider].name;
  const sorted = useMemo(() => mergeMessages([], messages), [messages]);
  useEffect(() => {
    setDraft('');
    setEditing(null);
    setUncertain(false);
  }, [thread]);
  useEffect(() => {
    const epoch = ++generationRef.current;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let initialized = false;
    const interval = account.provider === 'slack' ? 60000 : 15000;
    setMessages([]);
    setCursor('');
    setBusy(false);
    setError(false);
    setLoading(true);
    async function read(): Promise<void> {
      let delay = interval;
      try {
        if (document.visibilityState !== 'hidden') {
          const page = await api.messages(account.instance_id, channel.id, {
            ...(thread ? { thread } : {}),
            signal: controller.signal,
          });
          if (epoch !== generationRef.current || controller.signal.aborted) return;
          // The first page is authoritative for its recent time range, including external deletions.
          const ascending = account.provider === 'slack' && Boolean(thread);
          const times = page.items.map((message) => Date.parse(message.created_at));
          const boundary = ascending ? Math.max(...times) : Math.min(...times);
          // Without read access the page is always empty: keep what was sent from here.
          if (page.capabilities.read)
            setMessages((previous) =>
              mergeMessages(
                page.next_cursor
                  ? previous.filter((message) =>
                      ascending
                        ? Date.parse(message.created_at) > boundary
                        : Date.parse(message.created_at) < boundary,
                    )
                  : [],
                page.items,
              ),
            );
          setCapabilities(page.capabilities);
          if (!initialized) {
            setCursor(page.next_cursor ?? '');
            initialized = true;
          }
          setError(false);
          setUncertain(false);
        }
      } catch (err) {
        if (err instanceof HttpError) delay = Math.max(interval, err.retryAfterMs);
        if (!controller.signal.aborted && epoch === generationRef.current) setError(true);
      } finally {
        if (!controller.signal.aborted && epoch === generationRef.current) {
          setLoading(false);
          timer = setTimeout(() => void read(), delay);
        }
      }
    }
    void read();
    return () => {
      // oxlint-disable-next-line react-hooks/exhaustive-deps
      generationRef.current++;
      controller.abort();
      clearTimeout(timer);
    };
  }, [account.instance_id, account.provider, channel.id, thread, reload]);
  async function older(): Promise<void> {
    const epoch = generationRef.current;
    setBusy(true);
    try {
      const page = await api.messages(account.instance_id, channel.id, {
        cursor,
        ...(thread ? { thread } : {}),
      });
      if (epoch !== generationRef.current) return;
      setMessages((previous) => mergeMessages(previous, page.items));
      setCursor(page.next_cursor ?? '');
    } catch {
      if (epoch === generationRef.current) setError(true);
    } finally {
      if (epoch === generationRef.current) setBusy(false);
    }
  }
  async function send(event: Event): Promise<void> {
    event.preventDefault();
    if (busy || loading || uncertain || !draft.trim() || !capabilities.send) return;
    const epoch = generationRef.current;
    setBusy(true);
    setError(false);
    try {
      if (editing) {
        await api.action(
          account.instance_id,
          'edit',
          channel.id,
          editing.id,
          draft,
          thread || undefined,
        );
        if (epoch === generationRef.current) {
          setMessages((previous) =>
            previous.map((message) =>
              message.id === editing.id ? { ...message, text: draft, edited: true } : message,
            ),
          );
          setEditing(null);
        }
      } else {
        const message = await api.send(account.instance_id, channel.id, draft, thread || undefined);
        if (epoch === generationRef.current)
          setMessages((previous) => mergeMessages(previous, [message]));
      }
      if (epoch === generationRef.current) setDraft('');
    } catch {
      if (epoch === generationRef.current) {
        setError(true);
        setUncertain(true);
      }
    } finally {
      if (epoch === generationRef.current) setBusy(false);
    }
  }
  return (
    <div class="flex min-h-0 flex-col gap-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <Button variant="ghost" size="sm" disabled={busy || uncertain} onClick={onBack}>
          <Icon name="chevronLeft" size={16} />
          {t('appConnection.channels')}
        </Button>
        <Badge>{name}</Badge>
      </div>
      <p class="text-text-muted text-xs">{accountLabel(account)}</p>
      <InlineHelp>{t('external.protection', { provider: name })}</InlineHelp>
      {capabilities.read ? null : (
        <InlineHelp>{t('external.sendOnly', { provider: name })}</InlineHelp>
      )}
      {thread ? (
        <div class="flex items-center justify-between">
          <span class="text-text text-sm font-semibold">{t('thread.title')}</span>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy || uncertain}
            onClick={() => setThread('')}
          >
            {t('thread.close')}
          </Button>
        </div>
      ) : null}
      {error ? (
        <div role="alert">
          <InlineHelp tone="danger">
            {t(uncertain ? 'external.sendUnconfirmed' : 'message.loadFailed')}
          </InlineHelp>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            onClick={() => setReload((value) => value + 1)}
          >
            {t('action.retry')}
          </Button>
        </div>
      ) : null}
      <div
        class="border-border-subtle max-h-[48dvh] min-h-48 overflow-auto rounded-md border p-3"
        aria-label={t('message.listAria')}
        aria-busy={loading}
      >
        {cursor ? (
          <Button size="sm" variant="ghost" disabled={busy} onClick={() => void older()}>
            {t('action.loadMore')}
          </Button>
        ) : null}
        {loading ? (
          <Spinner />
        ) : !messages.length ? (
          <p class="text-text-muted py-6 text-center text-sm">{t('message.noneYet')}</p>
        ) : null}
        <ol class="space-y-4">
          {sorted.map((message) => (
            <li key={message.id}>
              <div class="text-text-muted flex flex-wrap items-baseline gap-2 text-xs">
                <span class="text-text font-semibold">
                  {message.author_name || message.author_id}
                </span>
                <time dateTime={message.created_at}>
                  {new Date(message.created_at).toLocaleString()}
                </time>
                {message.edited ? <span>{t('message.edited')}</span> : null}
              </div>
              <p class="text-text mt-1 text-sm break-words whitespace-pre-wrap">{message.text}</p>
              <div class="mt-1 flex flex-wrap gap-1">
                {capabilities.threads && !thread ? (
                  <Button
                    variant="ghost"
                    size="xs"
                    disabled={busy || uncertain}
                    onClick={() => setThread(message.thread_id || message.id)}
                  >
                    {t('message.replyInThread')}
                    {message.reply_count ? ` · ${message.reply_count}` : ''}
                  </Button>
                ) : null}
                {message.owned && capabilities.edit ? (
                  <Button
                    variant="ghost"
                    size="xs"
                    disabled={busy || uncertain}
                    onClick={() => {
                      setEditing(message);
                      setDraft(message.text);
                    }}
                  >
                    {t('action.edit')}
                  </Button>
                ) : null}
                {message.owned && capabilities.delete ? (
                  <Button
                    variant="ghost"
                    size="xs"
                    disabled={busy || uncertain}
                    onClick={() => setDeleting(message)}
                  >
                    {t('action.delete')}
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ol>
      </div>
      <form onSubmit={(event) => void send(event)} class="space-y-2">
        <FieldGroup
          htmlFor="external-message"
          label={t(editing ? 'message.edit' : 'composer.writeInAria', { channel: channel.name })}
        >
          <Textarea
            id="external-message"
            name="message"
            value={draft}
            maxLength={40000}
            required
            disabled={busy || loading || uncertain || !capabilities.send}
            onInput={(event) => setDraft(event.currentTarget.value)}
            rows={3}
          />
        </FieldGroup>
        <div class="flex justify-end gap-2">
          {editing ? (
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setEditing(null);
                setDraft('');
              }}
            >
              {t('action.cancel')}
            </Button>
          ) : null}
          <Button type="submit" disabled={busy || loading || uncertain || !capabilities.send}>
            {t(editing ? 'action.save' : 'composer.send')}
          </Button>
        </div>
      </form>
      <ConfirmDialog
        open={Boolean(deleting)}
        onClose={() => setDeleting(null)}
        title={t('action.delete')}
        tone="danger"
        confirmLabel={t('action.delete')}
        busy={busy}
        onConfirm={() => {
          if (!deleting) return;
          const epoch = generationRef.current;
          setBusy(true);
          void api
            .action(
              account.instance_id,
              'delete',
              channel.id,
              deleting.id,
              undefined,
              thread || undefined,
            )
            .then(() => {
              if (epoch === generationRef.current) {
                setMessages((previous) => previous.filter((message) => message.id !== deleting.id));
                setDeleting(null);
              }
            })
            .catch(() => {
              if (epoch === generationRef.current) setError(true);
            })
            .finally(() => {
              if (epoch === generationRef.current) setBusy(false);
            });
        }}
      >
        <p>{t('message.confirmDelete')}</p>
      </ConfirmDialog>
    </div>
  );
}
