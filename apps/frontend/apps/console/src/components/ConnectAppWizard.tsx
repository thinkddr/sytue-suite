/**
 * ConnectAppWizard — asistente «Conectar una aplicación» (Sytue Integration Gateway).
 *
 * Estrena el sistema de configuración guiada de @sytue/ui: elegir → qué va a pasar →
 * configurar → probar → resumen → listo. El admin no necesita leer documentación:
 * cada paso explica en lenguaje llano qué se activa, qué dato hace falta y dónde
 * conseguirlo, valida en vivo y resume antes de aplicar.
 *
 * No inventa API: usa los endpoints BFF existentes (crear instancia → secreto
 * write-only → test → activate). El borrador se crea al probar/activar y se borra
 * si el admin cancela o cambia los datos, para no dejar instancias huérfanas.
 *
 * Los textos vienen del catálogo (`wizard.app.*`): la `t` y el `locale` llegan de
 * AppsPanel, que es quien negocia el idioma. Las etiquetas de los CAMPOS las manda
 * el manifiesto del gateway, no el catálogo.
 */
import type { ComponentChildren } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  DataFlow,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { DataCategory, IconName, ReviewItem, WizardStep } from '@sytue/ui';
import { contiene, formatList, type Locale } from '@sytue/i18n';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { CatalogApp, CatalogField, ConnectedInstance } from '../lib/gateway.js';

interface ConnectAppWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Locale para las enumeraciones naturales (`formatList`). */
  locale: Locale;
  catalog: CatalogApp[];
  /** templateKey → nombre de la conexión ya existente (no se puede duplicar). */
  connected: Record<string, string>;
  /** Plantilla preseleccionada: el asistente arranca en «Qué va a pasar». */
  initialApp?: CatalogApp | undefined;
  onCancel: () => void;
  /** Instancia recién activada, para que la lista la incorpore. */
  onConnected: (inst: ConnectedInstance) => void;
  onDone: () => void;
}

/**
 * Plantillas que se conectan DESDE el producto que las usa, con el consentimiento de
 * cada persona: «Añadir almacenamiento» de Drive, las conexiones de Rooms y las cuentas
 * de Mail. En «Conectar una aplicación» solo eran ruido —Slack, Teams, Google Chat,
 * Dropbox y Google Drive salían dos veces— y crearlas aquí deja una instancia que el
 * producto no sabe usar. Siguen en el catálogo: la lista de conexiones las nombra.
 *
 * Los manifiestos no tienen un campo de propósito; si algún día lo tienen, esta lista
 * se sustituye por él.
 */
export const PLANTILLAS_DE_PRODUCTO: ReadonlySet<string> = new Set([
  // Sytue Drive › Añadir almacenamiento
  'google-drive',
  'onedrive',
  'dropbox-storage',
  // Sytue Rooms: puentes de canal, conversaciones, reuniones y WhatsApp
  'slack-rooms',
  'microsoft-teams-rooms',
  'google-chat',
  'slack-chat',
  'microsoft-teams-chat',
  'google-chat-conversations',
  'google-meet',
  'whatsapp-business',
  // Sytue Mail: cuentas personales
  'gmail-mail',
  'microsoft-mail',
  'yahoo-mail',
  'mail-imap',
  'mail-pop3',
]);

/** Lo que se ofrece en el paso «elegir»: el catálogo sin lo que se conecta desde un producto. */
export function conectablesDesdeAdmin(catalog: CatalogApp[]): CatalogApp[] {
  return catalog.filter((a) => !PLANTILLAS_DE_PRODUCTO.has(a.key));
}

/** Credenciales que devuelve `activate` y que el SaaS necesita registrar. */
interface ActivateResult {
  launchUrl?: string;
  oidcClientId?: string;
  oidcClientSecret?: string;
  samlMetadataUrl?: string;
}

type TestState = { kind: 'idle' | 'running' } | { kind: 'ok' } | { kind: 'fail'; fix: string };

/** Índices de los pasos (el orden del array que recibe el Wizard). */
const STEP = { choose: 0, explain: 1, configure: 2, test: 3, review: 4, done: 5 } as const;

// ---- Copy derivado del manifiesto (el backend no manda textos de guía) ----

/**
 * Qué aporta la integración, en una frase. Vive aquí y no en AppsPanel porque el
 * asistente es quien enseña el catálogo primero; el panel la reutiliza.
 */
export function capabilityLine(app: CatalogApp, t: T, locale: Locale): string {
  const parts: string[] = [];
  if (app.identity !== 'none') parts.push(t('apps.cap.sso'));
  if (app.provisioning === 'scim') parts.push(t('apps.cap.provisioning'));
  if (app.hasData) parts.push(t('apps.cap.data'));
  if (parts.length === 0) return t('apps.cap.none');
  return t('apps.cap.line', { what: formatList(locale, parts) });
}

/** Qué se activa exactamente, sin jerga, para el paso «Qué va a pasar». */
function effects(app: CatalogApp, t: T): { icon: IconName; title: string; body: string }[] {
  const list: { icon: IconName; title: string; body: string }[] = [
    {
      icon: 'globe',
      title: t('wizard.app.effect.portalTitle'),
      body: t('wizard.app.effect.portalBody', { app: app.name }),
    },
  ];
  if (app.identity !== 'none') {
    list.push({
      icon: 'lock',
      title: t('wizard.app.effect.ssoTitle'),
      body: t('wizard.app.effect.ssoBody', { app: app.name }),
    });
  }
  if (app.provisioning === 'scim') {
    list.push({
      icon: 'users',
      title: t('wizard.app.effect.scimTitle'),
      body: t('wizard.app.effect.scimBody', { app: app.name }),
    });
  }
  if (app.hasData) {
    list.push({
      icon: 'database',
      title: t('wizard.app.effect.dataTitle'),
      body: t('wizard.app.effect.dataBody', { app: app.name }),
    });
  }
  return list;
}

/**
 * Qué cruza al activar, sacado de las capacidades del manifiesto y no de un texto
 * aparte que pudiera contradecirlas: el inicio de sesión y SCIM llevan identidad y
 * usuarios HACIA la app; un driver de datos solo LEE de ella (consulta y búsqueda).
 */
export function dataFlow(app: CatalogApp): {
  incoming: DataCategory[];
  outgoing: DataCategory[];
} {
  const outgoing: DataCategory[] = [];
  if (app.identity !== 'none') outgoing.push('identity');
  if (app.provisioning === 'scim') outgoing.push('people');
  return { incoming: app.hasData ? ['records'] : [], outgoing };
}

/** Qué tiene que tener a mano antes de empezar, y dónde está. */
function checklist(app: CatalogApp, t: T): { label: string; where: string }[] {
  const items = app.fields.map((f) => ({
    label: f.label,
    where: f.help ?? t('wizard.app.need.fieldWhere', { app: app.name }),
  }));
  if (app.secret && !app.oauth) {
    items.push({
      label: app.secret.label,
      where: app.secret.help ?? t('wizard.app.need.secretWhere', { app: app.name }),
    });
  }
  return items;
}

/**
 * Validación en vivo de un campo del manifiesto. Hoy todos los campos del catálogo
 * son un nombre corto o un dominio (workspace, subdomain, host), así que una sola
 * regla los cubre y evita el 90% de los fallos reales (pegar la URL entera).
 * ponytail: sin tipo/patrón en el manifiesto; cuando lo haya, se lee de ahí.
 */
function fieldError(f: CatalogField, raw: string, t: T): string | null {
  const v = raw.trim();
  if (!v) return f.required ? t('wizard.app.err.required') : null;
  if (/\s/.test(v)) return t('wizard.app.err.noSpaces');
  if (/^https?:\/\//i.test(v)) return t('wizard.app.err.noScheme');
  if (v.includes('/')) return t('wizard.app.err.noPath');
  return null;
}

/** Traduce un fallo del test a algo accionable (nunca el error crudo). */
function testFix(status: number, app: CatalogApp, t: T): string {
  const field = app.fields[0]?.label ?? t('wizard.app.fix.fieldFallback');
  switch (status) {
    case 400:
    case 422:
      return t('wizard.app.fix.badData', { field });
    case 401:
    case 403:
      return app.secret
        ? t('wizard.app.fix.rejectedSecret', { app: app.name, secret: app.secret.label })
        : t('wizard.app.fix.rejected', { app: app.name });
    case 404:
      return t('wizard.app.fix.notFound', { app: app.name, field });
    case 502:
    case 503:
    case 504:
      return t('wizard.app.fix.down', { app: app.name });
    default:
      return t('wizard.app.fix.generic');
  }
}

// ---- Componente ----

export default function ConnectAppWizard({
  t,
  locale,
  catalog,
  connected,
  initialApp,
  onCancel,
  onConnected,
  onDone,
}: ConnectAppWizardProps) {
  const toast = useToast();
  const [step, setStep] = useState<number>(initialApp ? STEP.explain : STEP.choose);
  const [app, setApp] = useState<CatalogApp | null>(initialApp ?? null);
  const [query, setQuery] = useState('');
  const [name, setName] = useState(initialApp?.name ?? '');
  const [values, setValues] = useState<Record<string, string>>({});
  const [secret, setSecret] = useState('');
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [test, setTest] = useState<TestState>({ kind: 'idle' });
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ActivateResult | null>(null);
  /** Borrador creado en el gateway + huella de los datos con los que se creó. */
  const [draft, setDraft] = useState<{ id: string; key: string } | null>(null);
  const needsConfiguration = !app?.oauth || app.fields.length > 0;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const ofrecidas = conectablesDesdeAdmin(catalog);
    if (!q) return ofrecidas;
    return ofrecidas.filter((a) => contiene(`${a.name} ${a.category ?? ''} ${a.description}`, q));
  }, [catalog, query]);

  const errors = app ? app.fields.map((f) => fieldError(f, values[f.key] ?? '', t)) : [];
  const configValid = Boolean(name.trim()) && errors.every((e) => e === null);
  const dataKey = JSON.stringify({ name: name.trim(), values, secret: secret ? 1 : 0 });

  function pick(next: CatalogApp) {
    setApp(next);
    setName(next.name);
    setValues({});
    setSecret('');
    setTouched({});
    resetDraft();
  }

  /** Invalida el borrador: los datos han cambiado, hay que rehacerlo. */
  function resetDraft() {
    setTest({ kind: 'idle' });
    if (!draft) return;
    void fetch(`/api/admin/apps/instances/${draft.id}`, { method: 'DELETE' });
    setDraft(null);
  }

  /** Crea (o reutiliza) el borrador en el gateway con los datos actuales. */
  async function ensureInstance(current: CatalogApp): Promise<string> {
    if (draft && draft.key === dataKey) return draft.id;
    if (draft) void fetch(`/api/admin/apps/instances/${draft.id}`, { method: 'DELETE' });
    const created = (await send('/api/admin/apps/instances', 'POST', {
      template_key: current.key,
      name: name.trim(),
      config: Object.fromEntries(current.fields.map((f) => [f.key, (values[f.key] ?? '').trim()])),
    })) as { id: string };
    if (current.secret && secret.trim()) {
      await send(`/api/admin/apps/instances/${created.id}/secrets`, 'PUT', {
        secrets: { bearer: secret.trim() },
      });
    }
    setDraft({ id: created.id, key: dataKey });
    return created.id;
  }

  async function runTest() {
    if (!app || busy) return;
    setBusy(true);
    setTest({ kind: 'running' });
    try {
      const id = await ensureInstance(app);
      await send(`/api/admin/apps/instances/${id}/test`, 'POST', {});
      setTest({ kind: 'ok' });
    } catch (e) {
      setTest({ kind: 'fail', fix: testFix(errStatus(e), app, t) });
    } finally {
      setBusy(false);
    }
  }

  async function activate() {
    if (!app || busy) return;
    setBusy(true);
    try {
      const id = await ensureInstance(app);
      if (app.oauth) {
        const start = (await send(`/api/admin/apps/instances/${id}/oauth-start`, 'POST', {
          return_to: 'apps',
        })) as { authorize_url?: string };
        if (!start.authorize_url) throw new Error('missing_authorization_url');
        const url = new URL(start.authorize_url);
        if (url.protocol !== 'https:' || url.username || url.password)
          throw new Error('invalid_authorization_url');
        window.location.assign(url.href);
        return;
      }
      const out = (await send(`/api/admin/apps/instances/${id}/activate`, 'POST', {})) as {
        launch_url?: string;
        oidc_client_id?: string;
        oidc_client_secret?: string;
        saml_metadata_url?: string;
      };
      const res: ActivateResult = {};
      if (out.launch_url) res.launchUrl = out.launch_url;
      if (out.oidc_client_id) res.oidcClientId = out.oidc_client_id;
      if (out.oidc_client_secret) res.oidcClientSecret = out.oidc_client_secret;
      if (out.saml_metadata_url) res.samlMetadataUrl = out.saml_metadata_url;
      setResult(res);
      setDraft(null);
      onConnected({
        id,
        templateKey: app.key,
        name: name.trim(),
        status: 'active',
        hasSecrets: Boolean(app.secret && secret.trim()),
        ...(out.launch_url ? { launchUrl: out.launch_url } : {}),
      });
      setStep(STEP.done);
    } catch (e) {
      const status = errStatus(e);
      toast.notify({
        tone: 'danger',
        title: t('wizard.app.toast.activateFailed'),
        description: t(
          status === 400
            ? 'wizard.app.fix.activateBadData'
            : status === 502
              ? 'wizard.app.fix.activateIdpDown'
              : 'wizard.app.fix.activateGeneric',
        ),
      });
    } finally {
      setBusy(false);
    }
  }

  function cancel() {
    if (draft) void fetch(`/api/admin/apps/instances/${draft.id}`, { method: 'DELETE' });
    onCancel();
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      toast.notify({ tone: 'success', title: t('common.copied') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('sso.secret.copyFailed'),
        description: t('sso.secret.copyFailedHint'),
      });
    }
  }

  /** Título/encabezado que nombra la aplicación, con su versión sin ella. */
  const named = (withApp: ConsoleMessageKey, without: ConsoleMessageKey): string =>
    app ? t(withApp, { app: app.name }) : t(without);

  // ---- Pasos ----

  const stepChoose: WizardStep = {
    id: 'choose',
    title: t('wizard.app.step.choose'),
    heading: t('wizard.app.choose.heading'),
    description: t('wizard.app.choose.description'),
    canContinue: app !== null,
    content: (
      <div class="flex flex-col gap-4">
        <Input
          type="search"
          value={query}
          aria-label={t('wizard.app.choose.searchAria')}
          placeholder={t('wizard.app.choose.searchPlaceholder')}
          leadingIcon={<Icon name="search" size={16} />}
          class="max-w-sm"
          onInput={(e) => setQuery(e.currentTarget.value)}
        />
        {filtered.length === 0 ? (
          <p class="text-text-muted py-6 text-center text-sm">{t('wizard.app.choose.noResults')}</p>
        ) : (
          <ul class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {filtered.map((item) => {
              const already = connected[item.key];
              const selected = app?.key === item.key;
              return (
                <li key={item.key}>
                  <ChoiceCard
                    dense
                    class="h-full"
                    name="sytue-connect-app"
                    value={item.key}
                    title={item.name}
                    description={capabilityLine(item, t, locale)}
                    {...(item.category ? { footnote: item.category } : {})}
                    badge={
                      already ? (
                        <Badge tone="neutral">{t('wizard.app.choose.alreadyConnected')}</Badge>
                      ) : null
                    }
                    selected={selected}
                    disabled={Boolean(already)}
                    onSelect={() => pick(item)}
                    media={
                      <span
                        aria-hidden="true"
                        class="grid h-10 w-10 shrink-0 place-items-center rounded-lg"
                        style={{
                          backgroundColor: `color-mix(in srgb, ${item.color} 12%, transparent)`,
                        }}
                      >
                        {item.iconUrl ? (
                          <img src={item.iconUrl} alt="" width={22} height={22} />
                        ) : (
                          <Icon name="globe" size={20} />
                        )}
                      </span>
                    }
                  />
                </li>
              );
            })}
          </ul>
        )}
      </div>
    ),
  };

  const stepExplain: WizardStep = {
    id: 'explain',
    title: t('wizard.app.step.explain'),
    heading: named('wizard.app.explain.heading', 'wizard.app.explain.headingFallback'),
    description: t('wizard.app.explain.description'),
    content: app ? (
      <div class="flex flex-col gap-4">
        <DataFlow partner={app.name} {...dataFlow(app)} />
        <ul class="grid gap-3 sm:grid-cols-2">
          {effects(app, t).map((e) => (
            <li
              key={e.title}
              class="border-border bg-surface-raised flex gap-3 rounded-lg border p-3"
            >
              <Icon name={e.icon} size={18} class="text-text-muted mt-0.5 shrink-0" />
              <div class="min-w-0">
                <p class="text-text text-sm font-semibold">{e.title}</p>
                <p class="text-text-muted mt-0.5 text-sm leading-snug">{e.body}</p>
              </div>
            </li>
          ))}
        </ul>
        {checklist(app, t).length ? (
          <InlineHelp tone="info" title={t('wizard.app.explain.needTitle')}>
            <ul class="mt-1 flex flex-col gap-2">
              {checklist(app, t).map((c) => (
                <li key={c.label}>
                  <span class="text-text font-medium">{c.label}</span>
                  <span class="text-text-muted"> — {c.where}</span>
                </li>
              ))}
            </ul>
          </InlineHelp>
        ) : null}
      </div>
    ) : null,
  };

  const stepConfigure: WizardStep = {
    id: 'configure',
    title: t('wizard.app.step.configure'),
    heading: named('wizard.app.configure.heading', 'wizard.app.configure.headingFallback'),
    description: t('wizard.app.configure.description'),
    nextLabel: t('common.next'),
    onNext: () => {
      if (!app) return;
      if (!configValid) {
        const all: Record<string, boolean> = { name: true };
        for (const f of app.fields) all[f.key] = true;
        setTouched(all);
        return;
      }
      setStep(app.oauth ? STEP.review : STEP.test);
    },
    content: app ? (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('wizard.app.configure.nameLabel')} htmlFor="cfg-name" required>
          <Input
            id="cfg-name"
            value={name}
            aria-describedby="cfg-name-hint"
            invalid={touched['name'] === true && !name.trim()}
            onInput={(e) => {
              setName(e.currentTarget.value);
              resetDraft();
            }}
            onBlur={() => setTouched((prev) => ({ ...prev, name: true }))}
          />
          <FieldHint
            id="cfg-name-hint"
            tone={touched['name'] === true && !name.trim() ? 'danger' : 'neutral'}
          >
            {t(
              touched['name'] === true && !name.trim()
                ? 'wizard.app.configure.nameError'
                : 'wizard.app.configure.nameHint',
            )}
          </FieldHint>
        </FieldGroup>

        {app.fields.map((f, i) => {
          const value = values[f.key] ?? '';
          const err = errors[i] ?? null;
          const show = touched[f.key] === true;
          const tone = show && err ? 'danger' : value.trim() && !err ? 'success' : 'neutral';
          return (
            <FieldGroup
              key={f.key}
              label={f.label}
              htmlFor={`cfg-${f.key}`}
              required={Boolean(f.required)}
            >
              <Input
                id={`cfg-${f.key}`}
                value={value}
                placeholder={f.placeholder ?? ''}
                autocomplete="off"
                spellcheck={false}
                aria-describedby={`cfg-${f.key}-hint`}
                invalid={show && Boolean(err)}
                onInput={(e) => {
                  const v = e.currentTarget.value;
                  setValues((s) => ({ ...s, [f.key]: v }));
                  resetDraft();
                }}
                onBlur={() => setTouched((prev) => ({ ...prev, [f.key]: true }))}
              />
              <FieldHint id={`cfg-${f.key}-hint`} tone={tone}>
                {show && err
                  ? err
                  : tone === 'success'
                    ? t('wizard.app.configure.formatOk')
                    : (f.help ?? '')}
              </FieldHint>
            </FieldGroup>
          );
        })}

        {/* CONSENTIMIENTO antes que token pegado. Si la aplicación lo ofrece, aquí no
            hay ningún campo que rellenar: se enseña QUÉ se va a autorizar y un botón.
            Pedir un token obliga a salir de Sytue, entender un panel ajeno, elegir
            permisos que no se comprenden y volver con una cadena copiada — y además
            un token estático no caduca, no se acota por scope y no se revoca sin
            regenerarlo a mano. */}
        {app.oauth ? (
          <div class="border-border bg-bg-subtle flex flex-col gap-3 rounded-md border p-4">
            <p class="text-text text-sm font-medium">
              {t('wizard.app.oauth.title', { app: app.name })}
            </p>
            <p class="text-text-muted text-sm">{t('wizard.app.oauth.body', { app: app.name })}</p>
            {app.oauth.scopes.length > 0 ? (
              <div>
                <p class="text-text-muted mb-1.5 text-xs font-medium">
                  {t('wizard.app.oauth.willAuthorize')}
                </p>
                <ul class="text-text-muted flex flex-col gap-1 text-xs">
                  {app.oauth.scopes
                    .filter((sc) => sc !== 'offline_access')
                    .map((sc) => (
                      <li key={sc} class="flex items-start gap-1.5">
                        <Icon name="check" size={13} class="text-text-subtle mt-0.5 shrink-0" />
                        <code class="font-mono">{sc}</code>
                      </li>
                    ))}
                </ul>
              </div>
            ) : null}
            <InlineHelp tone="neutral" hideIcon>
              <span class="flex items-start gap-2">
                <Icon name="lock" size={15} class="text-text-subtle mt-0.5 shrink-0" />
                <span>{t('wizard.app.oauth.revocable', { app: app.name })}</span>
              </span>
            </InlineHelp>
          </div>
        ) : null}

        {app.secret && !app.oauth ? (
          <FieldGroup label={app.secret.label} htmlFor="cfg-secret">
            <Input
              id="cfg-secret"
              type="password"
              value={secret}
              autocomplete="off"
              aria-describedby="cfg-secret-hint"
              onInput={(e) => {
                setSecret(e.currentTarget.value);
                resetDraft();
              }}
            />
            <FieldHint id="cfg-secret-hint">{app.secret.help ?? ''}</FieldHint>
            <InlineHelp tone="neutral" hideIcon class="mt-1">
              <span class="flex items-start gap-2">
                <Icon name="lock" size={15} class="text-text-subtle mt-0.5 shrink-0" />
                <span>{t('wizard.app.configure.secretStored')}</span>
              </span>
            </InlineHelp>
          </FieldGroup>
        ) : null}
      </div>
    ) : null,
  };

  const stepTest: WizardStep = {
    id: 'test',
    title: t('wizard.app.step.test'),
    heading: named('wizard.app.test.heading', 'wizard.app.test.headingFallback'),
    description: t('wizard.app.test.description'),
    content: app ? (
      <div class="flex max-w-xl flex-col gap-4">
        {test.kind === 'ok' ? (
          <InlineHelp tone="success" title={t('wizard.app.test.okTitle')}>
            {t('wizard.app.test.okBody', { app: app.name })}
          </InlineHelp>
        ) : test.kind === 'fail' ? (
          <InlineHelp
            tone="danger"
            title={t('wizard.app.test.failTitle')}
            action={
              <Button variant="secondary" size="sm" onClick={() => setStep(STEP.configure)}>
                {t('wizard.app.test.reviewData')}
              </Button>
            }
          >
            {test.fix}
          </InlineHelp>
        ) : (
          <InlineHelp tone="neutral">{t('wizard.app.test.idleHelp', { app: app.name })}</InlineHelp>
        )}
        <div class="flex items-center gap-3">
          <Button
            variant={test.kind === 'ok' ? 'secondary' : 'primary'}
            loading={test.kind === 'running'}
            onClick={() => void runTest()}
          >
            {t(test.kind === 'idle' ? 'wizard.app.test.run' : 'wizard.app.test.rerun')}
          </Button>
          {test.kind === 'idle' ? <p class="sytue-meta">{t('wizard.app.test.skipHint')}</p> : null}
        </div>
      </div>
    ) : null,
  };

  const summary: ReviewItem[] = app
    ? [
        { label: t('wizard.app.summary.app'), value: app.name, onEdit: () => setStep(STEP.choose) },
        {
          label: t('wizard.app.summary.name'),
          value: name.trim(),
          ...(needsConfiguration ? { onEdit: () => setStep(STEP.configure) } : {}),
        },
        ...app.fields.map((f) => ({
          label: f.label,
          value: (values[f.key] ?? '').trim() || '—',
          mono: true,
          onEdit: () => setStep(STEP.configure),
        })),
        ...(app.secret && !app.oauth
          ? [
              {
                label: app.secret.label,
                value: t(
                  secret.trim()
                    ? 'wizard.app.summary.secretSaved'
                    : 'wizard.app.summary.secretNone',
                ),
                onEdit: () => setStep(STEP.configure),
              },
            ]
          : []),
        {
          label: t('wizard.app.summary.willEnable'),
          value: effects(app, t)
            .map((e) => e.title)
            .join(' · '),
        },
        ...(!app.oauth
          ? [
              {
                label: t('wizard.app.summary.test'),
                value: t(
                  test.kind === 'ok'
                    ? 'wizard.app.summary.testOk'
                    : test.kind === 'fail'
                      ? 'wizard.app.summary.testFail'
                      : 'wizard.app.summary.testNone',
                ),
                onEdit: () => setStep(STEP.test),
              },
            ]
          : []),
      ]
    : [];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.app.step.review'),
    heading: t('wizard.app.review.heading'),
    description: t('wizard.app.review.description'),
    nextLabel: app?.oauth
      ? t('wizard.app.oauth.title', { app: app.name })
      : named('wizard.app.review.activate', 'wizard.app.review.activateFallback'),
    onNext: () => void activate(),
    content: app ? (
      <div class="flex flex-col gap-4">
        <ReviewSummary items={summary} />
        <InlineHelp tone="neutral">{t('wizard.app.review.note', { app: app.name })}</InlineHelp>
      </div>
    ) : null,
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('wizard.app.step.done'),
    heading: named('wizard.app.done.heading', 'wizard.app.done.headingFallback'),
    done: true,
    content:
      app && result ? (
        <div class="flex flex-col gap-4">
          <InlineHelp tone="success" title={t('wizard.app.done.okTitle')}>
            {t('wizard.app.done.okBody')}
          </InlineHelp>

          <section class="border-border bg-surface-raised rounded-lg border p-4">
            <h3 class="text-text text-sm font-semibold">{t('wizard.app.done.nextTitle')}</h3>
            <ol class="mt-3 flex flex-col gap-3">
              <NextStep n={1} title={t('wizard.app.done.step1Title')}>
                {t('wizard.app.done.step1Body', { app: app.name })}
              </NextStep>
              {app.identity !== 'none' ? (
                <NextStep n={2} title={t('wizard.app.done.step2Title')}>
                  {t(
                    result.launchUrl
                      ? 'wizard.app.done.step2BodyLaunch'
                      : 'wizard.app.done.step2Body',
                    { app: app.name },
                  )}
                </NextStep>
              ) : null}
              {app.provisioning === 'scim' ? (
                <NextStep n={3} title={t('wizard.app.done.step3Title')}>
                  {t('wizard.app.done.step3Body', { app: app.name })}
                </NextStep>
              ) : null}
            </ol>
          </section>

          {result.oidcClientSecret ? (
            <section class="border-warning bg-warning-bg rounded-lg border p-4">
              <h3 class="text-warning-text text-sm font-semibold">
                {t('wizard.app.done.oidcTitle')}
              </h3>
              <p class="text-text-muted mt-1 text-sm">
                {t('wizard.app.done.oidcBody', { app: app.name })}
              </p>
              <div class="mt-3 flex flex-col gap-2">
                <Secret
                  label="client_id"
                  value={result.oidcClientId ?? ''}
                  copyLabel={t('common.copy')}
                  onCopy={copy}
                />
                <Secret
                  label="client_secret"
                  value={result.oidcClientSecret}
                  copyLabel={t('common.copy')}
                  onCopy={copy}
                />
              </div>
            </section>
          ) : null}

          {result.samlMetadataUrl ? (
            <section class="border-border bg-surface-raised rounded-lg border p-4">
              <h3 class="text-text text-sm font-semibold">{t('wizard.app.done.samlTitle')}</h3>
              <p class="text-text-muted mt-1 text-sm">
                {t('wizard.app.done.samlBody', { app: app.name })}
              </p>
              <div class="mt-3">
                <Secret
                  label="metadata"
                  value={result.samlMetadataUrl}
                  copyLabel={t('common.copy')}
                  onCopy={copy}
                />
              </div>
            </section>
          ) : null}

          <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
            <Button variant="primary" onClick={onDone}>
              {t('wizard.app.done.back')}
            </Button>
            {result.launchUrl ? (
              <Button
                variant="secondary"
                leadingIcon={<Icon name="globe" size={16} />}
                onClick={() => {
                  if (result.launchUrl)
                    window.open(result.launchUrl, '_blank', 'noopener,noreferrer');
                }}
              >
                {t('wizard.app.done.open', { app: app.name })}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null,
  };

  // An OAuth account cannot be tested until the user has granted access.
  const order: number[] = app?.oauth
    ? [
        STEP.choose,
        STEP.explain,
        ...(needsConfiguration ? [STEP.configure] : []),
        STEP.review,
        STEP.done,
      ]
    : [STEP.choose, STEP.explain, STEP.configure, STEP.test, STEP.review, STEP.done];
  const allSteps = [stepChoose, stepExplain, stepConfigure, stepTest, stepReview, stepDone];
  return (
    <Wizard
      steps={order.map((index) => allSteps[index]!)}
      current={Math.max(0, order.indexOf(step))}
      onCurrentChange={(index) => setStep(order[index]!)}
      busy={busy}
      onCancel={cancel}
    />
  );
}

// ---- Piezas locales ----

function NextStep({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: ComponentChildren;
}) {
  return (
    <li class="flex gap-3">
      <span
        aria-hidden="true"
        class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold"
      >
        {n}
      </span>
      <div class="min-w-0">
        <p class="text-text text-sm font-medium">{title}</p>
        <p class="text-text-muted mt-0.5 text-sm leading-snug">{children}</p>
      </div>
    </li>
  );
}

function Secret({
  label,
  value,
  copyLabel,
  onCopy,
}: {
  label: string;
  value: string;
  /** Texto del botón; ya traducido por quien lo usa (`t('common.copy')`). */
  copyLabel: string;
  onCopy: (v: string) => void | Promise<void>;
}) {
  return (
    <div class="border-border bg-surface flex items-center gap-2 rounded-md border px-3 py-2">
      <span class="sytue-meta shrink-0 font-mono">{label}</span>
      <code class="text-text min-w-0 flex-1 truncate font-mono text-xs">{value}</code>
      <Button variant="ghost" size="sm" onClick={() => void onCopy(value)}>
        {copyLabel}
      </Button>
    </div>
  );
}

// ---- fetch (BFF, cookie httpOnly) ----

class HttpStatus extends Error {
  constructor(readonly status: number) {
    super(String(status));
  }
}

async function send(url: string, method: string, body: unknown): Promise<unknown> {
  const res = await fetch(url, {
    method,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new HttpStatus(res.status);
  return res.json().catch(() => ({}));
}

function errStatus(e: unknown): number {
  return e instanceof HttpStatus ? e.status : 0;
}
