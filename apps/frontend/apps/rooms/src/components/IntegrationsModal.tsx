/**
 * IntegrationsModal — «Apps e integraciones» de Sytue Rooms, en DOS pestañas:
 *
 *  - **Apps**: Slack, Microsoft Teams, Google Chat y WhatsApp, cada una con su
 *    asistente de consentimiento, y debajo UNA lista con todo lo conectado (puentes,
 *    también los antiguos de URL pegada a mano, y números de WhatsApp). Conectar lo
 *    hace un owner/admin de la ORGANIZACIÓN: es lo que exige `/api/app-connections`,
 *    y enseñar las tarjetas a un admin de equipo acababa en un 403 con cara de avería.
 *  - **Desarrolladores**: bots, comandos y webhooks entrantes y salientes, con un
 *    único formulario (selector de tipo) y una sola lista.
 *
 * Ya no hay alta manual de puentes: la vuelta desde Slack, Teams o Google Chat la
 * cubre un webhook entrante, que acepta sus formatos tal cual (`hookVariants`).
 * El token/URL en CLARO se muestra UNA vez con «copiar»; el backend no lo vuelve a dar.
 */
import type { JSX, ComponentChildren } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Modal,
  Input,
  Button,
  Badge,
  Spinner,
  Select,
  Tabs,
  FieldHint,
  DataFlow,
  useToast,
  cx,
} from '@sytue/ui';
import type { InputProps, TabItem } from '@sytue/ui';
import {
  listIntegrations,
  createIncomingWebhook,
  deleteIncomingWebhook,
  rotateIncomingWebhook,
  createOutgoingWebhook,
  deleteOutgoingWebhook,
  createBot,
  deleteBot,
  deleteBridge,
  disconnectWhatsApp,
  addBotToChannel,
  removeBotFromChannel,
  listCommands,
  createCommand,
  deleteCommand,
  type IntegrationsData,
  type Channel,
  type OutgoingDialect,
  type BridgePlatform,
  type ServerCommand,
} from '../client/chatApi.js';
import { ChatHttpError } from '../lib/chat.js';
import { hookVariants, type HookVariant } from '../lib/hookVariants.js';
import { copyToClipboard } from '../lib/clipboard.js';
import type { T } from '../i18n/index.js';
import { AppConnectionsWizard } from './AppConnectionsWizard.js';
import { WhatsAppConnectionsWizard } from './WhatsAppConnectionsWizard.js';

interface Props {
  open: boolean;
  onClose: () => void;
  /** Owner/admin de la organización: el único que puede conectar apps. */
  isOrgAdmin: boolean;
  /** Canales del usuario (para ligar webhooks entrantes / añadir bots). */
  channels: Channel[];
  /** Canal activo: preselección del selector. */
  activeChannelId?: string;
  /** Traductor ya atado al idioma. */
  t: T;
}

const EMPTY: IntegrationsData = {
  incoming: [],
  outgoing: [],
  bots: [],
  bridges: [],
  whatsapp: [],
  teams: [],
};

/** Las apps que se conectan con consentimiento (marcas: no se traducen). */
const APPS = [
  { key: 'slack-rooms', name: 'Slack' },
  { key: 'microsoft-teams-rooms', name: 'Microsoft Teams' },
  { key: 'google-chat', name: 'Google Chat' },
  { key: 'whatsapp-business', name: 'WhatsApp Business' },
] as const;

/** Nombre de cada plataforma de puente (marcas: no se traducen). */
const PLATAFORMAS: Record<BridgePlatform, string> = {
  slack: 'Slack',
  teams: 'Microsoft Teams',
  googlechat: 'Google Chat',
};

/** Lo que se crea desde «Desarrolladores»: rótulo, ayuda y qué datos entran o salen. */
type Kind = 'bot' | 'command' | 'incoming' | 'outgoing';
const KINDS: Record<
  Kind,
  {
    label: Parameters<T>[0];
    help: Parameters<T>[0];
    flow: { incoming?: ['messages']; outgoing?: ['messages'] };
  }
> = {
  // El token de un bot solo publica: no lee nada del canal.
  bot: {
    label: 'integrations.bots',
    help: 'integrations.botsHelp',
    flow: { incoming: ['messages'] },
  },
  // La orden sale hacia la URL y su respuesta vuelve al canal.
  command: {
    label: 'integrations.commands',
    help: 'integrations.commandsHelp',
    flow: { incoming: ['messages'], outgoing: ['messages'] },
  },
  incoming: {
    label: 'integrations.incoming',
    help: 'integrations.incomingHelp',
    flow: { incoming: ['messages'] },
  },
  outgoing: {
    label: 'integrations.outgoing',
    help: 'integrations.outgoingHelp',
    flow: { outgoing: ['messages'] },
  },
};

const EMPTY_FORM = { name: '', url: '', words: '', desc: '', dialect: 'sytue' as OutgoingDialect };

export function IntegrationsModal({
  open,
  onClose,
  isOrgAdmin,
  channels,
  activeChannelId,
  t,
}: Props): JSX.Element | null {
  const toast = useToast();
  // Solo canales publicables (no DMs) para ligar integraciones.
  const targetable = channels.filter((c) => c.kind !== 'direct');
  const defaultChannel =
    (activeChannelId && targetable.some((c) => c.id === activeChannelId)
      ? activeChannelId
      : targetable[0]?.id) ?? '';

  const [data, setData] = useState<IntegrationsData>(EMPTY);
  const [loading, setLoading] = useState(false);
  // `forbidden` aparte de `load`: un 403 no es una avería sino un permiso que no se
  // tiene (p. ej. le han retirado el rol con el modal abierto).
  const [error, setError] = useState<null | 'forbidden' | 'load'>(null);
  const [busy, setBusy] = useState(false);
  const [connecting, setConnecting] = useState<string | true | null>(null);
  const [wizardFooter, setWizardFooter] = useState<HTMLDivElement | null>(null);
  const [channelId, setChannelId] = useState(defaultChannel);
  // Secreto recién creado, mostrado una vez (título + valor a copiar).
  const [secret, setSecret] = useState<{
    title: string;
    value: string;
    /** Direcciones alternativas del MISMO token (entrante: Slack, Teams, correo…). */
    variants?: HookVariant[];
  } | null>(null);
  const [kind, setKind] = useState<Kind>('bot');
  const [form, setForm] = useState(EMPTY_FORM);
  // Slash commands de terceros: van por su propio endpoint —no por /api/integrations—
  // porque listarlos lo puede hacer cualquiera (el autocompletado los necesita).
  const [commands, setCommands] = useState<ServerCommand[]>([]);

  async function reloadCommands(): Promise<void> {
    setCommands(await listCommands().catch(() => []));
  }

  function reload(chId = channelId): void {
    setLoading(true);
    setError(null);
    listIntegrations(chId || undefined)
      .then(setData)
      .catch((err) =>
        setError(err instanceof ChatHttpError && err.status === 403 ? 'forbidden' : 'load'),
      )
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    if (!open) return;
    // Vuelta del consentimiento OAuth: el asistente recoge la cuenta recién autorizada.
    setConnecting(
      new URLSearchParams(window.location.search).get('connect-app') === '1' ? true : null,
    );
    setSecret(null);
    setChannelId(defaultChannel);
    reload(defaultChannel);
    void reloadCommands();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  function channelName(id?: string): string {
    return channels.find((c) => c.id === id)?.name ?? t('channel.word');
  }

  async function run(fn: () => Promise<unknown>, onOk?: () => void): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      await fn();
      onOk?.();
      reload();
    } catch {
      toast.notify({ tone: 'danger', title: t('integrations.actionFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function copyValue(value: string): Promise<void> {
    const ok = await copyToClipboard(value);
    toast.notify(
      ok
        ? { tone: 'success', title: t('message.copied') }
        : { tone: 'danger', title: t('message.copyFailed') },
    );
  }

  function revealIncoming(title: string, url: string, mailDomain?: string): void {
    const variants = hookVariants(url, mailDomain ?? '', channelName(channelId));
    setSecret({ title, value: url, ...(variants.length > 1 ? { variants } : {}) });
  }

  /** Crea lo elegido en el selector de tipo; cada tipo valida sus propios campos. */
  function create(): void {
    const name = form.name.trim();
    const url = form.url.trim();
    const done = (): void => setForm(EMPTY_FORM);
    if (kind === 'bot') {
      void run(async () => {
        const res = await createBot(name);
        setSecret({ title: t('integrations.botToken', { name: res.bot.name }), value: res.token });
      }, done);
    } else if (kind === 'command') {
      void run(async () => {
        await createCommand({
          name,
          requestUrl: url,
          ...(form.desc.trim() ? { description: form.desc.trim() } : {}),
        });
        await reloadCommands();
      }, done);
    } else if (kind === 'incoming') {
      void run(async () => {
        const res = await createIncomingWebhook(channelId, name);
        revealIncoming(t('integrations.incomingUrl'), res.url, res.channelMailDomain);
      }, done);
    } else {
      const words = parseWords(form.words);
      void run(
        () =>
          createOutgoingWebhook({
            callbackUrl: url,
            ...(channelId ? { channelId } : {}),
            ...(words.length ? { triggerWords: words } : {}),
            ...(form.dialect !== 'sytue' ? { dialect: form.dialect } : {}),
          }),
        done,
      );
    }
  }

  if (!open) return null;

  const canCreate =
    !busy &&
    (kind === 'bot'
      ? Boolean(form.name.trim())
      : kind === 'command'
        ? Boolean(form.name.trim() && form.url.trim())
        : kind === 'incoming'
          ? Boolean(channelId)
          : Boolean(form.url.trim()));
  const field = (
    k: 'name' | 'url' | 'words' | 'desc',
    label: Parameters<T>[0],
    props: InputProps = {},
    hint?: string,
  ): JSX.Element => (
    <Field id={`int-${k}`} label={t(label)} hint={hint}>
      <Input
        id={`int-${k}`}
        value={form[k]}
        onInput={(e) => setForm({ ...form, [k]: e.currentTarget.value })}
        {...props}
      />
    </Field>
  );
  const cmdName = form.name.trim().replace(/^\//, '');
  const teamName = (id: string): string => data.teams.find((x) => x.id === id)?.name ?? '';
  const disconnect = (onClick: () => void): JSX.Element => (
    <Button size="sm" variant="ghost" disabled={busy} onClick={onClick}>
      {t('integrations.bridgeDisconnect')}
    </Button>
  );
  const remove = (onClick: () => void): JSX.Element => (
    <Button size="sm" variant="ghost" disabled={busy} onClick={onClick}>
      {t('action.delete')}
    </Button>
  );
  const kindBadge = (k: Kind): JSX.Element => <Badge tone="neutral">{t(KINDS[k].label)}</Badge>;
  const devEmpty =
    !data.bots.length && !commands.length && !data.incoming.length && !data.outgoing.length;

  // El cuerpo de cada pestaña comparte el spinner y una altura mínima para no saltar.
  const tabBody = (node: JSX.Element): JSX.Element => (
    <div class="min-h-[12rem]">
      {loading ? (
        <div class="flex items-center justify-center py-8" aria-busy="true">
          <Spinner size={22} />
          <span class="sr-only">{t('state.loading')}</span>
        </div>
      ) : (
        node
      )}
    </div>
  );

  const tabItems: TabItem[] = [
    {
      id: 'apps',
      label: t('integrations.tab.apps'),
      content: tabBody(
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-xs">{t('integrations.connectApps.body')}</p>
          {isOrgAdmin ? (
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {APPS.map((app) => (
                <Button
                  key={app.key}
                  variant="secondary"
                  aria-label={t('appConnection.continue', { provider: app.name })}
                  onClick={() => setConnecting(app.key)}
                >
                  {app.name}
                </Button>
              ))}
            </div>
          ) : (
            <p class="text-text-muted text-sm">{t('integrations.connectApps.adminOnly')}</p>
          )}
          {data.bridges.length + data.whatsapp.length === 0 ? (
            <p class="text-text-subtle text-sm">{t('integrations.appsEmpty')}</p>
          ) : (
            <ul class="flex flex-col gap-1.5">
              {data.bridges.map((b) => (
                <Row key={b.id} actions={disconnect(() => void run(() => deleteBridge(b.id)))}>
                  <p class="text-text flex items-center gap-2 text-sm font-medium">
                    <span>{PLATAFORMAS[b.platform]}</span>
                    <Badge tone={b.lastError ? 'danger' : 'success'}>
                      {t(b.lastError ? 'integrations.bridgeFailing' : 'integrations.bridgeOk')}
                    </Badge>
                  </p>
                  <p class="sytue-meta truncate">
                    #{channelName(b.channelId)} → {b.target}
                  </p>
                  {b.lastError ? (
                    <p class="text-danger-text mt-0.5 text-xs">{b.lastError}</p>
                  ) : null}
                </Row>
              ))}
              {data.whatsapp.map((a) => (
                <Row
                  key={a.id}
                  actions={disconnect(() => void run(() => disconnectWhatsApp(a.id)))}
                >
                  <p class="text-text truncate text-sm font-medium">
                    WhatsApp · {a.displayPhone || a.phoneNumberId}
                    {a.verifiedName ? ` · ${a.verifiedName}` : ''}
                  </p>
                  <p class="sytue-meta truncate">{teamName(a.teamId)}</p>
                </Row>
              ))}
            </ul>
          )}
        </div>,
      ),
    },
    {
      id: 'developers',
      label: t('integrations.tab.developers'),
      content: tabBody(
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-xs">{t(KINDS[kind].help)}</p>
          <DataFlow compact {...KINDS[kind].flow} />
          <div class="flex flex-wrap items-end gap-2">
            <Field id="int-kind" label={t('integrations.kind')}>
              <Select
                id="int-kind"
                value={kind}
                onChange={(e) => {
                  setKind(e.currentTarget.value as Kind);
                  setForm(EMPTY_FORM);
                }}
              >
                {(Object.keys(KINDS) as Kind[]).map((k) => (
                  <option key={k} value={k}>
                    {t(KINDS[k].label)}
                  </option>
                ))}
              </Select>
            </Field>
            {kind === 'bot'
              ? field('name', 'integrations.botName', {
                  maxLength: 100,
                  placeholder: t('integrations.botNamePlaceholder'),
                })
              : null}
            {kind === 'incoming'
              ? field('name', 'createChannel.name', {
                  maxLength: 100,
                  placeholder: t('integrations.incomingNamePlaceholder'),
                })
              : null}
            {kind === 'command' ? (
              <>
                {/* Se enseña el resultado, no la regla: «así se escribirá». */}
                {field(
                  'name',
                  'integrations.commandName',
                  { placeholder: 'desplegar' },
                  cmdName
                    ? t('integrations.commandNameLive', { name: cmdName })
                    : t('integrations.commandNameHint'),
                )}
                {field(
                  'url',
                  'integrations.commandUrl',
                  { type: 'url', placeholder: 'https://ejemplo.test/comandos' },
                  t('integrations.commandUrlHint'),
                )}
                {field('desc', 'integrations.commandDescription', {
                  placeholder: t('integrations.commandDescriptionHint'),
                })}
              </>
            ) : null}
            {kind === 'outgoing' ? (
              <>
                {field('url', 'integrations.callbackUrl', {
                  type: 'url',
                  placeholder: 'https://example.com/hook',
                })}
                {field('words', 'integrations.triggerWords', { placeholder: 'deploy, status' })}
                {/* El servicio traduce a Slack y a Teams: un bot hecho con Bolt o
                    slack_sdk no entiende el dialecto nativo. */}
                <Field id="int-dialect" label={t('integrations.dialect')}>
                  <Select
                    id="int-dialect"
                    value={form.dialect}
                    onChange={(e) =>
                      setForm({ ...form, dialect: e.currentTarget.value as OutgoingDialect })
                    }
                  >
                    <option value="sytue">{t('integrations.dialect.sytue')}</option>
                    <option value="slack">Slack</option>
                    <option value="teams">Microsoft Teams</option>
                  </Select>
                </Field>
              </>
            ) : null}
            <Button variant="primary" size="sm" disabled={!canCreate} onClick={create}>
              {t('action.create')}
            </Button>
          </div>

          {devEmpty ? (
            <p class="text-text-subtle text-sm">{t('integrations.developersEmpty')}</p>
          ) : (
            <ul class="flex flex-col gap-1.5">
              {data.bots.map((b) => (
                <Row
                  key={b.id}
                  actions={
                    <>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !channelId}
                        onClick={() => void run(() => addBotToChannel(b.id, channelId))}
                      >
                        {t('integrations.addToChannel', { channel: channelName(channelId) })}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !channelId}
                        onClick={() => void run(() => removeBotFromChannel(b.id, channelId))}
                      >
                        {t('action.remove')}
                      </Button>
                      {remove(() => void run(() => deleteBot(b.id)))}
                    </>
                  }
                >
                  <p class="text-text flex items-center gap-1.5 truncate text-sm font-medium">
                    {b.name}
                    {kindBadge('bot')}
                  </p>
                </Row>
              ))}
              {/* El secreto de firma se ve en la fila y no «una sola vez»: hay que
                  poder recuperarlo al configurar el otro lado, y nunca viaja en las
                  llamadas. Solo lo devuelve el servicio a un admin. */}
              {commands.map((c) => (
                <Row
                  key={c.id}
                  actions={remove(
                    () =>
                      void run(async () => {
                        await deleteCommand(c.id);
                        await reloadCommands();
                      }),
                  )}
                >
                  <p class="text-text flex items-center gap-1.5 truncate text-sm font-medium">
                    /{c.name}
                    {kindBadge('command')}
                  </p>
                  <p class="sytue-meta truncate">
                    {c.description ? `${c.description} · ` : ''}
                    {c.requestUrl}
                  </p>
                  {c.signingSecret ? (
                    <SigningSecret value={c.signingSecret} t={t} onCopy={copyValue} />
                  ) : null}
                </Row>
              ))}
              {data.incoming.map((w) => (
                <Row
                  key={w.id}
                  actions={
                    <>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy}
                        onClick={() =>
                          void run(async () => {
                            const res = await rotateIncomingWebhook(w.id);
                            revealIncoming(
                              t('integrations.incomingUrlNew'),
                              res.url,
                              res.channelMailDomain,
                            );
                          })
                        }
                      >
                        {t('action.rotate')}
                      </Button>
                      {remove(() => void run(() => deleteIncomingWebhook(w.id)))}
                    </>
                  }
                >
                  <p class="text-text flex items-center gap-1.5 truncate text-sm font-medium">
                    {w.name || 'Webhook'}
                    {kindBadge('incoming')}
                  </p>
                  <p class="sytue-meta truncate">#{channelName(w.channelId)}</p>
                </Row>
              ))}
              {data.outgoing.map((w) => (
                <Row key={w.id} actions={remove(() => void run(() => deleteOutgoingWebhook(w.id)))}>
                  <p class="text-text flex items-center gap-1.5 text-sm font-medium">
                    <span class="truncate">{w.callbackUrl}</span>
                    {kindBadge('outgoing')}
                    {/* Dos salientes a la misma URL con dialecto distinto no son lo
                        mismo: solo se etiqueta lo que no es nativo. */}
                    {w.dialect ? (
                      <Badge tone="neutral">{w.dialect === 'slack' ? 'Slack' : 'Teams'}</Badge>
                    ) : null}
                  </p>
                  <p class="sytue-meta truncate">
                    {w.channelId ? `#${channelName(w.channelId)}` : t('integrations.anyChannel')}
                    {w.triggerWords?.length ? ` · ${w.triggerWords.join(', ')}` : ''}
                  </p>
                  {w.signingSecret ? (
                    <SigningSecret value={w.signingSecret} t={t} onCopy={copyValue} />
                  ) : null}
                </Row>
              ))}
            </ul>
          )}
        </div>,
      ),
    },
  ];

  const closeAll = (): void => {
    setConnecting(null);
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={closeAll}
      title={t(connecting ? 'integrations.connectApps.title' : 'integrations.title')}
      size="lg"
      footer={connecting ? <div ref={setWizardFooter} class="w-full min-w-0" /> : undefined}
    >
      {connecting === 'whatsapp-business' ? (
        <WhatsAppConnectionsWizard
          teams={data.teams}
          footerTarget={wizardFooter}
          t={t}
          onClose={closeAll}
          onConnected={() => reload()}
        />
      ) : connecting ? (
        <AppConnectionsWizard
          initialProvider={typeof connecting === 'string' ? connecting : undefined}
          channels={targetable}
          activeChannelId={channelId}
          footerTarget={wizardFooter}
          t={t}
          onClose={closeAll}
          onConnected={(id) => {
            setChannelId(id);
            reload(id);
          }}
        />
      ) : (
        <div class="flex flex-col gap-4">
          {/* Secreto recién creado: se muestra una vez. */}
          {secret ? (
            <div class="border-warning bg-warning-bg rounded-md border p-3" role="alert">
              <p class="text-warning-text text-sm font-semibold">{secret.title}</p>
              <p class="text-text-muted mt-0.5 text-xs">{t('integrations.secretOnce')}</p>
              <div class="mt-2 flex items-center gap-2">
                <code class="border-border bg-surface text-text min-w-0 flex-1 truncate rounded border px-2 py-1 text-xs">
                  {secret.value}
                </code>
                <Button size="sm" variant="secondary" onClick={() => void copyValue(secret.value)}>
                  {t('action.copy')}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSecret(null)}>
                  {t('action.hide')}
                </Button>
              </div>

              {/* El MISMO token en las otras rutas. Los alias forzados existen para las
                herramientas que envían solo {"text":"…"}, que la autodetección no puede
                distinguir de un envío nativo; son también la vuelta desde Slack, Teams o
                Google Chat, ahora que el alta manual de puentes ya no existe. */}
              {secret.variants ? (
                <div class="border-warning/40 mt-3 border-t pt-2">
                  <p class="text-text-muted text-xs">{t('integrations.incomingCompat')}</p>
                  <ul class="mt-1.5 flex flex-col gap-1">
                    {secret.variants.slice(1).map((v) => (
                      <li key={v.label} class="flex items-center gap-2">
                        {/* El nombre del proveedor es marca: no se traduce. La única con
                          `labelKey` es la dirección de correo, que sí. */}
                        <span class="text-text-muted w-28 shrink-0 text-xs">
                          {v.labelKey ? t(v.labelKey as Parameters<T>[0]) : v.label}
                        </span>
                        <code class="border-border bg-surface text-text min-w-0 flex-1 truncate rounded border px-2 py-1 text-xs">
                          {v.url}
                        </code>
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={t('integrations.copyFor', { provider: v.label })}
                          onClick={() => void copyValue(v.url)}
                        >
                          {t('action.copy')}
                        </Button>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </div>
          ) : null}

          {error ? (
            <p
              class={cx('text-sm', error === 'forbidden' ? 'text-text-muted' : 'text-danger-text')}
              role="alert"
            >
              {t(error === 'forbidden' ? 'integrations.forbidden' : 'integrations.loadFailed')}
            </p>
          ) : null}

          {/* Un solo selector de canal para las dos pestañas: los puentes, los
              entrantes y el «añadir bot» son de un canal; los salientes, opcional. */}
          <Field id="int-channel" label={t('channel.word.capitalised')}>
            <Select
              id="int-channel"
              value={channelId}
              class="text-sm"
              onChange={(e) => {
                setChannelId(e.currentTarget.value);
                reload(e.currentTarget.value);
              }}
            >
              {targetable.length === 0 ? (
                <option value="">{t('integrations.noChannels')}</option>
              ) : null}
              {targetable.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
          </Field>

          <Tabs items={tabItems} />
        </div>
      )}
    </Modal>
  );
}

/** "a, b\nc" → ['a','b','c'] (separadores coma / salto de línea). */
function parseWords(raw: string): string[] {
  return raw
    .split(/[,\n]/)
    .map((w) => w.trim())
    .filter(Boolean);
}

/** Rótulo sobre su control, con pista opcional debajo. */
function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string | undefined;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <div class="flex min-w-[10rem] flex-1 flex-col gap-1 max-md:w-full">
      <label class="sytue-label font-medium" for={id}>
        {label}
      </label>
      {children}
      {hint ? <FieldHint>{hint}</FieldHint> : null}
    </div>
  );
}

/** Fila de una integración: lo suyo a la izquierda y las acciones a la derecha. */
function Row({
  children,
  actions,
}: {
  // ComponentChildren: una fila tiene partes condicionales (`… ? … : null`).
  children: ComponentChildren;
  actions: JSX.Element;
}): JSX.Element {
  return (
    // En el teléfono, el nombre entero arriba y las acciones debajo, a la derecha: con
    // tres acciones al lado, el nombre de un bot se quedaba en 53 px («Asist»).
    <li class="border-border bg-surface flex items-center justify-between gap-3 rounded-md border px-3 py-2 max-md:flex-col max-md:items-stretch max-md:gap-2">
      <div class="min-w-0">{children}</div>
      <div class="flex shrink-0 items-center gap-1.5 max-md:justify-end">{actions}</div>
    </li>
  );
}

/** Secreto de firma de un saliente o un comando, con su «copiar». */
function SigningSecret({
  value,
  onCopy,
  t,
}: {
  value: string;
  onCopy: (v: string) => Promise<void>;
  t: T;
}): JSX.Element {
  return (
    <div class="mt-1.5 flex items-center gap-2">
      <span class="text-text-muted shrink-0 text-xs" title={t('integrations.signingSecretWhat')}>
        {t('integrations.signingSecret')}
      </span>
      <code class="border-border bg-surface text-text min-w-0 flex-1 truncate rounded border px-2 py-0.5 text-xs">
        {value}
      </code>
      <Button
        size="sm"
        variant="ghost"
        aria-label={t('integrations.copySigningSecret')}
        onClick={() => void onCopy(value)}
      >
        {t('action.copy')}
      </Button>
    </div>
  );
}
