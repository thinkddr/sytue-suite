/**
 * CompliancePanel — Cumplimiento de Sytue Admin: legal hold + eDiscovery sobre
 * Sytue Mail (correo) y Sytue Drive (archivos).
 *
 * Las dos acciones de aquí son irreversibles o invasivas, así que ninguna es un
 * formulario a secas: crear una retención es un asistente corto (a quién → qué
 * implica en llano → motivo y caso → resumen → confirmar), liberarla explica la
 * consecuencia («vuelve el borrado normal»), y el export de eDiscovery dice qué se
 * descarga, de quién y que queda registrado antes de bajar un solo byte.
 *
 * Todo es ACCESO PRIVILEGIADO a datos ajenos, AUDITADO en el servicio Go; el BFF
 * (cookie httpOnly) reenvía y combina por-servicio best-effort — el token nunca está
 * aquí. Solo @sytue/ui, Graphite & Brass, denso, oscuro, WCAG AA.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  formatBytes,
  Icon,
  InlineHelp,
  Input,
  Modal,
  ReviewSummary,
  Select,
  SettingsSection,
  SettingsShell,
  Skeleton,
  Switch,
  Tabs,
  ToastProvider,
  Wizard,
  useToast,
  withBase,
  ConfirmDialog,
} from '@sytue/ui';
import type { IconName, ReviewItem, WizardStep } from '@sytue/ui';
import type {
  ComplianceService,
  EDiscoveryHit,
  LegalHold,
  ServiceResult,
} from '../lib/compliance.js';
import { formatDate, formatList, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { ConsoleMessageKey } from '../i18n/index.js';
import type { DirectoryUser } from '../lib/types.js';
import { formatWhen } from '../lib/format.js';

// SERVICE_KEY se redeclara aquí (no se importa el valor de lib/compliance.js, que
// es módulo SOLO servidor): los tipos sí se importan (import type se borra al build).
const SERVICE_KEY: Record<ComplianceService, ConsoleMessageKey> = {
  mail: 'comp.service.mail',
  drive: 'comp.service.drive',
  notes: 'comp.service.notes',
};

/** «su correo», «sus archivos», «sus cuadernos» — para el diálogo de liberar. */
const NOUN_KEY: Record<ComplianceService, ConsoleMessageKey> = {
  mail: 'comp.noun.mail',
  drive: 'comp.noun.drive',
  notes: 'comp.noun.notes',
};

// 'notes' SOLO vale para retención legal (Notepads no tiene eDiscovery — ver
// lib/compliance.ts): `SCOPES` lo incluye para el asistente de retención, y
// `SearchTab` (eDiscovery) filtra esa entrada al pintar su propio selector.
type Scope = 'mail' | 'drive' | 'notes' | 'both';

/** Cada ámbito contado por lo que preserva y por lo que NO preserva. */
const SCOPES: {
  value: Scope;
  label: ConsoleMessageKey;
  short: ConsoleMessageKey;
  consequence: ConsoleMessageKey;
}[] = [
  {
    value: 'both',
    label: 'comp.scope.bothLabel',
    short: 'comp.scope.bothShort',
    consequence: 'comp.scope.bothBody',
  },
  {
    value: 'mail',
    label: 'comp.scope.mailLabel',
    short: 'comp.scope.mailShort',
    consequence: 'comp.scope.mailBody',
  },
  {
    value: 'drive',
    label: 'comp.scope.driveLabel',
    short: 'comp.scope.driveShort',
    consequence: 'comp.scope.driveBody',
  },
  {
    value: 'notes',
    label: 'comp.scope.notesLabel',
    short: 'comp.scope.notesShort',
    consequence: 'comp.scope.notesBody',
  },
];

/** Ámbitos válidos para el asistente de retención (los tres + «ambos» = mail+drive). */
const HOLD_SCOPES = SCOPES;
/** Ámbitos válidos para eDiscovery: Notepads no los tiene. */
const SEARCH_SCOPES = SCOPES.filter((s) => s.value !== 'notes');

function scopeServices(scope: Scope): ComplianceService[] {
  return scope === 'both' ? ['mail', 'drive'] : [scope];
}

function scopeLabel(scope: Scope, t: T): string {
  const key = SCOPES.find((s) => s.value === scope)?.short;
  return key ? t(key) : '—';
}

/** «su correo», «sus archivos»… para meterlo dentro de una frase. */
function scopeNoun(scope: Scope, t: T): string {
  if (scope === 'mail') return t('comp.scope.mailNoun');
  if (scope === 'drive') return t('comp.scope.driveNoun');
  if (scope === 'notes') return t('comp.scope.notesNoun');
  return t('comp.scope.bothNoun');
}

interface CompliancePanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  /** Directorio de la org (para elegir el sujeto por email → resuelve su sub). */
  users: DirectoryUser[];
  /** Holds iniciales combinados (correo + ficheros + cuadernos), etiquetados por servicio. */
  holds: LegalHold[];
  /** false = el servicio no respondió; su parte se avisa como no accesible. */
  mailLive: boolean;
  driveLive: boolean;
  notesLive: boolean;
}

// ---- fetch helpers (BFF, cookie httpOnly) ----

class HttpStatus extends Error {
  constructor(readonly status: number) {
    super(String(status));
  }
}

async function send(url: string, method: string, body?: unknown): Promise<unknown> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'content-type': 'application/json' } : {},
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  if (!res.ok) throw new HttpStatus(res.status);
  return res.json().catch(() => ({}));
}

type PerService<T> = Partial<Record<ComplianceService, ServiceResult<T>>>;

function serviceBadge(s: ComplianceService, t: T) {
  if (s === 'mail') return <Badge tone="info">{t('comp.service.mail')}</Badge>;
  if (s === 'drive') return <Badge tone="accent">{t('comp.service.drive')}</Badge>;
  return <Badge tone="neutral">{t('comp.service.notes')}</Badge>;
}

/** Aviso de los servicios que no responden, para el banner de la página. */
function offlineNotice(mailLive: boolean, driveLive: boolean, notesLive: boolean, t: T) {
  if (mailLive && driveLive && notesLive) return undefined;
  const caidos: ComplianceService[] = (['mail', 'drive', 'notes'] as const).filter(
    (s) => !{ mail: mailLive, drive: driveLive, notes: notesLive }[s],
  );
  const title =
    caidos.length === 3
      ? t('comp.offline.all')
      : t('comp.offline.one', { service: caidos.map((s) => t(SERVICE_KEY[s])).join(', ') });
  return (
    <InlineHelp tone="warning" title={title}>
      {t('comp.offline.body')}
    </InlineHelp>
  );
}

// ---- Asistente: nueva retención legal ----------------------------------------

/** Lo que implica una retención, en lenguaje llano. Es el paso «qué va a pasar». */
function impacts(
  subject: string,
  scope: Scope,
  t: T,
): { icon: IconName; title: string; body: string }[] {
  return [
    {
      icon: 'lock',
      title: t('comp.impact.noDeleteTitle'),
      body: t('comp.impact.noDeleteBody', { subject, what: scopeNoun(scope, t) }),
    },
    {
      icon: 'user',
      title: t('comp.impact.silentTitle'),
      body: t('comp.impact.silentBody'),
    },
    {
      icon: 'audit',
      title: t('comp.impact.auditedTitle'),
      body: t('comp.impact.auditedBody'),
    },
    {
      icon: 'settings',
      title: t('comp.impact.releaseTitle'),
      body: t('comp.impact.releaseBody'),
    },
  ];
}

function LegalHoldWizard({
  users,
  mailLive,
  driveLive,
  notesLive,
  locale,
  t,
  onCreated,
  onCancel,
  onDone,
}: {
  users: DirectoryUser[];
  mailLive: boolean;
  driveLive: boolean;
  notesLive: boolean;
  locale: Locale;
  t: T;
  onCreated: (holds: LegalHold[]) => void;
  onCancel: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [scope, setScope] = useState<Scope>('both');
  const [orgWide, setOrgWide] = useState(false);
  const [subjectId, setSubjectId] = useState('');
  const [reason, setReason] = useState('');
  const [caseRef, setCaseRef] = useState('');
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<string[]>([]);

  const usersById = useMemo(() => new Map(users.map((u) => [u.id, u])), [users]);
  const subject = orgWide
    ? t('comp.subject.wholeOrgInline')
    : (usersById.get(subjectId)?.email ?? t('comp.subject.thatPerson'));
  const subjectSummary = orgWide
    ? t('comp.subject.wholeOrg')
    : (usersById.get(subjectId)?.email ?? t('comp.subject.notChosen'));
  const reasonError = reason.trim() ? null : t('comp.reason.required');
  const sentence = t('comp.sentence', { what: scopeNoun(scope, t), subject });

  const liveBy: Record<ComplianceService, boolean> = {
    mail: mailLive,
    drive: driveLive,
    notes: notesLive,
  };
  /** Los servicios del ámbito que no responden ahora mismo, si los hay. */
  const scopeOffline = scopeServices(scope)
    .filter((s) => !liveBy[s])
    .map((s) => t(SERVICE_KEY[s]));

  async function save() {
    if (busy) return;
    setBusy(true);
    try {
      const user = subjectId ? usersById.get(subjectId) : undefined;
      const res = (await send('/api/admin/compliance/legal-holds', 'POST', {
        scope,
        orgWide,
        subject: orgWide ? '' : subjectId,
        email: orgWide ? '' : (user?.email ?? ''),
        reason: reason.trim(),
        case: caseRef.trim(),
      })) as PerService<LegalHold | null>;

      const created: LegalHold[] = [];
      const bad: string[] = [];
      for (const s of scopeServices(scope)) {
        const r = res[s];
        if (r?.ok && r.data) created.push(r.data);
        else bad.push(t(SERVICE_KEY[s]));
      }
      if (created.length === 0) {
        toast.notify({
          tone: 'danger',
          title: t('comp.toast.createFailed'),
          description: t('comp.toast.createRejected'),
        });
        return;
      }
      onCreated(created);
      setFailed(bad);
      setStep(4);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('comp.toast.createFailed'),
        description:
          err instanceof HttpStatus && err.status === 400
            ? t('comp.toast.badSubject')
            : err instanceof HttpStatus && err.status === 403
              ? t('comp.toast.forbidden')
              : t('comp.toast.serviceDown'),
      });
    } finally {
      setBusy(false);
    }
  }

  const stepWho: WizardStep = {
    id: 'who',
    title: t('comp.step.who'),
    heading: t('comp.step.whoHeading'),
    description: t('comp.step.whoDescription'),
    canContinue: orgWide || Boolean(subjectId),
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        <div class="flex flex-col gap-3">
          <ChoiceCard
            name="sytue-hold-who"
            value="user"
            title={t('comp.who.person')}
            description={t('comp.who.personBody')}
            selected={!orgWide}
            onSelect={() => setOrgWide(false)}
          />
          <ChoiceCard
            name="sytue-hold-who"
            value="org"
            title={t('comp.subject.wholeOrg')}
            description={t('comp.who.orgBody')}
            selected={orgWide}
            onSelect={() => setOrgWide(true)}
          />
        </div>

        {!orgWide ? (
          <FieldGroup label={t('comp.who.person')} htmlFor="hold-subject" required>
            <Select
              id="hold-subject"
              value={subjectId}
              aria-describedby="hold-subject-hint"
              onChange={(e) => setSubjectId(e.currentTarget.value)}
            >
              <option value="">{t('comp.who.selectFromDirectory')}</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.fullName} · {u.email}
                </option>
              ))}
            </Select>
            <FieldHint id="hold-subject-hint" tone={subjectId ? 'success' : 'neutral'}>
              {subjectId
                ? t('comp.who.willPreserve', {
                    email: usersById.get(subjectId)?.email ?? t('comp.who.thisAccount'),
                  })
                : t('comp.who.pickHint')}
            </FieldHint>
          </FieldGroup>
        ) : null}

        <section class="border-border-subtle flex flex-col gap-3 border-t pt-4">
          <div>
            <h3 class="text-text text-sm font-semibold">{t('comp.scope.title')}</h3>
            <p class="text-text-muted mt-0.5 text-sm leading-relaxed">{t('comp.scope.help')}</p>
          </div>
          <div class="flex flex-col gap-3">
            {HOLD_SCOPES.map((s) => (
              <ChoiceCard
                key={s.value}
                name="sytue-hold-scope"
                value={s.value}
                title={t(s.label)}
                description={t(s.consequence)}
                badge={
                  s.value === 'both' ? <Badge tone="neutral">{t('comp.scope.usual')}</Badge> : null
                }
                selected={scope === s.value}
                onSelect={() => setScope(s.value)}
              />
            ))}
          </div>
          {scopeOffline.length > 0 ? (
            <InlineHelp
              tone="warning"
              title={t('comp.scope.offlineTitle', { services: formatList(locale, scopeOffline) })}
            >
              {t('comp.scope.offlineBody')}
            </InlineHelp>
          ) : null}
        </section>
      </div>
    ),
  };

  const stepImpact: WizardStep = {
    id: 'impact',
    title: t('comp.step.impact'),
    heading: t('comp.step.impactHeading'),
    description: t('comp.step.impactDescription'),
    content: (
      <div class="flex flex-col gap-4">
        <ul class="grid gap-3 sm:grid-cols-2">
          {impacts(subject, scope, t).map((e) => (
            <li
              key={e.title}
              class="border-border bg-surface-raised flex gap-3 rounded-lg border p-4"
            >
              <Icon name={e.icon} size={18} class="text-text-subtle mt-0.5 shrink-0" />
              <div class="min-w-0">
                <p class="text-text text-sm font-semibold">{e.title}</p>
                <p class="text-text-muted mt-0.5 text-sm leading-relaxed">{e.body}</p>
              </div>
            </li>
          ))}
        </ul>
        {orgWide ? (
          <InlineHelp tone="warning" title={t('comp.impact.orgWideTitle')}>
            {t('comp.impact.orgWideBody')}
          </InlineHelp>
        ) : null}
        <InlineHelp tone="info" title={t('comp.impact.whatForTitle')}>
          {t('comp.impact.whatForBody')}
        </InlineHelp>
      </div>
    ),
  };

  const stepReason: WizardStep = {
    id: 'reason',
    title: t('comp.reason.label'),
    heading: t('comp.step.reasonHeading'),
    description: t('comp.step.reasonDescription'),
    canContinue: !reasonError,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <FieldGroup label={t('comp.reason.label')} htmlFor="hold-reason" required>
          <Input
            id="hold-reason"
            value={reason}
            placeholder={t('comp.reason.placeholder')}
            aria-describedby="hold-reason-hint"
            invalid={Boolean(reason) && Boolean(reasonError)}
            onInput={(e) => setReason(e.currentTarget.value)}
          />
          <FieldHint
            id="hold-reason-hint"
            tone={reason.trim() ? 'success' : reason ? 'danger' : 'neutral'}
          >
            {reason.trim() ? t('comp.reason.ok') : reason ? reasonError : t('comp.reason.hint')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup label={t('comp.case.label')} htmlFor="hold-case">
          <Input
            id="hold-case"
            value={caseRef}
            placeholder="LIT-2026-014"
            class="max-w-xs font-mono"
            aria-describedby="hold-case-hint"
            onInput={(e) => setCaseRef(e.currentTarget.value)}
          />
          <FieldHint id="hold-case-hint">{t('comp.case.hint')}</FieldHint>
        </FieldGroup>
        <InlineHelp tone="neutral" title={t('comp.tipTitle')}>
          {t('comp.tipBody')}
        </InlineHelp>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    { label: t('comp.review.who'), value: subjectSummary, onEdit: () => setStep(0) },
    { label: t('comp.scope.title'), value: scopeLabel(scope, t), onEdit: () => setStep(0) },
    { label: t('comp.reason.label'), value: reason.trim() || '—', onEdit: () => setStep(2) },
    {
      label: t('comp.case.label'),
      value: caseRef.trim() || t('comp.case.none'),
      mono: Boolean(caseRef.trim()),
      onEdit: () => setStep(2),
    },
    { label: t('comp.review.starts'), value: t('comp.review.startsNow') },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('gov.step.review'),
    heading: t('comp.step.reviewHeading'),
    description: t('comp.step.reviewDescription'),
    nextLabel: t('comp.create'),
    onNext: () => void save(),
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone="neutral" title={t('comp.review.sentenceTitle')}>
          {sentence}
        </InlineHelp>
        <ReviewSummary items={summary} />
        <InlineHelp tone="warning" title={t('comp.review.immediateTitle')}>
          {t('comp.review.immediateBody', { what: scopeNoun(scope, t) })}
        </InlineHelp>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('gov.step.done'),
    heading: t('comp.done.heading'),
    done: true,
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone="success" title={subjectSummary}>
          {sentence}
        </InlineHelp>
        {failed.length > 0 ? (
          <InlineHelp
            tone="warning"
            title={t('comp.done.rejectedTitle', { services: formatList(locale, failed) })}
          >
            {t('comp.done.rejectedBody', { services: formatList(locale, failed) })}
          </InlineHelp>
        ) : null}
        <section class="border-border bg-surface-raised rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('comp.done.nextTitle')}</h3>
          <ul class="text-text-muted mt-1.5 flex list-disc flex-col gap-1 pl-5 text-sm leading-relaxed">
            <li>{t('comp.done.nextSearch')}</li>
            <li>{t('comp.done.nextRelease')}</li>
          </ul>
        </section>
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('comp.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWho, stepImpact, stepReason, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}

// ---- Retenciones legales -----------------------------------------------------

function HoldsTab({
  initial,
  users,
  mailLive,
  driveLive,
  notesLive,
  locale,
  t,
}: {
  initial: LegalHold[];
  users: DirectoryUser[];
  mailLive: boolean;
  driveLive: boolean;
  notesLive: boolean;
  locale: Locale;
  t: T;
}) {
  const toast = useToast();
  const usersById = useMemo(() => new Map(users.map((u) => [u.id, u])), [users]);
  const [holds, setHolds] = useState<LegalHold[]>(initial);
  const [includeReleased, setIncludeReleased] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [wizard, setWizard] = useState(false);

  // Liberación.
  const [confirmRelease, setConfirmRelease] = useState<LegalHold | null>(null);
  const [releasing, setReleasing] = useState(false);

  function subjectLabel(h: LegalHold): string {
    if (h.orgWide) return t('comp.subject.wholeOrg');
    const u = usersById.get(h.subject);
    return u ? u.email : `${h.subject.slice(0, 8)}…`;
  }

  async function reload(next: boolean) {
    setIncludeReleased(next);
    setReloading(true);
    try {
      const res = (await send(
        `/api/admin/compliance/legal-holds?include_released=${next ? 'true' : 'false'}`,
        'GET',
      )) as PerService<LegalHold[]>;
      setHolds([...(res.mail?.data ?? []), ...(res.drive?.data ?? []), ...(res.notes?.data ?? [])]);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('comp.toast.reloadFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setReloading(false);
    }
  }

  async function release(h: LegalHold) {
    setReleasing(true);
    try {
      await send('/api/admin/compliance/legal-holds/release', 'POST', {
        service: h.service,
        id: h.id,
      });
      setHolds((prev) =>
        includeReleased
          ? prev.map((x) =>
              x.service === h.service && x.id === h.id
                ? { ...x, active: false, releasedAt: new Date().toISOString() }
                : x,
            )
          : prev.filter((x) => !(x.service === h.service && x.id === h.id)),
      );
      setConfirmRelease(null);
      toast.notify({
        tone: 'success',
        title: t('comp.toast.released'),
        description: t('comp.toast.releasedHint', { subject: subjectLabel(h) }),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('comp.toast.releaseFailed'),
        description: t('comp.toast.releaseFailedHint'),
      });
    } finally {
      setReleasing(false);
    }
  }

  if (wizard) {
    return (
      <SettingsSection
        plain
        class="py-4"
        title={t('comp.newHold')}
        description={t('comp.wizardDescription')}
      >
        <LegalHoldWizard
          users={users}
          mailLive={mailLive}
          driveLive={driveLive}
          notesLive={notesLive}
          locale={locale}
          t={t}
          onCreated={(created) => setHolds((prev) => [...created, ...prev])}
          onCancel={() => setWizard(false)}
          onDone={() => setWizard(false)}
        />
      </SettingsSection>
    );
  }

  return (
    <div class="flex flex-col gap-4 py-4">
      <InlineHelp tone="neutral" title={t('comp.holds.whatTitle')}>
        {t('comp.holds.whatBody')}
      </InlineHelp>

      <SettingsSection
        title={t('comp.holds.title')}
        description={t(
          includeReleased ? 'comp.holds.descriptionAll' : 'comp.holds.descriptionActive',
        )}
        actions={
          <>
            <Switch
              label={t('comp.holds.showReleased')}
              checked={includeReleased}
              disabled={reloading}
              onChange={(e) => void reload(e.currentTarget.checked)}
            />
            <Button
              variant="primary"
              leadingIcon={<Icon name="plus" size={16} />}
              onClick={() => setWizard(true)}
            >
              {t('comp.newHoldShort')}
            </Button>
          </>
        }
      >
        {holds.length === 0 ? (
          <EmptyState
            title={t(includeReleased ? 'comp.holds.emptyEver' : 'comp.holds.emptyActive')}
            description={t('comp.holds.emptyDescription')}
            illustration={<Icon name="lock" size={28} />}
            action={
              <Button
                variant="primary"
                leadingIcon={<Icon name="plus" size={16} />}
                onClick={() => setWizard(true)}
              >
                {t('comp.holds.emptyAction')}
              </Button>
            }
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <div
              aria-hidden="true"
              class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
            >
              <span class="w-24 shrink-0">{t('comp.col.source')}</span>
              <span class="min-w-0 flex-1">{t('comp.col.subject')}</span>
              <span class="hidden w-32 shrink-0 md:block">{t('comp.col.case')}</span>
              <span class="hidden w-44 shrink-0 lg:block">{t('comp.col.created')}</span>
              <span class="w-28 shrink-0">{t('sso.col.status')}</span>
              <span class="w-24 shrink-0 text-right">{t('gov.col.action')}</span>
            </div>
            <ul class="max-h-[30rem] overflow-y-auto" aria-busy={reloading}>
              {holds.map((h) => (
                <li
                  key={`${h.service}:${h.id}`}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span class="w-24 shrink-0">{serviceBadge(h.service, t)}</span>
                  <span class="min-w-0 flex-1">
                    <span class="text-text block truncate font-medium" title={subjectLabel(h)}>
                      {subjectLabel(h)}
                    </span>
                    {h.reason ? (
                      <span class="sytue-meta block truncate" title={h.reason}>
                        {h.reason}
                      </span>
                    ) : null}
                  </span>
                  <span
                    class="text-text-muted hidden w-32 shrink-0 truncate font-mono text-xs md:block"
                    title={h.case}
                  >
                    {h.case || '—'}
                  </span>
                  <span class="text-text-muted hidden w-44 shrink-0 text-sm whitespace-nowrap tabular-nums lg:block">
                    {formatWhen(locale, h.createdAt)}
                  </span>
                  <span class="w-28 shrink-0">
                    {h.active ? (
                      <Badge tone="success">{t('comp.status.preserving')}</Badge>
                    ) : (
                      <Badge tone="neutral">{t('comp.status.released')}</Badge>
                    )}
                  </span>
                  <span class="flex w-24 shrink-0 items-center justify-end">
                    {h.active ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={t('comp.releaseAria', {
                          subject: subjectLabel(h),
                          service: t(SERVICE_KEY[h.service]),
                        })}
                        onClick={() => setConfirmRelease(h)}
                      >
                        {t('comp.release')}
                      </Button>
                    ) : (
                      <span class="sytue-meta">—</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </SettingsSection>

      {/* Liberar: la consecuencia, no un «¿seguro?» */}
      <ConfirmDialog
        open={Boolean(confirmRelease)}
        onClose={() => setConfirmRelease(null)}
        onConfirm={() => {
          if (confirmRelease) void release(confirmRelease);
        }}
        title={t('comp.release.title')}
        confirmLabel={t('comp.release.confirm')}
        tone="danger"
        busy={confirmRelease ? releasing : false}
      >
        {confirmRelease ? (
          <>
            <p class="text-text-muted text-sm leading-relaxed">
              {t('comp.release.intro', {
                what: t(NOUN_KEY[confirmRelease.service]),
                subject: subjectLabel(confirmRelease),
              })}
            </p>
            <InlineHelp tone="warning" title={t('gov.delete.afterTitle')}>
              <ul class="flex list-disc flex-col gap-1 pl-4">
                <li>{t('comp.release.point1')}</li>
                <li>{t('comp.release.point2')}</li>
                <li>{t('comp.release.point3')}</li>
              </ul>
            </InlineHelp>
            <p class="sytue-meta">{t('comp.release.audited')}</p>
          </>
        ) : null}
      </ConfirmDialog>
    </div>
  );
}

// ---- eDiscovery --------------------------------------------------------------

function SearchTab({
  users,
  mailLive,
  driveLive,
  locale,
  t,
}: {
  users: DirectoryUser[];
  mailLive: boolean;
  driveLive: boolean;
  locale: Locale;
  t: T;
}) {
  const toast = useToast();
  const usersById = useMemo(() => new Map(users.map((u) => [u.id, u])), [users]);

  const [subjectId, setSubjectId] = useState('');
  const [scope, setScope] = useState<Scope>('both');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [query, setQuery] = useState('');
  const [caseRef, setCaseRef] = useState('');

  const [results, setResults] = useState<EDiscoveryHit[]>([]);
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState<string[]>([]);
  // Servicios cuya búsqueda respondió OK (para ofrecer su export).
  const [exportable, setExportable] = useState<ComplianceService[]>([]);
  const [confirmExport, setConfirmExport] = useState<ComplianceService | null>(null);

  const subjectEmail = usersById.get(subjectId)?.email ?? subjectId;
  const periodText =
    from && to
      ? t('comp.period.range', {
          from: formatDate(locale, from),
          to: formatDate(locale, to),
        })
      : from
        ? t('comp.period.from', { from: formatDate(locale, from) })
        : to
          ? t('comp.period.to', { to: formatDate(locale, to) })
          : t('comp.period.all');

  async function search(e: Event) {
    e.preventDefault();
    if (loading) return;
    if (!subjectId) {
      toast.notify({
        tone: 'danger',
        title: t('comp.toast.noSubject'),
        description: t('comp.toast.noSubjectHint'),
      });
      return;
    }
    setLoading(true);
    setFailed([]);
    try {
      const user = usersById.get(subjectId);
      const res = (await send('/api/admin/compliance/ediscovery/search', 'POST', {
        scope,
        subject: subjectId,
        email: user?.email ?? '',
        from,
        to,
        query: query.trim(),
        case: caseRef.trim(),
      })) as PerService<EDiscoveryHit[]>;

      const hits: EDiscoveryHit[] = [];
      const ok: ComplianceService[] = [];
      const bad: string[] = [];
      for (const s of scopeServices(scope)) {
        const r = res[s];
        if (r?.ok) {
          hits.push(...(r.data ?? []));
          ok.push(s);
        } else {
          bad.push(t(SERVICE_KEY[s]));
        }
      }
      hits.sort((a, b) => (a.at < b.at ? 1 : -1));
      setResults(hits);
      setExportable(ok);
      setFailed(bad);
      setSearched(true);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('comp.toast.searchFailed'),
        description:
          err instanceof HttpStatus && err.status === 400
            ? t('comp.toast.badFilters')
            : t('comp.toast.serviceDown'),
      });
    } finally {
      setLoading(false);
    }
  }

  function exportUrl(service: ComplianceService): string {
    const user = usersById.get(subjectId);
    const p = new URLSearchParams({ service });
    p.set('subject', subjectId);
    if (user?.email) p.set('email', user.email);
    if (from) p.set('from', from);
    if (to) p.set('to', to);
    if (query.trim()) p.set('query', query.trim());
    if (caseRef.trim()) p.set('case', caseRef.trim());
    return withBase(
      import.meta.env.BASE_URL,
      `/api/admin/compliance/ediscovery/export?${p.toString()}`,
    );
  }

  function runExport(service: ComplianceService) {
    // Navegación de descarga (Content-Disposition attachment): la cookie httpOnly
    // autentica en el BFF; el servicio audita el export antes de emitir los bytes.
    window.location.href = exportUrl(service);
    setConfirmExport(null);
  }

  const exportSummary: ReviewItem[] = confirmExport
    ? [
        {
          label: t('comp.export.what'),
          value: t(confirmExport === 'mail' ? 'comp.export.whatMail' : 'comp.export.whatDrive'),
        },
        { label: t('comp.export.whose'), value: subjectEmail },
        { label: t('comp.export.period'), value: periodText },
        {
          label: t('comp.export.textFilter'),
          value: query.trim() || t('comp.export.noFilter'),
          mono: Boolean(query.trim()),
        },
        {
          label: t('comp.case.label'),
          value: caseRef.trim() || t('comp.case.none'),
          mono: Boolean(caseRef.trim()),
        },
      ]
    : [];

  return (
    <div class="flex flex-col gap-4 py-4">
      <InlineHelp tone="warning" title={t('comp.search.warningTitle')}>
        {t('comp.search.warningBody')}
      </InlineHelp>

      <SettingsSection title={t('comp.search.title')} description={t('comp.search.description')}>
        <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void search(e)}>
          <FieldGroup label={t('comp.who.person')} htmlFor="ed-subject" required class="w-72">
            <Select
              id="ed-subject"
              value={subjectId}
              required
              onChange={(e) => setSubjectId(e.currentTarget.value)}
            >
              <option value="">{t('pim.newElig.selectUser')}</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.fullName} · {u.email}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('comp.search.where')} htmlFor="ed-scope" class="w-52">
            <Select
              id="ed-scope"
              value={scope}
              onChange={(e) => setScope(e.currentTarget.value as Scope)}
            >
              {SEARCH_SCOPES.map((s) => (
                <option key={s.value} value={s.value}>
                  {t(s.short)}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('audit.since')} htmlFor="ed-from" class="w-40">
            <Input
              id="ed-from"
              type="date"
              value={from}
              onInput={(e) => setFrom(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('audit.until')} htmlFor="ed-to" class="w-40">
            <Input
              id="ed-to"
              type="date"
              value={to}
              onInput={(e) => setTo(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            label={t('comp.search.keyword')}
            htmlFor="ed-query"
            class="min-w-52 flex-1"
            help={t('comp.search.keywordHelp')}
          >
            <Input
              id="ed-query"
              value={query}
              placeholder={t('comp.search.keywordPlaceholder')}
              leadingIcon={<Icon name="search" size={16} />}
              onInput={(e) => setQuery(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            label={t('comp.case.label')}
            htmlFor="ed-case"
            class="w-48"
            help={t('comp.search.caseHelp')}
          >
            <Input
              id="ed-case"
              value={caseRef}
              placeholder="LIT-2026-014"
              class="font-mono"
              onInput={(e) => setCaseRef(e.currentTarget.value)}
            />
          </FieldGroup>
          <Button type="submit" variant="primary" loading={loading} class="shrink-0">
            {t('common.search')}
          </Button>
        </form>

        {!mailLive || !driveLive ? (
          <InlineHelp tone="warning">
            {!mailLive && !driveLive
              ? t('comp.search.bothDown')
              : t('comp.search.oneDown', {
                  service: t(!mailLive ? 'comp.service.mail' : 'comp.service.drive'),
                })}
          </InlineHelp>
        ) : null}
        {failed.length > 0 ? (
          <InlineHelp
            tone="danger"
            title={t('comp.search.partialTitle', { services: formatList(locale, failed) })}
          >
            {t('comp.search.partialBody')}
          </InlineHelp>
        ) : null}

        {loading ? (
          <div class="border-border bg-surface flex flex-col gap-2 rounded-lg border p-3">
            <span class="text-text-muted text-sm">
              {t('comp.search.searching', { subject: subjectEmail })}
            </span>
            {[0, 1, 2, 3, 4].map((i) => (
              <Skeleton key={i} height="1.75rem" />
            ))}
          </div>
        ) : !searched ? (
          <EmptyState
            title={t('comp.search.emptyTitle')}
            description={t('comp.search.emptyDescription')}
            illustration={<Icon name="search" size={28} />}
          />
        ) : results.length === 0 ? (
          <EmptyState
            title={t('comp.search.noResultsTitle')}
            description={t('comp.search.noResultsDescription')}
            illustration={<Icon name="search" size={28} />}
          />
        ) : (
          <>
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-text-muted mr-1 text-sm">
                {t('comp.search.resultCount', {
                  count: results.length,
                  subject: subjectEmail,
                })}
                {exportable.length > 0 ? ` ${t('comp.search.downloadFull')}` : ''}
              </span>
              {exportable.map((s) => (
                <Button
                  key={s}
                  variant="secondary"
                  size="sm"
                  leadingIcon={<Icon name="audit" size={14} />}
                  onClick={() => setConfirmExport(s)}
                >
                  {t(s === 'mail' ? 'comp.export.mailBtn' : 'comp.export.driveBtn')}
                </Button>
              ))}
            </div>
            <div class="border-border bg-surface overflow-hidden rounded-lg border">
              <div
                aria-hidden="true"
                class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
              >
                <span class="w-24 shrink-0">{t('comp.col.source')}</span>
                <span class="min-w-0 flex-1">{t('comp.col.item')}</span>
                <span class="hidden w-44 shrink-0 sm:block">{t('audit.col.date')}</span>
                <span class="hidden w-24 shrink-0 text-right md:block">{t('comp.col.size')}</span>
                <span class="w-32 shrink-0 text-right">{t('sso.col.status')}</span>
              </div>
              <ul class="max-h-[28rem] overflow-y-auto">
                {results.map((h) => (
                  <li
                    key={`${h.service}:${h.id}`}
                    class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 last:border-b-0"
                  >
                    <span class="w-24 shrink-0">{serviceBadge(h.service, t)}</span>
                    <span class="min-w-0 flex-1">
                      <span class="text-text block truncate font-medium" title={h.title}>
                        {h.title}
                      </span>
                      {h.subtitle ? (
                        <span class="sytue-meta block truncate" title={h.subtitle}>
                          {h.subtitle}
                        </span>
                      ) : null}
                    </span>
                    <span class="text-text-muted hidden w-44 shrink-0 text-sm whitespace-nowrap tabular-nums sm:block">
                      {formatWhen(locale, h.at)}
                    </span>
                    <span class="text-text-muted hidden w-24 shrink-0 text-right text-sm tabular-nums md:block">
                      {h.size ? formatBytes(locale, h.size) : '—'}
                    </span>
                    <span class="flex w-32 shrink-0 justify-end">
                      {h.preserved ? (
                        <Badge tone="warning">{t('comp.status.deletedPreserved')}</Badge>
                      ) : (
                        <span class="sytue-meta">—</span>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          </>
        )}
      </SettingsSection>

      {/* Export: qué se descarga exactamente, y que queda registrado */}
      <Modal
        open={Boolean(confirmExport)}
        onClose={() => setConfirmExport(null)}
        title={t('comp.export.title')}
      >
        {confirmExport ? (
          <div class="flex flex-col gap-4">
            <p class="text-text-muted text-sm leading-relaxed">{t('comp.export.intro')}</p>
            <ReviewSummary items={exportSummary} />
            <InlineHelp tone="warning" title={t('comp.export.auditedTitle')}>
              {t('comp.export.auditedBody')}
            </InlineHelp>
            <div class="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setConfirmExport(null)}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                leadingIcon={<Icon name="audit" size={14} />}
                onClick={() => runExport(confirmExport)}
              >
                {t(
                  confirmExport === 'mail' ? 'comp.export.downloadMbox' : 'comp.export.downloadZip',
                )}
              </Button>
            </div>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

// ---- Panel -------------------------------------------------------------------

function Inner(props: CompliancePanelProps) {
  const { locale } = props;
  const t = useT(locale);
  const tabs = [
    {
      id: 'holds',
      label: t('comp.tab.holds'),
      icon: <Icon name="lock" size={16} />,
      content: (
        <HoldsTab
          initial={props.holds}
          users={props.users}
          mailLive={props.mailLive}
          driveLive={props.driveLive}
          notesLive={props.notesLive}
          locale={locale}
          t={t}
        />
      ),
    },
    {
      id: 'ediscovery',
      label: 'eDiscovery',
      icon: <Icon name="search" size={16} />,
      content: (
        <SearchTab
          users={props.users}
          mailLive={props.mailLive}
          driveLive={props.driveLive}
          locale={locale}
          t={t}
        />
      ),
    },
  ];

  return (
    <SettingsShell
      title={t('page.compliance')}
      description={t('comp.subtitle')}
      banner={offlineNotice(props.mailLive, props.driveLive, props.notesLive, t)}
    >
      <Tabs items={tabs} />
    </SettingsShell>
  );
}

export default function CompliancePanel(props: CompliancePanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
