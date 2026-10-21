/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { useI18n } from '@sytue/i18n/preact';
import { formatDate, formatDateTime } from '@sytue/i18n';
import {
  Button,
  Checkbox,
  ChoiceCard,
  FieldGroup,
  Icon,
  Input,
  Modal,
  Select,
  Spinner,
  Textarea,
  withBase,
} from '@sytue/ui';
import {
  ExternalMailClient,
  HttpClient,
  HttpError,
  LinkedServicesClient,
  MAIL_PROVIDERS,
  type ExternalMailAccount,
  type ExternalMailAccounts,
  type LinkedService,
  type MailProvider,
  type RemoteCollection,
  type RemoteContact,
  type RemoteContactPatch,
  type RemoteEvent,
  type RemoteEventPatch,
  type RemoteNote,
} from '@sytue/api-clients';

type Service = Exclude<LinkedService, 'mail'>;
type Item = RemoteEvent | RemoteContact | RemoteNote;
export interface LinkedServicesProps {
  service: Service;
  basePath: string;
  /** Field names belong to the host app's existing translated vocabulary. */
  labels: {
    create: string;
    title: string;
    text: string;
    start?: string;
    end?: string;
    allDay?: string;
    location?: string;
    organization?: string;
    email?: string;
    phone?: string;
    add?: string;
    previousMonth?: string;
    nextMonth?: string;
  };
}

const EMPTY: ExternalMailAccounts = { items: [], providers: [] };
const PROVIDERS: MailProvider[] = ['gmail', 'microsoft', 'yahoo'];
function itemTitle(item: Item): string {
  return 'summary' in item ? item.summary : 'name' in item ? item.name : item.title;
}
function dateInput(value: string, allDay: boolean): string {
  if (allDay) return value.slice(0, 10);
  const date = new Date(value);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
function dateISO(value: string, allDay: boolean): string {
  return allDay ? `${value}T00:00:00Z` : new Date(value).toISOString();
}

/** One live-account surface shared by Calendar, Contacts and Notepads. The
 * gateway owns the OAuth vault; this app's cookie authenticates its own return. */
export function LinkedServices({ service, basePath, labels }: LinkedServicesProps): JSX.Element {
  const { t, locale } = useI18n();
  const clients = useMemo(() => {
    const client = new HttpClient({
      baseUrl: new URL(
        withBase(basePath, '/api'),
        typeof window === 'undefined' ? 'https://sytue.invalid' : window.location.origin,
      ).href,
      fetchImpl: ((url, options) =>
        fetch(url, { ...options, credentials: 'include' })) as typeof fetch,
    });
    return { mail: new ExternalMailClient(client), linked: new LinkedServicesClient(client) };
  }, [basePath]);
  const [open, setOpen] = useState(
    () =>
      typeof window !== 'undefined' &&
      new URLSearchParams(window.location.search).has('connect-linked'),
  );
  const [data, setData] = useState(EMPTY);
  const [account, setAccount] = useState<ExternalMailAccount | null>(null);
  const [collections, setCollections] = useState<RemoteCollection[]>([]);
  const [collection, setCollection] = useState('');
  const [items, setItems] = useState<Item[]>([]);
  const [cursor, setCursor] = useState('');
  const [month, setMonth] = useState(() => {
    const now = new Date();
    return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
  });
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  const [provider, setProvider] = useState<MailProvider>('gmail');
  const [setup, setSetup] = useState(false);
  const [drafts, setDrafts] = useState<Partial<Record<MailProvider, string>>>({});
  const generation = useRef(0);
  const alive = useRef(true);
  const returned = useRef(
    typeof window === 'undefined'
      ? new URLSearchParams()
      : new URLSearchParams(window.location.search),
  );
  const [editor, setEditor] = useState<{ original: Item | null } | null>(null);
  const [form, setForm] = useState({
    title: '',
    text: '',
    location: '',
    start: '',
    end: '',
    allDay: false,
    organization: '',
    emails: '',
  });
  const [writeBlocked, setWriteBlocked] = useState(false);
  useEffect(
    () => () => {
      alive.current = false;
      generation.current++;
    },
    [],
  );
  function resetView(): void {
    generation.current++;
    setLoading(false);
    setEditor(null);
    setItems([]);
    setCursor('');
    setCollection('');
    setCollections([]);
    setError('');
    setWriteBlocked(false);
  }
  function close(): void {
    generation.current++;
    setOpen(false);
    setEditor(null);
    setError('');
  }
  function message(err: unknown): string {
    const code = err instanceof HttpError ? err.code : '';
    if (
      code === 'external_reconnect_required' ||
      code === 'mail_auth' ||
      code === 'external_auth_failed' ||
      code === 'linked_service_not_granted'
    )
      return t('linkedServices.notGranted');
    if (code === 'external_resource_changed') return t('linkedServices.changed');
    if (code === 'external_write_unknown') return t('linkedServices.writeUnknown');
    return t('error.connection');
  }
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setLoading(true);
    setError('');
    void clients.mail
      .accounts(controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setData(result);
        const query = returned.current;
        const id = query.get('instance');
        const exact = result.items.find((item) => item.instance_id === id);
        if (query.has('oauth_error')) {
          setAccount(exact ?? null);
          if (exact) {
            setProvider(exact.provider);
            setDrafts((previous) => ({ ...previous, [exact.provider]: exact.instance_id }));
          }
          setSetup(true);
          setError(t('linkedServices.notGranted'));
        } else if (exact?.connected && exact.services?.includes(service)) {
          resetView();
          setAccount(exact);
          setSetup(false);
        } else if (query.has('connect-linked')) {
          resetView();
          setAccount(exact ?? null);
          if (exact) {
            setProvider(exact.provider);
            setDrafts((previous) => ({ ...previous, [exact.provider]: exact.instance_id }));
          }
          setSetup(true);
          setError(t('linkedServices.notGranted'));
        } else if (!account) {
          const first = result.items.find(
            (item) => item.connected && item.services?.includes(service),
          );
          if (first) {
            setAccount(first);
            setSetup(false);
          } else setSetup(true);
        } else {
          const current = result.items.find((item) => item.instance_id === account.instance_id);
          resetView();
          setAccount(current ?? null);
          if (current) setProvider(current.provider);
          setSetup(!current?.connected || !current.services?.includes(service));
        }
        if (query.has('connect-linked')) {
          const url = new URL(window.location.href);
          for (const key of ['connect-linked', 'instance', 'oauth_error'])
            url.searchParams.delete(key);
          window.history.replaceState(window.history.state, '', url);
          returned.current = new URLSearchParams();
        }
      })
      .catch((err) => {
        if (!controller.signal.aborted) setError(message(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
    // Account changes use the inventory already loaded. Reload is an explicit read.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, clients, reload]);
  useEffect(() => {
    if (!open || setup || !account?.connected || !account.services?.includes(service)) return;
    const controller = new AbortController();
    const epoch = generation.current;
    setLoading(true);
    void clients.linked
      .collections(account, service, controller.signal)
      .then((page) => {
        if (controller.signal.aborted || generation.current !== epoch) return;
        setCollections(page.items);
        setCollection(page.items[0]?.id ?? '');
      })
      .catch((err) => {
        if (!controller.signal.aborted && generation.current === epoch) setError(message(err));
      })
      .finally(() => {
        if (!controller.signal.aborted && generation.current === epoch) setLoading(false);
      });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [account, clients, service, setup, open, reload]);
  useEffect(() => {
    if (!open || setup || !account || !collection) return;
    const controller = new AbortController();
    void loadItems('', controller.signal);
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [account, collection, clients, service, open, setup, reload, month]);
  async function loadItems(next: string, signal?: AbortSignal): Promise<void> {
    if (!account || !collection) return;
    const epoch = generation.current;
    setLoading(true);
    setError('');
    try {
      const from = month.toISOString();
      const to = new Date(
        Date.UTC(month.getUTCFullYear(), month.getUTCMonth() + 1, 1),
      ).toISOString();
      const page =
        service === 'calendar'
          ? await clients.linked.events(account, collection, from, to, next, signal)
          : service === 'contacts'
            ? await clients.linked.contacts(account, collection, next, signal)
            : await clients.linked.notes(account, collection, next, signal);
      if (signal?.aborted || generation.current !== epoch || !alive.current) return;
      setItems((previous) => (next ? [...previous, ...page.items] : page.items));
      setCursor(page.next_cursor ?? '');
    } catch (err) {
      if (!signal?.aborted && generation.current === epoch && alive.current) setError(message(err));
    } finally {
      if (!signal?.aborted && generation.current === epoch && alive.current) setLoading(false);
    }
  }
  function chooseAccount(id: string): void {
    resetView();
    const selected = data.items.find((item) => item.instance_id === id) ?? null;
    setAccount(selected);
    if (selected) setProvider(selected.provider);
    setSetup(!selected?.connected || !selected.services?.includes(service));
  }
  function shiftMonth(delta: number): void {
    generation.current++;
    setItems([]);
    setCursor('');
    setEditor(null);
    setError('');
    setMonth(
      (previous) =>
        new Date(Date.UTC(previous.getUTCFullYear(), previous.getUTCMonth() + delta, 1)),
    );
  }
  function addAccount(): void {
    resetView();
    setAccount(null);
    setProvider('gmail');
    setDrafts({});
    setSetup(true);
  }
  async function connect(): Promise<void> {
    if (busy) return;
    const epoch = generation.current;
    setBusy(true);
    setError('');
    const id = account?.provider === provider ? account.instance_id : drafts[provider];
    const services = consentServices;
    try {
      const result = await clients.mail.connect(provider, `external-${service}`, id, services);
      if (!alive.current || generation.current !== epoch) return;
      setDrafts((previous) => ({ ...previous, [provider]: result.instance_id }));
      window.location.assign(result.authorize_url);
    } catch (err) {
      if (alive.current && generation.current === epoch) {
        setError(message(err));
        setBusy(false);
      }
    }
  }
  async function edit(item: Item | null): Promise<void> {
    if (!account || !collection || busy) return;
    const epoch = generation.current;
    setBusy(true);
    setError('');
    try {
      const original = item
        ? service === 'calendar'
          ? await clients.linked.event(account, collection, item.id)
          : service === 'contacts'
            ? await clients.linked.contact(account, collection, item.id)
            : await clients.linked.note(account, collection, item.id)
        : null;
      if (!alive.current || generation.current !== epoch) return;
      const event = original && 'summary' in original ? original : null;
      const contact = original && 'name' in original ? original : null;
      const note = original && 'title' in original ? original : null;
      const start = event?.start ?? new Date().toISOString();
      const end = event?.end ?? new Date(Date.now() + 3600000).toISOString();
      setForm({
        title: original ? itemTitle(original) : '',
        text: event?.description ?? contact?.note ?? '',
        location: event?.location ?? '',
        start: dateInput(start, event?.all_day ?? false),
        end: dateInput(end, event?.all_day ?? false),
        allDay: event?.all_day ?? false,
        organization: contact?.organization ?? '',
        emails: contact?.emails?.join(', ') ?? '',
      });
      // Existing OneNote text is displayed separately; the edit box appends only.
      if (note) setForm((previous) => ({ ...previous, text: '' }));
      setEditor({ original });
      setWriteBlocked(false);
    } catch (err) {
      if (alive.current && generation.current === epoch) setError(message(err));
    } finally {
      if (alive.current && generation.current === epoch) setBusy(false);
    }
  }
  async function save(event: JSX.TargetedSubmitEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (!account || !collection || !editor || busy || writeBlocked) return;
    const epoch = generation.current;
    const original = editor.original;
    setBusy(true);
    setError('');
    try {
      if (service === 'calendar') {
        const old = original && 'summary' in original ? original : null;
        const patch: RemoteEventPatch = {};
        if (!old || form.title !== old.summary) patch.summary = form.title;
        if (!old || form.text !== (old.description ?? '')) patch.description = form.text;
        if (!old || form.location !== (old.location ?? '')) patch.location = form.location;
        if (
          !old ||
          form.start !== dateInput(old.start, old.all_day) ||
          form.end !== dateInput(old.end, old.all_day) ||
          form.allDay !== old.all_day
        ) {
          patch.start = dateISO(form.start, form.allDay);
          patch.end = dateISO(form.end, form.allDay);
          patch.all_day = form.allDay;
          patch.time_zone = form.allDay
            ? 'UTC'
            : (old?.time_zone ?? Intl.DateTimeFormat().resolvedOptions().timeZone);
        }
        await clients.linked.saveEvent(account, collection, old?.id ?? '', old?.etag ?? '', patch);
      } else if (service === 'contacts') {
        const old = original && 'name' in original ? original : null;
        const patch: RemoteContactPatch = {};
        if (!old || form.title !== old.name) patch.name = form.title;
        if (!old || form.organization !== (old.organization ?? ''))
          patch.organization = form.organization;
        if (!old || form.text !== (old.note ?? '')) patch.note = form.text;
        if (!old || form.emails !== (old.emails?.join(', ') ?? ''))
          patch.emails = form.emails
            .split(',')
            .map((value) => value.trim())
            .filter(Boolean);
        await clients.linked.saveContact(
          account,
          collection,
          old?.id ?? '',
          old?.etag ?? '',
          patch,
        );
      } else if (original)
        await clients.linked.appendNote(account, collection, original.id, form.text);
      else await clients.linked.createNote(account, collection, form.title, form.text);
      if (!alive.current || generation.current !== epoch) return;
      setEditor(null);
      await loadItems('');
    } catch (err) {
      if (alive.current && generation.current === epoch) {
        setError(message(err));
        if (
          err instanceof HttpError &&
          ['external_write_unknown', 'external_resource_changed'].includes(err.code ?? '')
        )
          setWriteBlocked(true);
      }
    } finally {
      if (alive.current && generation.current === epoch) setBusy(false);
    }
  }
  const offered = data.providers.find((item) => item.provider === provider);
  const capability = offered?.services?.find((item) => item.service === service);
  const consentServices = Array.from(
    new Set<LinkedService>([
      ...(account?.provider === provider ? (account.services ?? []) : []),
      service,
    ]),
  ).filter((value) => ['mail', 'calendar', 'contacts', 'notes'].includes(value));
  const canConnect = offered?.available === true && capability?.available === true;
  const selectedCollection = collections.find((item) => item.id === collection);
  const original = editor?.original;
  const readOnly =
    !!editor &&
    (selectedCollection?.read_only === true ||
      (original?.read_only === true &&
        !(service === 'notes' && 'can_append' in original && original.can_append)));
  const accountOptions = data.items.filter((item) => PROVIDERS.includes(item.provider));
  return (
    <>
      <Button variant="ghost" size="sm" onClick={() => setOpen(true)}>
        {t('linkedServices.connected')}
      </Button>
      <Modal
        open={open}
        onClose={close}
        title={t('linkedServices.connected')}
        size="lg"
        dismissable={!busy}
      >
        {error ? (
          <p role="alert" class="text-danger mb-3 text-sm">
            {error}
          </p>
        ) : null}
        {editor ? (
          <>
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => {
                generation.current++;
                setEditor(null);
                setError('');
              }}
            >
              {t('ui.backToList')}
            </Button>
            <p class="text-text-muted my-2 text-xs break-words">
              {account?.address} · {selectedCollection?.name}
            </p>
            <p class="text-text-muted mb-4 text-sm">
              {readOnly ? t('sharedWithMe.readOnly') : t('linkedServices.providerData')}
            </p>
            <form class="space-y-4" onSubmit={(event) => void save(event)}>
              <FieldGroup htmlFor="linked-title" label={labels.title}>
                <Input
                  id="linked-title"
                  required
                  value={form.title}
                  disabled={readOnly || busy || (service === 'notes' && !!original)}
                  onInput={(e) => {
                    const value = e.currentTarget.value;
                    setForm((previous) => ({ ...previous, title: value }));
                  }}
                />
              </FieldGroup>
              {service === 'calendar' ? (
                <>
                  <Checkbox
                    label={labels.allDay ?? ''}
                    checked={form.allDay}
                    disabled={readOnly || busy}
                    onChange={(e) => {
                      const allDay = e.currentTarget.checked;
                      setForm((previous) => ({
                        ...previous,
                        allDay,
                        start: allDay ? previous.start.slice(0, 10) : `${previous.start}T09:00`,
                        end: allDay ? previous.end.slice(0, 10) : `${previous.end}T10:00`,
                      }));
                    }}
                  />
                  <div class="grid gap-4 sm:grid-cols-2">
                    <FieldGroup htmlFor="linked-start" label={labels.start ?? ''}>
                      <Input
                        id="linked-start"
                        required
                        type={form.allDay ? 'date' : 'datetime-local'}
                        value={form.start}
                        disabled={readOnly || busy}
                        onInput={(e) => {
                          const value = e.currentTarget.value;
                          setForm((previous) => ({ ...previous, start: value }));
                        }}
                      />
                    </FieldGroup>
                    <FieldGroup htmlFor="linked-end" label={labels.end ?? ''}>
                      <Input
                        id="linked-end"
                        required
                        type={form.allDay ? 'date' : 'datetime-local'}
                        min={form.start}
                        value={form.end}
                        disabled={readOnly || busy}
                        onInput={(e) => {
                          const value = e.currentTarget.value;
                          setForm((previous) => ({ ...previous, end: value }));
                        }}
                      />
                    </FieldGroup>
                  </div>
                  <FieldGroup htmlFor="linked-location" label={labels.location ?? ''}>
                    <Input
                      id="linked-location"
                      value={form.location}
                      disabled={readOnly || busy}
                      onInput={(e) => {
                        const value = e.currentTarget.value;
                        setForm((previous) => ({ ...previous, location: value }));
                      }}
                    />
                  </FieldGroup>
                </>
              ) : null}
              {service === 'contacts' ? (
                <>
                  <FieldGroup htmlFor="linked-organization" label={labels.organization ?? ''}>
                    <Input
                      id="linked-organization"
                      value={form.organization}
                      disabled={readOnly || busy}
                      onInput={(e) => {
                        const value = e.currentTarget.value;
                        setForm((previous) => ({ ...previous, organization: value }));
                      }}
                    />
                  </FieldGroup>
                  <FieldGroup htmlFor="linked-email" label={labels.email ?? ''}>
                    <Input
                      id="linked-email"
                      type="email"
                      multiple
                      value={form.emails}
                      disabled={readOnly || busy}
                      onInput={(e) => {
                        const value = e.currentTarget.value;
                        setForm((previous) => ({ ...previous, emails: value }));
                      }}
                    />
                  </FieldGroup>
                  {original && 'phones' in original && original.phones?.length ? (
                    <dl>
                      <dt class="text-text-muted text-sm">{labels.phone}</dt>
                      <dd class="text-sm break-words whitespace-pre-wrap">
                        {original.phones.join('\n')}
                      </dd>
                    </dl>
                  ) : null}
                </>
              ) : null}
              {service === 'notes' && original && 'text' in original ? (
                <p class="text-sm break-words whitespace-pre-wrap">{original.text}</p>
              ) : null}
              <FieldGroup
                htmlFor="linked-text"
                label={service === 'notes' && original ? (labels.add ?? labels.text) : labels.text}
              >
                <Textarea
                  id="linked-text"
                  rows={6}
                  required={service === 'notes'}
                  value={form.text}
                  disabled={readOnly || busy}
                  onInput={(e) => {
                    const value = e.currentTarget.value;
                    setForm((previous) => ({ ...previous, text: value }));
                  }}
                />
              </FieldGroup>
              {!readOnly ? (
                <Button type="submit" loading={busy} disabled={writeBlocked}>
                  {service === 'notes' && original ? (labels.add ?? t('ui.save')) : t('ui.save')}
                </Button>
              ) : null}
            </form>
          </>
        ) : (
          <>
            <div class="mb-4 flex flex-wrap items-end gap-2">
              {accountOptions.length ? (
                <div class="min-w-0 flex-1">
                  <FieldGroup htmlFor="linked-account" label={t('account.menu')}>
                    <Select
                      id="linked-account"
                      value={account?.instance_id ?? ''}
                      disabled={busy}
                      onChange={(e) => chooseAccount(e.currentTarget.value)}
                    >
                      <option value="" disabled>
                        {t('account.menu')}
                      </option>
                      {accountOptions.map((item) => (
                        <option key={item.instance_id} value={item.instance_id}>
                          {item.address || item.name || MAIL_PROVIDERS[item.provider].name}
                        </option>
                      ))}
                    </Select>
                  </FieldGroup>
                </div>
              ) : null}
              <Button
                variant="secondary"
                disabled={busy}
                leadingIcon={<Icon name="plus" />}
                onClick={addAccount}
              >
                {t('account.signIn')}
              </Button>
              <Button
                variant="ghost"
                disabled={busy || loading}
                onClick={() => {
                  resetView();
                  setReload((value) => value + 1);
                }}
              >
                {t('action.retry')}
              </Button>
            </div>
            {setup || !account?.connected || !account.services?.includes(service) ? (
              <>
                <p class="text-text-muted mb-4 text-sm">{t('linkedServices.choose')}</p>
                <div class="grid gap-2 sm:grid-cols-3">
                  {PROVIDERS.map((value) => (
                    <ChoiceCard
                      name="linked-provider"
                      key={value}
                      disabled={busy}
                      selected={provider === value}
                      title={MAIL_PROVIDERS[value].name}
                      onSelect={() => {
                        generation.current++;
                        setProvider(value);
                        setError('');
                        setBusy(false);
                      }}
                    />
                  ))}
                </div>
                <div class="my-4 space-y-2">
                  {consentServices.map((value) => (
                    <Checkbox key={value} label={t(`app.${value}.description`)} checked disabled />
                  ))}
                </div>
                {!capability?.available ? (
                  <p class="text-text-muted mb-3 text-sm">
                    {t(
                      capability?.reason === 'workspace_admin_required'
                        ? 'linkedServices.keepAdmin'
                        : 'linkedServices.notSupported',
                    )}
                  </p>
                ) : null}
                {capability?.available && !offered?.available ? (
                  <p class="text-text-muted mb-3 text-sm">{t('linkedServices.unavailable')}</p>
                ) : null}
                <Button disabled={!canConnect} loading={busy} onClick={() => void connect()}>
                  {t('account.signIn')}
                </Button>
              </>
            ) : (
              <>
                <p class="text-text-muted mb-3 text-sm">{t('linkedServices.providerData')}</p>
                {collections.length ? (
                  <FieldGroup htmlFor="linked-collection" label={t(`app.${service}.description`)}>
                    <Select
                      id="linked-collection"
                      value={collection}
                      disabled={busy}
                      onChange={(e) => {
                        generation.current++;
                        setCollection(e.currentTarget.value);
                        setItems([]);
                        setCursor('');
                        setError('');
                      }}
                    >
                      {collections.map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.name}
                        </option>
                      ))}
                    </Select>
                  </FieldGroup>
                ) : null}
                {service === 'calendar' && collection ? (
                  <div class="my-3 flex items-center justify-between gap-2">
                    <Button
                      variant="ghost"
                      disabled={busy || loading}
                      aria-label={labels.previousMonth ?? t('ui.wizard.back')}
                      onClick={() => shiftMonth(-1)}
                    >
                      <Icon name="chevronLeft" />
                    </Button>
                    <p class="text-sm font-medium" aria-live="polite">
                      {formatDate(locale, month, {
                        month: 'long',
                        year: 'numeric',
                        timeZone: 'UTC',
                      })}
                    </p>
                    <Button
                      variant="ghost"
                      disabled={busy || loading}
                      aria-label={labels.nextMonth ?? t('ui.wizard.next')}
                      onClick={() => shiftMonth(1)}
                    >
                      <Icon name="chevronRight" />
                    </Button>
                  </div>
                ) : null}
                {collection && !selectedCollection?.read_only ? (
                  <Button
                    class="my-3"
                    size="sm"
                    variant="secondary"
                    disabled={busy}
                    onClick={() => void edit(null)}
                  >
                    {labels.create}
                  </Button>
                ) : null}
                {loading ? (
                  <div class="py-4" role="status" aria-label={t('ui.loading')}>
                    <Spinner />
                  </div>
                ) : null}
                <ul class="divide-border divide-y">
                  {items.map((item) => (
                    <li key={item.id}>
                      <Button
                        variant="ghost"
                        fullWidth
                        class="justify-start text-left"
                        disabled={busy}
                        onClick={() => void edit(item)}
                      >
                        <span class="min-w-0">
                          <span class="block truncate">{itemTitle(item)}</span>
                          {'start' in item ? (
                            <span class="text-text-muted block text-xs">
                              {item.all_day
                                ? formatDate(locale, item.start, {
                                    dateStyle: 'medium',
                                    timeZone: 'UTC',
                                  })
                                : formatDateTime(locale, item.start)}
                            </span>
                          ) : null}
                        </span>
                      </Button>
                    </li>
                  ))}
                </ul>
                {!loading && !items.length ? (
                  <p class="text-text-muted py-4 text-sm">{t('ui.noResults')}</p>
                ) : null}
                {cursor ? (
                  <Button
                    size="sm"
                    disabled={busy || loading}
                    onClick={() => void loadItems(cursor)}
                  >
                    {t('notifications.loadMore')}
                  </Button>
                ) : null}
              </>
            )}
          </>
        )}
      </Modal>
    </>
  );
}
