/**
 * MigrateWizard — asistente «Migrar desde Microsoft 365 o Google Workspace».
 *
 * Mismo sistema de configuración guiada que ConnectIdpWizard: de dónde → qué traer
 * → para quién (con el directorio delante) → qué se trae y qué NO, en llano →
 * resumen → lanzar. Nada se copia hasta el último paso.
 *
 * Lo importante del paso «para quién»: el contenido aterriza en la cuenta de CADA
 * persona, no en la de quien lanza la migración. Por eso hay que decir de qué
 * cuenta del origen se copia y a qué persona de Sytue va: adivinarlo sería copiar
 * los datos de alguien en la cuenta equivocada.
 */
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  ChoiceCard,
  DataFlow,
  EmptyState,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Wizard,
} from '@sytue/ui';
import type { DataCategory, ReviewItem, WizardStep } from '@sytue/ui';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { MigrationJob } from '../lib/gateway.js';
import type { DirectoryUser } from '../lib/types.js';
import { r } from '../lib/rutas.js';
import { contiene } from '@sytue/i18n';

/** Conexión de origen ya configurada en Integraciones. */
export interface MigrationSourceOption {
  instanceId: string;
  name: string;
  source: 'm365' | 'google';
}

type Bring = 'drive' | 'contacts' | 'both';
type OwnDomain = 'drive' | 'contacts' | 'mail' | 'calendar';

const OWN_DOMAINS: OwnDomain[] = ['drive', 'contacts', 'mail', 'calendar'];

const CATEGORY: Record<OwnDomain, DataCategory> = {
  drive: 'files',
  contacts: 'contacts',
  mail: 'mail',
  calendar: 'events',
};

interface Target {
  /** Persona de Sytue (correo): en su cuenta aterriza el contenido. */
  targetUser: string;
  name: string;
  /** Cuenta equivalente en el origen. */
  sourceUser: string;
}

interface MigrateWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  sources: MigrationSourceOption[];
  onLaunched: (jobs: MigrationJob[]) => void;
  onCancel: () => void;
  /**
   * 'out' invierte el sentido: lee de Sytue y ESCRIBE en el tercero. El asistente
   * es el mismo a propósito —quien ya sabe traer sabe llevar— y solo cambian los
   * textos y el destino de cada trabajo.
   *
   * 'self' es llevarse LO SUYO: sin elegir personas, con correo y agenda, y leído
   * de las mismas entregas que la copia de «Tus datos» (no pide delegación).
   */
  direction?: 'in' | 'out' | 'self';
  /** Desde 'self', pasar a llevarse el contenido de otras personas. */
  onOthers?: () => void;
}

// Marcas comerciales: no se traducen (son el nombre del producto ajeno).
const SOURCE_LABEL: Record<'m365' | 'google', string> = {
  m365: 'Microsoft 365',
  google: 'Google Workspace',
};

const BRING_KEY: Record<Bring, ConsoleMessageKey> = {
  drive: 'wizard.migrate.bring.drive',
  contacts: 'wizard.migrate.bring.contacts',
  both: 'wizard.migrate.bring.both',
};

const BRING_HINT_KEY: Record<Bring, ConsoleMessageKey> = {
  drive: 'wizard.migrate.what.driveHint',
  contacts: 'wizard.migrate.what.contactsHint',
  both: 'wizard.migrate.what.bothHint',
};

function domainsOf(b: Bring): OwnDomain[] {
  return b === 'both' ? ['drive', 'contacts'] : [b];
}

/** Lo que SÍ se trae y lo que NO, en lenguaje llano y sin adornos. */
function whatYouGet(
  source: 'm365' | 'google',
  bring: Bring,
  t: T,
): { yes: string[]; no: string[] } {
  const yes: string[] = [];
  const no: string[] = [];
  if (bring !== 'contacts') {
    yes.push(t('wizard.migrate.gets.files'));
    if (source === 'google') no.push(t('wizard.migrate.nots.googleNative'));
    no.push(t('wizard.migrate.nots.tooBig'));
    no.push(t('wizard.migrate.nots.modifiedDate'));
    no.push(t('wizard.migrate.nots.permissions'));
  }
  if (bring !== 'drive') {
    yes.push(t('wizard.migrate.gets.contacts'));
    no.push(t('wizard.migrate.nots.namelessContacts'));
  }
  no.push(t('wizard.migrate.nots.mailCalendar'));
  return { yes, no };
}

/** Lo que llega y lo que no al llevarse lo suyo. */
function whatYouTake(
  source: 'm365' | 'google',
  doms: OwnDomain[],
  t: T,
): { yes: string[]; no: string[] } {
  const yes: string[] = [];
  const no: string[] = [];
  if (doms.includes('drive')) {
    yes.push(t('wizard.migrate.gets.files'));
    no.push(t('wizard.migrate.nots.permissions'));
  }
  if (doms.includes('contacts')) yes.push(t('wizard.migrate.gets.contacts'));
  if (doms.includes('mail')) {
    yes.push(t('wizard.migrate.self.gets.mail'));
    if (source === 'm365') no.push(t('wizard.migrate.self.nots.outlookDrafts'));
  }
  if (doms.includes('calendar')) {
    yes.push(t('wizard.migrate.self.gets.calendar'));
    no.push(t('wizard.migrate.self.nots.invites'));
    if (source === 'm365') no.push(t('wizard.migrate.self.nots.outlookRepeats'));
  }
  return { yes, no };
}

export default function MigrateWizard({
  t,
  sources,
  onLaunched,
  onCancel,
  direction = 'in',
  onOthers,
}: MigrateWizardProps) {
  const self = direction === 'self';
  const outbound = direction !== 'in';
  const [instanceId, setInstanceId] = useState(sources[0]?.instanceId ?? '');
  const [bring, setBring] = useState<Bring>('both');
  const [own, setOwn] = useState<OwnDomain[]>(OWN_DOMAINS);
  const [targets, setTargets] = useState<Target[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [step, setStep] = useState(0);

  // Directorio de la organización (para elegir a quién). Se carga por páginas: no
  // se trae medio directorio de golpe ni se finge una búsqueda global.
  const [people, setPeople] = useState<DirectoryUser[]>([]);
  const [cursor, setCursor] = useState('');
  const [loadingPeople, setLoadingPeople] = useState(true);
  const [query, setQuery] = useState('');

  const chosen = sources.find((s) => s.instanceId === instanceId) ?? sources[0];

  async function loadPeople(next: string) {
    setLoadingPeople(true);
    try {
      const res = (await fetch(
        `/api/admin/users?limit=200${next ? `&cursor=${encodeURIComponent(next)}` : ''}`,
      ).then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))) as {
        items?: DirectoryUser[];
        nextCursor?: string;
      };
      setPeople((prev) => [...prev, ...(res.items ?? [])]);
      setCursor(typeof res.nextCursor === 'string' ? res.nextCursor : '');
    } catch {
      setCursor('');
    } finally {
      setLoadingPeople(false);
    }
  }

  // Solo al montar: las páginas siguientes las pide el botón «Cargar más». Quien se
  // lleva lo suyo no elige a nadie.
  useEffect(() => {
    if (!self) void loadPeople('');
  }, [self]);

  const ids = self
    ? ['origin', 'what', 'limits', 'review']
    : ['origin', 'what', 'who', 'limits', 'review'];
  const go = (id: string) => setStep(ids.indexOf(id));

  function toggle(u: DirectoryUser) {
    setTargets((prev) => {
      // Ojo con el nombre: una lambda `t` ensombrecería la `t` de traducción.
      const i = prev.findIndex((x) => x.targetUser === u.email);
      if (i >= 0) return prev.filter((_, n) => n !== i);
      return [...prev, { targetUser: u.email, name: u.fullName || u.email, sourceUser: u.email }];
    });
  }

  function setSourceUser(targetUser: string, value: string) {
    setTargets((prev) =>
      prev.map((x) => (x.targetUser === targetUser ? { ...x, sourceUser: value } : x)),
    );
  }

  const visible = query.trim()
    ? people.filter((u) => contiene(`${u.fullName} ${u.email}`, query.trim()))
    : people;
  const incomplete = targets.filter((x) => !x.sourceUser.trim());

  // La conexión con la que se TRAE contenido solo lee. Para llevarlo, el proveedor
  // tiene que conceder además permiso de escritura: se pide aquí, a propósito aparte,
  // y la persona vuelve a Migraciones ya autorizada.
  async function autorizarEscritura() {
    if (busy || !chosen) return;
    setBusy(true);
    setError('');
    try {
      const res = await fetch(
        `/api/admin/apps/instances/${encodeURIComponent(chosen.instanceId)}/oauth-start`,
        {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ return_to: 'migrations', purpose: 'out' }),
        },
      );
      const start = (res.ok ? await res.json() : {}) as { authorize_url?: string };
      if (!start.authorize_url) throw new Error(String(res.status));
      window.location.assign(start.authorize_url);
    } catch {
      setError(t('wizard.migrate.out.authFailed'));
      setBusy(false);
    }
  }

  async function launch() {
    if (busy || !chosen) return;
    setBusy(true);
    setError('');
    try {
      const res = self
        ? await fetch('/api/admin/migrations/self-out', {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ instance_id: chosen.instanceId, domains: own }),
          })
        : await fetch('/api/admin/migrations', {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({
              instance_id: chosen.instanceId,
              source: chosen.source,
              domains: domainsOf(bring),
              targets: targets.map((x) => ({
                source_user: x.sourceUser.trim(),
                target_user: x.targetUser,
              })),
              direction,
            }),
          });
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { migrations?: MigrationJob[] };
      onLaunched(out.migrations ?? []);
    } catch (e) {
      const status = e instanceof Error ? Number(e.message) : 0;
      setError(
        t(
          status === 403
            ? 'wizard.migrate.error.forbidden'
            : status === 404
              ? 'wizard.migrate.error.sourceGone'
              : status === 409
                ? 'wizard.migrate.self.conflict'
                : 'wizard.migrate.error.generic',
        ),
      );
    } finally {
      setBusy(false);
    }
  }

  // ---- Pasos ----

  const stepOrigin: WizardStep = {
    id: 'origin',
    title: t(outbound ? 'wizard.migrate.step.destination' : 'wizard.migrate.step.origin'),
    heading: t(outbound ? 'wizard.migrate.out.heading' : 'wizard.migrate.origin.heading'),
    description: t(
      self
        ? 'wizard.migrate.self.description'
        : outbound
          ? 'wizard.migrate.out.description'
          : 'wizard.migrate.origin.description',
    ),
    canContinue: Boolean(chosen),
    content:
      sources.length === 0 ? (
        <InlineHelp tone="warning" title={t('wizard.migrate.origin.noneTitle')}>
          {t('wizard.migrate.origin.noneBody')}
          <span class="mt-2 block">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => window.location.assign(r('/apps'))}
            >
              {t('wizard.migrate.goToApps')}
            </Button>
          </span>
        </InlineHelp>
      ) : (
        <ul class="flex max-w-2xl flex-col gap-3">
          {outbound ? (
            <li>
              <InlineHelp
                tone="info"
                title={t('wizard.migrate.out.noticeTitle')}
                {...(self && onOthers
                  ? {
                      action: (
                        <Button variant="ghost" size="sm" onClick={onOthers}>
                          {t('wizard.migrate.self.others')}
                        </Button>
                      ),
                    }
                  : {})}
              >
                {t(self ? 'wizard.migrate.self.noticeBody' : 'wizard.migrate.out.noticeBody')}
              </InlineHelp>
            </li>
          ) : null}
          {outbound && chosen ? (
            <li>
              <InlineHelp
                tone="neutral"
                title={t('wizard.migrate.out.authTitle', { provider: SOURCE_LABEL[chosen.source] })}
                action={
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={busy}
                    onClick={() => void autorizarEscritura()}
                  >
                    {t('wizard.migrate.out.authAction')}
                  </Button>
                }
              >
                {t('wizard.migrate.out.authBody', { provider: SOURCE_LABEL[chosen.source] })}
              </InlineHelp>
              {error ? (
                <p class="text-danger-text mt-2 text-sm" role="alert">
                  {error}
                </p>
              ) : null}
            </li>
          ) : null}
          {sources.map((s) => (
            <li key={s.instanceId}>
              <ChoiceCard
                name="sytue-migrate-source"
                value={s.instanceId}
                icon="globe"
                title={SOURCE_LABEL[s.source]}
                description={t('wizard.migrate.origin.connection', { name: s.name })}
                selected={chosen?.instanceId === s.instanceId}
                onSelect={() => setInstanceId(s.instanceId)}
              />
            </li>
          ))}
        </ul>
      ),
  };

  const stepWhat: WizardStep = {
    id: 'what',
    title: t('wizard.migrate.step.what'),
    heading: t(outbound ? 'wizard.migrate.out.whatHeading' : 'wizard.migrate.what.heading'),
    description: t('wizard.migrate.what.description'),
    canContinue: !self || own.length > 0,
    content: self ? (
      <ul class="flex max-w-2xl flex-col gap-3">
        {OWN_DOMAINS.map((d) => (
          <li key={d}>
            <Checkbox
              label={t(`migrations.domain.${d}` as const)}
              checked={own.includes(d)}
              onChange={() =>
                setOwn((prev) => (prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d]))
              }
            />
          </li>
        ))}
      </ul>
    ) : (
      <ul class="flex max-w-2xl flex-col gap-3">
        {(['both', 'drive', 'contacts'] as Bring[]).map((b) => (
          <li key={b}>
            <ChoiceCard
              name="sytue-migrate-bring"
              value={b}
              icon={b === 'contacts' ? 'users' : 'database'}
              title={t(BRING_KEY[b])}
              description={t(BRING_HINT_KEY[b])}
              badge={
                b === 'both' ? (
                  <Badge tone="neutral">{t('wizard.migrate.what.mostCommon')}</Badge>
                ) : null
              }
              selected={bring === b}
              onSelect={() => setBring(b)}
            />
          </li>
        ))}
      </ul>
    ),
  };

  const stepWho: WizardStep = {
    id: 'who',
    title: t('wizard.migrate.step.who'),
    heading: t(outbound ? 'wizard.migrate.out.whoHeading' : 'wizard.migrate.who.heading'),
    description: t(
      outbound ? 'wizard.migrate.out.whoDescription' : 'wizard.migrate.who.description',
    ),
    canContinue: targets.length > 0 && incomplete.length === 0,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <div>
          <Input
            type="search"
            value={query}
            placeholder={t('wizard.migrate.who.searchPlaceholder')}
            aria-label={t('wizard.migrate.who.searchAria')}
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
          <FieldHint>{t('wizard.migrate.who.loadedHint', { count: people.length })}</FieldHint>
        </div>

        <div class="border-border max-h-72 overflow-y-auto rounded-lg border">
          {visible.length === 0 ? (
            <p class="text-text-muted p-4 text-sm">
              {t(loadingPeople ? 'wizard.migrate.who.loading' : 'wizard.migrate.who.noMatch')}
            </p>
          ) : (
            <ul class="divide-border-subtle divide-y">
              {visible.map((u) => (
                <li key={u.id} class="flex items-start justify-between gap-3 px-3 py-2">
                  <span class="min-w-0">
                    <Checkbox
                      label={u.fullName || u.email}
                      checked={targets.some((x) => x.targetUser === u.email)}
                      onChange={() => toggle(u)}
                    />
                    <span class="text-text-muted ml-7 block truncate text-xs">{u.email}</span>
                  </span>
                  {u.enabled ? null : (
                    <Badge tone="neutral">{t('wizard.migrate.who.suspended')}</Badge>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
        {cursor ? (
          <Button
            variant="secondary"
            size="sm"
            class="self-start"
            loading={loadingPeople}
            onClick={() => void loadPeople(cursor)}
          >
            {t('wizard.migrate.who.loadMore')}
          </Button>
        ) : null}

        {targets.length === 0 ? (
          <InlineHelp tone="neutral">{t('wizard.migrate.who.pickHelp')}</InlineHelp>
        ) : (
          <section class="border-border bg-surface-raised rounded-lg border p-3">
            <h3 class="text-text text-sm font-semibold">
              {t('wizard.migrate.who.sourceTitle', { count: targets.length })}
            </h3>
            <p class="text-text-muted mt-0.5 text-sm leading-snug">
              {t('wizard.migrate.who.sourceHint', {
                product: SOURCE_LABEL[chosen?.source ?? 'm365'],
              })}
            </p>
            <ul class="mt-3 flex flex-col gap-2">
              {targets.map((x) => (
                <li key={x.targetUser} class="flex flex-wrap items-center gap-2">
                  <span class="text-text w-full min-w-0 truncate text-sm sm:w-52">{x.name}</span>
                  <Icon name="chevronRight" size={14} class="text-text-subtle hidden sm:block" />
                  <Input
                    class="min-w-0 flex-1 font-mono"
                    value={x.sourceUser}
                    aria-label={t('wizard.migrate.who.sourceAria', { name: x.name })}
                    spellcheck={false}
                    invalid={!x.sourceUser.trim()}
                    onInput={(e) => setSourceUser(x.targetUser, e.currentTarget.value)}
                  />
                </li>
              ))}
            </ul>
            {incomplete.length > 0 ? (
              <FieldHint tone="danger">
                {t('wizard.migrate.who.missingSource', { count: incomplete.length })}
              </FieldHint>
            ) : null}
          </section>
        )}
      </div>
    ),
  };

  const picked = self ? own : domainsOf(bring);
  const gets = self
    ? whatYouTake(chosen?.source ?? 'm365', own, t)
    : whatYouGet(chosen?.source ?? 'm365', bring, t);

  const stepLimits: WizardStep = {
    id: 'limits',
    title: t('wizard.migrate.step.limits'),
    heading: t('wizard.migrate.limits.heading'),
    description: t('wizard.migrate.limits.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text flex items-center gap-2 text-sm font-semibold">
            <Icon name="check" size={16} class="text-success-text" />
            {t('wizard.migrate.limits.yes')}
          </h3>
          <ul class="mt-2 flex flex-col gap-1.5">
            {gets.yes.map((l) => (
              <li key={l} class="text-text-muted text-sm leading-snug">
                {l}
              </li>
            ))}
          </ul>
        </section>
        <section class="border-warning bg-warning-bg rounded-lg border p-4">
          <h3 class="text-warning-text text-sm font-semibold">{t('wizard.migrate.limits.no')}</h3>
          <ul class="mt-2 flex flex-col gap-1.5">
            {gets.no.map((l) => (
              <li key={l} class="text-text-muted text-sm leading-snug">
                {l}
              </li>
            ))}
          </ul>
        </section>
        <InlineHelp tone="info" title={t('wizard.migrate.limits.safeTitle')}>
          {t(self ? 'wizard.migrate.self.safeBody' : 'wizard.migrate.limits.safeBody')}
        </InlineHelp>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('wizard.migrate.step.origin'),
      value: chosen
        ? t('wizard.migrate.summary.source', {
            product: SOURCE_LABEL[chosen.source],
            name: chosen.name,
          })
        : '—',
      onEdit: () => go('origin'),
    },
    {
      label: t('wizard.migrate.summary.what'),
      value: self
        ? own.map((d) => t(`migrations.domain.${d}` as const)).join(', ')
        : t(BRING_KEY[bring]),
      onEdit: () => go('what'),
    },
  ];
  if (!self) {
    summary.push(
      {
        label: t('wizard.migrate.summary.who'),
        value:
          targets.length === 1
            ? t('wizard.migrate.summary.whoOne', {
                name: targets[0]?.name ?? '',
                email: targets[0]?.targetUser ?? '',
              })
            : t('wizard.migrate.summary.whoMany', { count: targets.length }),
        onEdit: () => go('who'),
      },
      {
        label: t('wizard.migrate.summary.jobs'),
        value: t('wizard.migrate.summary.jobsValue', {
          count: targets.length * domainsOf(bring).length,
        }),
      },
    );
  }
  summary.push({
    label: t('wizard.migrate.summary.skipped'),
    value:
      gets.no.length > 0
        ? t('wizard.migrate.summary.skippedValue', { count: gets.no.length })
        : t('wizard.migrate.summary.skippedNone'),
    onEdit: () => go('limits'),
  });

  const crosses = picked.map((d) => CATEGORY[d]);
  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.migrate.step.review'),
    heading: t('wizard.migrate.review.heading'),
    description: t('wizard.migrate.review.description'),
    nextLabel: self
      ? t('wizard.migrate.self.launch')
      : t('wizard.migrate.review.launch', { count: targets.length }),
    onNext: () => void launch(),
    content: (
      <div class="flex flex-col gap-4">
        <DataFlow
          partner={SOURCE_LABEL[chosen?.source ?? 'm365']}
          incoming={outbound ? [] : crosses}
          outgoing={outbound ? crosses : []}
        />
        <ReviewSummary items={summary} />
        {error ? (
          <InlineHelp tone="danger" title={t('wizard.migrate.review.failedTitle')}>
            {error}
          </InlineHelp>
        ) : null}
        {self ? (
          <InlineHelp tone="info" title={t('wizard.migrate.self.reviewTitle')}>
            {t('wizard.migrate.self.reviewBody', {
              provider: SOURCE_LABEL[chosen?.source ?? 'm365'],
            })}
          </InlineHelp>
        ) : (
          <InlineHelp tone="info" title={t('wizard.migrate.review.ownAccountTitle')}>
            {t('wizard.migrate.review.ownAccountBody')}
          </InlineHelp>
        )}
      </div>
    ),
  };

  if (sources.length === 0) {
    return (
      <EmptyState
        title={t('wizard.migrate.empty.title')}
        description={t('wizard.migrate.empty.description')}
        illustration={<Icon name="globe" size={28} />}
        action={
          <Button variant="primary" onClick={() => window.location.assign(r('/apps'))}>
            {t('wizard.migrate.goToApps')}
          </Button>
        }
      />
    );
  }

  const byId: Record<string, WizardStep> = {
    origin: stepOrigin,
    what: stepWhat,
    who: stepWho,
    limits: stepLimits,
    review: stepReview,
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
