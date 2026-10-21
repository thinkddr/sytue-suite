/**
 * RegisterOidcAppWizard — asistente «Registrar una aplicación que entre con Sytue».
 *
 * El sentido CONTRARIO a ConnectIdpWizard: aquí Sytue ID es el proveedor y la
 * aplicación de terceros es la que inicia sesión contra él. Cuatro pasos: qué
 * aplicación es → a dónde debe volver el usuario (con validación en vivo y el fallo
 * típico explicado) → resumen → los datos que hay que pegar en la aplicación, con el
 * secreto visible UNA sola vez.
 *
 * No inventa API: POST /api/admin/apps/oidc-clients del BFF. El secreto solo existe en
 * la respuesta del alta; ni se guarda ni se vuelve a pedir (si se pierde, se rota).
 *
 * Los textos vienen del catálogo (`wizard.oidc.*`): la `t` llega ya atada al idioma
 * desde SsoPanel, que es quien lo negocia.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  ChoiceCard,
  DataFlow,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { DataCategory, ReviewItem, WizardStep } from '@sytue/ui';
import { CopyRow, NextStep } from './ConnectIdpWizard.js';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { OidcApp } from '../lib/types.js';

interface RegisterOidcAppWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Se llama con la aplicación creada (ya sin secreto para la lista). */
  onCreated: (app: OidcApp) => void;
  onCancel: () => void;
  onDone: () => void;
}

type AppKind = 'server' | 'browser';

const KINDS: {
  value: AppKind;
  title: ConsoleMessageKey;
  blurb: ConsoleMessageKey;
  examples: ConsoleMessageKey;
}[] = [
  {
    value: 'server',
    title: 'wizard.oidc.kind.serverTitle',
    blurb: 'wizard.oidc.kind.serverBlurb',
    examples: 'wizard.oidc.kind.serverExamples',
  },
  {
    value: 'browser',
    title: 'wizard.oidc.kind.browserTitle',
    blurb: 'wizard.oidc.kind.browserBlurb',
    examples: 'wizard.oidc.kind.browserExamples',
  },
];

/** Qué se activa al registrar la aplicación, en lenguaje llano. */
const EFFECTS: {
  icon: 'lock' | 'users' | 'globe';
  title: ConsoleMessageKey;
  body: ConsoleMessageKey;
}[] = [
  {
    icon: 'users',
    title: 'wizard.oidc.effect.signInTitle',
    body: 'wizard.oidc.effect.signInBody',
  },
  {
    icon: 'lock',
    title: 'wizard.oidc.effect.controlTitle',
    body: 'wizard.oidc.effect.controlBody',
  },
  {
    icon: 'globe',
    title: 'wizard.oidc.effect.minimalTitle',
    body: 'wizard.oidc.effect.minimalBody',
  },
];

// ---- Acceso a datos como una cuenta de Google ----

type GoogleLevel = 'none' | 'read' | 'write';

const G = 'https://www.googleapis.com/auth/';

/** Producto → scopes de Google de lectura y de lectura y escritura. */
const GOOGLE_DATA: {
  key: ConsoleMessageKey;
  category: DataCategory;
  read: string;
  write: string;
}[] = [
  {
    key: 'wizard.oidc.google.calendar',
    category: 'events',
    read: `${G}calendar.readonly`,
    write: `${G}calendar`,
  },
  {
    key: 'wizard.oidc.google.drive',
    category: 'files',
    read: `${G}drive.readonly`,
    write: `${G}drive`,
  },
  {
    key: 'wizard.oidc.google.contacts',
    category: 'contacts',
    read: `${G}contacts.readonly`,
    write: `${G}contacts`,
  },
];

const GOOGLE_DIRECTORY: { key: ConsoleMessageKey; scopes: string[] }[] = [
  { key: 'wizard.oidc.google.directoryUsers', scopes: [`${G}admin.directory.user.readonly`] },
  {
    key: 'wizard.oidc.google.directoryGroups',
    scopes: [`${G}admin.directory.group.readonly`, `${G}admin.directory.group.member.readonly`],
  },
];

// ---- Validación en vivo (los fallos típicos, no la sintaxis por la sintaxis) ----

function redirectError(raw: string, t: T): string | null {
  const v = raw.trim();
  if (!v) return null;
  if (/\s/.test(v)) return t('wizard.oidc.err.hasSpace', { url: v });
  if (v.includes('*')) return t('wizard.oidc.err.noWildcards');
  if (v.includes('#')) return t('wizard.oidc.err.noFragment');
  let u: URL;
  try {
    u = new URL(v);
  } catch {
    return t('wizard.oidc.err.notUrl', { url: v });
  }
  const local = ['localhost', '127.0.0.1', '::1'].includes(u.hostname);
  if (u.protocol === 'http:' && !local) return t('wizard.oidc.err.insecure');
  if (u.protocol !== 'https:' && u.protocol !== 'http:')
    return t('wizard.oidc.err.notWeb', { url: v });
  if (!local && !u.hostname.includes('.')) return t('wizard.oidc.err.noDomain', { url: v });
  return null;
}

function parseUrls(s: string): string[] {
  return s
    .split(/[\s,]+/)
    .map((u) => u.trim())
    .filter(Boolean);
}

function firstError(list: string[], t: T): string | null {
  for (const u of list) {
    const e = redirectError(u, t);
    if (e) return e;
  }
  return null;
}

/** Traduce un fallo del servicio a algo que el admin pueda hacer. */
function saveFix(status: number, t: T): string {
  switch (status) {
    case 400:
      return t('wizard.oidc.fix.badUrls');
    case 403:
      return t('wizard.oidc.fix.forbidden');
    case 502:
    case 503:
    case 504:
      return t('wizard.oidc.fix.idpDown');
    default:
      return t('wizard.oidc.fix.generic');
  }
}

export default function RegisterOidcAppWizard({
  t,
  onCreated,
  onCancel,
  onDone,
}: RegisterOidcAppWizardProps) {
  const toast = useToast();
  const [kind, setKind] = useState<AppKind>('server');
  const [name, setName] = useState('');
  const [redirects, setRedirects] = useState('');
  const [logoutUrls, setLogoutUrls] = useState('');
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<OidcApp | null>(null);
  const [google, setGoogle] = useState<Record<string, GoogleLevel>>({});
  const [directory, setDirectory] = useState<Record<string, boolean>>({});

  // Los scopes de siempre y, además, los de Google que se hayan marcado: Sytue ID solo
  // los concede si llegan en el alta, así que la app no puede pedirse más después.
  const googleScopes = [
    ...GOOGLE_DATA.flatMap((g) =>
      google[g.key] === 'read' ? [g.read] : google[g.key] === 'write' ? [g.write] : [],
    ),
    ...GOOGLE_DIRECTORY.flatMap((d) => (directory[d.key] ? d.scopes : [])),
  ];
  // Lo que la app podrá LEER de Sytue One sale hacia ella; lo que podrá escribir,
  // además, entra. El nombre y el correo salen siempre: es lo que da el inicio de sesión.
  const outgoing: DataCategory[] = [
    'identity',
    ...GOOGLE_DATA.filter((g) => google[g.key] === 'read' || google[g.key] === 'write').map(
      (g) => g.category,
    ),
    ...(GOOGLE_DIRECTORY.some((d) => directory[d.key]) ? (['directory'] as const) : []),
  ];
  const incoming = GOOGLE_DATA.filter((g) => google[g.key] === 'write').map((g) => g.category);

  const ids = ['app', 'urls', 'review', 'done'] as const;
  const [step, setStep] = useState(0);
  const go = (id: (typeof ids)[number]) => setStep(ids.indexOf(id));

  const redirectList = parseUrls(redirects);
  const logoutList = parseUrls(logoutUrls);
  const errName = name.trim() ? null : t('wizard.oidc.err.nameRequired');
  const errRedirects =
    redirectList.length === 0 ? t('wizard.oidc.err.redirectRequired') : firstError(redirectList, t);
  const errLogout = firstError(logoutList, t);

  async function finish() {
    if (busy) return;
    setBusy(true);
    try {
      const res = await fetch('/api/admin/apps/oidc-clients', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          name: name.trim(),
          redirect_uris: redirectList,
          post_logout_redirect_uris: logoutList,
          scopes: ['openid', 'profile', 'email', 'offline_access', ...googleScopes],
          public: kind === 'browser',
        }),
      });
      if (!res.ok) throw new HttpStatus(res.status);
      const app = (await res.json()) as OidcApp;
      setCreated(app);
      // La lista no debe quedarse con el secreto en memoria.
      const { secret: _secret, ...withoutSecret } = app;
      onCreated(withoutSecret);
      go('done');
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('wizard.oidc.toast.saveFailed'),
        description: saveFix(e instanceof HttpStatus ? e.status : 0, t),
      });
    } finally {
      setBusy(false);
    }
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

  // ---- Pasos ----

  const stepApp: WizardStep = {
    id: 'app',
    title: t('wizard.oidc.step.app'),
    heading: t('wizard.oidc.app.heading'),
    description: t('wizard.oidc.app.description'),
    onNext: () => {
      if (errName) {
        setTouched((prev) => ({ ...prev, name: true }));
        return;
      }
      go('urls');
    },
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        <FieldGroup label={t('wizard.oidc.app.nameLabel')} htmlFor="app-name" required>
          <Input
            id="app-name"
            value={name}
            placeholder={t('wizard.oidc.app.namePlaceholder')}
            aria-describedby="app-name-hint"
            invalid={touched.name === true && Boolean(errName)}
            onInput={(e) => setName(e.currentTarget.value)}
            onBlur={() => setTouched((prev) => ({ ...prev, name: true }))}
          />
          <FieldHint
            id="app-name-hint"
            tone={touched.name === true && errName ? 'danger' : 'neutral'}
          >
            {touched.name === true && errName ? errName : t('wizard.oidc.app.nameHint')}
          </FieldHint>
        </FieldGroup>

        <fieldset class="flex flex-col gap-3">
          <legend class="text-text text-sm font-semibold">
            {t('wizard.oidc.app.whereLegend')}
          </legend>
          <p class="sytue-meta">{t('wizard.oidc.app.whereHint')}</p>
          <ul class="mt-1 flex flex-col gap-3">
            {KINDS.map((k) => (
              <li key={k.value}>
                <ChoiceCard
                  name="sytue-app-kind"
                  value={k.value}
                  icon={k.value === 'server' ? 'settings' : 'globe'}
                  title={t(k.title)}
                  description={t(k.blurb)}
                  footnote={t(k.examples)}
                  badge={
                    k.value === 'server' ? (
                      <Badge tone="neutral">{t('wizard.oidc.kind.mostCommon')}</Badge>
                    ) : null
                  }
                  selected={kind === k.value}
                  onSelect={() => setKind(k.value)}
                />
              </li>
            ))}
          </ul>
        </fieldset>

        <ul class="grid gap-3 sm:grid-cols-3">
          {EFFECTS.map((e) => (
            <li
              key={e.title}
              class="border-border bg-surface-raised flex gap-3 rounded-lg border p-3"
            >
              <Icon name={e.icon} size={18} class="text-text-muted mt-0.5 shrink-0" />
              <div class="min-w-0">
                <p class="text-text text-sm font-semibold">{t(e.title)}</p>
                <p class="text-text-muted mt-0.5 text-sm leading-snug">{t(e.body)}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    ),
  };

  const stepUrls: WizardStep = {
    id: 'urls',
    title: t('wizard.oidc.step.urls'),
    heading: t('wizard.oidc.urls.heading'),
    description: t('wizard.oidc.urls.description'),
    onNext: () => {
      if (errRedirects || errLogout) {
        setTouched((prev) => ({ ...prev, redirects: true, logout: true }));
        return;
      }
      go('review');
    },
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('wizard.oidc.urls.redirectLabel')} htmlFor="app-redirects" required>
          <Textarea
            id="app-redirects"
            rows={3}
            value={redirects}
            class="font-mono text-xs"
            spellcheck={false}
            placeholder="https://grafana.acme.com/login/generic_oauth"
            aria-describedby="app-redirects-hint"
            invalid={touched.redirects === true && Boolean(errRedirects)}
            onInput={(e) => setRedirects(e.currentTarget.value)}
            onBlur={() => setTouched((prev) => ({ ...prev, redirects: true }))}
          />
          <FieldHint
            id="app-redirects-hint"
            tone={
              touched.redirects === true && errRedirects
                ? 'danger'
                : redirectList.length > 0 && !errRedirects
                  ? 'success'
                  : 'neutral'
            }
          >
            {touched.redirects === true && errRedirects
              ? errRedirects
              : redirectList.length > 0 && !errRedirects
                ? t('wizard.oidc.urls.redirectOk', { count: redirectList.length })
                : t('wizard.oidc.urls.redirectHint')}
          </FieldHint>
        </FieldGroup>

        <InlineHelp tone="info" title={t('wizard.oidc.urls.slashTitle')}>
          {t('wizard.oidc.urls.slashBody')}
        </InlineHelp>

        <details class="border-border-subtle border-t pt-3" open={googleScopes.length > 0}>
          <summary class="text-text-muted hover:text-text sytue-focus-ring-inset cursor-pointer rounded text-sm font-medium">
            {t('wizard.oidc.google.title')}
          </summary>
          <div class="mt-3 flex flex-col gap-4">
            <p class="text-text-muted text-sm leading-relaxed">{t('wizard.oidc.google.body')}</p>
            <div class="grid gap-3 sm:grid-cols-3">
              {GOOGLE_DATA.map((g) => (
                <FieldGroup key={g.key} label={t(g.key)} htmlFor={`google-${g.read}`}>
                  <Select
                    id={`google-${g.read}`}
                    value={google[g.key] ?? 'none'}
                    onChange={(e) =>
                      setGoogle((prev) => ({
                        ...prev,
                        [g.key]: e.currentTarget.value as GoogleLevel,
                      }))
                    }
                  >
                    <option value="none">{t('wizard.oidc.google.none')}</option>
                    <option value="read">{t('wizard.oidc.google.read')}</option>
                    <option value="write">{t('wizard.oidc.google.write')}</option>
                  </Select>
                </FieldGroup>
              ))}
            </div>
            <fieldset class="flex flex-col gap-2">
              <legend class="text-text text-sm font-semibold">
                {t('wizard.oidc.google.directory')}
              </legend>
              {GOOGLE_DIRECTORY.map((d) => (
                <Checkbox
                  key={d.key}
                  label={t(d.key)}
                  checked={directory[d.key] === true}
                  onChange={(e) =>
                    setDirectory((prev) => ({ ...prev, [d.key]: e.currentTarget.checked }))
                  }
                />
              ))}
              <FieldHint>{t('wizard.oidc.google.directoryHint')}</FieldHint>
            </fieldset>
          </div>
        </details>

        <details class="border-border-subtle border-t pt-3">
          <summary class="text-text-muted hover:text-text sytue-focus-ring-inset cursor-pointer rounded text-sm font-medium">
            {t('wizard.oidc.urls.advanced')}
          </summary>
          <div class="mt-3">
            <FieldGroup label={t('wizard.oidc.urls.logoutLabel')} htmlFor="app-logout">
              <Textarea
                id="app-logout"
                rows={2}
                value={logoutUrls}
                class="font-mono text-xs"
                spellcheck={false}
                aria-describedby="app-logout-hint"
                invalid={touched.logout === true && Boolean(errLogout)}
                onInput={(e) => setLogoutUrls(e.currentTarget.value)}
                onBlur={() => setTouched((prev) => ({ ...prev, logout: true }))}
              />
              <FieldHint
                id="app-logout-hint"
                tone={touched.logout === true && errLogout ? 'danger' : 'neutral'}
              >
                {touched.logout === true && errLogout
                  ? errLogout
                  : t('wizard.oidc.urls.logoutHint')}
              </FieldHint>
            </FieldGroup>
          </div>
        </details>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    { label: t('wizard.oidc.summary.app'), value: name.trim() || '—', onEdit: () => go('app') },
    {
      label: t('wizard.oidc.summary.runsOn'),
      value: t(
        kind === 'server' ? 'wizard.oidc.summary.runsServer' : 'wizard.oidc.summary.runsBrowser',
      ),
      onEdit: () => go('app'),
    },
    {
      label: t('wizard.oidc.summary.redirect'),
      value: redirectList.join('\n') || '—',
      mono: true,
      onEdit: () => go('urls'),
    },
    ...(logoutList.length > 0
      ? [
          {
            label: t('wizard.oidc.summary.logout'),
            value: logoutList.join('\n'),
            mono: true,
            onEdit: () => go('urls'),
          },
        ]
      : []),
    {
      label: t('wizard.oidc.summary.sees'),
      value: t('wizard.oidc.summary.seesValue'),
      onEdit: () => go('app'),
    },
    ...(googleScopes.length > 0
      ? [
          {
            label: t('wizard.oidc.summary.google'),
            value: [
              ...GOOGLE_DATA.filter((g) => (google[g.key] ?? 'none') !== 'none').map(
                (g) =>
                  `${t(g.key)}: ${t(google[g.key] === 'write' ? 'wizard.oidc.google.write' : 'wizard.oidc.google.read')}`,
              ),
              ...GOOGLE_DIRECTORY.filter((d) => directory[d.key]).map(
                (d) => `${t('wizard.oidc.google.directory')}: ${t(d.key)}`,
              ),
            ].join('\n'),
            onEdit: () => go('urls'),
          },
        ]
      : []),
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.oidc.step.review'),
    heading: t('wizard.oidc.review.heading'),
    description: t('wizard.oidc.review.description'),
    nextLabel: t('wizard.oidc.review.submit'),
    onNext: () => void finish(),
    content: (
      <div class="flex flex-col gap-4">
        <DataFlow partner={name.trim()} incoming={incoming} outgoing={outgoing} />
        <ReviewSummary items={summary} />
        {kind === 'server' ? (
          <InlineHelp tone="warning" title={t('wizard.oidc.review.secretTitle')}>
            {t('wizard.oidc.review.secretBody')}
          </InlineHelp>
        ) : (
          <InlineHelp tone="info" title={t('wizard.oidc.review.pkceTitle')}>
            {t('wizard.oidc.review.pkceBody')}
          </InlineHelp>
        )}
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('wizard.oidc.step.done'),
    heading: t('wizard.oidc.done.heading', {
      name: created?.name || t('wizard.oidc.done.fallbackName'),
    }),
    done: true,
    content: created ? (
      <div class="flex flex-col gap-4">
        {created.secret ? (
          <section class="border-warning bg-warning-bg rounded-lg border p-4">
            <h3 class="text-warning-text text-sm font-semibold">
              {t('wizard.oidc.done.secretTitle')}
            </h3>
            <p class="text-text-muted mt-1 text-sm leading-relaxed">
              {t('wizard.oidc.done.secretBody')}
            </p>
            <div class="mt-3">
              <CopyRow
                label={t('wizard.oidc.done.secretLabel')}
                value={created.secret}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
            </div>
          </section>
        ) : null}

        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.oidc.done.pasteTitle')}</h3>
          <p class="text-text-muted mt-1 text-sm leading-relaxed">
            {t('wizard.oidc.done.pasteBody')}
          </p>
          <div class="mt-3 flex flex-col gap-2">
            <CopyRow
              label={t('wizard.oidc.done.issuer')}
              value={created.issuer}
              copyLabel={t('common.copy')}
              onCopy={(v) => void copy(v)}
            />
            <CopyRow
              label={t('wizard.oidc.done.clientId')}
              value={created.clientId}
              copyLabel={t('common.copy')}
              onCopy={(v) => void copy(v)}
            />
            <CopyRow
              label={t('wizard.oidc.done.discovery')}
              value={created.discoveryUrl}
              copyLabel={t('common.copy')}
              onCopy={(v) => void copy(v)}
            />
            {created.scopes.some((sc) => sc.startsWith(G)) ? (
              <CopyRow
                label={t('wizard.oidc.done.googleDiscovery')}
                value={`${created.issuer}/compat/google/.well-known/openid-configuration`}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
            ) : null}
          </div>
          <details class="mt-3">
            <summary class="text-text-muted hover:text-text sytue-focus-ring-inset cursor-pointer rounded text-sm font-medium">
              {t('wizard.oidc.done.separateTitle')}
            </summary>
            <div class="mt-3 flex flex-col gap-2">
              <CopyRow
                label={t('wizard.oidc.done.authorization')}
                value={created.authorizationEndpoint}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
              <CopyRow
                label={t('wizard.oidc.done.token')}
                value={created.tokenEndpoint}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
              <CopyRow
                label={t('wizard.oidc.done.userinfo')}
                value={created.userinfoEndpoint}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
              <CopyRow
                label={t('wizard.oidc.done.jwks')}
                value={created.jwksUri}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
              <CopyRow
                label={t('wizard.oidc.done.endSession')}
                value={created.endSessionEndpoint}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
              <CopyRow
                label={t('wizard.oidc.done.scopes')}
                value={created.scopes.join(' ')}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
            </div>
          </details>
        </section>

        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.oidc.done.nextTitle')}</h3>
          <ol class="mt-3 flex flex-col gap-3">
            <NextStep n={1} title={t('wizard.oidc.done.step1Title')}>
              {t(
                created.secret ? 'wizard.oidc.done.step1BodySecret' : 'wizard.oidc.done.step1Body',
              )}
            </NextStep>
            <NextStep n={2} title={t('wizard.oidc.done.step2Title')}>
              {t('wizard.oidc.done.step2Body')}
            </NextStep>
            <NextStep n={3} title={t('wizard.oidc.done.step3Title')}>
              {t('wizard.oidc.done.step3Body')}
            </NextStep>
          </ol>
        </section>

        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('wizard.oidc.done.back')}
          </Button>
        </div>
      </div>
    ) : null,
  };

  const byId: Record<string, WizardStep> = {
    app: stepApp,
    urls: stepUrls,
    review: stepReview,
    done: stepDone,
  };

  return (
    <Wizard
      steps={ids.map((id) => byId[id] as WizardStep)}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}

class HttpStatus extends Error {
  constructor(readonly status: number) {
    super(String(status));
  }
}
