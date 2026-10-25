/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useI18n } from '@sytue/i18n/preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  HttpError,
  MAIL_PROVIDERS,
  type MailProvider,
  type ExternalMailAccount,
  type ExternalMailAccounts,
  type ExternalMailConfiguration,
  type ExternalMailFolder,
  type ExternalMailMessage,
  type ExternalMailAttachment,
  type LinkedService,
  type LinkedServiceCapability,
} from '@sytue/api-clients';
import {
  Button,
  ChoiceCard,
  Checkbox,
  ConfirmDialog,
  FieldGroup,
  Input,
  InlineHelp,
  Modal,
  Select,
  Spinner,
  Textarea,
  Wizard,
  type WizardStep,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { externalMailClient } from '../client/externalMail.js';
import { fileToAttachment } from '../client/attachments.js';
import { parseAddresses, formatBytes, shortDate, fullDate } from './utils.js';

const EMPTY: ExternalMailAccounts = { items: [], providers: [] };
const LIMIT = 18 * 1024 * 1024; // MIME/base64 overhead fits the 25 MiB message limit.
function errorKey(
  error: unknown,
):
  | 'externalMail.error'
  | 'externalMail.tlsError'
  | 'externalMail.authError'
  | 'externalMail.sendUnknown'
  | 'externalMail.blockedError'
  | 'externalMail.tooLarge' {
  const code = error instanceof HttpError ? error.code : '';
  if (code === 'mail_tls') return 'externalMail.tlsError';
  if (code === 'mail_auth' || code === 'external_reconnect_required')
    return 'externalMail.authError';
  if (code === 'mail_send_unknown') return 'externalMail.sendUnknown';
  if (code === 'mail_blocked') return 'externalMail.blockedError';
  if (code === 'mail_too_large') return 'externalMail.tooLarge';
  return 'externalMail.error';
}

function newAccountForm() {
  return {
    name: '',
    address: '',
    host: '',
    port: '993',
    username: '',
    password: '',
    smtpHost: '',
    smtpPort: '465',
    smtpUser: '',
    smtpPassword: '',
    smtpSecurity: 'tls' as 'tls' | 'starttls',
  };
}

export function ExternalMail({ onClose, t }: { onClose: () => void; t: T }): JSX.Element {
  const { t: commonT } = useI18n();
  const api = useMemo(externalMailClient, []);
  const [data, setData] = useState(EMPTY);
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const [provider, setProvider] = useState<MailProvider>('gmail');
  const [step, setStep] = useState(0);
  const [reconnectInstance, setReconnectInstance] = useState('');
  const [services, setServices] = useState<LinkedService[]>(['mail']);
  const [connecting, setConnecting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ReturnType<typeof errorKey> | ''>('');
  const [selected, setSelected] = useState<ExternalMailAccount | null>(null);
  const [disconnecting, setDisconnecting] = useState<ExternalMailAccount | null>(null);
  const [footer, setFooter] = useState<HTMLDivElement | null>(null);
  const [form, setForm] = useState(newAccountForm);
  const formRef = useRef<HTMLFormElement | null>(null);
  const alive = useRef(true);
  const returned = useRef(
    new URLSearchParams(typeof window === 'undefined' ? '' : window.location.search),
  );
  useEffect(
    () => () => {
      alive.current = false;
    },
    [],
  );
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    void api
      .accounts(controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setData(result);
        const id = returned.current.get('instance');
        const account = result.items.find((item) => item.instance_id === id);
        if (account) {
          setProvider(account.provider);
          setReconnectInstance(account.instance_id);
        }
        if (returned.current.has('oauth_error')) {
          setConnecting(true);
          setError('externalMail.authError');
          setStep(1);
        } else if (account?.connected && (account.services ?? ['mail']).includes('mail')) {
          setSelected(account);
          setConnecting(false);
        }
        if (returned.current.has('connect-mail')) {
          const url = new URL(window.location.href);
          for (const key of ['connect-mail', 'instance', 'oauth_error'])
            url.searchParams.delete(key);
          window.history.replaceState(window.history.state, '', url);
          returned.current = new URLSearchParams();
        }
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(errorKey(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [api, attempt]);
  const available = data.providers.some((item) => item.provider === provider && item.available);
  const oauth = MAIL_PROVIDERS[provider].oauth;
  const offeredServices: LinkedServiceCapability[] = data.providers.find(
    (item) => item.provider === provider,
  )?.services ?? [{ service: 'mail', available: true }];
  function field(key: keyof typeof form, value: string): void {
    setForm((previous) => ({ ...previous, [key]: value }));
    setError('');
  }
  function clearCredentials(): void {
    setForm((previous) => ({ ...previous, password: '', smtpPassword: '' }));
  }
  function reconnect(account: ExternalMailAccount): void {
    const cfg = account.configuration;
    setSelected(null);
    setProvider(account.provider);
    setReconnectInstance(account.instance_id);
    setServices(Array.from(new Set<LinkedService>(['mail', ...(account.services ?? [])])));
    setConnecting(true);
    setStep(1);
    setError('');
    setForm({
      name: account.name,
      address: account.address,
      host: cfg?.incoming.host ?? '',
      port: String(cfg?.incoming.port ?? (account.provider === 'pop3' ? 995 : 993)),
      username: cfg?.incoming.username ?? '',
      password: '',
      smtpHost: cfg?.outgoing.host ?? '',
      smtpPort: String(cfg?.outgoing.port ?? 465),
      smtpUser: cfg?.outgoing.username ?? '',
      smtpPassword: '',
      smtpSecurity: cfg?.outgoing.security === 'starttls' ? 'starttls' : 'tls',
    });
  }
  function close(): void {
    if (!busy) {
      clearCredentials();
      onClose();
    }
  }
  async function connect(): Promise<void> {
    if (!available) return;
    setBusy(true);
    setError('');
    try {
      if (oauth) {
        const result = await api.connect(
          provider,
          'mail',
          reconnectInstance || undefined,
          services,
        );
        if (alive.current) window.location.assign(result.authorize_url);
        return;
      }
      const configuration: ExternalMailConfiguration = {
        provider: provider === 'pop3' ? 'pop3' : 'imap',
        name: form.name,
        address: form.address,
        incoming: {
          host: form.host,
          port: Number(form.port),
          username: form.username || form.address,
          security: 'tls',
        },
        outgoing: {
          host: form.smtpHost,
          port: Number(form.smtpPort),
          username: form.smtpUser || form.username || form.address,
          security: form.smtpSecurity,
        },
        password: form.password,
        ...(reconnectInstance ? { instance_id: reconnectInstance } : {}),
        ...(form.smtpPassword ? { smtp_password: form.smtpPassword } : {}),
      };
      const result = await api.configure(configuration);
      if (!alive.current) return;
      clearCredentials();
      const updated = await api.accounts();
      if (!alive.current) return;
      setData(updated);
      setSelected(updated.items.find((item) => item.instance_id === result.instance_id) ?? null);
      setConnecting(false);
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function disconnect(): Promise<void> {
    if (!disconnecting) return;
    setBusy(true);
    setError('');
    try {
      await api.disconnect(disconnecting.instance_id);
      if (alive.current) {
        setDisconnecting(null);
        setAttempt((value) => value + 1);
      }
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  const steps: WizardStep[] = [
    {
      id: 'provider',
      title: t('externalMail.provider'),
      description: t('externalMail.body'),
      content: (
        <fieldset class="grid gap-3 sm:grid-cols-2">
          <legend class="sr-only">{t('externalMail.provider')}</legend>
          {(Object.keys(MAIL_PROVIDERS) as MailProvider[]).map((key) => (
            <ChoiceCard
              key={key}
              name="mail-provider"
              title={MAIL_PROVIDERS[key].name}
              selected={provider === key}
              onSelect={() => {
                setProvider(key);
                setReconnectInstance('');
                setServices(['mail']);
                setStep(0);
                setError('');
                clearCredentials();
                setForm((previous) => ({ ...previous, port: key === 'pop3' ? '995' : '993' }));
              }}
            />
          ))}
        </fieldset>
      ),
    },
    {
      id: 'configure',
      title: oauth ? commonT('linkedServices.services') : t('externalMail.account'),
      description: oauth ? commonT('linkedServices.choose') : t('externalMail.tlsBody'),
      canContinue: available,
      onNext: () => {
        if (oauth) {
          void connect();
          return;
        }
        if (formRef.current?.reportValidity()) setStep(2);
      },
      nextLabel: oauth ? t('externalMail.authorize') : t('externalMail.review'),
      content: (
        <div class="space-y-4">
          {!available ? (
            <InlineHelp tone="warning">{t('externalMail.unavailable')}</InlineHelp>
          ) : null}
          {oauth ? (
            <fieldset class="space-y-3">
              <legend class="sr-only">{commonT('linkedServices.services')}</legend>
              <p class="text-text-muted text-sm">{t('externalMail.oauthBody')}</p>
              {offeredServices.map((capability) => (
                <div key={capability.service}>
                  <Checkbox
                    label={commonT(`app.${capability.service}.description`)}
                    checked={services.includes(capability.service)}
                    disabled={busy || !capability.available || capability.service === 'mail'}
                    onChange={(event) => {
                      const checked = event.currentTarget.checked;
                      setServices((previous) =>
                        checked
                          ? previous.includes(capability.service)
                            ? previous
                            : [...previous, capability.service]
                          : previous.filter((service) => service !== capability.service),
                      );
                    }}
                  />
                  {!capability.available ? (
                    <p class="text-text-muted mt-1 text-sm">
                      {commonT(
                        capability.reason === 'workspace_admin_required'
                          ? 'linkedServices.keepAdmin'
                          : 'linkedServices.notSupported',
                      )}
                    </p>
                  ) : null}
                </div>
              ))}
            </fieldset>
          ) : (
            <form
              ref={formRef}
              onSubmit={(event) => {
                event.preventDefault();
                setStep(2);
              }}
              class="space-y-4"
              autoComplete="off"
            >
              <FieldGroup label={t('common.name')} htmlFor="external-mail-display_name">
                <Input
                  id="external-mail-display_name"
                  name="display_name"
                  value={form.name}
                  maxLength={200}
                  onInput={(event) => field('name', event.currentTarget.value)}
                />
              </FieldGroup>
              <FieldGroup label={t('common.address')} htmlFor="external-mail-email">
                <Input
                  id="external-mail-email"
                  name="email"
                  type="email"
                  autoComplete="username"
                  required
                  value={form.address}
                  onInput={(event) => field('address', event.currentTarget.value)}
                />
              </FieldGroup>
              <fieldset class="space-y-3">
                <legend class="text-text text-sm font-semibold">
                  {t('externalMail.incoming')}
                </legend>
                <FieldGroup label={t('externalMail.host')} htmlFor="external-mail-incoming_host">
                  <Input
                    id="external-mail-incoming_host"
                    name="incoming_host"
                    required
                    autoCapitalize="none"
                    spellcheck={false}
                    value={form.host}
                    onInput={(event) => field('host', event.currentTarget.value)}
                  />
                </FieldGroup>
                <details
                  class="space-y-3"
                  onInvalidCapture={(event) => {
                    event.currentTarget.open = true;
                  }}
                >
                  <summary class="text-text cursor-pointer text-sm font-medium">
                    {t('externalMail.advanced')}
                  </summary>
                  <FieldGroup label={t('externalMail.port')} htmlFor="external-mail-incoming_port">
                    <Input
                      id="external-mail-incoming_port"
                      name="incoming_port"
                      type="number"
                      min={1}
                      max={65535}
                      required
                      value={form.port}
                      onInput={(event) => field('port', event.currentTarget.value)}
                    />
                  </FieldGroup>
                  <FieldGroup
                    label={t('externalMail.username')}
                    htmlFor="external-mail-incoming_user"
                  >
                    <Input
                      id="external-mail-incoming_user"
                      name="incoming_user"
                      value={form.username}
                      autoCapitalize="none"
                      spellcheck={false}
                      onInput={(event) => field('username', event.currentTarget.value)}
                    />
                  </FieldGroup>
                </details>
                <FieldGroup
                  label={t('externalMail.password')}
                  htmlFor="external-mail-incoming_password"
                >
                  <Input
                    id="external-mail-incoming_password"
                    name="incoming_password"
                    type="password"
                    autoComplete="current-password"
                    required
                    value={form.password}
                    onInput={(event) => field('password', event.currentTarget.value)}
                  />
                </FieldGroup>
              </fieldset>
              <fieldset class="space-y-3">
                <legend class="text-text text-sm font-semibold">
                  {t('externalMail.outgoing')}
                </legend>
                <FieldGroup label={t('externalMail.host')} htmlFor="external-mail-smtp_host">
                  <Input
                    id="external-mail-smtp_host"
                    name="smtp_host"
                    required
                    autoCapitalize="none"
                    spellcheck={false}
                    value={form.smtpHost}
                    onInput={(event) => field('smtpHost', event.currentTarget.value)}
                  />
                </FieldGroup>
                <details
                  class="space-y-3"
                  onInvalidCapture={(event) => {
                    event.currentTarget.open = true;
                  }}
                >
                  <summary class="text-text cursor-pointer text-sm font-medium">
                    {t('externalMail.advanced')}
                  </summary>
                  <FieldGroup label={t('externalMail.port')} htmlFor="external-mail-smtp_port">
                    <Input
                      id="external-mail-smtp_port"
                      name="smtp_port"
                      type="number"
                      min={1}
                      max={65535}
                      required
                      value={form.smtpPort}
                      onInput={(event) => field('smtpPort', event.currentTarget.value)}
                    />
                  </FieldGroup>
                  <FieldGroup
                    label={t('externalMail.security')}
                    htmlFor="external-mail-smtp_security"
                  >
                    <Select
                      id="external-mail-smtp_security"
                      name="smtp_security"
                      value={form.smtpSecurity}
                      onChange={(event) => {
                        const value = event.currentTarget.value === 'starttls' ? 'starttls' : 'tls';
                        field('smtpSecurity', value);
                        field('smtpPort', value === 'tls' ? '465' : '587');
                      }}
                    >
                      <option value="tls">TLS</option>
                      <option value="starttls">STARTTLS</option>
                    </Select>
                  </FieldGroup>
                  <FieldGroup label={t('externalMail.username')} htmlFor="external-mail-smtp_user">
                    <Input
                      id="external-mail-smtp_user"
                      name="smtp_user"
                      value={form.smtpUser}
                      autoCapitalize="none"
                      spellcheck={false}
                      onInput={(event) => field('smtpUser', event.currentTarget.value)}
                    />
                  </FieldGroup>
                  <FieldGroup
                    label={t('externalMail.smtpPassword')}
                    htmlFor="external-mail-smtp_password"
                  >
                    <Input
                      id="external-mail-smtp_password"
                      name="smtp_password"
                      type="password"
                      autoComplete="current-password"
                      value={form.smtpPassword}
                      onInput={(event) => field('smtpPassword', event.currentTarget.value)}
                    />
                  </FieldGroup>
                </details>
              </fieldset>
              <button hidden aria-hidden="true" tabIndex={-1} type="submit">
                {t('externalMail.review')}
              </button>
            </form>
          )}
        </div>
      ),
    },
    ...(oauth
      ? []
      : [
          {
            id: 'review',
            title: t('externalMail.review'),
            description: t('externalMail.testBody'),
            nextLabel: t('externalMail.connect'),
            onNext: () => void connect(),
            content: (
              <dl class="text-text space-y-3 text-sm">
                <div>
                  <dt class="text-text-muted">{t('common.address')}</dt>
                  <dd>{form.address}</dd>
                </div>
                <div>
                  <dt class="text-text-muted">{t('externalMail.incoming')}</dt>
                  <dd>
                    {form.host}:{form.port} · TLS
                  </dd>
                </div>
                <div>
                  <dt class="text-text-muted">{t('externalMail.outgoing')}</dt>
                  <dd>
                    {form.smtpHost}:{form.smtpPort} · {form.smtpSecurity.toUpperCase()}
                  </dd>
                </div>
              </dl>
            ),
          },
        ]),
  ];
  if (selected)
    return (
      <ExternalMailbox
        account={selected}
        api={api}
        t={t}
        onBack={() => setSelected(null)}
        onClose={close}
        onReconnect={() => reconnect(selected)}
      />
    );
  return (
    <>
      <Modal
        open
        onClose={close}
        title={t('externalMail.title')}
        size="lg"
        dismissable={!busy}
        footer={
          connecting ? (
            <div ref={setFooter} class="flex w-full flex-wrap items-center justify-end gap-2" />
          ) : (
            <Button
              onClick={() => {
                setProvider('gmail');
                setServices(['mail']);
                setForm(newAccountForm());
                setReconnectInstance('');
                setConnecting(true);
                setStep(0);
                setError('');
              }}
            >
              {t('externalMail.add')}
            </Button>
          )
        }
      >
        {loading ? (
          <Spinner />
        ) : connecting ? (
          <Wizard
            current={step}
            onCurrentChange={setStep}
            busy={busy}
            footerTarget={footer}
            onCancel={() => {
              clearCredentials();
              setConnecting(false);
              setError('');
            }}
            steps={steps}
          />
        ) : (
          <div class="space-y-3">
            <p class="text-text-muted text-sm">{t('externalMail.body')}</p>
            {data.items.length === 0 ? (
              <p class="text-text-muted text-sm">{t('externalMail.empty')}</p>
            ) : null}
            {data.items.map((account) => (
              <div
                class="border-border flex flex-wrap items-center gap-2 rounded-lg border p-3"
                key={account.instance_id}
              >
                <div class="min-w-0 flex-1">
                  <p class="text-text font-medium">{account.name}</p>
                  <p class="text-text-muted text-sm break-all">
                    {account.address || MAIL_PROVIDERS[account.provider].name}
                  </p>
                  {account.connected ? (
                    <p class="text-text-muted mt-1 text-xs">
                      {commonT('linkedServices.connected')}:{' '}
                      {(account.services ?? ['mail'])
                        .map((service) => commonT(`app.${service}.description`))
                        .join(' · ')}
                    </p>
                  ) : null}
                </div>
                {account.connected && (account.services ?? ['mail']).includes('mail') ? (
                  <Button variant="secondary" onClick={() => setSelected(account)}>
                    {t('externalMail.open')}
                  </Button>
                ) : null}
                <Button
                  variant="ghost"
                  disabled={
                    !data.providers.some(
                      (item) => item.provider === account.provider && item.available,
                    )
                  }
                  onClick={() => reconnect(account)}
                >
                  {t('externalMail.connect')}
                </Button>
                <Button variant="ghost" onClick={() => setDisconnecting(account)}>
                  {t('externalMail.disconnect')}
                </Button>
              </div>
            ))}
          </div>
        )}
        {error ? (
          <div role="alert" class="mt-4">
            <InlineHelp tone="warning">{t(error)}</InlineHelp>
            <Button
              variant="ghost"
              onClick={() => {
                setError('');
                setAttempt((value) => value + 1);
              }}
            >
              {t('common.retry')}
            </Button>
          </div>
        ) : null}
      </Modal>
      <ConfirmDialog
        open={Boolean(disconnecting)}
        onClose={() => setDisconnecting(null)}
        title={t('externalMail.disconnect')}
        confirmLabel={t('externalMail.disconnect')}
        busy={busy}
        tone="danger"
        onConfirm={() => void disconnect()}
      >
        <p>{t('externalMail.disconnectBody')}</p>
      </ConfirmDialog>
    </>
  );
}

function ExternalMailbox({
  account,
  api,
  t,
  onBack,
  onClose,
  onReconnect,
}: {
  account: ExternalMailAccount;
  api: ReturnType<typeof externalMailClient>;
  t: T;
  onBack: () => void;
  onClose: () => void;
  onReconnect: () => void;
}): JSX.Element {
  const { locale } = useI18n();
  const [folders, setFolders] = useState<ExternalMailFolder[]>([]);
  const [folder, setFolder] = useState('');
  const [messages, setMessages] = useState<ExternalMailMessage[]>([]);
  const [message, setMessage] = useState<ExternalMailMessage | null>(null);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const [error, setError] = useState<ReturnType<typeof errorKey> | ''>('');
  const [draft, setDraft] = useState({ to: '', cc: '', bcc: '', subject: '', text: '' });
  const [attachments, setAttachments] = useState<ExternalMailAttachment[]>([]);
  const [compose, setCompose] = useState(false);
  const [sent, setSent] = useState(false);
  const [sentNoCopy, setSentNoCopy] = useState(false);
  const [folderReload, setFolderReload] = useState(0);
  const [leaving, setLeaving] = useState<(() => void) | null>(null);
  const epochRef = useRef(0);
  const alive = useRef(true);
  const fileInput = useRef<HTMLInputElement | null>(null);
  const [moreLoaded, setMoreLoaded] = useState(false);
  useEffect(
    () => () => {
      alive.current = false;
      epochRef.current++;
    },
    [],
  );
  useEffect(() => {
    const controller = new AbortController();
    void api
      .folders(account.instance_id, controller.signal, account.address)
      .then((result) => {
        if (controller.signal.aborted) return;
        setFolders(result.items);
        if (!result.items.length) setLoading(false);
        setFolder(
          result.items.find((item) => item.role === 'inbox')?.id ?? result.items[0]?.id ?? '',
        );
      })
      .catch((err) => {
        if (!controller.signal.aborted) {
          setError(errorKey(err));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [api, account.instance_id, account.address, folderReload]);
  useEffect(() => {
    if (!folder) return;
    const controller = new AbortController();
    epochRef.current += 1;
    const version = epochRef.current;
    setLoading(true);
    setError('');
    setMessage(null);
    setMoreLoaded(false);
    void api
      .messages(account.instance_id, folder, '', controller.signal, account.address)
      .then((page) => {
        if (version !== epochRef.current) return;
        setMessages(page.items);
        setCursor(page.next_cursor ?? '');
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(errorKey(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [api, account.instance_id, account.address, folder, reload]);
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (
        document.visibilityState === 'visible' &&
        !busy &&
        !loading &&
        !compose &&
        !message &&
        !moreLoaded
      )
        setReload((value) => value + 1);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [busy, loading, compose, message, moreLoaded]);
  function leave(action: () => void): void {
    if (busy) return;
    if (draft.text || draft.to || draft.cc || draft.bcc || draft.subject || attachments.length)
      setLeaving(() => action);
    else action();
  }
  async function loadMessage(item: ExternalMailMessage): Promise<void> {
    const version = epochRef.current;
    setBusy(true);
    setError('');
    try {
      const full = await api.message(
        account.instance_id,
        folder,
        item.id,
        undefined,
        account.address,
      );
      if (alive.current && version === epochRef.current) setMessage(full);
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function more(): Promise<void> {
    const version = epochRef.current;
    setBusy(true);
    setError('');
    try {
      const page = await api.messages(
        account.instance_id,
        folder,
        cursor,
        undefined,
        account.address,
      );
      if (alive.current && version === epochRef.current) {
        setMessages((previous) => [
          ...new Map([...previous, ...page.items].map((item) => [item.id, item])).values(),
        ]);
        setCursor(page.next_cursor ?? '');
        setMoreLoaded(true);
      }
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function send(event: Event): Promise<void> {
    event.preventDefault();
    setBusy(true);
    setError('');
    setSent(false);
    try {
      const result = await api.send(account.instance_id, {
        expected_address: account.address,
        to: parseAddresses(draft.to),
        cc: parseAddresses(draft.cc),
        bcc: parseAddresses(draft.bcc),
        subject: draft.subject,
        text: draft.text,
        attachments,
      });
      if (alive.current) {
        setDraft({ to: '', cc: '', bcc: '', subject: '', text: '' });
        setAttachments([]);
        setCompose(false);
        setSent(true);
        setSentNoCopy(result.sent_copy_saved === false);
        setReload((value) => value + 1);
      }
    } catch (err) {
      if (alive.current)
        setError(
          !(err instanceof HttpError) || err.status >= 500
            ? 'externalMail.sendUnknown'
            : errorKey(err),
        );
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function attach(files: FileList | null): Promise<void> {
    if (!files) return;
    let total = attachments.reduce((sum, item) => sum + (item.size ?? 0), 0);
    for (const file of Array.from(files)) {
      total += file.size;
      if (total > LIMIT) {
        setError('externalMail.tooLarge');
        return;
      }
    }
    setBusy(true);
    try {
      const incoming = await Promise.all(Array.from(files).map(fileToAttachment));
      if (alive.current)
        setAttachments((previous) => [
          ...previous,
          ...incoming.map((item) => ({
            name: item.name,
            type: item.type,
            size: item.size,
            content_base64: item.contentBase64,
          })),
        ]);
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  async function action(kind: 'read' | 'unread' | 'flag' | 'unflag' | 'trash'): Promise<void> {
    if (!message) return;
    setBusy(true);
    setError('');
    try {
      await api.action(account.instance_id, folder, message.id, kind, undefined, account.address);
      if (alive.current) {
        setMessage(null);
        setReload((value) => value + 1);
      }
    } catch (err) {
      if (alive.current) setError(errorKey(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  }
  function download(attachment: ExternalMailAttachment): void {
    if (!attachment.content_base64) return;
    const binary = atob(attachment.content_base64);
    const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
    const url = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }));
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = attachment.name;
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <>
      <Modal
        open
        onClose={() => leave(onClose)}
        title={account.address || account.name}
        size="xl"
        dismissable={!busy}
        footer={
          <>
            <Button variant="ghost" disabled={busy} onClick={() => leave(onBack)}>
              {t('common.back')}
            </Button>
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => {
                setCompose(!compose);
                setSent(false);
              }}
            >
              {t('nav.compose')}
            </Button>
            <Button
              variant="ghost"
              disabled={busy || loading}
              onClick={() => {
                if (!folders.length) setFolderReload((value) => value + 1);
                else setReload((value) => value + 1);
              }}
            >
              {t('nav.refresh')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted mb-4 text-sm">
          {t(account.provider === 'pop3' ? 'externalMail.pop3Note' : 'externalMail.refreshNote')}
        </p>
        {error ? (
          <div role="alert">
            <InlineHelp tone="warning">{t(error)}</InlineHelp>
            {error === 'externalMail.authError' ? (
              <Button variant="secondary" disabled={busy} onClick={() => leave(onReconnect)}>
                {t('externalMail.connect')}
              </Button>
            ) : null}
          </div>
        ) : null}
        {sent ? (
          <p role="status" class="text-text text-sm">
            {t(sentNoCopy ? 'externalMail.sentNoCopy' : 'externalMail.sent')}
          </p>
        ) : null}
        {compose ? (
          <form onSubmit={(event) => void send(event)} class="space-y-3">
            <p class="text-text-muted text-sm">{t('externalMail.draftNote')}</p>
            <FieldGroup label={t('compose.to')} htmlFor="external-mail-to">
              <Input
                id="external-mail-to"
                name="to"
                required
                value={draft.to}
                onInput={(event) => setDraft({ ...draft, to: event.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup label={t('compose.cc')} htmlFor="external-mail-cc">
              <Input
                id="external-mail-cc"
                name="cc"
                value={draft.cc}
                onInput={(event) => setDraft({ ...draft, cc: event.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup
              label={t('compose.bcc')}
              htmlFor="external-mail-bcc"
              help={t('compose.bccHelp')}
            >
              <Input
                id="external-mail-bcc"
                name="bcc"
                value={draft.bcc}
                onInput={(event) => setDraft({ ...draft, bcc: event.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup label={t('common.subject')} htmlFor="external-mail-subject">
              <Input
                id="external-mail-subject"
                name="subject"
                value={draft.subject}
                onInput={(event) => setDraft({ ...draft, subject: event.currentTarget.value })}
              />
            </FieldGroup>
            <FieldGroup label={t('compose.bodyLabel')} htmlFor="external-mail-body">
              <Textarea
                id="external-mail-body"
                name="body"
                required
                rows={8}
                value={draft.text}
                onInput={(event) => setDraft({ ...draft, text: event.currentTarget.value })}
              />
            </FieldGroup>
            <input
              ref={fileInput}
              type="file"
              multiple
              class="sr-only"
              onChange={(event) => {
                void attach(event.currentTarget.files);
                event.currentTarget.value = '';
              }}
            />
            <Button
              variant="secondary"
              type="button"
              disabled={busy}
              onClick={() => fileInput.current?.click()}
            >
              {t('externalMail.attach')}
            </Button>
            {attachments.map((item, index) => (
              <div key={index} class="flex items-center gap-2 text-sm">
                <span>
                  {item.name} · {formatBytes(locale, item.size ?? 0)}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={busy}
                  aria-label={t('compose.removeAttachment', { name: item.name })}
                  onClick={() =>
                    setAttachments((previous) => previous.filter((_, i) => i !== index))
                  }
                >
                  ×
                </Button>
              </div>
            ))}
            <Button type="submit" loading={busy} disabled={busy}>
              {t('compose.send')}
            </Button>
          </form>
        ) : message ? (
          <article class="space-y-4">
            <Button variant="ghost" disabled={busy} onClick={() => setMessage(null)}>
              {t('common.back')}
            </Button>
            <h2 class="text-text text-lg font-semibold">
              {message.subject || t('common.noSubject')}
            </h2>
            <p class="text-text-muted text-sm break-all">
              {message.from
                .map((item) => (item.name ? `${item.name} <${item.email}>` : item.email))
                .join(', ')}
            </p>
            <p class="text-text-muted text-sm">{fullDate(locale, message.date)}</p>
            <div class="text-text text-sm break-words whitespace-pre-wrap">
              {message.text || message.snippet || ''}
            </div>
            {message.attachments?.map((item, index) => (
              <Button
                key={item.id ?? index}
                variant="secondary"
                disabled={!item.content_base64}
                onClick={() => download(item)}
              >
                {item.name}
              </Button>
            ))}
            <div class="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => {
                  leave(() => {
                    setDraft({
                      to: message.from[0]?.email ?? '',
                      cc: '',
                      bcc: '',
                      subject: /^re:/i.test(message.subject)
                        ? message.subject
                        : `Re: ${message.subject}`,
                      text: '',
                    });
                    setCompose(true);
                  });
                }}
              >
                {t('compose.replyTitle')}
              </Button>
              {account.capabilities.actions ? (
                <>
                  <Button
                    variant="ghost"
                    disabled={busy}
                    onClick={() => void action(message.seen ? 'unread' : 'read')}
                  >
                    {t(message.seen ? 'externalMail.markUnread' : 'externalMail.markRead')}
                  </Button>
                  <Button
                    variant="ghost"
                    disabled={busy}
                    aria-label={t(message.flagged ? 'externalMail.unflag' : 'externalMail.flag')}
                    onClick={() => void action(message.flagged ? 'unflag' : 'flag')}
                  >
                    {message.flagged ? '★' : '☆'}
                  </Button>
                  <Button variant="ghost" disabled={busy} onClick={() => void action('trash')}>
                    {t('externalMail.trash')}
                  </Button>
                </>
              ) : null}
            </div>
          </article>
        ) : (
          <div class="grid gap-4 sm:grid-cols-[180px_1fr]">
            <nav aria-label={t('nav.folders')} class="space-y-1">
              {folders.map((item) => (
                <Button
                  key={item.id}
                  variant={folder === item.id ? 'secondary' : 'ghost'}
                  disabled={busy}
                  class="w-full justify-start"
                  onClick={() => setFolder(item.id)}
                >
                  {item.name}
                  {item.unread ? ` (${item.unread})` : ''}
                </Button>
              ))}
            </nav>
            <section>
              {loading ? (
                <Spinner />
              ) : messages.length === 0 ? (
                <p class="text-text-muted text-sm">{t('externalMail.noMessages')}</p>
              ) : (
                <ul class="divide-border divide-y">
                  {messages.map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        class="sytue-focus-ring-inset hover:bg-bg-subtle w-full space-y-1 rounded p-3 text-start"
                        disabled={busy}
                        onClick={() => void loadMessage(item)}
                      >
                        <p class={item.seen ? 'text-text' : 'text-text font-semibold'}>
                          {item.subject || t('common.noSubject')}
                        </p>
                        <p class="text-text-muted truncate text-sm">
                          {item.from.map((sender) => sender.name || sender.email).join(', ')}
                        </p>
                        <p class="text-text-muted truncate text-sm">{item.snippet}</p>
                        <p class="text-text-subtle text-xs">{shortDate(locale, item.date)}</p>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {cursor ? (
                <Button variant="ghost" disabled={busy} onClick={() => void more()}>
                  {t('common.loadMore')}
                </Button>
              ) : null}
            </section>
          </div>
        )}
      </Modal>
      <ConfirmDialog
        open={Boolean(leaving)}
        onClose={() => setLeaving(null)}
        title={t('externalMail.discardDraft')}
        confirmLabel={t('common.discard')}
        onConfirm={() => {
          const action = leaving;
          setLeaving(null);
          setDraft({ to: '', cc: '', bcc: '', subject: '', text: '' });
          setAttachments([]);
          action?.();
        }}
      >
        <p>{t('externalMail.draftNote')}</p>
      </ConfirmDialog>
    </>
  );
}
