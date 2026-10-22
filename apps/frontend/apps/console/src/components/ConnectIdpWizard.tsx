/**
 * ConnectIdpWizard — asistente «Conectar tu proveedor de identidad» (SSO upstream).
 *
 * Mismo sistema de configuración guiada que ConnectAppWizard: elegir tipo → qué va a
 * pasar (y qué tener a mano, con DÓNDE está cada dato) → configurar con validación en
 * vivo → probar → resumen → listo, con los datos que el admin debe pegar en SU
 * proveedor. No inventa API: usa los endpoints BFF que ya existen (POST/PATCH/DELETE
 * /api/admin/sso/providers y POST …/:id/test).
 *
 * El proveedor se crea DESACTIVADO al probar (el endpoint /test necesita que exista);
 * si el admin cancela, se borra, así no quedan proveedores huérfanos. Nadie puede
 * entrar por él hasta que se activa en el paso de resumen.
 *
 * Los textos vienen del catálogo (`wizard.idp.*`): la `t` llega ya atada al idioma
 * desde SsoPanel, que es quien lo negocia.
 */
import type { ComponentChildren } from 'preact';
import { useState } from 'preact/hooks';
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
  Switch,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { IconName, ReviewItem, WizardStep } from '@sytue/ui';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { SsoProvider, SsoProviderInput } from '../lib/types.js';

type IdpKind = 'oidc' | 'saml';

export interface IdpFormState {
  type: IdpKind;
  name: string;
  slug: string;
  domains: string;
  mapEmail: string;
  mapName: string;
  mapGroups: string;
  enabled: boolean;
  // OIDC
  issuer: string;
  clientId: string;
  clientSecret: string;
  scopes: string;
  // SAML
  metadataXml: string;
  idpEntityId: string;
  idpSsoUrl: string;
  idpCertificate: string;
  spEntityId: string;
  samlBinding: 'redirect' | 'post';
  nameIdFormat: string;
}

const EMPTY_FORM: IdpFormState = {
  type: 'oidc',
  name: '',
  slug: '',
  domains: '',
  mapEmail: '',
  mapName: '',
  mapGroups: '',
  enabled: true,
  issuer: '',
  clientId: '',
  clientSecret: '',
  scopes: 'openid email profile',
  metadataXml: '',
  idpEntityId: '',
  idpSsoUrl: '',
  idpCertificate: '',
  spEntityId: '',
  samlBinding: 'redirect',
  nameIdFormat: '',
};

function formFromProvider(p: SsoProvider): IdpFormState {
  return {
    ...EMPTY_FORM,
    type: p.type,
    name: p.name,
    slug: p.slug,
    domains: p.allowedEmailDomains.join(', '),
    mapEmail: p.attributeMapping.email ?? '',
    mapName: p.attributeMapping.name ?? '',
    mapGroups: p.attributeMapping.groups ?? '',
    enabled: p.enabled,
    issuer: p.issuer,
    clientId: p.clientId,
    clientSecret: '',
    scopes: p.scopes || 'openid email profile',
    idpEntityId: p.idpEntityId,
    idpSsoUrl: p.idpSsoUrl,
    idpCertificate: p.idpCertificate,
    spEntityId: p.spEntityId,
    samlBinding: p.samlBinding,
    nameIdFormat: p.nameIdFormat,
  };
}

interface ConnectIdpWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Proveedor existente: el asistente arranca en «Configurar» y guarda cambios. */
  editing?: SsoProvider | undefined;
  /** Slugs ya usados en la organización (no se pueden repetir). */
  takenSlugs: string[];
  /** Se llama con el proveedor tal como lo devuelve el servicio tras cada guardado. */
  onSaved: (p: SsoProvider) => void;
  /** Cancelar. Recibe el id del borrador que el asistente acaba de borrar, si lo hubo. */
  onCancel: (discardedId?: string) => void;
  onDone: () => void;
}

type TestState =
  { kind: 'idle' | 'running' } | { kind: 'ok'; pkce: boolean } | { kind: 'fail'; fix: string };

// ---- Copy de guía (el servicio no manda textos de ayuda) ----

// El nombre del estándar (OIDC, SAML 2.0) no se traduce; lo que lo explica, sí.
const KINDS: {
  value: IdpKind;
  title: string;
  blurb: ConsoleMessageKey;
  examples: ConsoleMessageKey;
}[] = [
  {
    value: 'oidc',
    title: 'OpenID Connect (OIDC)',
    blurb: 'wizard.idp.kind.oidcBlurb',
    examples: 'wizard.idp.kind.oidcExamples',
  },
  {
    value: 'saml',
    title: 'SAML 2.0',
    blurb: 'wizard.idp.kind.samlBlurb',
    examples: 'wizard.idp.kind.samlExamples',
  },
];

/** Qué se activa al federar, en lenguaje llano. */
const EFFECTS: { icon: IconName; title: ConsoleMessageKey; body: ConsoleMessageKey }[] = [
  {
    icon: 'lock',
    title: 'wizard.idp.effect.signInTitle',
    body: 'wizard.idp.effect.signInBody',
  },
  {
    icon: 'users',
    title: 'wizard.idp.effect.noLossTitle',
    body: 'wizard.idp.effect.noLossBody',
  },
  {
    icon: 'globe',
    title: 'wizard.idp.effect.whoTitle',
    body: 'wizard.idp.effect.whoBody',
  },
  {
    icon: 'settings',
    title: 'wizard.idp.effect.offTitle',
    body: 'wizard.idp.effect.offBody',
  },
];

/** Qué tener a mano y, sobre todo, DÓNDE está en el proveedor. */
function checklist(kind: IdpKind): { label: ConsoleMessageKey; where: ConsoleMessageKey }[] {
  if (kind === 'saml') {
    return [
      { label: 'wizard.idp.need.samlXml', where: 'wizard.idp.need.samlXmlWhere' },
      { label: 'wizard.idp.need.samlManual', where: 'wizard.idp.need.samlManualWhere' },
      { label: 'wizard.idp.need.domains', where: 'wizard.idp.need.domainsWhere' },
    ];
  }
  return [
    { label: 'wizard.idp.need.issuer', where: 'wizard.idp.need.issuerWhere' },
    { label: 'wizard.idp.need.client', where: 'wizard.idp.need.clientWhere' },
    { label: 'wizard.idp.need.domains', where: 'wizard.idp.need.domainsWhere' },
  ];
}

// ---- Validación en vivo (ataca los fallos típicos, no la sintaxis por la sintaxis) ----

const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const DOMAIN_RE = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/;

function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
    .replace(/-+$/g, '');
}

function issuerError(raw: string, t: T): string | null {
  const v = raw.trim();
  if (!v) return t('wizard.idp.err.issuerRequired');
  if (/\s/.test(v)) return t('wizard.idp.err.hasSpace');
  if (/\.well-known/i.test(v)) return t('wizard.idp.err.wellKnown');
  if (/^http:\/\//i.test(v)) return t('wizard.idp.err.insecure');
  if (!/^https:\/\//i.test(v)) return t('wizard.idp.err.needHttps');
  try {
    const u = new URL(v);
    if (!u.hostname.includes('.')) return t('wizard.idp.err.noDomain');
  } catch {
    return t('wizard.idp.err.badIssuerUrl');
  }
  return null;
}

function urlError(raw: string, t: T): string | null {
  const v = raw.trim();
  if (!v) return t('wizard.idp.err.urlRequired');
  if (/\s/.test(v)) return t('wizard.idp.err.hasSpace');
  if (!/^https:\/\//i.test(v)) return t('wizard.idp.err.mustBeHttps');
  try {
    new URL(v);
  } catch {
    return t('wizard.idp.err.badUrl');
  }
  return null;
}

function domainsError(raw: string, t: T): string | null {
  const bad = parseDomains(raw).filter((d) => !DOMAIN_RE.test(d));
  if (bad.length === 0) return null;
  return t('wizard.idp.err.badDomain', { domain: bad[0] ?? '' });
}

function certError(raw: string, t: T): string | null {
  const v = raw.trim();
  if (!v) return t('wizard.idp.err.certRequired');
  if (/-----BEGIN CERTIFICATE-----/.test(v) || /^[A-Za-z0-9+/=\s]+$/.test(v)) return null;
  return t('wizard.idp.err.notCert');
}

function parseDomains(s: string): string[] {
  return s
    .split(/[\s,]+/)
    .map((d) => d.trim().toLowerCase())
    .filter(Boolean);
}

/** Traduce un fallo del servicio a algo que el admin pueda hacer. */
function testFix(status: number, kind: IdpKind, t: T): string {
  switch (status) {
    case 400:
      return t(kind === 'saml' ? 'wizard.idp.fix.samlData' : 'wizard.idp.fix.oidcDiscovery');
    case 409:
      return t('wizard.idp.fix.slugTaken');
    case 403:
      return t('wizard.idp.fix.forbidden');
    case 502:
    case 503:
    case 504:
      return t('wizard.idp.fix.idpDown');
    default:
      return t('wizard.idp.fix.generic');
  }
}

// ---- Componente ----

export default function ConnectIdpWizard({
  t,
  editing,
  takenSlugs,
  onSaved,
  onCancel,
  onDone,
}: ConnectIdpWizardProps) {
  const toast = useToast();
  const [form, setForm] = useState<IdpFormState>(editing ? formFromProvider(editing) : EMPTY_FORM);
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [slugEdited, setSlugEdited] = useState(Boolean(editing));
  const [manualSaml, setManualSaml] = useState(Boolean(editing?.idpEntityId));
  const [test, setTest] = useState<TestState>({ kind: 'idle' });
  const [busy, setBusy] = useState(false);
  /** Proveedor ya persistido (el que se edita, o el borrador que creamos al probar). */
  const [saved, setSaved] = useState<SsoProvider | null>(editing ?? null);
  const [savedKey, setSavedKey] = useState<string | null>(null);
  /** Borrador creado por el asistente: si se cancela, se borra. */
  const [createdId, setCreatedId] = useState<string | null>(null);

  // En edición no se ofrecen «Tipo» ni «Qué va a pasar»: el tipo es inmutable y el
  // proveedor ya existe. Los índices se derivan de esta lista, no se codifican.
  const ids: readonly string[] = editing
    ? ['configure', 'test', 'review', 'done']
    : ['type', 'explain', 'configure', 'test', 'review', 'done'];
  const [step, setStep] = useState(0);
  const go = (id: string) => setStep(ids.indexOf(id));

  const set = <K extends keyof IdpFormState>(k: K, v: IdpFormState[K]) => {
    setForm((f) => ({ ...f, [k]: v }));
    setTest({ kind: 'idle' });
  };
  const mark = (k: string) => setTouched((prev) => ({ ...prev, [k]: true }));
  const isOidc = form.type === 'oidc';

  // Validación en vivo del paso «Configurar».
  const slug = form.slug.trim().toLowerCase();
  const errName = form.name.trim() ? null : t('wizard.idp.err.nameRequired');
  const errSlug = !slug
    ? t('wizard.idp.err.slugRequired')
    : !SLUG_RE.test(slug)
      ? t('wizard.idp.err.slugFormat')
      : // `saved` es el proveedor que edita el asistente (o el borrador que ya creó al
        // probar): su propio slug no cuenta como repetido.
        takenSlugs.includes(slug) && slug !== saved?.slug
        ? t('wizard.idp.err.slugTaken')
        : null;
  const errIssuer = isOidc ? issuerError(form.issuer, t) : null;
  const errClientId = isOidc && !form.clientId.trim() ? t('wizard.idp.err.clientIdRequired') : null;
  const usesXml = !isOidc && !manualSaml;
  const errXml =
    usesXml && !form.metadataXml.trim()
      ? t('wizard.idp.err.xmlRequired')
      : usesXml && !/EntityDescriptor/i.test(form.metadataXml)
        ? t('wizard.idp.err.xmlFormat')
        : null;
  const errEntity =
    !isOidc && manualSaml && !form.idpEntityId.trim() ? t('wizard.idp.err.entityRequired') : null;
  const errSso = !isOidc && manualSaml ? urlError(form.idpSsoUrl, t) : null;
  const errCert = !isOidc && manualSaml ? certError(form.idpCertificate, t) : null;
  const errDomains = domainsError(form.domains, t);
  const configValid = ![
    errName,
    errSlug,
    errIssuer,
    errClientId,
    errXml,
    errEntity,
    errSso,
    errCert,
    errDomains,
  ].some(Boolean);

  const dataKey = JSON.stringify({ ...form, clientSecret: form.clientSecret ? 1 : 0 });

  function buildPayload(enabled: boolean): SsoProviderInput {
    const mapping: Record<string, string> = {};
    if (form.mapEmail.trim()) mapping.email = form.mapEmail.trim();
    if (form.mapName.trim()) mapping.name = form.mapName.trim();
    if (form.mapGroups.trim()) mapping.groups = form.mapGroups.trim();
    const payload: SsoProviderInput = {
      type: form.type,
      name: form.name.trim(),
      slug,
      attribute_mapping: mapping,
      allowed_email_domains: parseDomains(form.domains),
      enabled,
    };
    if (isOidc) {
      payload.issuer = form.issuer.trim().replace(/\/+$/, '');
      payload.client_id = form.clientId.trim();
      payload.scopes = form.scopes.trim() || 'openid email profile';
      // El secreto solo se envía si el admin lo escribió (vacío = se conserva el actual).
      if (form.clientSecret) payload.client_secret = form.clientSecret;
    } else {
      if (usesXml && form.metadataXml.trim()) payload.metadata_xml = form.metadataXml.trim();
      else {
        payload.idp_entity_id = form.idpEntityId.trim();
        payload.idp_sso_url = form.idpSsoUrl.trim();
        payload.idp_certificate = form.idpCertificate.trim();
      }
      if (form.spEntityId.trim()) payload.sp_entity_id = form.spEntityId.trim();
      payload.saml_binding = form.samlBinding;
      if (form.nameIdFormat.trim()) payload.name_id_format = form.nameIdFormat.trim();
    }
    return payload;
  }

  /** Crea (la primera vez, desactivado) o actualiza el proveedor con los datos actuales. */
  async function persist(enabled: boolean): Promise<SsoProvider> {
    const payload = buildPayload(enabled);
    const p = (
      saved
        ? await send(`/api/admin/sso/providers/${encodeURIComponent(saved.id)}`, 'PATCH', payload)
        : await send('/api/admin/sso/providers', 'POST', payload)
    ) as SsoProvider;
    if (!saved) setCreatedId(p.id);
    setSaved(p);
    setSavedKey(dataKey);
    onSaved(p);
    return p;
  }

  async function runTest() {
    if (busy) return;
    setBusy(true);
    setTest({ kind: 'running' });
    try {
      // El endpoint /test comprueba lo GUARDADO: guardamos primero (desactivado si es
      // nuevo; si se edita uno activo, conserva su estado hasta el paso de resumen).
      const p =
        saved && savedKey === dataKey ? saved : await persist(saved ? saved.enabled : false);
      const out = (await send(
        `/api/admin/sso/providers/${encodeURIComponent(p.id)}/test`,
        'POST',
        {},
      )) as { pkce?: boolean; provider?: SsoProvider };
      if (out.provider) {
        setSaved(out.provider);
        onSaved(out.provider);
      }
      setTest({ kind: 'ok', pkce: Boolean(out.pkce) });
    } catch (e) {
      setTest({ kind: 'fail', fix: testFix(errStatus(e), form.type, t) });
    } finally {
      setBusy(false);
    }
  }

  async function finish() {
    if (busy) return;
    setBusy(true);
    try {
      await persist(form.enabled);
      go('done');
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description: testFix(errStatus(e), form.type, t),
      });
    } finally {
      setBusy(false);
    }
  }

  function cancel() {
    if (createdId) {
      void fetch(`/api/admin/sso/providers/${encodeURIComponent(createdId)}`, { method: 'DELETE' });
      onCancel(createdId);
      return;
    }
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

  // ---- Pasos ----

  const stepType: WizardStep = {
    id: 'type',
    title: t('wizard.idp.step.type'),
    heading: t('wizard.idp.type.heading'),
    description: t('wizard.idp.type.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ul class="flex flex-col gap-3">
          {KINDS.map((k) => (
            <li key={k.value}>
              <ChoiceCard
                name="sytue-idp-kind"
                value={k.value}
                icon="lock"
                title={k.title}
                description={t(k.blurb)}
                footnote={t(k.examples)}
                badge={
                  k.value === 'oidc' ? (
                    <Badge tone="neutral">{t('wizard.idp.kind.mostCommon')}</Badge>
                  ) : null
                }
                selected={form.type === k.value}
                onSelect={() => set('type', k.value)}
              />
            </li>
          ))}
        </ul>
        <InlineHelp tone="info" title={t('wizard.idp.kind.bothTitle')}>
          {t('wizard.idp.kind.bothBody')}
        </InlineHelp>
      </div>
    ),
  };

  const stepExplain: WizardStep = {
    id: 'explain',
    title: t('wizard.idp.step.explain'),
    heading: t('wizard.idp.explain.heading'),
    description: t('wizard.idp.explain.description'),
    content: (
      <div class="flex flex-col gap-4">
        <ul class="grid gap-3 sm:grid-cols-2">
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
        <InlineHelp tone="info" title={t('wizard.idp.explain.needTitle')}>
          <ul class="mt-1 flex flex-col gap-2">
            {checklist(form.type).map((c) => (
              <li key={c.label}>
                <span class="text-text font-medium">{t(c.label)}</span>
                <span class="text-text-muted"> — {t(c.where)}</span>
              </li>
            ))}
          </ul>
        </InlineHelp>
      </div>
    ),
  };

  const stepConfigure: WizardStep = {
    id: 'configure',
    title: t('wizard.idp.step.configure'),
    heading: t(isOidc ? 'wizard.idp.configure.headingOidc' : 'wizard.idp.configure.headingSaml'),
    description: t('wizard.idp.configure.description'),
    onNext: () => {
      if (!configValid) {
        setTouched({
          name: true,
          slug: true,
          issuer: true,
          clientId: true,
          xml: true,
          entity: true,
          sso: true,
          cert: true,
          domains: true,
        });
        return;
      }
      go('test');
    },
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('wizard.idp.field.name')} htmlFor="idp-name" required>
          <Input
            id="idp-name"
            value={form.name}
            placeholder="Microsoft Entra ID"
            aria-describedby="idp-name-hint"
            invalid={touched.name === true && Boolean(errName)}
            onInput={(e) => {
              const v = e.currentTarget.value;
              setForm((f) => ({ ...f, name: v, slug: slugEdited ? f.slug : slugify(v) }));
              setTest({ kind: 'idle' });
            }}
            onBlur={() => mark('name')}
          />
          <FieldHint
            id="idp-name-hint"
            tone={touched.name === true && errName ? 'danger' : 'neutral'}
          >
            {touched.name === true && errName ? errName : t('wizard.idp.field.nameHint')}
          </FieldHint>
        </FieldGroup>

        <FieldGroup label={t('wizard.idp.field.slug')} htmlFor="idp-slug" required>
          <Input
            id="idp-slug"
            value={form.slug}
            class="font-mono"
            placeholder="entra"
            autocomplete="off"
            spellcheck={false}
            aria-describedby="idp-slug-hint"
            invalid={touched.slug === true && Boolean(errSlug)}
            onInput={(e) => {
              setSlugEdited(true);
              set('slug', e.currentTarget.value);
            }}
            onBlur={() => mark('slug')}
          />
          <FieldHint
            id="idp-slug-hint"
            tone={
              touched.slug === true && errSlug ? 'danger' : slug && !errSlug ? 'success' : 'neutral'
            }
          >
            {touched.slug === true && errSlug ? errSlug : t('wizard.idp.field.slugHint')}
          </FieldHint>
        </FieldGroup>

        {isOidc ? (
          <>
            <FieldGroup label={t('wizard.idp.field.issuer')} htmlFor="idp-issuer" required>
              <Input
                id="idp-issuer"
                type="url"
                value={form.issuer}
                placeholder="https://login.microsoftonline.com/TENANT/v2.0"
                autocomplete="off"
                spellcheck={false}
                aria-describedby="idp-issuer-hint"
                invalid={touched.issuer === true && Boolean(errIssuer)}
                onInput={(e) => set('issuer', e.currentTarget.value)}
                onBlur={() => mark('issuer')}
              />
              <FieldHint
                id="idp-issuer-hint"
                tone={
                  touched.issuer === true && errIssuer
                    ? 'danger'
                    : form.issuer.trim() && !errIssuer
                      ? 'success'
                      : 'neutral'
                }
              >
                {touched.issuer === true && errIssuer
                  ? errIssuer
                  : form.issuer.trim() && !errIssuer
                    ? t('wizard.idp.field.issuerOk')
                    : t('wizard.idp.field.issuerHint')}
              </FieldHint>
            </FieldGroup>

            <FieldGroup label={t('wizard.idp.field.clientId')} htmlFor="idp-cid" required>
              <Input
                id="idp-cid"
                value={form.clientId}
                class="font-mono"
                autocomplete="off"
                spellcheck={false}
                aria-describedby="idp-cid-hint"
                invalid={touched.clientId === true && Boolean(errClientId)}
                onInput={(e) => set('clientId', e.currentTarget.value)}
                onBlur={() => mark('clientId')}
              />
              <FieldHint
                id="idp-cid-hint"
                tone={touched.clientId === true && errClientId ? 'danger' : 'neutral'}
              >
                {touched.clientId === true && errClientId
                  ? errClientId
                  : t('wizard.idp.field.clientIdHint')}
              </FieldHint>
            </FieldGroup>

            <FieldGroup label={t('wizard.idp.field.secret')} htmlFor="idp-secret">
              <Input
                id="idp-secret"
                type="password"
                value={form.clientSecret}
                autocomplete="off"
                placeholder={editing?.hasSecret ? t('wizard.idp.field.secretPlaceholder') : ''}
                aria-describedby="idp-secret-hint"
                onInput={(e) => set('clientSecret', e.currentTarget.value)}
              />
              <FieldHint id="idp-secret-hint">
                {t(
                  editing?.hasSecret
                    ? 'wizard.idp.field.secretKeepHint'
                    : 'wizard.idp.field.secretHint',
                )}
              </FieldHint>
              <InlineHelp tone="neutral" hideIcon class="mt-1">
                <span class="flex items-start gap-2">
                  <Icon name="lock" size={15} class="text-text-subtle mt-0.5 shrink-0" />
                  <span>{t('wizard.idp.field.secretStored')}</span>
                </span>
              </InlineHelp>
            </FieldGroup>
          </>
        ) : (
          <>
            {usesXml ? (
              <FieldGroup label={t('wizard.idp.field.xml')} htmlFor="idp-xml" required>
                <Textarea
                  id="idp-xml"
                  rows={5}
                  value={form.metadataXml}
                  class="font-mono text-xs"
                  spellcheck={false}
                  placeholder="<EntityDescriptor …>"
                  aria-describedby="idp-xml-hint"
                  invalid={touched.xml === true && Boolean(errXml)}
                  onInput={(e) => set('metadataXml', e.currentTarget.value)}
                  onBlur={() => mark('xml')}
                />
                <FieldHint
                  id="idp-xml-hint"
                  tone={
                    touched.xml === true && errXml
                      ? 'danger'
                      : form.metadataXml.trim() && !errXml
                        ? 'success'
                        : 'neutral'
                  }
                >
                  {touched.xml === true && errXml
                    ? errXml
                    : form.metadataXml.trim() && !errXml
                      ? t('wizard.idp.field.xmlOk')
                      : t('wizard.idp.field.xmlHint')}
                </FieldHint>
                <button
                  type="button"
                  class="text-primary-text sytue-focus-ring-inset mt-1 self-start rounded text-sm font-semibold hover:underline"
                  onClick={() => setManualSaml(true)}
                >
                  {t('wizard.idp.field.noXmlLink')}
                </button>
              </FieldGroup>
            ) : (
              <>
                <FieldGroup label={t('wizard.idp.field.entity')} htmlFor="idp-entity" required>
                  <Input
                    id="idp-entity"
                    value={form.idpEntityId}
                    class="font-mono"
                    autocomplete="off"
                    spellcheck={false}
                    aria-describedby="idp-entity-hint"
                    invalid={touched.entity === true && Boolean(errEntity)}
                    onInput={(e) => set('idpEntityId', e.currentTarget.value)}
                    onBlur={() => mark('entity')}
                  />
                  <FieldHint
                    id="idp-entity-hint"
                    tone={touched.entity === true && errEntity ? 'danger' : 'neutral'}
                  >
                    {touched.entity === true && errEntity
                      ? errEntity
                      : t('wizard.idp.field.entityHint')}
                  </FieldHint>
                </FieldGroup>

                <FieldGroup label={t('wizard.idp.field.ssoUrl')} htmlFor="idp-sso" required>
                  <Input
                    id="idp-sso"
                    type="url"
                    value={form.idpSsoUrl}
                    autocomplete="off"
                    spellcheck={false}
                    aria-describedby="idp-sso-hint"
                    invalid={touched.sso === true && Boolean(errSso)}
                    onInput={(e) => set('idpSsoUrl', e.currentTarget.value)}
                    onBlur={() => mark('sso')}
                  />
                  <FieldHint
                    id="idp-sso-hint"
                    tone={
                      touched.sso === true && errSso
                        ? 'danger'
                        : form.idpSsoUrl.trim() && !errSso
                          ? 'success'
                          : 'neutral'
                    }
                  >
                    {touched.sso === true && errSso ? errSso : t('wizard.idp.field.ssoUrlHint')}
                  </FieldHint>
                </FieldGroup>

                <FieldGroup label={t('wizard.idp.field.cert')} htmlFor="idp-cert" required>
                  <Textarea
                    id="idp-cert"
                    rows={4}
                    value={form.idpCertificate}
                    class="font-mono text-xs"
                    spellcheck={false}
                    placeholder="-----BEGIN CERTIFICATE-----"
                    aria-describedby="idp-cert-hint"
                    invalid={touched.cert === true && Boolean(errCert)}
                    onInput={(e) => set('idpCertificate', e.currentTarget.value)}
                    onBlur={() => mark('cert')}
                  />
                  <FieldHint
                    id="idp-cert-hint"
                    tone={
                      touched.cert === true && errCert
                        ? 'danger'
                        : form.idpCertificate.trim() && !errCert
                          ? 'success'
                          : 'neutral'
                    }
                  >
                    {touched.cert === true && errCert ? errCert : t('wizard.idp.field.certHint')}
                  </FieldHint>
                </FieldGroup>

                <button
                  type="button"
                  class="text-primary-text sytue-focus-ring-inset -mt-2 self-start rounded text-sm font-semibold hover:underline"
                  onClick={() => setManualSaml(false)}
                >
                  {t('wizard.idp.field.useXmlLink')}
                </button>
              </>
            )}
          </>
        )}

        <FieldGroup label={t('wizard.idp.field.domains')} htmlFor="idp-domains">
          <Textarea
            id="idp-domains"
            rows={2}
            value={form.domains}
            placeholder="acme.com, acme.eu"
            spellcheck={false}
            aria-describedby="idp-domains-hint"
            invalid={touched.domains === true && Boolean(errDomains)}
            onInput={(e) => set('domains', e.currentTarget.value)}
            onBlur={() => mark('domains')}
          />
          <FieldHint
            id="idp-domains-hint"
            tone={
              touched.domains === true && errDomains
                ? 'danger'
                : form.domains.trim() && !errDomains
                  ? 'success'
                  : 'neutral'
            }
          >
            {touched.domains === true && errDomains
              ? errDomains
              : t('wizard.idp.field.domainsHint')}
          </FieldHint>
        </FieldGroup>

        <details class="border-border-subtle border-t pt-3">
          <summary class="text-text-muted hover:text-text sytue-focus-ring-inset cursor-pointer rounded text-sm font-medium">
            {t('wizard.idp.field.advanced')}
          </summary>
          <p class="sytue-meta mt-2 leading-relaxed">{t('wizard.idp.field.advancedHint')}</p>
          <div class="mt-3 flex flex-col gap-4">
            {isOidc ? (
              <FieldGroup label={t('wizard.idp.field.scopes')} htmlFor="idp-scopes">
                <Input
                  id="idp-scopes"
                  value={form.scopes}
                  class="font-mono"
                  aria-describedby="idp-scopes-hint"
                  onInput={(e) => set('scopes', e.currentTarget.value)}
                />
                <FieldHint id="idp-scopes-hint">{t('wizard.idp.field.scopesHint')}</FieldHint>
              </FieldGroup>
            ) : null}
            <fieldset class="flex flex-col gap-2">
              <legend class="text-text text-sm font-semibold">
                {t('wizard.idp.mapping.legend')}
              </legend>
              <p class="sytue-meta">{t('wizard.idp.mapping.hint')}</p>
              <div class="mt-1 grid gap-2 sm:grid-cols-3">
                <Input
                  aria-label={t('wizard.idp.mapping.emailAria')}
                  value={form.mapEmail}
                  placeholder="email"
                  class="font-mono"
                  onInput={(e) => set('mapEmail', e.currentTarget.value)}
                />
                <Input
                  aria-label={t('wizard.idp.mapping.nameAria')}
                  value={form.mapName}
                  placeholder="name"
                  class="font-mono"
                  onInput={(e) => set('mapName', e.currentTarget.value)}
                />
                <Input
                  aria-label={t('wizard.idp.mapping.groupsAria')}
                  value={form.mapGroups}
                  placeholder="groups"
                  class="font-mono"
                  onInput={(e) => set('mapGroups', e.currentTarget.value)}
                />
              </div>
            </fieldset>
          </div>
        </details>
      </div>
    ),
  };

  const stepTest: WizardStep = {
    id: 'test',
    title: t('wizard.idp.step.test'),
    heading: t(isOidc ? 'wizard.idp.test.headingOidc' : 'wizard.idp.test.headingSaml'),
    description: t(isOidc ? 'wizard.idp.test.descriptionOidc' : 'wizard.idp.test.descriptionSaml'),
    content: (
      <div class="flex max-w-xl flex-col gap-4">
        {test.kind === 'ok' ? (
          <InlineHelp tone="success" title={t('wizard.idp.test.okTitle')}>
            {t(
              isOidc
                ? test.pkce
                  ? 'wizard.idp.test.okOidcPkce'
                  : 'wizard.idp.test.okOidc'
                : 'wizard.idp.test.okSaml',
            )}
          </InlineHelp>
        ) : test.kind === 'fail' ? (
          <InlineHelp
            tone="danger"
            title={t('wizard.idp.test.failTitle')}
            action={
              <Button variant="secondary" size="sm" onClick={() => go('configure')}>
                {t('wizard.idp.test.reviewData')}
              </Button>
            }
          >
            {test.fix}
          </InlineHelp>
        ) : (
          <InlineHelp tone="neutral">{t('wizard.idp.test.idleHelp')}</InlineHelp>
        )}
        <div class="flex flex-wrap items-center gap-3">
          <Button
            variant={test.kind === 'ok' ? 'secondary' : 'primary'}
            loading={test.kind === 'running'}
            onClick={() => void runTest()}
          >
            {t(test.kind === 'idle' ? 'wizard.idp.test.run' : 'wizard.idp.test.rerun')}
          </Button>
          {test.kind === 'idle' ? <p class="sytue-meta">{t('wizard.idp.test.skipHint')}</p> : null}
        </div>
        {saved && !editing ? <p class="sytue-meta">{t('wizard.idp.test.draftNote')}</p> : null}
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('wizard.idp.summary.type'),
      value: isOidc ? 'OpenID Connect (OIDC)' : 'SAML 2.0',
      ...(editing ? {} : { onEdit: () => go('type') }),
    },
    {
      label: t('wizard.idp.summary.name'),
      value: form.name.trim() || '—',
      onEdit: () => go('configure'),
    },
    {
      label: t('wizard.idp.summary.slug'),
      value: slug || '—',
      mono: true,
      onEdit: () => go('configure'),
    },
    ...(isOidc
      ? [
          {
            label: t('wizard.idp.summary.issuer'),
            value: form.issuer.trim() || '—',
            mono: true,
            onEdit: () => go('configure'),
          },
          {
            label: t('wizard.idp.summary.clientId'),
            value: form.clientId.trim() || '—',
            mono: true,
            onEdit: () => go('configure'),
          },
          {
            label: t('wizard.idp.summary.secret'),
            value: t(
              form.clientSecret
                ? 'wizard.idp.summary.secretSaved'
                : editing?.hasSecret
                  ? 'wizard.idp.summary.secretKept'
                  : 'wizard.idp.summary.secretNone',
            ),
            onEdit: () => go('configure'),
          },
        ]
      : [
          {
            label: t('wizard.idp.summary.samlData'),
            value: usesXml
              ? t('wizard.idp.summary.fromXml')
              : t('wizard.idp.summary.entityValue', { id: form.idpEntityId.trim() || '—' }),
            onEdit: () => go('configure'),
          },
          ...(usesXml
            ? []
            : [
                {
                  label: t('wizard.idp.summary.ssoUrl'),
                  value: form.idpSsoUrl.trim() || '—',
                  mono: true,
                  onEdit: () => go('configure'),
                },
                {
                  label: t('wizard.idp.summary.cert'),
                  value: t('wizard.idp.summary.certPasted'),
                  onEdit: () => go('configure'),
                },
              ]),
        ]),
    {
      label: t('wizard.idp.summary.who'),
      value: parseDomains(form.domains).length
        ? t('wizard.idp.summary.whoDomains', { domains: parseDomains(form.domains).join(', ') })
        : t('wizard.idp.summary.whoAny'),
      onEdit: () => go('configure'),
    },
    {
      label: t('wizard.idp.summary.test'),
      value: t(
        test.kind === 'ok'
          ? 'wizard.idp.summary.testOk'
          : test.kind === 'fail'
            ? 'wizard.idp.summary.testFail'
            : 'wizard.idp.summary.testNone',
      ),
      onEdit: () => go('test'),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.idp.step.review'),
    heading: t(editing ? 'wizard.idp.review.headingEdit' : 'wizard.idp.review.headingNew'),
    description: t('wizard.idp.review.description'),
    nextLabel: t(
      editing
        ? 'wizard.idp.review.saveEdit'
        : form.enabled
          ? 'wizard.idp.review.saveEnable'
          : 'wizard.idp.review.save',
    ),
    onNext: () => void finish(),
    content: (
      <div class="flex flex-col gap-4">
        {/* Al entrar, el proveedor manda nombre y correo (y los grupos, si se mapean). */}
        <DataFlow
          partner={form.name.trim()}
          incoming={form.mapGroups.trim() ? ['identity', 'people'] : ['identity']}
        />
        <ReviewSummary items={summary} />
        <div class="border-border bg-surface-raised rounded-lg border p-3">
          <Switch
            label={t('wizard.idp.review.enableLabel')}
            checked={form.enabled}
            onChange={(e) => setForm((f) => ({ ...f, enabled: e.currentTarget.checked }))}
          />
          <p class="text-text-muted mt-1.5 text-sm leading-snug">
            {t(form.enabled ? 'wizard.idp.review.enableOn' : 'wizard.idp.review.enableOff')}
          </p>
        </div>
        <InlineHelp tone="warning" title={t('wizard.idp.review.pendingTitle')}>
          {t(isOidc ? 'wizard.idp.review.pendingOidc' : 'wizard.idp.review.pendingSaml')}
        </InlineHelp>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('wizard.idp.step.done'),
    heading: editing
      ? t('wizard.idp.done.headingEdit')
      : t('wizard.idp.done.headingNew', {
          name: form.name.trim() || t('wizard.idp.done.fallbackName'),
        }),
    done: true,
    content: saved ? (
      <div class="flex flex-col gap-4">
        <InlineHelp
          tone={saved.enabled ? 'success' : 'neutral'}
          title={t('wizard.idp.done.savedTitle')}
        >
          {t(saved.enabled ? 'wizard.idp.done.savedOn' : 'wizard.idp.done.savedOff')}
        </InlineHelp>

        <section class="border-warning bg-warning-bg rounded-lg border p-4">
          <h3 class="text-warning-text text-sm font-semibold">{t('wizard.idp.done.pasteTitle')}</h3>
          <p class="text-text-muted mt-1 text-sm leading-relaxed">
            {t(isOidc ? 'wizard.idp.done.pasteOidc' : 'wizard.idp.done.pasteSaml')}
          </p>
          <div class="mt-3 flex flex-col gap-2">
            {isOidc ? (
              <CopyRow
                label={t('wizard.idp.done.redirectUri')}
                value={saved.redirectUri}
                copyLabel={t('common.copy')}
                onCopy={(v) => void copy(v)}
              />
            ) : (
              <>
                <CopyRow
                  label={t('wizard.idp.done.acsUrl')}
                  value={saved.acsUrl}
                  copyLabel={t('common.copy')}
                  onCopy={(v) => void copy(v)}
                />
                <CopyRow
                  label={t('wizard.idp.done.spEntityId')}
                  value={saved.spEntityId}
                  copyLabel={t('common.copy')}
                  onCopy={(v) => void copy(v)}
                />
                {saved.metadataUrl ? (
                  <CopyRow
                    label={t('wizard.idp.done.metadataUrl')}
                    value={saved.metadataUrl}
                    copyLabel={t('common.copy')}
                    onCopy={(v) => void copy(v)}
                  />
                ) : null}
              </>
            )}
          </div>
        </section>

        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.idp.done.nextTitle')}</h3>
          <ol class="mt-3 flex flex-col gap-3">
            <NextStep n={1} title={t('wizard.idp.done.step1Title')}>
              {t(isOidc ? 'wizard.idp.done.step1Oidc' : 'wizard.idp.done.step1Saml')}
            </NextStep>
            <NextStep n={2} title={t('wizard.idp.done.step2Title')}>
              {t('wizard.idp.done.step2Body')}
            </NextStep>
            <NextStep n={3} title={t('wizard.idp.done.step3Title')}>
              {t('wizard.idp.done.step3Body')}
            </NextStep>
          </ol>
        </section>

        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('wizard.idp.done.back')}
          </Button>
        </div>
      </div>
    ) : null,
  };

  const byId: Record<string, WizardStep> = {
    type: stepType,
    explain: stepExplain,
    configure: stepConfigure,
    test: stepTest,
    review: stepReview,
    done: stepDone,
  };

  return (
    <Wizard
      steps={ids.map((id) => byId[id] as WizardStep)}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={cancel}
    />
  );
}

// ---- Piezas reutilizables (las comparte RegisterOidcAppWizard) ----

export function NextStep({
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

export function CopyRow({
  label,
  value,
  copyLabel,
  onCopy,
}: {
  label: string;
  value: string;
  /** Texto del botón; ya traducido por quien lo usa (`t('common.copy')`). */
  copyLabel: string;
  onCopy: (value: string) => void;
}) {
  return (
    <div class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border px-3 py-2">
      <span class="sytue-meta w-full shrink-0 sm:w-52">{label}</span>
      <code class="text-text min-w-0 flex-1 font-mono text-xs break-all">{value || '—'}</code>
      <Button variant="ghost" size="sm" disabled={!value} onClick={() => onCopy(value)}>
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
