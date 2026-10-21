/* SPDX-License-Identifier: Apache-2.0 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { HttpError, teamsMeetingAppURL, type ExternalMeetingAccount } from '@sytue/api-clients';
import { useT as useCommonT } from '@sytue/i18n/preact';
import { Badge, Button, FieldGroup, InlineHelp, Select } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { externalConversations as api } from '../client/externalConversations.js';

export interface MeetingChannel {
  id: string;
  name: string;
  kind?: string;
  archived?: boolean;
}
/** Bound to the Matrix identity before the provider write, including logout generation. */
export interface MeetingScope {
  assertCurrent: () => void;
  share: (channel: string, url: string, transaction: string) => Promise<void>;
}
interface Props {
  t: T;
  chatInstances?: string[] | undefined;
  onlyProvider?: ExternalMeetingAccount['provider'] | undefined;
  meetings: ExternalMeetingAccount[];
  returnedInstance: string;
  returnedError: boolean;
  onReload: () => void;
  channels: MeetingChannel[];
  activeChannelId?: string | undefined;
  captureScope?: (() => MeetingScope) | undefined;
}
export function ExternalMeetings(props: Props) {
  return (
    <>
      {(['google-meet', 'microsoft-teams'] as const)
        .filter((provider) => !props.onlyProvider || props.onlyProvider === provider)
        .map((provider) => (
          <MeetingBlock key={provider} {...props} provider={provider} />
        ))}
    </>
  );
}

function MeetingBlock({
  t,
  meetings,
  provider,
  returnedInstance,
  returnedError,
  onReload,
  channels,
  activeChannelId,
  captureScope,
  chatInstances = [],
}: Props & { provider: ExternalMeetingAccount['provider'] }) {
  const commonT = useCommonT();
  const teams = provider === 'microsoft-teams';
  const name = teams ? 'Microsoft Teams' : 'Google Meet';
  const [busy, setBusy] = useState(false);
  const [link, setLink] = useState('');
  const [error, setError] = useState<
    | ''
    | 'external.teamsMeeting.unknown'
    | 'external.teamsMeeting.unprotected'
    | 'appConnection.failed'
    | 'external.meet.failed'
    | 'share.failed'
  >('');
  const [instance, setInstance] = useState('');
  const [pending, setPending] = useState('');
  const [channel, setChannel] = useState(activeChannelId ?? '');
  const [shared, setShared] = useState(false);
  const current = useRef('');
  const alive = useRef(true);
  const appliedReturn = useRef('');
  const uncertain = useRef(new Set<string>());
  const scope = useRef<MeetingScope | null>(null);
  const transaction = useRef<{ channel: string; url: string; id: string } | null>(null);
  const entries = meetings.filter((item) => item.provider === provider);
  const accounts = useMemo(
    () =>
      meetings.filter(
        (item) => item.provider === provider && item.instance_id && (teams || item.connected),
      ),
    [meetings, provider, teams],
  );
  const available = entries.some((item) => item.available && !item.disabled_by_org);
  const selected = accounts.find((item) => item.instance_id === instance);
  const waiting = Boolean(pending && !selected?.connected && !returnedError);
  useEffect(
    () => () => {
      alive.current = false;
    },
    [],
  );
  useEffect(() => {
    const returnedMeeting = meetings.find(
      (item) => item.provider === provider && item.instance_id === returnedInstance,
    );
    if (returnedInstance && appliedReturn.current !== returnedInstance && returnedMeeting) {
      appliedReturn.current = returnedInstance;
      choose(returnedInstance);
      if (!returnedMeeting.connected) setPending(returnedInstance);
      return;
    }
    if (!current.current && accounts.length === 1) choose(accounts[0]!.instance_id!);
  }, [accounts, meetings, provider, returnedInstance]);

  function choose(id: string) {
    current.current = id;
    setInstance(id);
    setPending('');
    setLink('');
    setError('');
    setShared(false);
    scope.current = null;
    transaction.current = null;
  }
  async function run(newAccount = false) {
    const id = current.current;
    if (busy || (!newAccount && uncertain.current.has(id))) return;
    setBusy(true);
    setError('');
    setLink('');
    setShared(false);
    scope.current = null;
    transaction.current = null;
    let writing = false;
    try {
      if (!newAccount && selected?.connected) {
        const owner = captureScope?.();
        owner?.assertCurrent();
        writing = true;
        const result = teams
          ? await api.protectedTeamsMeeting(id, selected.account_id ?? '')
          : await api.meeting(id);
        owner?.assertCurrent();
        if (alive.current && current.current === id) {
          scope.current = owner ?? null;
          setLink(result.url);
        }
      } else {
        const result = await api.connectMeeting(
          provider,
          newAccount ? undefined : pending || id || undefined,
        );
        if (alive.current) {
          setPending(result.instance_id);
          current.current = result.instance_id;
          setInstance(result.instance_id);
          window.location.assign(result.authorize_url);
        }
      }
    } catch (err) {
      if (!alive.current || current.current !== id) return;
      const code = err instanceof HttpError ? err.code : undefined;
      // Any missing response after a write may mean Microsoft accepted it.
      // Never turn a transport/parse error into another POST.
      if (teams && writing && (code === 'external_meeting_unknown' || !code)) {
        uncertain.current.add(id);
        setError('external.teamsMeeting.unknown');
      } else
        setError(
          teams && code === 'external_meeting_unprotected'
            ? 'external.teamsMeeting.unprotected'
            : teams
              ? 'appConnection.failed'
              : 'external.meet.failed',
        );
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function share() {
    const owner = scope.current;
    if (busy || !link || !channel || !owner || shared) return;
    if (
      !transaction.current ||
      transaction.current.channel !== channel ||
      transaction.current.url !== link
    )
      transaction.current = { channel, url: link, id: crypto.randomUUID() };
    const operation = transaction.current;
    setBusy(true);
    setError('');
    try {
      owner.assertCurrent();
      await owner.share(operation.channel, operation.url, operation.id);
      owner.assertCurrent();
      if (alive.current && transaction.current === operation) setShared(true);
    } catch {
      if (alive.current) setError('share.failed');
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  if (!entries.length) return null;
  const channelChoices = channels.filter((item) => item.kind !== 'direct' && !item.archived);
  return (
    <section class="border-border mt-4 space-y-3 border-t pt-4" aria-label={name}>
      <h3 class="text-text text-sm font-semibold">{t('link.joinMeeting', { provider: name })}</h3>
      <p class="text-text-muted text-sm">
        {t(teams ? 'external.teamsMeeting.body' : 'external.meet.body')}
      </p>
      {accounts.length ? (
        <FieldGroup
          htmlFor={teams ? `meeting-${provider}` : 'external-meet-account'}
          label={commonT('account.menu')}
        >
          <Select
            id={teams ? `meeting-${provider}` : 'external-meet-account'}
            value={instance}
            disabled={busy}
            onChange={(event) => choose(event.currentTarget.value)}
          >
            <option value="">{t('appConnection.authorize')}</option>
            {accounts.map((item) => (
              <option key={item.instance_id} value={item.instance_id}>
                {item.name || name}
              </option>
            ))}
          </Select>
        </FieldGroup>
      ) : null}
      {teams && selected && !selected.connected && chatInstances.includes(selected.instance_id!) ? (
        <InlineHelp>{t('external.teamsMeeting.extend')}</InlineHelp>
      ) : null}
      {!available ? <InlineHelp>{t('external.unavailable', { provider: name })}</InlineHelp> : null}
      <div class="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          disabled={
            busy ||
            Boolean(selected?.disabled_by_org) ||
            uncertain.current.has(instance) ||
            (!selected && accounts.length > 0 && !pending) ||
            (!selected?.connected && !available && !waiting)
          }
          onClick={() => (waiting ? onReload() : void run())}
        >
          {t(
            waiting
              ? 'action.retry'
              : selected?.connected || (!teams && accounts.length > 0 && !pending)
                ? teams
                  ? 'external.teamsMeeting.create'
                  : 'external.meet.create'
                : teams
                  ? 'appConnection.authorize'
                  : 'external.meet.connect',
          )}
        </Button>
        {accounts.length ? (
          <Button variant="ghost" disabled={busy || !available} onClick={() => void run(true)}>
            {t('external.addAccount')}
          </Button>
        ) : null}
      </div>
      {link ? (
        <div class="space-y-2">
          {teams ? <Badge tone="success">{t('external.teamsMeeting.verified')}</Badge> : null}
          <div class="flex flex-wrap items-center gap-2">
            <a
              class="text-primary-text min-w-0 truncate text-sm underline"
              href={teams ? teamsMeetingAppURL(link) : link}
              {...(!teams ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
            >
              {teams ? t('action.open') : link}
            </a>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => void navigator.clipboard?.writeText(link)}
            >
              {t('action.copy')}
            </Button>
          </div>
          {scope.current && channelChoices.length ? (
            <>
              <FieldGroup
                htmlFor={`meeting-channel-${provider}`}
                label={t('appConnection.roomsChannel')}
              >
                <Select
                  id={`meeting-channel-${provider}`}
                  value={channel}
                  disabled={busy}
                  onChange={(event) => {
                    setChannel(event.currentTarget.value);
                    transaction.current = null;
                    setShared(false);
                  }}
                >
                  <option value="">{t('appConnection.chooseChannel')}</option>
                  {channelChoices.map((item) => (
                    <option key={item.id} value={item.id}>
                      #{item.name}
                    </option>
                  ))}
                </Select>
              </FieldGroup>
              <Button
                variant="secondary"
                disabled={busy || !channel || shared}
                onClick={() => void share()}
              >
                {t(shared ? 'action.done' : 'share.title')}
              </Button>
            </>
          ) : null}
        </div>
      ) : null}
      {error ? <InlineHelp tone="warning">{t(error)}</InlineHelp> : null}
    </section>
  );
}
