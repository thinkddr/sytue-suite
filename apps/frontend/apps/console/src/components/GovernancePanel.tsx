/**
 * GovernancePanel — gobernanza L5 del Sytue Integration Gateway (DLP/política de
 * egress + auditoría unificada).
 *
 * Una regla de egress puede dejar sin funcionar a una integración en producción, así
 * que crearla no es un formulario: es un asistente corto (qué tráfico afecta → qué
 * hacer con él, contado por su consecuencia real → resumen), y antes de guardar
 * comprueba contra el registro reciente si la regla estaría cortando llamadas que se
 * están haciendo ahora mismo.
 *
 * El enforcement vive en el gateway (único RoundTripper); esto es su plano de
 * administración. Solo @sytue/ui, Graphite & Brass, denso, oscuro, WCAG AA. El BFF
 * (cookie httpOnly) reenvía; el token nunca está aquí.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  SettingsSection,
  SettingsShell,
  Switch,
  Tabs,
  ToastProvider,
  Wizard,
  useToast,
  withBase,
  ConfirmDialog,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import type {
  EgressAuditRow,
  PolicyAction,
  PolicyDecisionRow,
  PolicyRule,
} from '../lib/gateway.js';
import { formatNumber, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { ConsoleMessageKey } from '../i18n/index.js';
import { formatWhen } from '../lib/format.js';

interface GovernancePanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  policies: PolicyRule[];
  egress: EgressAuditRow[];
  egressCursor: string;
  decisions: PolicyDecisionRow[];
  decisionsCursor: string;
  /** false = gateway no accesible; los paneles avisan y no editan. */
  live: boolean;
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

function errStatus(e: unknown): number {
  return e instanceof HttpStatus ? e.status : 0;
}

// ---- Reglas de política ------------------------------------------------------

/** Cada acción contada por lo que le pasa a la integración, no por su nombre técnico. */
const ACTIONS: {
  value: PolicyAction;
  title: ConsoleMessageKey;
  body: ConsoleMessageKey;
  badge?: ConsoleMessageKey;
}[] = [
  { value: 'warn', title: 'gov.act.warn', body: 'gov.act.warnBody', badge: 'gov.act.warnBadge' },
  { value: 'block', title: 'gov.act.block', body: 'gov.act.blockBody' },
  { value: 'allow', title: 'gov.act.allow', body: 'gov.act.allowBody' },
];

const CONDITION_FIELDS: {
  key: keyof EditableCondition;
  label: ConsoleMessageKey;
  placeholder: string;
  help: ConsoleMessageKey;
}[] = [
  {
    key: 'host',
    label: 'gov.field.host',
    placeholder: '.salesforce.com',
    help: 'gov.field.hostHelp',
  },
  {
    key: 'api_family',
    label: 'gov.field.family',
    placeholder: 'salesforce',
    help: 'gov.field.familyHelp',
  },
  { key: 'method', label: 'gov.field.method', placeholder: 'POST', help: 'gov.field.methodHelp' },
  {
    key: 'path_prefix',
    label: 'gov.field.path',
    placeholder: '/services/data',
    help: 'gov.field.pathHelp',
  },
  {
    key: 'body_contains',
    label: 'gov.field.body',
    placeholder: 'IBAN',
    help: 'gov.field.bodyHelp',
  },
];

interface EditableCondition {
  api_family: string;
  host: string;
  method: string;
  path_prefix: string;
  body_contains: string;
}

function emptyCondition(): EditableCondition {
  return { api_family: '', host: '', method: '', path_prefix: '', body_contains: '' };
}

function actionBadge(a: PolicyAction, t: T) {
  if (a === 'block') return <Badge tone="danger">{t('gov.badge.block')}</Badge>;
  if (a === 'warn') return <Badge tone="warning">{t('gov.badge.warn')}</Badge>;
  return <Badge tone="success">{t('gov.badge.allow')}</Badge>;
}

/** Resumen legible de la condición de una regla. */
function conditionSummary(c: PolicyRule['condition'], t: T): string {
  const parts = Object.entries(c)
    .filter(([, v]) => typeof v === 'string' && v)
    .map(([k, v]) => `${k}=${v}`);
  return parts.length ? parts.join(' · ') : t('gov.cond.any');
}

/** Mayúscula inicial: la frase de la condición se compone y a veces abre párrafo. */
function sentenceCase(text: string): string {
  return `${text.charAt(0).toUpperCase()}${text.slice(1)}`;
}

/** La condición en llano («las llamadas a … con método …»). */
function conditionSentence(c: EditableCondition, t: T): string {
  const parts: string[] = [];
  if (c.host.trim())
    parts.push(
      c.host.trim().startsWith('.')
        ? t('gov.cond.subdomain', { host: c.host.trim().slice(1) })
        : t('gov.cond.host', { host: c.host.trim() }),
    );
  if (c.api_family.trim()) parts.push(t('gov.cond.family', { family: c.api_family.trim() }));
  if (c.method.trim()) parts.push(t('gov.cond.method', { method: c.method.trim().toUpperCase() }));
  if (c.path_prefix.trim()) parts.push(t('gov.cond.path', { path: c.path_prefix.trim() }));
  if (c.body_contains.trim()) parts.push(t('gov.cond.body', { text: c.body_contains.trim() }));
  if (parts.length === 0) return t('gov.cond.all');
  return t('gov.cond.calls', { what: parts.join(' ') });
}

/**
 * ¿Esta fila del registro habría casado con la condición? Solo se puede comprobar lo
 * que el registro guarda: host y método. La ruta y el contenido no están ahí, así que
 * la cuenta es un TOPE (nunca casarían más llamadas de las que decimos).
 */
function matchesEgress(c: EditableCondition, row: EgressAuditRow): boolean {
  const host = c.host.trim().toLowerCase();
  if (host) {
    const h = (row.host || '').toLowerCase();
    const ok = host.startsWith('.') ? h.endsWith(host) || h === host.slice(1) : h === host;
    if (!ok) return false;
  }
  const method = c.method.trim().toUpperCase();
  if (method && (row.method || '').toUpperCase() !== method) return false;
  return true;
}

function hasAnyCondition(c: EditableCondition): boolean {
  return CONDITION_FIELDS.some((f) => c[f.key].trim() !== '');
}

function suggestName(c: EditableCondition, action: PolicyAction, t: T): string {
  const key =
    action === 'block'
      ? 'gov.suggest.block'
      : action === 'warn'
        ? 'gov.suggest.warn'
        : 'gov.suggest.allow';
  const text = t(key, { what: conditionSentence(c, t) });
  return text.length > 90 ? `${text.slice(0, 87)}…` : text;
}

// ---- Asistente: regla de egress ----------------------------------------------

function PolicyWizard({
  editing,
  isNew,
  egress,
  locale,
  t,
  onSaved,
  onCancel,
  onDone,
}: {
  editing: PolicyRule;
  isNew: boolean;
  /** Registro reciente de egress: sirve para avisar de lo que la regla cortaría. */
  egress: EgressAuditRow[];
  locale: Locale;
  t: T;
  onSaved: (r: PolicyRule) => void;
  onCancel: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  // En edición se entra por el resumen: todo está relleno y desde ahí se salta a lo
  // que haya que cambiar (los pasos anteriores quedan accesibles en el progreso).
  const [step, setStep] = useState(isNew ? 0 : 2);
  const [cond, setCond] = useState<EditableCondition>({
    ...emptyCondition(),
    ...editing.condition,
  });
  const [action, setAction] = useState<PolicyAction>(editing.action);
  const [name, setName] = useState(editing.name);
  const [enabled, setEnabled] = useState(editing.enabled);
  const [busy, setBusy] = useState(false);

  const suggested = suggestName(cond, action, t);
  const finalName = name.trim() || suggested;
  const wide = !hasAnyCondition(cond);

  /** Llamadas del registro reciente que esta regla habría alcanzado. */
  const impact = useMemo(() => {
    const matched = egress.filter((r) => matchesEgress(cond, r));
    const byHost = new Map<string, number>();
    for (const r of matched) byHost.set(r.host || '—', (byHost.get(r.host || '—') ?? 0) + 1);
    const top = [...byHost.entries()].sort((a, b) => b[1] - a[1]).slice(0, 3);
    return { matched: matched.length, total: egress.length, top };
  }, [cond, egress]);

  const impactHelp = (() => {
    if (action === 'allow') {
      return {
        tone: 'success' as const,
        title: t('gov.impact.allowTitle'),
        body: t('gov.impact.allowBody'),
      };
    }
    if (wide && action === 'block') {
      return {
        tone: 'danger' as const,
        title: t('gov.impact.wideTitle'),
        body: t('gov.impact.wideBody'),
      };
    }
    if (impact.total === 0) {
      return {
        tone: 'warning' as const,
        title: t('gov.impact.unknownTitle'),
        body: t('gov.impact.unknownBody'),
      };
    }
    if (impact.matched === 0) {
      return {
        tone: 'success' as const,
        title: t('gov.impact.noneTitle'),
        body: t('gov.impact.noneBody', { count: impact.total }),
      };
    }
    const hosts = impact.top.map(([h, n]) => `${h} (${formatNumber(locale, n)})`).join(', ');
    const counts = { matched: impact.matched, count: impact.total };
    return {
      tone: action === 'block' ? ('danger' as const) : ('warning' as const),
      title: t(action === 'block' ? 'gov.impact.blockTitle' : 'gov.impact.warnTitle', counts),
      body: `${t(action === 'block' ? 'gov.impact.blockBody' : 'gov.impact.warnBody', { hosts })} ${t('gov.impact.upperBound')}`,
    };
  })();

  async function save() {
    if (busy) return;
    setBusy(true);
    const condition: Record<string, string> = {};
    for (const f of CONDITION_FIELDS) {
      const v = cond[f.key].trim();
      if (v) condition[f.key] = v;
    }
    try {
      const rule = (
        isNew
          ? await send('/api/admin/policies', 'POST', {
              name: finalName,
              condition,
              action,
              enabled,
            })
          : await send(`/api/admin/policies/${editing.id}`, 'PATCH', {
              name: finalName,
              condition,
              action,
              enabled,
            })
      ) as PolicyRule;
      onSaved(rule);
      setStep(3);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description:
          errStatus(err) === 400
            ? t('gov.toast.badCondition')
            : errStatus(err) === 403
              ? t('gov.toast.forbidden')
              : t('gov.toast.gatewayDown'),
      });
    } finally {
      setBusy(false);
    }
  }

  const stepWhat: WizardStep = {
    id: 'traffic',
    title: t('gov.step.traffic'),
    heading: t('gov.step.trafficHeading'),
    description: t('gov.step.trafficDescription'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
          {CONDITION_FIELDS.map((f) => (
            <FieldGroup key={f.key} label={t(f.label)} htmlFor={`cond-${f.key}`}>
              <Input
                id={`cond-${f.key}`}
                value={cond[f.key]}
                placeholder={f.placeholder}
                class="font-mono"
                spellcheck={false}
                aria-describedby={`cond-${f.key}-hint`}
                onInput={(e) =>
                  setCond((c) => ({ ...c, [f.key]: (e.target as HTMLInputElement).value }))
                }
              />
              <FieldHint id={`cond-${f.key}-hint`}>{t(f.help)}</FieldHint>
            </FieldGroup>
          ))}
        </div>
        <InlineHelp
          tone={wide ? 'warning' : 'info'}
          title={t(wide ? 'gov.step.wideTitle' : 'gov.step.willMatch')}
        >
          {wide ? t('gov.step.wideBody') : `${sentenceCase(conditionSentence(cond, t))}.`}
        </InlineHelp>
      </div>
    ),
  };

  const stepAction: WizardStep = {
    id: 'action',
    title: t('gov.step.action'),
    heading: t('gov.step.actionHeading'),
    description: t('gov.step.actionDescription'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <div class="flex flex-col gap-3">
          {ACTIONS.map((a) => (
            <ChoiceCard
              key={a.value}
              name="sytue-policy-action"
              value={a.value}
              title={t(a.title)}
              description={t(a.body)}
              badge={a.badge ? <Badge tone="neutral">{t(a.badge)}</Badge> : null}
              selected={action === a.value}
              onSelect={() => setAction(a.value)}
            />
          ))}
        </div>
        <p class="sytue-meta leading-relaxed">{t('gov.step.orderNote')}</p>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('gov.review.affects'),
      value: sentenceCase(conditionSentence(cond, t)),
      onEdit: () => setStep(0),
    },
    {
      label: t('gov.review.happens'),
      value: (() => {
        const key = ACTIONS.find((a) => a.value === action)?.title;
        return key ? t(key) : '—';
      })(),
      onEdit: () => setStep(1),
    },
    {
      label: t('sso.col.status'),
      value: t(enabled ? 'gov.review.willApply' : 'gov.review.savedOff'),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('gov.step.review'),
    heading: t(isNew ? 'gov.step.reviewNew' : 'gov.step.reviewEdit'),
    description: t('gov.step.reviewDescription'),
    nextLabel: t(isNew ? 'gov.createRule' : 'roles.saveChanges'),
    onNext: () => void save(),
    content: (
      <div class="flex flex-col gap-4">
        <ReviewSummary items={summary} />
        <InlineHelp tone={impactHelp.tone} title={impactHelp.title}>
          {impactHelp.body}
        </InlineHelp>
        <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
          <FieldGroup label={t('gov.ruleName')} htmlFor="pol-name">
            <Input
              id="pol-name"
              value={name}
              placeholder={suggested}
              aria-describedby="pol-name-hint"
              onInput={(e) => setName(e.currentTarget.value)}
            />
            <FieldHint id="pol-name-hint">{t('gov.ruleNameHint', { suggested })}</FieldHint>
          </FieldGroup>
          <div>
            <Switch
              label={t('gov.applyNow')}
              checked={enabled}
              onChange={(e) => setEnabled(e.currentTarget.checked)}
            />
            <p class="text-text-muted mt-1.5 text-sm leading-snug">
              {t(enabled ? 'gov.applyNowOn' : 'gov.applyNowOff')}
            </p>
          </div>
        </div>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('gov.step.done'),
    heading: t(isNew ? 'gov.toast.created' : 'gov.step.savedChanges'),
    done: true,
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone={enabled ? 'success' : 'neutral'} title={finalName}>
          {enabled ? t('gov.done.on', { what: conditionSentence(cond, t) }) : t('gov.done.off')}
        </InlineHelp>
        {enabled && action !== 'allow' ? (
          <section class="border-border bg-surface-raised rounded-lg border p-4">
            <h3 class="text-text text-sm font-semibold">{t('gov.done.checkTitle')}</h3>
            <p class="text-text-muted mt-1 text-sm leading-relaxed">
              {t(action === 'block' ? 'gov.done.checkBodyBlock' : 'gov.done.checkBodyWarn')}
            </p>
          </section>
        ) : null}
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('gov.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWhat, stepAction, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}

function PoliciesTab({
  initial,
  egress,
  live,
  locale,
  t,
}: {
  initial: PolicyRule[];
  egress: EgressAuditRow[];
  live: boolean;
  locale: Locale;
  t: T;
}) {
  const toast = useToast();
  const [rules, setRules] = useState<PolicyRule[]>(initial);
  const [wizard, setWizard] = useState<{ rule: PolicyRule; isNew: boolean } | null>(null);
  const [confirmDel, setConfirmDel] = useState<PolicyRule | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  function upsert(rule: PolicyRule) {
    setRules((prev) => {
      const i = prev.findIndex((x) => x.id === rule.id);
      if (i < 0) return [rule, ...prev];
      const copy = prev.slice();
      copy[i] = rule;
      return copy;
    });
  }

  async function toggle(r: PolicyRule) {
    setBusyId(r.id);
    try {
      const updated = (await send(`/api/admin/policies/${r.id}`, 'PATCH', {
        name: r.name,
        condition: r.condition,
        action: r.action,
        enabled: !r.enabled,
      })) as PolicyRule;
      upsert(updated);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('gov.toast.toggleFailed'),
        description: t('common.toggleFailedHint', { name: r.name }),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function remove(r: PolicyRule) {
    setBusyId(r.id);
    try {
      await send(`/api/admin/policies/${r.id}`, 'DELETE');
      setRules((prev) => prev.filter((x) => x.id !== r.id));
      setConfirmDel(null);
      toast.notify({ tone: 'success', title: t('access.toast.deleted'), description: r.name });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('access.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  const newRule: PolicyRule = { id: '', name: '', condition: {}, action: 'warn', enabled: true };

  if (wizard) {
    return (
      <SettingsSection
        plain
        class="py-4"
        title={
          wizard.isNew
            ? t('gov.newRuleTitle')
            : t('labels.wizard.editTitle', { name: wizard.rule.name })
        }
        description={t('gov.wizardDescription')}
      >
        <PolicyWizard
          editing={wizard.rule}
          isNew={wizard.isNew}
          egress={egress}
          locale={locale}
          t={t}
          onSaved={upsert}
          onCancel={() => setWizard(null)}
          onDone={() => setWizard(null)}
        />
      </SettingsSection>
    );
  }

  return (
    <div class="flex flex-col gap-4 py-4">
      <SettingsSection
        title={t('gov.rules.title')}
        description={t('gov.rules.description')}
        actions={
          <Button
            variant="primary"
            disabled={!live}
            leadingIcon={<Icon name="plus" size={16} />}
            onClick={() => setWizard({ rule: newRule, isNew: true })}
          >
            {t('access.new')}
          </Button>
        }
      >
        {rules.length === 0 ? (
          <EmptyState
            title={t('access.empty.title')}
            description={t('gov.rules.emptyDescription')}
            illustration={<Icon name="lock" size={28} />}
            action={
              <Button
                variant="primary"
                disabled={!live}
                leadingIcon={<Icon name="plus" size={16} />}
                onClick={() => setWizard({ rule: newRule, isNew: true })}
              >
                {t('gov.rules.emptyAction')}
              </Button>
            }
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <div
              aria-hidden="true"
              class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
            >
              <span class="w-32 shrink-0">{t('gov.col.action')}</span>
              <span class="min-w-0 flex-1">{t('gov.col.rule')}</span>
              <span class="w-20 shrink-0 text-center">{t('gov.col.enabled')}</span>
              <span class="w-40 shrink-0 text-right">{t('devices.col.manage')}</span>
            </div>
            <ul>
              {rules.map((r) => (
                <li
                  key={r.id}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2.5 last:border-b-0"
                >
                  <span class="w-32 shrink-0">{actionBadge(r.action, t)}</span>
                  <span class="min-w-0 flex-1">
                    <span class="text-text block truncate font-medium">{r.name}</span>
                    <span
                      class="sytue-meta block truncate font-mono"
                      title={conditionSummary(r.condition, t)}
                    >
                      {conditionSummary(r.condition, t)}
                    </span>
                  </span>
                  <span class="w-20 shrink-0 text-center">
                    <Switch
                      label=""
                      checked={r.enabled}
                      disabled={!live || busyId === r.id}
                      aria-label={t('gov.toggleAria', { name: r.name })}
                      onChange={() => void toggle(r)}
                    />
                  </span>
                  <span class="flex w-40 shrink-0 items-center justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!live}
                      aria-label={t('common.editName', { name: r.name })}
                      onClick={() => setWizard({ rule: r, isNew: false })}
                    >
                      {t('common.edit')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!live || busyId === r.id}
                      aria-label={t('common.deleteName', { name: r.name })}
                      onClick={() => setConfirmDel(r)}
                    >
                      {t('common.delete')}
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </SettingsSection>

      {/* Borrar: la consecuencia, no un «¿seguro?» */}
      <ConfirmDialog
        open={Boolean(confirmDel)}
        onClose={() => setConfirmDel(null)}
        onConfirm={() => {
          if (confirmDel) void remove(confirmDel);
        }}
        title={t('access.delete.title')}
        confirmLabel={t('gov.delete.confirm')}
        tone="danger"
        busy={confirmDel ? busyId === confirmDel.id : false}
      >
        {confirmDel ? (
          <>
            <p class="text-text-muted text-sm leading-relaxed">
              {t('gov.delete.intro', { name: confirmDel.name })}
            </p>
            <InlineHelp
              tone={confirmDel.action === 'block' ? 'warning' : 'neutral'}
              title={t('gov.delete.afterTitle')}
            >
              {t(confirmDel.action === 'block' ? 'gov.delete.afterBlock' : 'gov.delete.afterWarn')}
            </InlineHelp>
          </>
        ) : null}
      </ConfirmDialog>
    </div>
  );
}

// ---- Registro de egress ------------------------------------------------------

function statusBadge(status: number, t: T) {
  if (status === 0) return <Badge tone="danger">{t('gov.networkError')}</Badge>;
  const tone =
    status < 300 ? 'success' : status < 400 ? 'neutral' : status < 500 ? 'warning' : 'danger';
  return <Badge tone={tone}>{status}</Badge>;
}

function EgressTab({
  initial,
  initialCursor,
  live,
  locale,
  t,
}: {
  initial: EgressAuditRow[];
  initialCursor: string;
  live: boolean;
  locale: Locale;
  t: T;
}) {
  const [items, setItems] = useState<EgressAuditRow[]>(initial);
  const [cursor, setCursor] = useState(initialCursor);
  const [host, setHost] = useState('');
  const [status, setStatus] = useState('');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [failed, setFailed] = useState(false);

  const filterParams = useMemo(() => {
    const p = new URLSearchParams();
    if (host.trim()) p.set('host', host.trim());
    if (status.trim()) p.set('status', status.trim());
    if (since) p.set('since', since);
    if (until) p.set('until', until);
    return p;
  }, [host, status, since, until]);

  const exportHref = useMemo(() => {
    const qs = filterParams.toString();
    return withBase(
      import.meta.env.BASE_URL,
      `/api/admin/egress-audit/export.csv${qs ? `?${qs}` : ''}`,
    );
  }, [filterParams]);

  async function fetchPage(reset: boolean, params: URLSearchParams = filterParams) {
    if (reset ? loading : loadingMore || !cursor) return;
    if (reset) setLoading(true);
    else setLoadingMore(true);
    setFailed(false);
    try {
      const p = new URLSearchParams(params);
      if (!reset && cursor) p.set('cursor', cursor);
      const res = await fetch(`/api/admin/egress-audit?${p.toString()}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const page = (await res.json()) as { items?: EgressAuditRow[]; nextCursor?: string };
      setItems((prev) => (reset ? (page.items ?? []) : [...prev, ...(page.items ?? [])]));
      setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
    } catch {
      setFailed(true);
    } finally {
      if (reset) setLoading(false);
      else setLoadingMore(false);
    }
  }

  return (
    <div class="flex flex-col gap-4 py-4">
      <SettingsSection
        title={t('gov.egress.title')}
        description={t('gov.egress.description')}
        actions={
          <a
            href={exportHref}
            class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex items-center gap-1.5 border px-3 py-1.5 text-sm font-semibold"
          >
            <Icon name="audit" size={16} aria-hidden="true" /> {t('gov.exportCsv')}
          </a>
        }
      >
        <form
          class="border-border bg-surface flex flex-wrap items-end gap-2 rounded-lg border px-3 py-2.5"
          onSubmit={(e) => {
            e.preventDefault();
            void fetchPage(true);
          }}
        >
          <Input
            type="search"
            value={host}
            placeholder={t('gov.filterHost')}
            aria-label={t('gov.filterHostAria')}
            leadingIcon={<Icon name="search" size={16} />}
            class="w-52"
            onInput={(e) => setHost(e.currentTarget.value)}
          />
          <Input
            type="number"
            value={status}
            placeholder={t('gov.col.response')}
            aria-label={t('gov.filterStatusAria')}
            class="w-28"
            onInput={(e) => setStatus(e.currentTarget.value)}
          />
          <Input
            type="date"
            value={since}
            aria-label={t('audit.since')}
            class="w-40"
            onInput={(e) => setSince(e.currentTarget.value)}
          />
          <Input
            type="date"
            value={until}
            aria-label={t('audit.until')}
            class="w-40"
            onInput={(e) => setUntil(e.currentTarget.value)}
          />
          <Button type="submit" variant="secondary" loading={loading}>
            {t('gov.applyFilters')}
          </Button>
        </form>

        {!live ? <InlineHelp tone="warning">{t('gov.egress.offline')}</InlineHelp> : null}
        {failed ? <InlineHelp tone="danger">{t('gov.egress.loadFailed')}</InlineHelp> : null}

        {items.length === 0 ? (
          <EmptyState
            title={t('gov.egress.emptyTitle')}
            description={t('gov.egress.emptyDescription')}
            illustration={<Icon name="globe" size={28} />}
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <div
              aria-hidden="true"
              class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
            >
              <span class="w-44 shrink-0">{t('audit.col.date')}</span>
              <span class="min-w-0 flex-1">{t('gov.col.site')}</span>
              <span class="hidden w-20 shrink-0 sm:block">{t('gov.col.call')}</span>
              <span class="w-24 shrink-0">{t('gov.col.response')}</span>
              <span class="hidden w-20 shrink-0 text-right md:block">{t('gov.col.took')}</span>
            </div>
            <ul class="max-h-[28rem] overflow-y-auto" aria-busy={loading}>
              {items.map((e) => (
                <li
                  key={e.id}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 last:border-b-0"
                >
                  <span class="text-text-muted w-44 shrink-0 text-sm whitespace-nowrap tabular-nums">
                    {formatWhen(locale, e.occurredAt)}
                  </span>
                  <span class="text-text min-w-0 flex-1 truncate font-mono text-sm" title={e.host}>
                    {e.host || '—'}
                    {e.error ? <span class="text-danger-text ml-2 text-xs">{e.error}</span> : null}
                  </span>
                  <span class="text-text-muted hidden w-20 shrink-0 text-sm sm:block">
                    {e.method}
                  </span>
                  <span class="w-24 shrink-0">{statusBadge(e.status, t)}</span>
                  <span class="text-text-muted hidden w-20 shrink-0 text-right text-sm tabular-nums md:block">
                    {formatNumber(locale, e.latencyMs, {
                      style: 'unit',
                      unit: 'millisecond',
                      unitDisplay: 'short',
                    })}
                  </span>
                </li>
              ))}
            </ul>
            {cursor ? (
              <div class="border-border bg-surface flex items-center justify-center border-t px-3 py-2">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void fetchPage(false)}
                >
                  {t('common.loadMore')}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </SettingsSection>
    </div>
  );
}

// ---- Decisiones de política --------------------------------------------------

function DecisionsTab({
  initial,
  initialCursor,
  live,
  locale,
  t,
}: {
  initial: PolicyDecisionRow[];
  initialCursor: string;
  live: boolean;
  locale: Locale;
  t: T;
}) {
  const [items, setItems] = useState<PolicyDecisionRow[]>(initial);
  const [cursor, setCursor] = useState(initialCursor);
  const [action, setAction] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [failed, setFailed] = useState(false);

  async function fetchPage(reset: boolean, act: string = action) {
    if (reset ? loading : loadingMore || !cursor) return;
    if (reset) setLoading(true);
    else setLoadingMore(true);
    setFailed(false);
    try {
      const p = new URLSearchParams();
      if (act) p.set('action', act);
      if (!reset && cursor) p.set('cursor', cursor);
      const res = await fetch(`/api/admin/policy-decisions?${p.toString()}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const page = (await res.json()) as { items?: PolicyDecisionRow[]; nextCursor?: string };
      setItems((prev) => (reset ? (page.items ?? []) : [...prev, ...(page.items ?? [])]));
      setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
    } catch {
      setFailed(true);
    } finally {
      if (reset) setLoading(false);
      else setLoadingMore(false);
    }
  }

  return (
    <div class="flex flex-col gap-4 py-4">
      <SettingsSection
        title={t('gov.decisions.title')}
        description={t('gov.decisions.description')}
        actions={
          <Select
            value={action}
            aria-label={t('gov.decisions.filterAria')}
            class="w-40"
            onChange={(e) => {
              const v = e.currentTarget.value;
              setAction(v);
              void fetchPage(true, v);
            }}
          >
            <option value="">{t('common.all')}</option>
            <option value="block">{t('gov.decisions.onlyBlocks')}</option>
            <option value="warn">{t('gov.decisions.onlyWarnings')}</option>
          </Select>
        }
      >
        {!live ? <InlineHelp tone="warning">{t('gov.decisions.offline')}</InlineHelp> : null}
        {failed ? <InlineHelp tone="danger">{t('gov.decisions.loadFailed')}</InlineHelp> : null}

        {items.length === 0 ? (
          <EmptyState
            title={t('gov.decisions.emptyTitle')}
            description={t('gov.decisions.emptyDescription')}
            illustration={<Icon name="check" size={28} />}
          />
        ) : (
          <div class="border-border bg-surface overflow-hidden rounded-lg border">
            <div
              aria-hidden="true"
              class="border-border bg-surface-sunken text-text-muted sytue-overline flex items-center gap-3 border-b px-3 py-2.5"
            >
              <span class="w-44 shrink-0">{t('audit.col.date')}</span>
              <span class="w-28 shrink-0">{t('gov.col.decision')}</span>
              <span class="min-w-0 flex-1">{t('gov.col.rule')}</span>
              <span class="hidden w-48 shrink-0 lg:block">{t('gov.col.destination')}</span>
            </div>
            <ul class="max-h-[28rem] overflow-y-auto" aria-busy={loading}>
              {items.map((d) => (
                <li
                  key={d.id}
                  class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 last:border-b-0"
                >
                  <span class="text-text-muted w-44 shrink-0 text-sm whitespace-nowrap tabular-nums">
                    {formatWhen(locale, d.occurredAt)}
                  </span>
                  <span class="w-28 shrink-0">
                    {d.action === 'block' ? (
                      <Badge tone="danger">{t('gov.decision.blocked')}</Badge>
                    ) : (
                      <Badge tone="warning">{t('gov.decision.warned')}</Badge>
                    )}
                  </span>
                  <span class="text-text min-w-0 flex-1 truncate" title={d.policyName}>
                    {d.policyName || '—'}
                  </span>
                  <span
                    class="text-text-muted hidden w-48 shrink-0 truncate font-mono text-xs lg:block"
                    title={`${d.method} ${d.host}`}
                  >
                    {d.method} {d.host}
                  </span>
                </li>
              ))}
            </ul>
            {cursor ? (
              <div class="border-border bg-surface flex items-center justify-center border-t px-3 py-2">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void fetchPage(false)}
                >
                  {t('common.loadMore')}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </SettingsSection>
    </div>
  );
}

// ---- Panel ------------------------------------------------------------------

function Inner(props: GovernancePanelProps) {
  const { locale } = props;
  const t = useT(locale);
  const tabs = [
    {
      id: 'policies',
      label: t('gov.tab.rules'),
      icon: <Icon name="lock" size={16} />,
      content: (
        <PoliciesTab
          initial={props.policies}
          egress={props.egress}
          live={props.live}
          locale={locale}
          t={t}
        />
      ),
    },
    {
      id: 'egress',
      label: t('gov.tab.egress'),
      icon: <Icon name="globe" size={16} />,
      content: (
        <EgressTab
          initial={props.egress}
          initialCursor={props.egressCursor}
          live={props.live}
          locale={locale}
          t={t}
        />
      ),
    },
    {
      id: 'decisions',
      label: t('gov.tab.decisions'),
      icon: <Icon name="audit" size={16} />,
      content: (
        <DecisionsTab
          initial={props.decisions}
          initialCursor={props.decisionsCursor}
          live={props.live}
          locale={locale}
          t={t}
        />
      ),
    },
  ];

  return (
    <SettingsShell
      title={t('page.governance')}
      description={t('gov.subtitle')}
      banner={
        props.live ? undefined : (
          <InlineHelp tone="warning" title={t('gov.offlineTitle')}>
            {t('gov.offlineBody')}
          </InlineHelp>
        )
      }
    >
      <Tabs items={tabs} />
    </SettingsShell>
  );
}

export default function GovernancePanel(props: GovernancePanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
