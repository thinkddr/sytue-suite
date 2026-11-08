/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useI18n } from '@sytue/i18n/preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  HttpError,
  ICLOUD_MAIL,
  cuerpoDeError,
  MAIL_PROVIDERS,
  forgetMailConsent,
  isICloudMail,
  mailProviderName,
  pendingMailConsent,
  servesMail,
  type MailProvider,
  type ExternalMailAccount,
  type ExternalMailAccounts,
  type ExternalMailConfiguration,
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
  Wizard,
  type WizardStep,
} from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { externalMailClient } from '../client/externalMail.js';
import { errorKey, returnErrorKey, type ExternalMailErrorKey } from './externalMailErrors.js';

const EMPTY: ExternalMailAccounts = { items: [], providers: [] };
/** The wizard's choices: the providers, with iCloud as a preset of IMAP. */
type Choice = MailProvider | 'icloud';
const CHOICES: Choice[] = ['gmail', 'microsoft', 'yahoo', 'icloud', 'imap', 'pop3'];
function choiceName(choice: Choice): string {
  return choice === 'icloud' ? ICLOUD_MAIL.name : MAIL_PROVIDERS[choice].name;
}
/** iCloud's fixed servers, with the address as the user name of both. */
function icloudServers() {
  return {
    host: ICLOUD_MAIL.incoming.host,
    port: String(ICLOUD_MAIL.incoming.port),
    username: '',
    smtpHost: ICLOUD_MAIL.outgoing.host,
    smtpPort: String(ICLOUD_MAIL.outgoing.port),
    smtpUser: '',
    smtpSecurity: ICLOUD_MAIL.outgoing.security,
  };
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

export function ExternalMail({
  onClose,
  onAccounts,
  onOpenAccount,
  t,
  initialInstance,
}: {
  onClose: () => void;
  /**
   * Each list this dialog loads: back from the provider, and after connecting or
   * disconnecting an account. The sidebar shows the same accounts at once.
   */
  onAccounts?: (items: ExternalMailAccount[]) => void;
  /**
   * «Open mailbox», and an account that just connected: its inbox opens in the
   * main panel, with the list and the reader of Sytue's own folders. The dialog
   * used to carry a mailbox of its own; the parent closes it. `connected` is true
   * when the account has just connected (back from the provider, or saved by
   * hand), not when an existing one is opened: the parent says so in a toast.
   */
  onOpenAccount: (account: ExternalMailAccount, connected: boolean) => void;
  t: T;
  /**
   * The account to sign in again, from the sidebar («Sign in again») or from a
   * mailbox the provider no longer lets in: the dialog opens on its sign-in.
   */
  initialInstance?: string;
}): JSX.Element {
  const { t: commonT } = useI18n();
  const api = useMemo(externalMailClient, []);
  const [data, setData] = useState(EMPTY);
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const [provider, setProvider] = useState<MailProvider>('gmail');
  const [icloud, setICloud] = useState(false);
  const [step, setStep] = useState(0);
  const [reconnectInstance, setReconnectInstance] = useState('');
  const [services, setServices] = useState<LinkedService[]>(['mail']);
  const [connecting, setConnecting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ExternalMailErrorKey | ''>('');
  // A notice, not an error: nothing failed and there is nothing to retry.
  const [notice, setNotice] = useState<'externalMail.exists' | ''>('');
  const [disconnecting, setDisconnecting] = useState<ExternalMailAccount | null>(null);
  const [footer, setFooter] = useState<HTMLDivElement | null>(null);
  const [form, setForm] = useState(newAccountForm);
  const formRef = useRef<HTMLFormElement | null>(null);
  const listRef = useRef<HTMLDivElement | null>(null);
  const noticeRef = useRef<HTMLDivElement | null>(null);
  // After a 409 the wizard, and the focused «Connect» in it, is gone: the focus
  // goes to the account already there (or the notice) instead of <body>.
  const focusAfterList = useRef<string | null>(null);
  const [focusTarget, setFocusTarget] = useState<string | null>(null);
  const alive = useRef(true);
  // The latest callbacks, without reloading the list each time the parent paints.
  const onAccountsRef = useRef(onAccounts);
  onAccountsRef.current = onAccounts;
  const onOpenAccountRef = useRef(onOpenAccount);
  onOpenAccountRef.current = onOpenAccount;
  const returned = useRef(
    new URLSearchParams(typeof window === 'undefined' ? '' : window.location.search),
  );
  // The account to sign in again is taken once. A later reload (a 409 on «Add», a
  // disconnect) must go back to the list, not restart that sign-in.
  const appliedInitial = useRef<string | undefined>(undefined);
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
        // Accounts connected only for Calendar, Contacts or Notes have no mailbox here.
        const mailboxes = { ...result, items: result.items.filter(servesMail) };
        setData(mailboxes);
        onAccountsRef.current?.(mailboxes.items);
        // Only the list this reload paints: the one before it is about to go.
        setFocusTarget(focusAfterList.current);
        focusAfterList.current = null;
        const initial = initialInstance !== appliedInitial.current ? initialInstance : undefined;
        appliedInitial.current = initialInstance;
        const back = returned.current.get('instance');
        const id = back ?? initial;
        const account = mailboxes.items.find((item) => item.instance_id === id);
        const failure = returned.current.get('oauth_error');
        if (account && back === null && failure === null) {
          // «Sign in again»: straight to its sign-in, not to the list.
          reconnect(account);
        } else if (account) {
          setProvider(account.provider);
          setICloud(isICloudMail(account));
          setReconnectInstance(account.instance_id);
        } else if (failure !== null && failure !== 'exists') {
          // Drafts are not listed: the provider of a cancelled sign-in is the one
          // this tab sent there, or the wizard came back with Gmail chosen.
          const pending = pendingMailConsent(id);
          if (pending) {
            setProvider(pending.provider);
            setICloud(false);
            setServices(Array.from(new Set<LinkedService>(['mail', ...pending.services])));
          }
        }
        if (failure === 'exists') {
          // The address was already connected: show it in the list, not the wizard.
          setReconnectInstance('');
          setConnecting(false);
          setNotice('externalMail.exists');
        } else if (failure !== null) {
          setConnecting(true);
          setError(returnErrorKey(failure));
          setStep(1);
        } else if (account?.connected && back !== null) {
          // Back from the provider and connected: its inbox, in the main panel.
          setConnecting(false);
          onOpenAccountRef.current(account, true);
        }
        if (returned.current.has('connect-mail')) {
          const url = new URL(window.location.href);
          for (const key of ['connect-mail', 'instance', 'oauth_error'])
            url.searchParams.delete(key);
          window.history.replaceState(window.history.state, '', url);
          // The return is handled: a later one must not name this provider.
          if (returned.current.has('instance') || failure !== null) forgetMailConsent();
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
  }, [api, attempt, initialInstance]);
  useEffect(() => {
    if (focusTarget === null || loading || connecting) return;
    setFocusTarget(null);
    const row = [...(listRef.current?.querySelectorAll<HTMLElement>('[data-instance]') ?? [])].find(
      (element) => element.dataset.instance === focusTarget,
    );
    (row?.querySelector<HTMLElement>('button') ?? noticeRef.current)?.focus();
  }, [focusTarget, loading, connecting]);
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
  /** Opening a mailbox answers the «already connected» notice: it is not kept. */
  function openMailbox(account: ExternalMailAccount, connected = false): void {
    setNotice('');
    onOpenAccount(account, connected);
  }
  function reconnect(account: ExternalMailAccount): void {
    const cfg = account.configuration;
    const isICloud = isICloudMail(account);
    // iCloud's user name is only kept when it is not the address: otherwise it
    // follows the address, also if it is edited now.
    const icloudUser = (username: string | undefined) =>
      username && username.toLowerCase() !== account.address.toLowerCase() ? username : '';
    setProvider(account.provider);
    setICloud(isICloud);
    setReconnectInstance(account.instance_id);
    setServices(Array.from(new Set<LinkedService>(['mail', ...(account.services ?? [])])));
    setConnecting(true);
    setStep(1);
    setError('');
    setNotice('');
    setForm({
      name: account.name,
      address: account.address,
      host: cfg?.incoming.host ?? '',
      port: String(cfg?.incoming.port ?? (account.provider === 'pop3' ? 995 : 993)),
      username: isICloud ? icloudUser(cfg?.incoming.username) : (cfg?.incoming.username ?? ''),
      password: '',
      smtpHost: cfg?.outgoing.host ?? '',
      smtpPort: String(cfg?.outgoing.port ?? 465),
      smtpUser: isICloud ? '' : (cfg?.outgoing.username ?? ''),
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
          // iCloud signs both servers in with the same account.
          username: (icloud ? '' : form.smtpUser) || form.username || form.address,
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
      const mailboxes = updated.items.filter(servesMail);
      setData({ ...updated, items: mailboxes });
      onAccountsRef.current?.(mailboxes);
      setConnecting(false);
      const added = mailboxes.find((item) => item.instance_id === result.instance_id);
      if (added) openMailbox(added, true);
    } catch (err) {
      if (!alive.current) return;
      const key = errorKey(err, icloud);
      if (key === 'externalMail.exists') {
        const existing =
          err instanceof HttpError ? cuerpoDeError(err.body).params?.['instance_id'] : undefined;
        focusAfterList.current = typeof existing === 'string' ? existing : '';
        clearCredentials();
        setConnecting(false);
        setNotice(key);
        setAttempt((value) => value + 1);
      } else setError(key);
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
          {CHOICES.map((key) => (
            <ChoiceCard
              key={key}
              name="mail-provider"
              title={choiceName(key)}
              selected={icloud ? key === 'icloud' : provider === key}
              onSelect={() => {
                setProvider(key === 'icloud' ? 'imap' : key);
                setICloud(key === 'icloud');
                setReconnectInstance('');
                setServices(['mail']);
                setStep(0);
                setError('');
                clearCredentials();
                setForm((previous) => {
                  const { name, address } = previous;
                  if (key === 'icloud') return { ...previous, ...icloudServers() };
                  // Leaving iCloud does not keep its servers in a generic form.
                  const base = icloud ? { ...newAccountForm(), name, address } : previous;
                  return { ...base, port: key === 'pop3' ? '995' : '993' };
                });
              }}
            />
          ))}
        </fieldset>
      ),
    },
    {
      id: 'configure',
      title: oauth ? commonT('linkedServices.services') : t('externalMail.account'),
      description: oauth
        ? commonT('linkedServices.choose')
        : t(icloud ? 'externalMail.icloudBody' : 'externalMail.tlsBody'),
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
              {icloud ? (
                <>
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
                  {/* Where the app-specific password is made: Fyne opens the same page. */}
                  <a
                    href={ICLOUD_MAIL.accountPage}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="text-primary-text sytue-focus-ring inline-block rounded text-sm font-medium hover:underline"
                  >
                    {t('externalMail.icloudLink')}
                  </a>
                  {/* An iCloud+ custom domain or alias signs in with the main iCloud
                      address: empty, the address above is the user name. */}
                  <details class="space-y-3" open={form.username !== ''}>
                    <summary class="text-text cursor-pointer text-sm font-medium">
                      {t('externalMail.advanced')}
                    </summary>
                    <FieldGroup
                      label={t('externalMail.username')}
                      htmlFor="external-mail-incoming_user"
                    >
                      <Input
                        id="external-mail-incoming_user"
                        name="incoming_user"
                        type="email"
                        value={form.username}
                        autoCapitalize="none"
                        spellcheck={false}
                        placeholder={form.address}
                        onInput={(event) => field('username', event.currentTarget.value)}
                      />
                    </FieldGroup>
                  </details>
                </>
              ) : (
                <>
                  <fieldset class="space-y-3">
                    <legend class="text-text text-sm font-semibold">
                      {t('externalMail.incoming')}
                    </legend>
                    <FieldGroup
                      label={t('externalMail.host')}
                      htmlFor="external-mail-incoming_host"
                    >
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
                      <FieldGroup
                        label={t('externalMail.port')}
                        htmlFor="external-mail-incoming_port"
                      >
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
                            const value =
                              event.currentTarget.value === 'starttls' ? 'starttls' : 'tls';
                            field('smtpSecurity', value);
                            field('smtpPort', value === 'tls' ? '465' : '587');
                          }}
                        >
                          <option value="tls">TLS</option>
                          <option value="starttls">STARTTLS</option>
                        </Select>
                      </FieldGroup>
                      <FieldGroup
                        label={t('externalMail.username')}
                        htmlFor="external-mail-smtp_user"
                      >
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
                </>
              )}
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
                setICloud(false);
                setServices(['mail']);
                setForm(newAccountForm());
                setReconnectInstance('');
                setConnecting(true);
                setStep(0);
                setError('');
                setNotice('');
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
          <div ref={listRef} class="space-y-3">
            <p class="text-text-muted text-sm">{t('externalMail.body')}</p>
            {data.items.length === 0 ? (
              <p class="text-text-muted text-sm">{t('externalMail.empty')}</p>
            ) : null}
            {data.items.map((account) => (
              <div
                class="border-border flex flex-wrap items-center gap-2 rounded-lg border p-3"
                key={account.instance_id}
                data-instance={account.instance_id}
              >
                <div class="min-w-0 flex-1">
                  <p class="text-text font-medium">{account.name}</p>
                  <p class="text-text-muted text-sm break-all">
                    {[account.address, mailProviderName(account)].filter(Boolean).join(' · ')}
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
                {account.connected ? (
                  <Button variant="secondary" onClick={() => openMailbox(account)}>
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
        {notice && !connecting ? (
          <div ref={noticeRef} role="status" tabIndex={-1} class="mt-4">
            <InlineHelp tone="info">{t(notice)}</InlineHelp>
          </div>
        ) : null}
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
