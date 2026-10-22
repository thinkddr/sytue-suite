/**
 * Onboarding — asistente de alta inicial de Sytue (el primer contacto).
 *
 * Cuatro o seis pasos según sea para una persona o para una organización. Cada
 * paso HACE algo de verdad contra Sytue ID y guarda el progreso al avanzar: si
 * cierras el navegador, vuelves donde lo dejaste con los datos que ya pusiste.
 *
 * Honestidad del copy (regla de la casa): aquí no se pinta nada que no funcione.
 * La disponibilidad del identificador se consulta contra la base de datos; la
 * dirección acme.sytue.eu se publica de verdad en el DNS y se enseña si está
 * publicada o no; el dominio propio se comprueba con una consulta DNS real y se
 * enseña el estado que devuelve (verificado / aún sin propagar / registro que no
 * coincide); la disponibilidad y el precio de un dominio nuevo salen del
 * registrador, en vivo; las invitaciones son invitaciones reales y, si el correo
 * no sale, se dice; los roles se crean con permisos reales; y el resumen final se
 * lee del servidor, no del formulario.
 *
 * Aquí NO se compran dominios y no hay ningún botón que lo insinúe: se enseña el
 * precio real y se dice en llano dónde registrarlo y cómo volver a conectarlo.
 *
 * Los textos salen del catálogo del portal (`ob.*`) y el idioma llega por props
 * desde el `.astro`: nunca se detecta aquí (desajustaría la hidratación).
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  Checkbox,
  EmptyState,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Spinner,
  Textarea,
  ToastProvider,
  Wizard,
  useToast,
  type BadgeTone,
  type ReviewItem,
  type WizardStep,
} from '@sytue/ui';
import { adminUrl } from '@sytue/config';
import { formatCurrency, type Locale } from '@sytue/i18n';
import { useT, type PortalMessageKey, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';

// ---- Forma de los datos que devuelve Sytue ID (BFF /api/onboarding) ----

type Mode = 'individual' | 'organization';

interface RecommendedRole {
  key: string;
  name: string;
  summary: string;
  permissions: string[];
}

/** Estado REAL del registro DNS de la dirección acme.sytue.eu (lo publica Sytue). */
interface SubdomainWire {
  host: string;
  /** live = el registro está publicado · failed = no se pudo · unconfigured = aquí no se publica */
  status: 'live' | 'failed' | 'unconfigured';
  detail?: string;
}

interface OrgWire {
  id: string;
  slug: string;
  name: string;
  subdomain: string;
  dns?: SubdomainWire;
}

/** Consulta en vivo al registrador: ¿está libre ese dominio y cuánto cuesta? */
interface AvailabilityWire {
  domain: string;
  status: 'available' | 'taken' | 'unsupported' | 'unknown' | 'unconfigured' | 'invalid';
  /** Siempre false: Sytue no vende dominios (ver la cabecera del fichero). */
  purchasable_here: boolean;
  price?: { currency: string; registration: string; renewal: string };
}

interface DomainWire {
  host: string;
  record_name: string;
  record_value: string;
  verified: boolean;
  verified_at?: string;
  status?: 'verified' | 'pending' | 'mismatch' | 'unreachable';
  found?: string[];
}

/**
 * Un registro del plan de Cloudflare. `state` es lo que Sytue ha VISTO en la zona
 * del cliente (o lo que ha hecho en ella), nunca una suposición.
 */
interface CloudflareRecordWire {
  key: string;
  type: string;
  name: string;
  content?: string;
  priority?: number;
  purpose: string;
  state: 'ok' | 'missing' | 'conflict' | 'created' | 'replaced' | 'failed' | 'pending';
  /** Lo que hay AHORA ocupando ese hueco (solo en los conflictos). */
  current?: string;
  detail?: string;
}

interface CloudflareWire {
  zone: { id: string; name: string; status: string };
  records: CloudflareRecordWire[];
  /** Cuántos registros se han tocado de verdad en esta pasada. */
  applied: number;
  /** Qué ha hecho Sytue con el token: guardarlo sellado o descartarlo. */
  token_stored: boolean;
  domain: DomainWire;
}

interface BillingWire {
  legal_name: string;
  tax_id: string;
  address: string;
  billing_email: string;
}

interface SubscriptionWire {
  status: 'pending' | 'active' | 'past_due' | 'canceled';
  seats: number;
  active_members: number;
  /** Precio por usuario que se cobrará (plan vigente, con el descuento educativo si lo hay). */
  unit_price_cents: number;
  currency: 'EUR';
  paid_until?: string | null;
  entitled: boolean;
}

interface InviteResult {
  email: string;
  status: 'sent' | 'already_member' | 'invalid' | 'not_sent' | 'failed';
  detail?: string;
}

interface State {
  mode: '' | Mode;
  step: string;
  completed: boolean;
  suite_domain: string;
  account?: { email: string; name: string };
  organization: OrgWire | null;
  domain?: DomainWire;
  billing?: BillingWire;
  invitations?: { email: string; role: string }[];
  roles?: { id: string; name: string; permissions: string[] }[];
  recommended_roles: RecommendedRole[];
}

// Identificadores de paso: los mismos que valida Sytue ID (domain/onboarding.go).
const STEP = {
  mode: 'modo',
  identity: 'identidad',
  people: 'personas',
  billing: 'facturacion',
  roles: 'roles',
  done: 'listo',
} as const;

// Solo los valores: la etiqueta de cada uno vive en el catálogo
// (`ob.roleOption.<valor>`), que es de donde sale traducida.
const ROLE_VALUES = ['member', 'admin', 'billing'] as const;

function stepIds(mode: Mode): string[] {
  return mode === 'individual'
    ? [STEP.mode, STEP.identity, STEP.billing, STEP.done]
    : [STEP.mode, STEP.identity, STEP.people, STEP.billing, STEP.roles, STEP.done];
}

/** Propone un identificador a partir del nombre (sin acentos ni símbolos). */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/g, '');
}

async function call(path: string, method: string, body?: unknown): Promise<unknown> {
  const res = await fetch(`/api/onboarding${path}`, {
    method,
    ...(body === undefined
      ? {}
      : { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }),
  });
  const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  if (!res.ok) {
    // Sin mensaje del servidor se lanza vacío a propósito: el texto de relleno lo
    // pone quien lo enseña, que es quien tiene la `t` del catálogo.
    throw new Error(typeof data.message === 'string' ? data.message : '');
  }
  return data;
}

function OnboardingWizard({ locale }: { locale: Locale }) {
  const t = useT(locale);
  const toast = useToast();
  const [state, setState] = useState<State | null>(null);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [step, setStep] = useState(0);

  // Formulario en curso (lo persistido manda; esto es solo lo que se escribe ahora).
  const [mode, setMode] = useState<Mode | null>(null);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [slugEdited, setSlugEdited] = useState(false);
  // Qué quiere hacer con su dirección: quedarse con la de Sytue, conectar un
  // dominio que ya tiene, o mirar cuánto costaría uno nuevo (solo consulta).
  const [domainChoice, setDomainChoice] = useState<'subdomain' | 'own' | 'search'>('subdomain');
  const [domainInput, setDomainInput] = useState('');
  // Dominio propio: quién crea los registros. Con Cloudflare los creamos nosotros
  // con un permiso que da el cliente; en cualquier otro caso se los damos para
  // copiar. El camino manual sigue estando siempre a mano.
  const [dnsHost, setDnsHost] = useState<'cloudflare' | 'manual'>('cloudflare');
  const [cfToken, setCfToken] = useState('');
  const [cfMail, setCfMail] = useState(false);
  const [cfRemember, setCfRemember] = useState(false);
  const [cfReplace, setCfReplace] = useState<string[]>([]);
  const [cfResult, setCfResult] = useState<CloudflareWire | null>(null);
  const [searchInput, setSearchInput] = useState('');
  const [availability, setAvailability] = useState<AvailabilityWire | null>(null);
  const [emails, setEmails] = useState('');
  const [inviteRole, setInviteRole] = useState('member');
  const [inviteResults, setInviteResults] = useState<InviteResult[] | null>(null);
  const [billing, setBilling] = useState<BillingWire>({
    legal_name: '',
    tax_id: '',
    address: '',
    billing_email: '',
  });
  const [subscription, setSubscription] = useState<SubscriptionWire | null>(null);
  const [seats, setSeats] = useState(1);
  const [paymentReturned, setPaymentReturned] = useState(false);
  const [chosenRoles, setChosenRoles] = useState<string[]>([]);
  const [slugCheck, setSlugCheck] = useState<{
    slug: string;
    available: boolean;
    reason?: string;
    subdomain?: string;
  } | null>(null);

  const effMode: Mode = mode ?? 'organization';
  const ids = useMemo(() => stepIds(effMode), [effMode]);
  const org = state?.organization ?? null;
  const isOrg = effMode === 'organization';

  // --- Carga inicial: retoma donde lo dejó ---
  useEffect(() => {
    void (async () => {
      try {
        const returned = new URLSearchParams(window.location.search).get('payment') === 'return';
        setPaymentReturned(returned);
        const s = (await call('', 'GET')) as State;
        setState(s);
        if (s.mode) setMode(s.mode);
        if (s.organization) {
          setName(s.organization.name);
          setSlug(s.organization.slug);
          setSlugEdited(true);
          let sub = (await call('/subscription', 'GET')) as SubscriptionWire;
          // El redirect del checkout puede ganar al webhook. Esperar unos
          // segundos evita ofrecer un segundo cobro mientras Mollie entrega el
          // primero; el usuario siempre conserva el botón para reintentar.
          for (let attempt = 0; returned && !sub.entitled && attempt < 6; attempt += 1) {
            await new Promise((resolve) => window.setTimeout(resolve, 1500));
            sub = (await call('/subscription', 'GET')) as SubscriptionWire;
          }
          setSubscription(sub);
          setSeats(Math.max(sub.seats, sub.active_members, 1));
        }
        if (s.domain) {
          setDomainChoice('own');
          setDomainInput(s.domain.host);
        }
        if (s.billing) setBilling(s.billing);
        if (!s.account) return;
        setBilling((b) => (b.billing_email ? b : { ...b, billing_email: s.account?.email ?? '' }));
        if (!s.organization && s.mode === 'individual') setName(s.account.name);
        // Retomar: colocarse en el paso guardado (o en el primero pendiente).
        const target = stepIds(s.mode === 'individual' ? 'individual' : 'organization').indexOf(
          s.completed ? STEP.done : s.step,
        );
        if (target > 0) setStep(target);
      } catch {
        setFailed(true);
      }
    })();
  }, []);

  // --- Disponibilidad del identificador, en vivo y contra la BD ---
  const slugTimerRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (org) return; // ya creada: el identificador es inmutable
    const v = slug.trim().toLowerCase();
    setSlugCheck(null);
    if (!v) return;
    window.clearTimeout(slugTimerRef.current);
    slugTimerRef.current = window.setTimeout(() => {
      void (async () => {
        try {
          const r = (await call(`/slug?slug=${encodeURIComponent(v)}`, 'GET')) as {
            slug: string;
            available: boolean;
            reason?: string;
            subdomain?: string;
          };
          setSlugCheck(r);
        } catch {
          setSlugCheck(null);
        }
      })();
    }, 350);
    return () => window.clearTimeout(slugTimerRef.current);
  }, [slug, org]);

  /** Guarda el paso alcanzado (lo que hace real la reanudación) y avanza. */
  async function saveAndGo(next: string, extra?: Record<string, unknown>) {
    await call('', 'PATCH', { mode: mode ?? '', step: next, ...extra });
    setStep(ids.indexOf(next));
  }

  function fail(e: unknown) {
    toast.notify({
      tone: 'danger',
      title: t('ob.saveFailedTitle'),
      // El mensaje del servidor manda cuando lo hay (lo traduce Sytue ID); si no,
      // el genérico del catálogo.
      description: e instanceof Error ? e.message || t('ob.error.generic') : t('ob.saveFailedBody'),
    });
  }

  async function run(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  }

  /** Relee del servidor lo que REALMENTE existe (el resumen no se fía del formulario). */
  async function refresh(): Promise<State> {
    const s = (await call('', 'GET')) as State;
    setState(s);
    return s;
  }

  async function refreshSubscription(): Promise<SubscriptionWire> {
    const sub = (await call('/subscription', 'GET')) as SubscriptionWire;
    setSubscription(sub);
    setSeats((current) => Math.max(current, sub.active_members, 1));
    return sub;
  }

  // --- Acciones reales de cada paso ---

  async function createOrg() {
    const created = (await call('/organization', 'POST', {
      name: name.trim(),
      slug: slug.trim().toLowerCase(),
    })) as OrgWire;
    setState((s) => (s ? { ...s, organization: created } : s));
    await refreshSubscription();
    return created;
  }

  /** Consulta REAL al registrador. No reserva ni compra: solo pregunta. */
  async function checkAvailability() {
    const q = searchInput.trim();
    if (!q) return;
    setAvailability(null);
    setAvailability(
      (await call(
        `/domain-availability?domain=${encodeURIComponent(q)}`,
        'GET',
      )) as AvailabilityWire,
    );
  }

  async function checkDomain() {
    const d = (await call('/domain', 'POST', { domain: domainInput.trim() })) as DomainWire;
    setState((s) => (s ? { ...s, domain: d } : s));
    // El servidor devuelve el dominio ya normalizado (sin https:// ni www.): se
    // refleja en el campo para que lo que se ve sea lo que se comprobó.
    setDomainInput(d.host);
    if (d.status === 'verified') {
      toast.notify({ tone: 'success', title: t('ob.domain.verified', { host: d.host }) });
    }
  }

  /**
   * Configura el dominio en la cuenta de Cloudflare DEL CLIENTE. En dos tiempos:
   * `apply:false` solo mira y devuelve el plan (qué ya está, qué falta y qué
   * choca); `apply:true` lo aplica, y solo sustituye lo que él haya marcado.
   *
   * El token se envía por el BFF (jamás se guarda en el navegador) y se borra del
   * formulario en cuanto se ha usado, salvo que él haya pedido recordarlo.
   */
  async function runCloudflare(apply: boolean) {
    const out = (await call('/domain/cloudflare', 'POST', {
      domain: domainInput.trim(),
      token: cfToken.trim(),
      mail: cfMail,
      apply,
      replace: apply ? cfReplace : [],
      remember_token: cfRemember,
    })) as CloudflareWire;
    setCfResult(out);
    setState((s) => (s ? { ...s, domain: out.domain } : s));
    setDomainInput(out.domain.host);
    if (!apply) return;
    if (!cfRemember) setCfToken('');
    const failedCount = out.records.filter((r) => r.state === 'failed').length;
    if (failedCount > 0) {
      toast.notify({
        tone: 'warning',
        title: t('ob.cf.someFailed', { count: failedCount }),
        description: t('ob.cf.someFailedBody'),
      });
    } else if (out.domain.verified) {
      toast.notify({
        tone: 'success',
        title: t('ob.cf.configured', { host: out.domain.host }),
      });
    } else {
      toast.notify({ tone: 'success', title: t('ob.cf.created') });
    }
  }

  async function sendInvites() {
    const out = (await call('/invitations', 'POST', {
      emails,
      role: inviteRole,
    })) as { sent: number; results: InviteResult[] };
    setInviteResults(out.results);
    if (out.sent > 0) setEmails('');
    await refresh();
  }

  async function createRoles() {
    await call('/roles', 'POST', { keys: chosenRoles });
    await refresh();
  }

  /** `what` es la clave del catálogo que dice QUÉ se ha copiado. */
  async function copy(text: string, what: PortalMessageKey) {
    try {
      await navigator.clipboard.writeText(text);
      toast.notify({ tone: 'success', title: t(what) });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('ob.copyFailTitle'),
        description: t('ob.copyFailBody'),
      });
    }
  }

  // --- Estados de carga / error ---

  if (failed) {
    return (
      <EmptyState
        title={t('ob.failedTitle')}
        description={t('ob.failedBody')}
        illustration={<Icon name="danger" size={32} />}
        action={
          <Button variant="primary" onClick={() => window.location.reload()}>
            {t('ob.retry')}
          </Button>
        }
      />
    );
  }
  if (!state) {
    return (
      <div class="flex items-center justify-center gap-3 py-16">
        <Spinner />
        <p class="text-text-muted text-sm">{t('ob.preparing')}</p>
      </div>
    );
  }

  const suite = state.suite_domain;
  const domainState = state.domain;

  // ---- Paso 1: ¿para ti o para tu organización? ----

  const stepMode: WizardStep = {
    id: STEP.mode,
    title: t('ob.mode.title'),
    heading: t('ob.mode.heading'),
    description: t('ob.mode.description'),
    canContinue: mode !== null,
    onNext: () => void run(() => saveAndGo(STEP.identity)),
    content: (
      <ul class="flex max-w-2xl flex-col gap-3">
        <li>
          <ChoiceCard
            name="sytue-onboarding-mode"
            value="individual"
            icon="user"
            title={t('ob.mode.individual.title')}
            description={t('ob.mode.individual.body')}
            selected={mode === 'individual'}
            onSelect={() => setMode('individual')}
          />
        </li>
        <li>
          <ChoiceCard
            name="sytue-onboarding-mode"
            value="organization"
            icon="building"
            title={t('ob.mode.org.title')}
            description={t('ob.mode.org.body')}
            badge={<Badge tone="neutral">{t('ob.mode.org.badge')}</Badge>}
            selected={mode === 'organization'}
            onSelect={() => setMode('organization')}
          />
        </li>
      </ul>
    ),
  };

  // ---- Paso 2: identidad y dirección ----

  const slugValue = slug.trim().toLowerCase();
  const slugOK = Boolean(org) || slugCheck?.available === true;
  const identityReady = name.trim().length > 0 && slugOK;

  const stepIdentity: WizardStep = {
    id: STEP.identity,
    title: isOrg ? t('ob.identity.titleOrg') : t('ob.identity.titleMe'),
    heading: isOrg ? t('ob.identity.headingOrg') : t('ob.identity.headingMe'),
    description: org ? t('ob.identity.descriptionCreated') : t('ob.identity.description'),
    canContinue: identityReady,
    nextLabel: org
      ? t('ob.identity.nextCreated')
      : domainChoice === 'own'
        ? t('ob.identity.nextOwnDomain')
        : t('ob.identity.next'),
    onNext: () =>
      void run(async () => {
        const first = org === null;
        if (first || org.name !== name.trim()) await createOrg();
        // Con dominio propio nos quedamos en este paso justo después de crear el
        // espacio: es cuando podemos enseñar el registro EXACTO que hay que añadir
        // (antes no existe token). El siguiente clic ya avanza.
        if (first && domainChoice === 'own') return;
        await saveAndGo(isOrg ? STEP.people : STEP.billing);
      }),
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('ob.identity.nameLabel')} htmlFor="ob-name" required>
          <Input
            id="ob-name"
            value={name}
            placeholder={
              isOrg ? t('ob.identity.namePlaceholderOrg') : t('ob.identity.namePlaceholderMe')
            }
            aria-describedby="ob-name-hint"
            onInput={(e) => {
              const v = e.currentTarget.value;
              setName(v);
              if (!slugEdited && !org) setSlug(slugify(v));
            }}
          />
          <FieldHint id="ob-name-hint">
            {isOrg ? t('ob.identity.nameHintOrg') : t('ob.identity.nameHintMe')}
          </FieldHint>
        </FieldGroup>

        <FieldGroup label={t('ob.identity.slugLabel')} htmlFor="ob-slug" required>
          <div class="flex items-stretch">
            <Input
              id="ob-slug"
              value={slug}
              class="rounded-r-none font-mono"
              placeholder={t('ob.identity.slugPlaceholder')}
              autocomplete="off"
              spellcheck={false}
              disabled={Boolean(org)}
              aria-describedby="ob-slug-hint"
              invalid={Boolean(slugValue) && slugCheck?.available === false}
              onInput={(e) => {
                setSlugEdited(true);
                setSlug(e.currentTarget.value.toLowerCase());
              }}
            />
            <span class="border-border-strong bg-bg-subtle text-text-muted -ml-px flex items-center rounded-r-md border px-3 font-mono text-sm">
              .{suite}
            </span>
          </div>
          <FieldHint
            id="ob-slug-hint"
            tone={
              org
                ? 'success'
                : !slugValue
                  ? 'neutral'
                  : slugCheck === null
                    ? 'neutral'
                    : slugCheck.available
                      ? 'success'
                      : 'danger'
            }
          >
            {org
              ? t('ob.identity.slugCreated', { subdomain: org.subdomain })
              : !slugValue
                ? t('ob.identity.slugEmpty', { suite })
                : slugCheck === null
                  ? t('ob.identity.slugChecking')
                  : slugCheck.available
                    ? t('ob.identity.slugFree', { subdomain: slugCheck.subdomain ?? '' })
                    : // El motivo lo da Sytue ID (ya traducido allí); si no lo manda, el catálogo.
                      (slugCheck.reason ?? t('ob.identity.slugTaken'))}
          </FieldHint>
        </FieldGroup>

        {/* Estado REAL de la dirección: no se dice «funciona» hasta que el
            registro está publicado de verdad en el DNS. */}
        {org?.dns ? <SubdomainStatus t={t} dns={org.dns} /> : null}

        <fieldset class="flex flex-col gap-3">
          <legend class="text-text mb-1 text-sm font-semibold">{t('ob.domain.legend')}</legend>
          <ChoiceCard
            name="sytue-onboarding-domain"
            value="subdomain"
            icon="rocket"
            title={t('ob.domain.keep.title', {
              host: `${slugValue || t('ob.domain.keep.placeholder')}.${suite}`,
            })}
            description={t('ob.domain.keep.body')}
            selected={domainChoice === 'subdomain'}
            onSelect={() => setDomainChoice('subdomain')}
          />
          <ChoiceCard
            name="sytue-onboarding-domain"
            value="own"
            icon="globe"
            title={t('ob.domain.own.title')}
            description={t('ob.domain.own.body')}
            selected={domainChoice === 'own'}
            onSelect={() => setDomainChoice('own')}
          />
          <ChoiceCard
            name="sytue-onboarding-domain"
            value="search"
            icon="search"
            title={t('ob.domain.search.title')}
            description={t('ob.domain.search.body')}
            selected={domainChoice === 'search'}
            onSelect={() => setDomainChoice('search')}
          />
        </fieldset>

        {domainChoice === 'own' ? (
          org ? (
            <div class="flex flex-col gap-4">
              <FieldGroup label={t('ob.domain.hostLabel')} htmlFor="ob-domain">
                <Input
                  id="ob-domain"
                  value={domainInput}
                  class="font-mono"
                  placeholder={t('ob.domain.hostPlaceholder')}
                  autocomplete="off"
                  spellcheck={false}
                  aria-describedby="ob-domain-hint"
                  onInput={(e) => setDomainInput(e.currentTarget.value)}
                />
                <FieldHint id="ob-domain-hint">{t('ob.domain.hostHint')}</FieldHint>
              </FieldGroup>

              <fieldset class="flex flex-col gap-3">
                <legend class="text-text mb-1 text-sm font-semibold">
                  {t('ob.dns.legend', { domain: domainInput.trim() || t('ob.dns.fallback') })}
                </legend>
                <ChoiceCard
                  name="sytue-onboarding-dnshost"
                  value="cloudflare"
                  icon="cloud"
                  title={t('ob.dns.cf.title')}
                  description={t('ob.dns.cf.body')}
                  badge={<Badge tone="success">{t('ob.dns.cf.badge')}</Badge>}
                  selected={dnsHost === 'cloudflare'}
                  onSelect={() => setDnsHost('cloudflare')}
                />
                <ChoiceCard
                  name="sytue-onboarding-dnshost"
                  value="manual"
                  icon="globe"
                  title={t('ob.dns.manual.title')}
                  description={t('ob.dns.manual.body')}
                  selected={dnsHost === 'manual'}
                  onSelect={() => setDnsHost('manual')}
                />
              </fieldset>

              {dnsHost === 'cloudflare' ? (
                <CloudflarePanel
                  t={t}
                  domain={domainInput}
                  token={cfToken}
                  onToken={setCfToken}
                  mail={cfMail}
                  onMail={setCfMail}
                  remember={cfRemember}
                  onRemember={setCfRemember}
                  replace={cfReplace}
                  onToggleReplace={(key) =>
                    setCfReplace((keys) =>
                      keys.includes(key) ? keys.filter((k) => k !== key) : [...keys, key],
                    )
                  }
                  result={cfResult}
                  busy={busy}
                  onPreview={() => void run(() => runCloudflare(false))}
                  onApply={() => void run(() => runCloudflare(true))}
                  onCopy={copy}
                />
              ) : null}

              {/* Plan B: la tabla copiable está SIEMPRE disponible, tanto si el
                  dominio no está en Cloudflare como si prefiere no dar un token. */}
              {dnsHost === 'cloudflare' ? (
                <details class="border-border-subtle rounded-md border p-3">
                  <summary class="text-text cursor-pointer text-sm font-medium">
                    {t('ob.dns.manualSummary')}
                  </summary>
                  <div class="mt-3">
                    <DomainPanel
                      t={t}
                      state={domainState}
                      busy={busy}
                      onCheck={() => void run(checkDomain)}
                      onCopy={copy}
                    />
                  </div>
                </details>
              ) : (
                <DomainPanel
                  t={t}
                  state={domainState}
                  busy={busy}
                  onCheck={() => void run(checkDomain)}
                  onCopy={copy}
                />
              )}
            </div>
          ) : (
            <InlineHelp tone="info" title={t('ob.identity.firstTitle')}>
              {t('ob.identity.firstBody')}
            </InlineHelp>
          )
        ) : null}

        {domainChoice === 'search' ? (
          <DomainSearchPanel
            t={t}
            locale={locale}
            value={searchInput}
            onChange={setSearchInput}
            result={availability}
            busy={busy}
            onCheck={() => void run(checkAvailability)}
            onUseOwn={() => {
              setDomainChoice('own');
              setDomainInput(availability?.domain ?? searchInput.trim());
            }}
          />
        ) : null}
      </div>
    ),
  };

  // ---- Paso 3: primeras personas (solo organización) ----

  const stepPeople: WizardStep = {
    id: STEP.people,
    title: t('ob.people.title'),
    heading: t('ob.people.heading'),
    description: t('ob.people.description'),
    nextLabel: t('ob.people.next'),
    onNext: () => void run(() => saveAndGo(STEP.billing)),
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        <FieldGroup label={t('ob.people.emailsLabel')} htmlFor="ob-emails">
          <Textarea
            id="ob-emails"
            rows={3}
            value={emails}
            placeholder={t('ob.people.emailsPlaceholder')}
            spellcheck={false}
            aria-describedby="ob-emails-hint"
            onInput={(e) => setEmails(e.currentTarget.value)}
          />
          <FieldHint id="ob-emails-hint">{t('ob.people.emailsHint')}</FieldHint>
        </FieldGroup>

        <FieldGroup label={t('ob.people.roleLabel')} htmlFor="ob-role">
          <Select
            id="ob-role"
            value={inviteRole}
            aria-describedby="ob-role-hint"
            onChange={(e) => setInviteRole(e.currentTarget.value)}
          >
            {ROLE_VALUES.map((v) => (
              <option key={v} value={v}>
                {t(`ob.roleOption.${v}`)}
              </option>
            ))}
          </Select>
          <FieldHint id="ob-role-hint">{t('ob.people.roleHint')}</FieldHint>
        </FieldGroup>

        <div class="flex flex-wrap items-center gap-3">
          <Button
            variant="primary"
            loading={busy}
            disabled={!emails.trim()}
            onClick={() => void run(sendInvites)}
          >
            {t('ob.people.send')}
          </Button>
          <p class="sytue-meta">{t('ob.people.sendNote')}</p>
        </div>

        {inviteResults ? <InviteResults t={t} results={inviteResults} /> : null}

        {state.invitations && state.invitations.length > 0 ? (
          <div class="border-border bg-surface-raised rounded-lg border p-3">
            <p class="text-text text-sm font-semibold">{t('ob.people.pendingTitle')}</p>
            <ul class="mt-2 flex flex-col gap-1">
              {state.invitations.map((i) => (
                <li key={i.email} class="text-text-muted flex items-center gap-2 text-sm">
                  <Icon name="check" size={14} class="text-success-text shrink-0" />
                  <span class="font-mono text-xs">{i.email}</span>
                  <span class="sytue-meta">· {roleLabel(t, i.role)}</span>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <EmptyState
            class="border-border-subtle rounded-lg border border-dashed"
            title={t('ob.people.emptyTitle')}
            description={t('ob.people.emptyBody')}
            illustration={<Icon name="users" size={28} />}
          />
        )}
      </div>
    ),
  };

  // ---- Paso 4: contacto y facturación ----

  const stepBilling: WizardStep = {
    id: STEP.billing,
    title: t('ob.billing.title'),
    heading: t('ob.billing.heading'),
    description: t('ob.billing.description'),
    nextLabel: subscription?.entitled
      ? t('ob.billing.next')
      : paymentReturned
        ? t('ob.billing.verify')
        : t('ob.billing.pay'),
    canContinue: Boolean(org) && seats >= Math.max(1, subscription?.active_members ?? 1),
    onNext: () =>
      void run(async () => {
        await call('', 'PATCH', { mode: mode ?? '', step: STEP.billing, billing });
        const current = await refreshSubscription();
        if (!current.entitled) {
          if (paymentReturned) throw new Error(t('ob.billing.pendingBody'));
          const checkout = (await call('/subscription/checkout', 'POST', { seats })) as {
            checkout_url: string;
          };
          window.location.assign(checkout.checkout_url);
          return;
        }
        await saveAndGo(isOrg ? STEP.roles : STEP.done);
      }),
    content: (
      <div class="flex max-w-xl flex-col gap-5">
        {isOrg ? (
          <>
            <FieldGroup label={t('ob.billing.legalLabel')} htmlFor="ob-legal">
              <Input
                id="ob-legal"
                value={billing.legal_name}
                placeholder={t('ob.billing.legalPlaceholder')}
                aria-describedby="ob-legal-hint"
                onInput={(e) => setBilling({ ...billing, legal_name: e.currentTarget.value })}
              />
              <FieldHint id="ob-legal-hint">{t('ob.billing.legalHint')}</FieldHint>
            </FieldGroup>
            <FieldGroup label={t('ob.billing.taxLabel')} htmlFor="ob-tax">
              <Input
                id="ob-tax"
                value={billing.tax_id}
                class="font-mono"
                placeholder={t('ob.billing.taxPlaceholder')}
                autocomplete="off"
                aria-describedby="ob-tax-hint"
                onInput={(e) => setBilling({ ...billing, tax_id: e.currentTarget.value })}
              />
              <FieldHint id="ob-tax-hint">{t('ob.billing.taxHint')}</FieldHint>
            </FieldGroup>
            <FieldGroup label={t('ob.billing.addressLabel')} htmlFor="ob-address">
              <Textarea
                id="ob-address"
                rows={2}
                value={billing.address}
                placeholder={t('ob.billing.addressPlaceholder')}
                aria-describedby="ob-address-hint"
                onInput={(e) => setBilling({ ...billing, address: e.currentTarget.value })}
              />
              <FieldHint id="ob-address-hint">{t('ob.billing.addressHint')}</FieldHint>
            </FieldGroup>
          </>
        ) : null}
        <FieldGroup label={t('ob.billing.emailLabel')} htmlFor="ob-billmail">
          <Input
            id="ob-billmail"
            type="email"
            value={billing.billing_email}
            placeholder={state.account?.email ?? t('ob.billing.emailPlaceholder')}
            autocomplete="email"
            aria-describedby="ob-billmail-hint"
            onInput={(e) => setBilling({ ...billing, billing_email: e.currentTarget.value })}
          />
          <FieldHint id="ob-billmail-hint">{t('ob.billing.emailHint')}</FieldHint>
        </FieldGroup>
        <FieldGroup label={t('ob.billing.seatsLabel')} htmlFor="ob-seats" required>
          <Input
            id="ob-seats"
            type="number"
            min={Math.max(1, subscription?.active_members ?? 1)}
            max={10000}
            step={1}
            value={String(seats)}
            onInput={(e) => {
              const value = Number.parseInt(e.currentTarget.value, 10);
              if (Number.isFinite(value)) setSeats(Math.min(10000, Math.max(1, value)));
            }}
          />
          {/* El precio lo dice Sytue ID (unit_price_cents), no el portal: aquí
              ponía 6 € fijos cuando el asiento ya costaba 5 €, y a una institución
              educativa le cobra la mitad. Sin contrato cargado, no se inventa. */}
          {subscription ? (
            <FieldHint>
              {t('ob.billing.seatsHint', {
                seats,
                price: formatCurrency(locale, subscription.unit_price_cents / 100, 'EUR'),
                total: formatCurrency(locale, (seats * subscription.unit_price_cents) / 100, 'EUR'),
              })}
            </FieldHint>
          ) : null}
        </FieldGroup>
        {subscription?.entitled ? (
          <InlineHelp tone="success" title={t('ob.billing.active')}>
            {t('ob.billing.activeBody')}
          </InlineHelp>
        ) : paymentReturned ? (
          <InlineHelp tone="warning" title={t('ob.billing.pending')}>
            {t('ob.billing.pendingBody')}
          </InlineHelp>
        ) : null}
        <InlineHelp tone="neutral" hideIcon>
          {t('ob.billing.note')}
        </InlineHelp>
      </div>
    ),
  };

  // ---- Paso 5: roles recomendados (solo organización) ----

  const created = new Set((state.roles ?? []).map((r) => r.name));

  const stepRoles: WizardStep = {
    id: STEP.roles,
    title: t('ob.roles.title'),
    heading: t('ob.roles.heading'),
    description: t('ob.roles.description'),
    nextLabel: t('ob.roles.next'),
    onNext: () =>
      void run(async () => {
        const pending = chosenRoles.filter(
          (k) => !created.has(state.recommended_roles.find((r) => r.key === k)?.name ?? ''),
        );
        if (pending.length > 0) await createRoles();
        await saveAndGo(STEP.done);
      }),
    content: (
      <div class="flex max-w-2xl flex-col gap-3">
        {state.recommended_roles.map((r) => {
          const already = created.has(r.name);
          const checked = already || chosenRoles.includes(r.key);
          return (
            <div
              key={r.key}
              class="border-border bg-surface has-[:checked]:border-primary rounded-lg border p-3 transition-colors"
            >
              {/* El desplegable de permisos va FUERA de la etiqueta: dentro,
                  abrirlo marcaría la casilla sin querer. */}
              <Checkbox
                id={`ob-role-${r.key}`}
                class="items-start"
                label={r.name}
                checked={checked}
                disabled={already || busy}
                onChange={(e) =>
                  setChosenRoles((prev) =>
                    e.currentTarget.checked
                      ? [...new Set([...prev, r.key])]
                      : prev.filter((k) => k !== r.key),
                  )
                }
              />
              {already ? (
                <Badge tone="success" class="ml-2">
                  {t('ob.roles.already')}
                </Badge>
              ) : null}
              <div class="mt-1 pl-6">
                <p class="text-text-muted text-sm leading-snug">{r.summary}</p>
                <details class="mt-1.5">
                  <summary class="sytue-meta hover:text-text-muted cursor-pointer">
                    {t('ob.roles.seePermissions')}
                  </summary>
                  <ul class="mt-1 flex flex-wrap gap-1">
                    {r.permissions.map((p) => (
                      <li
                        key={p}
                        class="bg-bg-subtle text-text-muted rounded px-1.5 py-0.5 font-mono text-xs"
                      >
                        {p}
                      </li>
                    ))}
                  </ul>
                </details>
              </div>
            </div>
          );
        })}
        <InlineHelp tone="info" title={t('ob.roles.helpTitle')}>
          {t('ob.roles.helpBody')}
        </InlineHelp>
      </div>
    ),
  };

  // ---- Paso final: resumen de lo que hay DE VERDAD ----

  const summary: ReviewItem[] = [
    {
      label: isOrg ? t('ob.sum.org') : t('ob.sum.account'),
      value: org?.name ?? '—',
      onEdit: () => setStep(ids.indexOf(STEP.identity)),
    },
    {
      label: t('ob.sum.address'),
      // Publicada = el registro DNS existe de verdad, no que el nombre esté guardado.
      value: org
        ? org.dns?.status === 'live'
          ? t('ob.sum.addressLive', { host: org.subdomain })
          : org.dns?.status === 'failed'
            ? t('ob.sum.addressPending', { host: org.subdomain })
            : org.subdomain
        : '—',
      mono: true,
    },
    {
      label: t('ob.sum.ownDomain'),
      value: domainState
        ? domainState.verified
          ? t('ob.sum.domainVerified', { host: domainState.host })
          : t('ob.sum.domainPending', { host: domainState.host })
        : t('ob.sum.noDomain'),
      onEdit: () => setStep(ids.indexOf(STEP.identity)),
    },
    ...(isOrg
      ? [
          {
            label: t('ob.sum.invitations'),
            value:
              state.invitations && state.invitations.length > 0
                ? t('ob.sum.invitationsPending', { count: state.invitations.length })
                : t('ob.sum.noInvitations'),
            onEdit: () => setStep(ids.indexOf(STEP.people)),
          },
        ]
      : []),
    {
      label: t('ob.sum.billing'),
      value:
        state.billing && (state.billing.legal_name || state.billing.billing_email)
          ? (state.billing.legal_name ?? '') || (state.billing.billing_email ?? '')
          : t('ob.sum.billingEmpty'),
      onEdit: () => setStep(ids.indexOf(STEP.billing)),
    },
    ...(isOrg
      ? [
          {
            label: t('ob.sum.roles'),
            // Los nombres de los roles los pone Sytue ID, no el catálogo.
            value:
              state.roles && state.roles.length > 0
                ? state.roles.map((r) => r.name).join(', ')
                : t('ob.sum.noRoles'),
            onEdit: () => setStep(ids.indexOf(STEP.roles)),
          },
        ]
      : []),
  ];

  const stepDone: WizardStep = {
    id: STEP.done,
    title: t('ob.done.title'),
    heading: state.completed ? t('ob.done.headingCompleted') : t('ob.done.heading'),
    description: t('ob.done.description'),
    nextLabel: t('ob.done.next'),
    onNext: () =>
      void run(async () => {
        await call('', 'PATCH', { step: STEP.done, completed: true });
        // El token de bootstrap no lleva tenant. Cerrar la sesión y volver a
        // entrar obliga a Sytue ID a emitir uno nuevo tras comprobar el pago.
        window.location.href = r('/auth/logout');
      }),
    content: (
      <div class="flex flex-col gap-5">
        <ReviewSummary items={summary} editLabel={t('ob.sum.edit')} />

        {domainState && !domainState.verified ? (
          <InlineHelp tone="warning" title={t('ob.done.warnTitle', { host: domainState.host })}>
            {t('ob.done.warnBody', {
              subdomain: org?.subdomain ?? '',
              step: isOrg ? t('ob.identity.titleOrg') : t('ob.identity.titleMe'),
            })}
          </InlineHelp>
        ) : null}

        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('ob.done.nextTitle')}</h3>
          <ul class="mt-3 flex flex-col gap-3">
            <NextLink
              icon="rocket"
              // Sin org SIEMPRE (ADR 0056): esta isla vive en el alta SIN
              // organización activa y en una página `sinOrg`; `orgActual`
              // confundiría `bienvenida` con una org. La forma sin org sirve
              // siempre y la canónica entra sola cuando la hay.
              href={r('/dispositivos', null)}
              title={t('ob.next.devices.title')}
              body={t('ob.next.devices.body')}
            />
            <NextLink
              icon="cloud"
              href={adminUrl('migrations')}
              title={t('ob.next.migrations.title')}
              body={t('ob.next.migrations.body')}
            />
            <NextLink
              icon="grid"
              href={adminUrl('apps')}
              title={t('ob.next.apps.title')}
              body={t('ob.next.apps.body')}
            />
            {isOrg ? (
              <NextLink
                icon="users"
                href={adminUrl('users')}
                title={t('ob.next.users.title')}
                body={t('ob.next.users.body')}
              />
            ) : null}
          </ul>
        </section>
      </div>
    ),
  };

  const byId: Record<string, WizardStep> = {
    [STEP.mode]: stepMode,
    [STEP.identity]: stepIdentity,
    [STEP.people]: stepPeople,
    [STEP.billing]: stepBilling,
    [STEP.roles]: stepRoles,
    [STEP.done]: stepDone,
  };

  return (
    <Wizard
      steps={ids.map((id) => byId[id] as WizardStep)}
      current={Math.min(step, ids.length - 1)}
      onCurrentChange={setStep}
      busy={busy}
    />
  );
}

// ---- Piezas del paso de dominio ----

/**
 * Estado REAL de la dirección de Sytue. El identificador guardado NO basta para
 * decir que funciona: lo que se enseña aquí es si el registro DNS está publicado.
 */
function SubdomainStatus({ t, dns }: { t: T; dns: SubdomainWire }) {
  if (dns.status === 'live') {
    return (
      <InlineHelp tone="success" title={t('ob.sub.liveTitle')}>
        {t('ob.sub.liveBody', { host: dns.host })}
      </InlineHelp>
    );
  }
  if (dns.status === 'failed') {
    return (
      <InlineHelp tone="warning" title={t('ob.sub.failedTitle')}>
        {/* El motivo lo da Sytue ID; la coletilla de qué hacer ahora, el catálogo. */}
        {dns.detail ?? t('ob.sub.failedDefault')} {t('ob.sub.failedTail')}
      </InlineHelp>
    );
  }
  // «unconfigured»: no afirmamos que resuelva. Se dice qué hay y de quién depende.
  return (
    <InlineHelp tone="neutral" title={t('ob.sub.reservedTitle')}>
      {t('ob.sub.reservedBody', { host: dns.host })}
    </InlineHelp>
  );
}

/** Importe tal cual lo da el registrador, con el formato del idioma activo. */
function money(locale: Locale, amount: string, currency: string): string {
  const n = Number(amount);
  // Si el registrador no manda un número o una moneda que `Intl` conozca, se
  // enseña su respuesta literal antes que un importe inventado.
  if (!Number.isFinite(n)) return `${amount} ${currency}`;
  try {
    return formatCurrency(locale, n, currency);
  } catch {
    return `${amount} ${currency}`;
  }
}

/**
 * Consulta de disponibilidad y precio de un dominio que el cliente aún no tiene.
 * Es SOLO una consulta: aquí no se compra nada y se dice claramente. El camino
 * real —registrarlo donde prefiera y volver a conectarlo— está a la vista.
 */
function DomainSearchPanel({
  t,
  locale,
  value,
  onChange,
  result,
  busy,
  onCheck,
  onUseOwn,
}: {
  t: T;
  locale: Locale;
  value: string;
  onChange: (v: string) => void;
  result: AvailabilityWire | null;
  busy: boolean;
  onCheck: () => void;
  onUseOwn: () => void;
}) {
  return (
    <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
      <FieldGroup label={t('ob.search.label')} htmlFor="ob-domain-search">
        <Input
          id="ob-domain-search"
          value={value}
          class="font-mono"
          placeholder={t('ob.search.placeholder')}
          autocomplete="off"
          spellcheck={false}
          aria-describedby="ob-domain-search-hint"
          onInput={(e) => onChange(e.currentTarget.value)}
        />
        <FieldHint id="ob-domain-search-hint">{t('ob.search.hint')}</FieldHint>
      </FieldGroup>

      {result ? <AvailabilityResult t={t} locale={locale} result={result} /> : null}

      <div class="flex flex-wrap items-center gap-3">
        <Button variant="primary" loading={busy} disabled={!value.trim()} onClick={onCheck}>
          {t('ob.search.check')}
        </Button>
        {result?.status === 'available' ? (
          <Button variant="secondary" onClick={onUseOwn}>
            {t('ob.search.useOwn')}
          </Button>
        ) : null}
      </div>

      <InlineHelp tone="neutral" hideIcon>
        {t('ob.search.note')}
      </InlineHelp>
    </div>
  );
}

/** Lo que respondió el registrador, dicho en llano. Nunca se afirma de más. */
function AvailabilityResult({
  t,
  locale,
  result,
}: {
  t: T;
  locale: Locale;
  result: AvailabilityWire;
}) {
  switch (result.status) {
    case 'available':
      return (
        <InlineHelp tone="success" title={t('ob.avail.freeTitle', { domain: result.domain })}>
          {result.price
            ? t('ob.avail.freePrice', {
                registration: money(locale, result.price.registration, result.price.currency),
                renewal: money(locale, result.price.renewal, result.price.currency),
              })
            : t('ob.avail.freeNoPrice')}
        </InlineHelp>
      );
    case 'taken':
      return (
        <InlineHelp tone="warning" title={t('ob.avail.takenTitle', { domain: result.domain })}>
          {t('ob.avail.takenBody')}
        </InlineHelp>
      );
    case 'unsupported':
      return (
        <InlineHelp tone="neutral" title={t('ob.avail.unsupportedTitle')}>
          {t('ob.avail.unsupportedBody')}
        </InlineHelp>
      );
    case 'invalid':
      return (
        <InlineHelp tone="warning" title={t('ob.avail.invalidTitle')}>
          {t('ob.avail.invalidBody')}
        </InlineHelp>
      );
    case 'unconfigured':
      return (
        <InlineHelp tone="neutral" title={t('ob.avail.unconfiguredTitle')}>
          {t('ob.avail.unconfiguredBody')}
        </InlineHelp>
      );
    default:
      return (
        <InlineHelp tone="danger" title={t('ob.avail.unknownTitle')}>
          {t('ob.avail.unknownBody')}
        </InlineHelp>
      );
  }
}

/**
 * El camino manual: el registro exacto para copiarlo en cualquier proveedor de
 * DNS y comprobarlo aquí. Es el plan B que SIEMPRE está disponible, también para
 * quien no quiera dar un token de Cloudflare. El dominio se escribe fuera (lo
 * comparte con el camino de Cloudflare): aquí solo se ven los valores y el estado.
 */
function DomainPanel({
  t,
  state,
  busy,
  onCheck,
  onCopy,
}: {
  t: T;
  state: DomainWire | undefined;
  busy: boolean;
  onCheck: () => void;
  onCopy: (text: string, what: PortalMessageKey) => Promise<void>;
}) {
  return (
    <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
      {state ? (
        <>
          <div>
            <p class="text-text text-sm font-semibold">{t('ob.dnsRec.title')}</p>
            <p class="text-text-muted mt-1 text-sm leading-relaxed">{t('ob.dnsRec.body')}</p>
            <div class="mt-3 flex flex-col gap-2">
              <CopyRow
                label={t('ob.dnsRec.type')}
                value="TXT"
                copyLabel={t('devices.copy')}
                onCopy={() => void onCopy('TXT', 'ob.copied.recordType')}
              />
              <CopyRow
                label={t('ob.dnsRec.name')}
                value={state.record_name}
                copyLabel={t('devices.copy')}
                onCopy={() => void onCopy(state.record_name, 'ob.copied.recordName')}
              />
              <CopyRow
                label={t('ob.dnsRec.value')}
                value={state.record_value}
                copyLabel={t('devices.copy')}
                onCopy={() => void onCopy(state.record_value, 'ob.copied.recordValue')}
              />
            </div>
          </div>

          {state.verified ? (
            <InlineHelp tone="success" title={t('ob.dnsRec.verifiedTitle')}>
              {t('ob.dnsRec.verifiedBody', { host: state.host })}
            </InlineHelp>
          ) : state.status === 'mismatch' ? (
            <InlineHelp tone="warning" title={t('ob.dnsRec.mismatchTitle')}>
              {t('ob.dnsRec.mismatchBody', {
                name: state.record_name,
                found: state.found?.[0] ?? '',
              })}
            </InlineHelp>
          ) : state.status === 'unreachable' ? (
            <InlineHelp tone="danger" title={t('ob.dnsRec.unreachableTitle')}>
              {t('ob.dnsRec.unreachableBody')}
            </InlineHelp>
          ) : state.status === 'pending' ? (
            <InlineHelp tone="neutral" title={t('ob.dnsRec.pendingTitle')}>
              {t('ob.dnsRec.pendingBody')}
            </InlineHelp>
          ) : (
            // Sin `status` no hemos consultado el DNS en esta pantalla: no se afirma
            // nada sobre el registro.
            <InlineHelp tone="neutral" title={t('ob.dnsRec.uncheckedTitle')}>
              {t('ob.dnsRec.uncheckedBody')}
            </InlineHelp>
          )}
        </>
      ) : (
        <p class="text-text-subtle text-sm">{t('ob.dnsRec.empty')}</p>
      )}

      <div>
        <Button
          variant={state?.verified === true ? 'secondary' : 'primary'}
          loading={busy}
          onClick={onCheck}
        >
          {state ? t('ob.dnsRec.check') : t('ob.dnsRec.save')}
        </Button>
      </div>
    </div>
  );
}

/** Cómo se lee cada estado del plan. Texto además de color (nunca solo color). */
const CF_STATE: Record<
  CloudflareRecordWire['state'],
  { tone: BadgeTone; labelKey: PortalMessageKey }
> = {
  ok: { tone: 'success', labelKey: 'ob.cf.state.ok' },
  created: { tone: 'success', labelKey: 'ob.cf.state.created' },
  replaced: { tone: 'success', labelKey: 'ob.cf.state.replaced' },
  missing: { tone: 'info', labelKey: 'ob.cf.state.missing' },
  conflict: { tone: 'warning', labelKey: 'ob.cf.state.conflict' },
  failed: { tone: 'danger', labelKey: 'ob.cf.state.failed' },
  pending: { tone: 'neutral', labelKey: 'ob.cf.state.pending' },
};

/**
 * Camino asistido: el dominio está en Cloudflare y Sytue lo configura.
 *
 * Dos tiempos, siempre en el mismo orden: primero se MIRA (y se enseña lo que se
 * va a cambiar y lo que ya está bien), y solo después se toca la zona. Un registro
 * que choca con algo del cliente no se sustituye jamás sin que él marque la
 * casilla, viendo antes el valor que hay ahora.
 */
function CloudflarePanel({
  t,
  domain,
  token,
  onToken,
  mail,
  onMail,
  remember,
  onRemember,
  replace,
  onToggleReplace,
  result,
  busy,
  onPreview,
  onApply,
  onCopy,
}: {
  t: T;
  domain: string;
  token: string;
  onToken: (v: string) => void;
  mail: boolean;
  onMail: (v: boolean) => void;
  remember: boolean;
  onRemember: (v: boolean) => void;
  replace: string[];
  onToggleReplace: (key: string) => void;
  result: CloudflareWire | null;
  busy: boolean;
  onPreview: () => void;
  onApply: () => void;
  onCopy: (text: string, what: PortalMessageKey) => Promise<void>;
}) {
  const records = result?.records ?? [];
  // Qué queda por hacer: lo que falta más los conflictos que él ha autorizado.
  const toDo =
    records.filter((r) => r.state === 'missing').length +
    records.filter((r) => r.state === 'conflict' && replace.includes(r.key)).length;
  const conflicts = records.filter((r) => r.state === 'conflict');
  const failures = records.filter((r) => r.state === 'failed');
  // Sin token no se puede pedir nada, salvo que el servidor confirme que guarda
  // uno del cliente (entonces él no tiene que volver a pegarlo).
  const canAsk =
    domain.trim().length > 0 && (token.trim().length > 0 || result?.token_stored === true);

  return (
    <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
      <div>
        <p class="text-text text-sm font-semibold">{t('ob.cf.permTitle')}</p>
        <ol class="text-text-muted mt-2 flex list-decimal flex-col gap-1 pl-5 text-sm leading-relaxed">
          <li>
            {t('ob.cf.step1')}{' '}
            <a
              class="text-primary-text font-medium hover:underline"
              href="https://dash.cloudflare.com/profile/api-tokens"
              target="_blank"
              rel="noreferrer noopener"
            >
              {t('ob.cf.step1Link')}
            </a>
          </li>
          <li>{t('ob.cf.step2')}</li>
          <li>{t('ob.cf.step3', { domain: domain.trim() || t('ob.dns.fallback') })}</li>
        </ol>
      </div>

      <FieldGroup label={t('ob.cf.tokenField')} htmlFor="ob-cf-token">
        <Input
          id="ob-cf-token"
          type="password"
          value={token}
          class="font-mono"
          placeholder={t('ob.cf.tokenPlaceholder')}
          autocomplete="off"
          spellcheck={false}
          aria-describedby="ob-cf-token-hint"
          onInput={(e) => onToken(e.currentTarget.value)}
        />
        <FieldHint id="ob-cf-token-hint">{t('ob.cf.tokenHint')}</FieldHint>
      </FieldGroup>

      <div class="flex flex-col gap-2">
        <Checkbox
          label={t('ob.cf.mailLabel', { domain: domain.trim() || t('ob.dns.fallback') })}
          checked={mail}
          onChange={(e) => onMail(e.currentTarget.checked)}
        />
        <FieldHint tone={mail ? 'warning' : 'neutral'}>
          {mail ? t('ob.cf.mailHintOn') : t('ob.cf.mailHintOff')}
        </FieldHint>
        <Checkbox
          label={t('ob.cf.remember')}
          checked={remember}
          onChange={(e) => onRemember(e.currentTarget.checked)}
        />
        <FieldHint>{t('ob.cf.rememberHint')}</FieldHint>
      </div>

      <div class="flex flex-wrap gap-2">
        <Button
          variant={result ? 'secondary' : 'primary'}
          loading={busy}
          disabled={!canAsk}
          onClick={onPreview}
        >
          {result ? t('ob.cf.previewAgain') : t('ob.cf.preview')}
        </Button>
        {result ? (
          <Button
            variant="primary"
            loading={busy}
            disabled={toDo === 0 || !canAsk}
            onClick={onApply}
          >
            {toDo === 0 ? t('ob.cf.nothing') : t('ob.cf.apply', { count: toDo })}
          </Button>
        ) : null}
      </div>

      {result ? (
        <>
          <ReviewSummary
            items={[
              { label: t('ob.cf.zone'), value: result.zone.name, mono: true },
              {
                label: t('ob.cf.token'),
                value: result.token_stored ? t('ob.cf.tokenStored') : t('ob.cf.tokenDiscarded'),
              },
              {
                label: t('ob.cf.domainState'),
                value: result.domain.verified ? t('ob.cf.verified') : t('ob.cf.pending'),
              },
            ]}
          />

          <ul class="flex flex-col gap-2">
            {records.map((r) => (
              <li
                key={r.key}
                class="border-border bg-surface flex flex-col gap-1.5 rounded-md border px-3 py-2.5"
              >
                <div class="flex flex-wrap items-center gap-2">
                  <Badge tone={CF_STATE[r.state].tone}>{t(CF_STATE[r.state].labelKey)}</Badge>
                  <code class="text-text min-w-0 flex-1 font-mono text-xs break-all">
                    {r.type} {r.name}
                  </code>
                  {r.content ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => void onCopy(r.content ?? '', 'ob.copied.recordValue')}
                    >
                      {t('devices.copy')}
                    </Button>
                  ) : null}
                </div>
                <p class="text-text-muted text-sm leading-snug">{r.purpose}</p>
                {r.content ? (
                  <code class="sytue-meta font-mono break-all">
                    {r.priority ? `${r.priority} ` : ''}
                    {r.content}
                  </code>
                ) : null}
                {r.detail ? <p class="sytue-meta leading-snug">{r.detail}</p> : null}
                {r.state === 'conflict' ? (
                  <div class="mt-1 flex flex-col gap-1">
                    <p class="text-text-muted text-xs leading-snug break-all">
                      {t('ob.cf.nowThere', { current: r.current ?? '' })}
                    </p>
                    <Checkbox
                      label={t('ob.cf.replaceWithSytue')}
                      checked={replace.includes(r.key)}
                      onChange={() => onToggleReplace(r.key)}
                    />
                  </div>
                ) : null}
              </li>
            ))}
          </ul>

          {failures.length > 0 ? (
            <InlineHelp tone="danger" title={t('ob.cf.failTitle')}>
              {t('ob.cf.failBody')}
            </InlineHelp>
          ) : conflicts.length > 0 ? (
            <InlineHelp tone="warning" title={t('ob.cf.conflictTitle')}>
              {t('ob.cf.conflictBody')}
            </InlineHelp>
          ) : result.domain.verified ? (
            <InlineHelp tone="success" title={t('ob.cf.okTitle')}>
              {t('ob.cf.okBody', { host: result.domain.host })}
            </InlineHelp>
          ) : (
            <InlineHelp tone="neutral" title={t('ob.cf.todoTitle')}>
              {t('ob.cf.todoBody')}
            </InlineHelp>
          )}
        </>
      ) : null}
    </div>
  );
}

function CopyRow({
  label,
  value,
  copyLabel,
  onCopy,
}: {
  label: string;
  value: string;
  /** Rótulo del botón: lo pasa el padre, que es quien tiene la `t`. */
  copyLabel: string;
  onCopy: () => void;
}) {
  return (
    <div class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border px-3 py-2">
      <span class="sytue-meta w-full shrink-0 sm:w-24">{label}</span>
      <code class="text-text min-w-0 flex-1 font-mono text-xs break-all">{value}</code>
      <Button variant="ghost" size="sm" onClick={onCopy}>
        {copyLabel}
      </Button>
    </div>
  );
}

function InviteResults({ t, results }: { t: T; results: InviteResult[] }) {
  const tone: Record<InviteResult['status'], 'success' | 'warning' | 'danger'> = {
    sent: 'success',
    already_member: 'warning',
    invalid: 'warning',
    not_sent: 'danger',
    failed: 'danger',
  };
  const label: Record<InviteResult['status'], string> = {
    sent: t('ob.invite.sent'),
    already_member: t('ob.invite.already'),
    invalid: t('ob.invite.invalid'),
    not_sent: t('ob.invite.notSent'),
    failed: t('ob.invite.failed'),
  };
  return (
    <ul class="flex flex-col gap-2">
      {results.map((r) => (
        <li key={r.email}>
          <InlineHelp tone={tone[r.status]} title={`${r.email} — ${label[r.status]}`}>
            {r.detail ?? t('ob.invite.default')}
          </InlineHelp>
        </li>
      ))}
    </ul>
  );
}

function NextLink({
  icon,
  href,
  title,
  body,
}: {
  icon: 'rocket' | 'cloud' | 'grid' | 'users';
  href: string;
  title: string;
  body: string;
}) {
  return (
    <li>
      <a
        href={href}
        class="hover:border-border-strong border-border-subtle sytue-focus-ring-inset flex gap-3 rounded-md border p-3 transition-colors"
      >
        <Icon name={icon} size={18} class="text-text-muted mt-0.5 shrink-0" />
        <span class="min-w-0">
          <span class="text-text block text-sm font-medium">{title}</span>
          <span class="text-text-muted mt-0.5 block text-sm leading-snug">{body}</span>
        </span>
      </a>
    </li>
  );
}

// El rol viene del backend como identificador; su NOMBRE visible sale del
// catálogo. Un rol desconocido se devuelve tal cual: es mejor enseñar el
// identificador que una cadena vacía.
function roleLabel(t: T, role: string): string {
  const claves: Record<string, PortalMessageKey> = {
    owner: 'ob.roleName.owner',
    admin: 'ob.roleName.admin',
    member: 'ob.roleName.member',
    billing: 'ob.roleName.billing',
  };
  const clave = claves[role];
  return clave ? t(clave) : role;
}

export function Onboarding({ locale }: { locale: Locale }) {
  return (
    <ToastProvider>
      <OnboardingWizard locale={locale} />
    </ToastProvider>
  );
}

export default Onboarding;
