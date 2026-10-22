/**
 * AccessPolicyWizard — constructor guiado de una regla de Acceso Condicional.
 *
 * Cuatro preguntas, una por paso: a QUIÉN afecta (con sus exenciones break-glass),
 * CUÁNDO se aplica (red, aplicación, dispositivo de confianza, horario), QUÉ pasa
 * (permitir / exigir verificación / bloquear, contado por sus consecuencias) y un
 * RESUMEN que traduce la regla entera a una frase llana y avisa, antes de guardar,
 * de a quién dejaría fuera.
 *
 * Usa los endpoints BFF que ya existen (POST/PATCH /api/admin/access-policies). El
 * enforcement vive en el login de Sytue ID; esto es solo su plano de administración.
 *
 * Los textos vienen del catálogo (`wizard.accessPolicy.*`). La frase de la regla se
 * arma con PLANTILLAS completas —nunca concatenando trozos—, y las enumeraciones y
 * los nombres de los días los pone `Intl` con el locale.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Switch,
  Textarea,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import { contiene, formatDate, formatList, type Locale } from '@sytue/i18n';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
import type { AccessAction, AccessPolicy, AccessPolicyInput, AdminGroup } from '../lib/types.js';

export interface PickUser {
  id: string;
  email: string;
  name: string;
}

/** Días en orden Lun→Dom; el valor es el índice time.Weekday (domingo = 0). */
const WEEKDAYS = [1, 2, 3, 4, 5, 6, 0] as const;

/**
 * Nombre del día en el idioma activo. El 7 de enero de 2024 fue domingo, así que
 * `7 + i` cae en el día `i` de la máscara: los nombres los pone `Intl` y no hay
 * catorce cadenas que traducir cinco veces.
 */
function weekdayName(locale: Locale, i: number, weekday: 'long' | 'narrow'): string {
  return formatDate(locale, Date.UTC(2024, 0, 7 + i), { weekday, timeZone: 'UTC' });
}

const ACTIONS: { value: AccessAction; title: ConsoleMessageKey; body: ConsoleMessageKey }[] = [
  {
    value: 'require_mfa',
    title: 'wizard.accessPolicy.action.mfaTitle',
    body: 'wizard.accessPolicy.action.mfaBody',
  },
  {
    value: 'block',
    title: 'wizard.accessPolicy.action.blockTitle',
    body: 'wizard.accessPolicy.action.blockBody',
  },
  {
    value: 'allow',
    title: 'wizard.accessPolicy.action.allowTitle',
    body: 'wizard.accessPolicy.action.allowBody',
  },
];

export interface PolicyFormState {
  name: string;
  action: AccessAction;
  priority: number;
  enabled: boolean;
  appliesAll: boolean;
  includeUsers: string[];
  includeGroups: string[];
  excludeUsers: string[];
  excludeGroups: string[];
  ipRanges: string;
  ipNegate: boolean;
  clientIds: string;
  requireTrustedDevice: boolean;
  days: number[];
  startTime: string;
  endTime: string;
}

function emptyForm(nextPriority: number): PolicyFormState {
  return {
    name: '',
    action: 'require_mfa',
    priority: nextPriority,
    enabled: true,
    appliesAll: true,
    includeUsers: [],
    includeGroups: [],
    excludeUsers: [],
    excludeGroups: [],
    ipRanges: '',
    ipNegate: true,
    clientIds: '',
    requireTrustedDevice: false,
    days: [],
    startTime: '',
    endTime: '',
  };
}

function formFromPolicy(p: AccessPolicy): PolicyFormState {
  return {
    name: p.name,
    action: p.action,
    priority: p.priority,
    enabled: p.enabled,
    appliesAll: p.includeUsers.length === 0 && p.includeGroups.length === 0,
    includeUsers: p.includeUsers,
    includeGroups: p.includeGroups,
    excludeUsers: p.excludeUsers,
    excludeGroups: p.excludeGroups,
    ipRanges: p.ipRanges.join('\n'),
    ipNegate: p.ipNegate,
    clientIds: p.clientIds.join(', '),
    requireTrustedDevice: p.requireTrustedDevice,
    days: maskToDays(p.daysMask),
    startTime: minToTime(p.startMinute),
    endTime: minToTime(p.endMinute),
  };
}

// ---- Conversiones máscara/tiempo ↔ formulario ----

function maskToDays(mask: number): number[] {
  return WEEKDAYS.filter((i) => (mask & (1 << i)) !== 0);
}
function daysToMask(days: number[]): number {
  return days.reduce((m, d) => m | (1 << d), 0);
}
function minToTime(min: number): string {
  if (min < 0) return '';
  return `${String(Math.floor(min / 60)).padStart(2, '0')}:${String(min % 60).padStart(2, '0')}`;
}
function timeToMin(hhmm: string): number {
  const parts = hhmm.split(':');
  if (parts.length !== 2) return -1;
  const h = Number(parts[0]);
  const m = Number(parts[1]);
  if (!Number.isFinite(h) || !Number.isFinite(m)) return -1;
  return h * 60 + m;
}
function parseLines(s: string): string[] {
  return s
    .split(/[\n,]+/)
    .map((v) => v.trim())
    .filter(Boolean);
}
function parseTokens(s: string): string[] {
  return s
    .split(/[\s,]+/)
    .map((v) => v.trim())
    .filter(Boolean);
}
function toggle<V>(list: V[], v: V): V[] {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}

// ---- Validación en vivo ----

/** true si el rango no es un CIDR usable (v4 o v6). */
function badCidr(v: string): boolean {
  const slash = v.indexOf('/');
  if (slash < 1) return true;
  const addr = v.slice(0, slash);
  const bits = Number(v.slice(slash + 1));
  if (!Number.isInteger(bits) || bits < 0) return true;
  if (addr.includes(':')) return !/^[0-9a-f:]+$/i.test(addr) || bits > 128;
  const parts = addr.split('.');
  if (parts.length !== 4) return true;
  if (parts.some((p) => !/^\d{1,3}$/.test(p) || Number(p) > 255)) return true;
  return bits > 32;
}

function ipError(raw: string, t: T): string | null {
  const bad = parseLines(raw).find(badCidr);
  if (!bad) return null;
  return t('wizard.accessPolicy.err.badCidr', { range: bad });
}

function timeError(form: PolicyFormState, t: T): string | null {
  const hasStart = Boolean(form.startTime);
  const hasEnd = Boolean(form.endTime);
  if (hasStart !== hasEnd) return t('wizard.accessPolicy.err.bothTimes');
  return null;
}

// ---- La regla, contada en lenguaje llano ----

/** Enumeración natural; a partir de tres nombres, los dos primeros y «y N más». */
function listText(names: string[], t: T, locale: Locale): string {
  if (names.length <= 3) return formatList(locale, names);
  return formatList(locale, [
    ...names.slice(0, 2),
    t('wizard.accessPolicy.andMore', { count: names.length - 2 }),
  ]);
}

function namesOf(
  groupIds: string[],
  userIds: string[],
  groups: AdminGroup[],
  users: PickUser[],
  t: T,
): string[] {
  return [
    ...groupIds.map(
      (id) => groups.find((g) => g.id === id)?.name ?? t('wizard.accessPolicy.aGroup'),
    ),
    ...userIds.map((id) => {
      const u = users.find((x) => x.id === id);
      return u ? u.name || u.email : t('wizard.accessPolicy.aUser');
    }),
  ];
}

function whoText(
  form: PolicyFormState,
  groups: AdminGroup[],
  users: PickUser[],
  t: T,
  locale: Locale,
): string {
  if (form.appliesAll) return t('wizard.accessPolicy.who.everyone');
  const names = namesOf(form.includeGroups, form.includeUsers, groups, users, t);
  if (names.length === 0) return t('wizard.accessPolicy.who.nobodyYet');
  return listText(names, t, locale);
}

/** Cada condición marcada, en una frase impersonal e independiente. */
function conditionClauses(form: PolicyFormState, t: T, locale: Locale): string[] {
  const out: string[] = [];
  const ips = parseLines(form.ipRanges);
  if (ips.length) {
    out.push(
      t(form.ipNegate ? 'wizard.accessPolicy.cond.outside' : 'wizard.accessPolicy.cond.inside', {
        count: ips.length,
      }),
    );
  }
  if (form.requireTrustedDevice) out.push(t('wizard.accessPolicy.cond.untrusted'));
  const apps = parseTokens(form.clientIds);
  if (apps.length)
    out.push(t('wizard.accessPolicy.cond.apps', { apps: listText(apps, t, locale) }));
  if (form.days.length || form.startTime) {
    const days = form.days.length
      ? listText(
          WEEKDAYS.filter((i) => form.days.includes(i)).map((i) => weekdayName(locale, i, 'long')),
          t,
          locale,
        )
      : t('wizard.accessPolicy.cond.anyDay');
    out.push(
      form.startTime
        ? t('wizard.accessPolicy.cond.scheduleHours', {
            days,
            from: form.startTime,
            to: form.endTime,
          })
        : t('wizard.accessPolicy.cond.scheduleDays', { days }),
    );
  }
  return out;
}

function actionText(action: AccessAction, t: T): string {
  if (action === 'block') return t('wizard.accessPolicy.what.block');
  if (action === 'require_mfa') return t('wizard.accessPolicy.what.mfa');
  return t('wizard.accessPolicy.what.allow');
}

/** La regla entera en una frase. Es el texto que ve el admin en varios pasos. */
function ruleSentence(
  form: PolicyFormState,
  groups: AdminGroup[],
  users: PickUser[],
  t: T,
  locale: Locale,
): string {
  const who = whoText(form, groups, users, t, locale);
  const clauses = conditionClauses(form, t, locale);
  const what = actionText(form.action, t);
  if (clauses.length === 0) return t('wizard.accessPolicy.sentence.always', { who, what });
  return t('wizard.accessPolicy.sentence.when', {
    who,
    what,
    when: formatList(locale, clauses),
  });
}

function suggestName(form: PolicyFormState, t: T, locale: Locale): string {
  const verb = t(
    form.action === 'block'
      ? 'wizard.accessPolicy.suggest.block'
      : form.action === 'require_mfa'
        ? 'wizard.accessPolicy.suggest.mfa'
        : 'wizard.accessPolicy.suggest.allow',
  );
  const first = conditionClauses(form, t, locale)[0];
  const text = first
    ? t('wizard.accessPolicy.suggest.withCondition', { verb, condition: first })
    : t('wizard.accessPolicy.suggest.plain', { verb });
  return text.length > 90 ? `${text.slice(0, 87)}…` : text;
}

// ---- Aviso anti-lockout ----

type RiskTone = 'danger' | 'warning' | 'success';
interface Risk {
  tone: RiskTone;
  title: string;
  body: string;
  /** Regla catastrófica: exige confirmación explícita antes de guardar. */
  blocking: boolean;
}

function lockoutRisk(
  form: PolicyFormState,
  groups: AdminGroup[],
  users: PickUser[],
  t: T,
  locale: Locale,
): Risk {
  const exempt = namesOf(form.excludeGroups, form.excludeUsers, groups, users, t);
  const sentence = ruleSentence(form, groups, users, t, locale);
  const noConditions = conditionClauses(form, t, locale).length === 0;

  if (form.action === 'allow') {
    return {
      tone: 'success',
      title: t('wizard.accessPolicy.risk.allowTitle'),
      body: t('wizard.accessPolicy.risk.allowBody'),
      blocking: false,
    };
  }

  if (exempt.length === 0 && form.appliesAll && noConditions && form.action === 'block') {
    return {
      tone: 'danger',
      title: t('wizard.accessPolicy.risk.lockoutTitle'),
      body: t('wizard.accessPolicy.risk.lockoutBody', { sentence }),
      blocking: true,
    };
  }

  // El aviso del MFA no se pega con «+»: cada caso tiene su plantilla entera.
  const mfa = form.action === 'require_mfa';

  if (exempt.length === 0) {
    return {
      tone: form.appliesAll ? 'danger' : 'warning',
      title: t(
        form.appliesAll
          ? 'wizard.accessPolicy.risk.affectsYouTitle'
          : 'wizard.accessPolicy.risk.noExitTitle',
      ),
      body: t(
        mfa ? 'wizard.accessPolicy.risk.noExitBodyMfa' : 'wizard.accessPolicy.risk.noExitBody',
        { sentence },
      ),
      blocking: false,
    };
  }

  return {
    tone: 'success',
    title: t('wizard.accessPolicy.risk.hasExitTitle'),
    body: t(
      mfa ? 'wizard.accessPolicy.risk.hasExitBodyMfa' : 'wizard.accessPolicy.risk.hasExitBody',
      {
        sentence,
        exempt: listText(exempt, t, locale),
      },
    ),
    blocking: false,
  };
}

// ---- Selector múltiple (grupos / usuarios) ----

interface PickOption {
  id: string;
  label: string;
  sub: string;
}

function PickList({
  t,
  options,
  selected,
  onToggle,
  filterable,
  emptyText,
}: {
  t: T;
  options: PickOption[];
  selected: string[];
  onToggle: (id: string) => void;
  filterable: boolean;
  emptyText: string;
}) {
  const [q, setQ] = useState('');
  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return options;
    return options.filter((o) => contiene(o.label, needle) || contiene(o.sub, needle));
  }, [options, q]);
  if (options.length === 0) {
    return <p class="sytue-meta px-1 py-2">{emptyText}</p>;
  }
  return (
    <div class="border-border bg-surface flex flex-col gap-1 rounded-md border p-2">
      {filterable ? (
        <Input
          type="search"
          value={q}
          placeholder={t('wizard.accessPolicy.pick.filterPlaceholder')}
          aria-label={t('wizard.accessPolicy.pick.filterAria')}
          class="mb-1"
          onInput={(e) => setQ(e.currentTarget.value)}
        />
      ) : null}
      <div class="flex max-h-40 flex-col gap-1.5 overflow-y-auto">
        {shown.map((o) => (
          <Checkbox
            key={o.id}
            label={o.sub ? `${o.label} · ${o.sub}` : o.label}
            checked={selected.includes(o.id)}
            onChange={() => onToggle(o.id)}
          />
        ))}
        {shown.length === 0 ? (
          <p class="sytue-meta px-1 py-1">{t('wizard.accessPolicy.pick.noMatches')}</p>
        ) : null}
      </div>
    </div>
  );
}

// ---- Componente ----

interface AccessPolicyWizardProps {
  /** `t` ya atada al locale; llega del panel, que es quien negocia el idioma. */
  t: T;
  /** Locale para enumeraciones y nombres de los días. */
  locale: Locale;
  /** Regla existente: el asistente arranca en el resumen, con todo relleno. */
  editing?: AccessPolicy | undefined;
  groups: AdminGroup[];
  users: PickUser[];
  /** Prioridad propuesta para una regla nueva (va al final de la lista). */
  nextPriority: number;
  onSaved: (p: AccessPolicy) => void;
  onCancel: () => void;
  onDone: () => void;
}

export default function AccessPolicyWizard({
  t,
  locale,
  editing,
  groups,
  users,
  nextPriority,
  onSaved,
  onCancel,
  onDone,
}: AccessPolicyWizardProps) {
  const toast = useToast();
  const [form, setForm] = useState<PolicyFormState>(
    editing ? formFromPolicy(editing) : emptyForm(nextPriority),
  );
  // En edición se entra por el resumen: todo está relleno y desde ahí se salta a lo
  // que haya que cambiar (los pasos anteriores quedan accesibles en el progreso).
  const [step, setStep] = useState(editing ? 3 : 0);
  const [busy, setBusy] = useState(false);
  const [understood, setUnderstood] = useState(false);

  const set = <K extends keyof PolicyFormState>(k: K, v: PolicyFormState[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const groupOptions: PickOption[] = useMemo(
    () => groups.map((g) => ({ id: g.id, label: g.name, sub: '' })),
    [groups],
  );
  const userOptions: PickOption[] = useMemo(
    () => users.map((u) => ({ id: u.id, label: u.name || u.email, sub: u.email })),
    [users],
  );

  const errIp = ipError(form.ipRanges, t);
  const errTime = timeError(form, t);
  const risk = lockoutRisk(form, groups, users, t, locale);
  const suggested = suggestName(form, t, locale);
  const finalName = form.name.trim() || suggested;
  const sentence = ruleSentence(form, groups, users, t, locale);
  const exemptNames = namesOf(form.excludeGroups, form.excludeUsers, groups, users, t);
  const clauses = conditionClauses(form, t, locale);

  async function save() {
    if (busy) return;
    setBusy(true);
    const payload: AccessPolicyInput = {
      name: finalName,
      action: form.action,
      enabled: form.enabled,
      priority: form.priority,
      include_users: form.appliesAll ? [] : form.includeUsers,
      include_groups: form.appliesAll ? [] : form.includeGroups,
      exclude_users: form.excludeUsers,
      exclude_groups: form.excludeGroups,
      ip_ranges: parseLines(form.ipRanges),
      ip_negate: form.ipNegate,
      client_ids: parseTokens(form.clientIds),
      require_trusted_device: form.requireTrustedDevice,
      days_mask: daysToMask(form.days),
      start_minute: timeToMin(form.startTime),
      end_minute: timeToMin(form.endTime),
    };
    try {
      const res = await fetch(
        editing
          ? `/api/admin/access-policies/${encodeURIComponent(editing.id)}`
          : '/api/admin/access-policies',
        {
          method: editing ? 'PATCH' : 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(payload),
        },
      );
      if (!res.ok) throw new Error(String(res.status));
      onSaved((await res.json()) as AccessPolicy);
      setStep(4);
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description: t(
          code === '400'
            ? 'wizard.accessPolicy.fix.badData'
            : code === '403'
              ? 'wizard.accessPolicy.fix.forbidden'
              : 'wizard.accessPolicy.fix.idpDown',
        ),
      });
    } finally {
      setBusy(false);
    }
  }

  // ---- Pasos ----

  const stepWho: WizardStep = {
    id: 'who',
    title: t('wizard.accessPolicy.step.who'),
    heading: t('wizard.accessPolicy.who.heading'),
    description: t('wizard.accessPolicy.who.description'),
    canContinue: form.appliesAll || form.includeGroups.length > 0 || form.includeUsers.length > 0,
    content: (
      <div class="flex max-w-2xl flex-col gap-5">
        <div class="flex flex-col gap-3">
          <ChoiceCard
            name="sytue-policy-who"
            selected={form.appliesAll}
            title={t('wizard.accessPolicy.who.allTitle')}
            description={t('wizard.accessPolicy.who.allBody')}
            onSelect={() => set('appliesAll', true)}
          />
          <ChoiceCard
            name="sytue-policy-who"
            selected={!form.appliesAll}
            title={t('wizard.accessPolicy.who.someTitle')}
            description={t('wizard.accessPolicy.who.someBody')}
            onSelect={() => set('appliesAll', false)}
          />
        </div>

        {!form.appliesAll ? (
          <div class="flex flex-col gap-4">
            <div>
              <p class="text-text mb-1.5 text-sm font-semibold">
                {t('wizard.accessPolicy.who.groupsLabel')}
              </p>
              <PickList
                t={t}
                options={groupOptions}
                selected={form.includeGroups}
                onToggle={(id) => set('includeGroups', toggle(form.includeGroups, id))}
                filterable={groupOptions.length > 8}
                emptyText={t('wizard.accessPolicy.who.noGroups')}
              />
            </div>
            <div>
              <p class="text-text mb-1.5 text-sm font-semibold">
                {t('wizard.accessPolicy.who.usersLabel')}
              </p>
              <PickList
                t={t}
                options={userOptions}
                selected={form.includeUsers}
                onToggle={(id) => set('includeUsers', toggle(form.includeUsers, id))}
                filterable
                emptyText={t('wizard.accessPolicy.who.noUsers')}
              />
            </div>
            {form.includeGroups.length === 0 && form.includeUsers.length === 0 ? (
              <FieldHint tone="danger">{t('wizard.accessPolicy.who.pickAtLeastOne')}</FieldHint>
            ) : null}
          </div>
        ) : null}

        <section class="border-border-subtle flex flex-col gap-3 border-t pt-4">
          <div>
            <h3 class="text-text text-sm font-semibold">
              {t('wizard.accessPolicy.exempt.heading')}
            </h3>
            <p class="text-text-muted mt-0.5 text-sm leading-relaxed">
              {t('wizard.accessPolicy.exempt.description')}
            </p>
          </div>
          <div>
            <p class="text-text-muted mb-1.5 text-xs font-medium">
              {t('wizard.accessPolicy.exempt.groupsLabel')}
            </p>
            <PickList
              t={t}
              options={groupOptions}
              selected={form.excludeGroups}
              onToggle={(id) => set('excludeGroups', toggle(form.excludeGroups, id))}
              filterable={groupOptions.length > 8}
              emptyText={t('wizard.accessPolicy.who.noGroups')}
            />
          </div>
          <div>
            <p class="text-text-muted mb-1.5 text-xs font-medium">
              {t('wizard.accessPolicy.exempt.usersLabel')}
            </p>
            <PickList
              t={t}
              options={userOptions}
              selected={form.excludeUsers}
              onToggle={(id) => set('excludeUsers', toggle(form.excludeUsers, id))}
              filterable
              emptyText={t('wizard.accessPolicy.who.noUsers')}
            />
          </div>
          {exemptNames.length === 0 ? (
            <InlineHelp tone="warning">{t('wizard.accessPolicy.exempt.none')}</InlineHelp>
          ) : (
            <InlineHelp tone="success">
              {t('wizard.accessPolicy.exempt.some', {
                names: listText(exemptNames, t, locale),
              })}
            </InlineHelp>
          )}
        </section>
      </div>
    ),
  };

  const stepWhen: WizardStep = {
    id: 'when',
    title: t('wizard.accessPolicy.step.when'),
    heading: t('wizard.accessPolicy.when.heading'),
    description: t('wizard.accessPolicy.when.description'),
    canContinue: !errIp && !errTime,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <section class="border-border bg-surface-raised flex flex-col gap-3 rounded-lg border p-4">
          <div>
            <h3 class="text-text text-sm font-semibold">
              {t('wizard.accessPolicy.when.networkTitle')}
            </h3>
            <p class="text-text-muted mt-0.5 text-sm leading-relaxed">
              {t('wizard.accessPolicy.when.networkBody')}
            </p>
          </div>
          <FieldGroup label={t('wizard.accessPolicy.when.networksLabel')} htmlFor="pol-ip">
            <Textarea
              id="pol-ip"
              rows={2}
              value={form.ipRanges}
              placeholder="203.0.113.0/24"
              class="font-mono"
              spellcheck={false}
              aria-describedby="pol-ip-hint"
              invalid={Boolean(errIp)}
              onInput={(e) => set('ipRanges', e.currentTarget.value)}
            />
            <FieldHint
              id="pol-ip-hint"
              tone={errIp ? 'danger' : form.ipRanges.trim() ? 'success' : 'neutral'}
            >
              {errIp
                ? errIp
                : form.ipRanges.trim()
                  ? t('wizard.accessPolicy.when.networksOk', {
                      count: parseLines(form.ipRanges).length,
                    })
                  : t('wizard.accessPolicy.when.networksHint')}
            </FieldHint>
          </FieldGroup>
          {parseLines(form.ipRanges).length > 0 ? (
            <div class="flex flex-col gap-2">
              <ChoiceCard
                name="sytue-policy-ip"
                selected={form.ipNegate}
                title={t('wizard.accessPolicy.when.outsideTitle')}
                description={t('wizard.accessPolicy.when.outsideBody')}
                onSelect={() => set('ipNegate', true)}
              />
              <ChoiceCard
                name="sytue-policy-ip"
                selected={!form.ipNegate}
                title={t('wizard.accessPolicy.when.insideTitle')}
                description={t('wizard.accessPolicy.when.insideBody')}
                onSelect={() => set('ipNegate', false)}
              />
            </div>
          ) : null}
        </section>

        <section class="border-border bg-surface-raised flex flex-col gap-3 rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">
            {t('wizard.accessPolicy.when.deviceTitle')}
          </h3>
          <Switch
            label={t('wizard.accessPolicy.when.deviceSwitch')}
            checked={form.requireTrustedDevice}
            onChange={(e) => set('requireTrustedDevice', e.currentTarget.checked)}
          />
          <p class="text-text-muted text-sm leading-relaxed">
            {t('wizard.accessPolicy.when.deviceBody')}
          </p>
        </section>

        <section class="border-border bg-surface-raised flex flex-col gap-3 rounded-lg border p-4">
          <h3 class="text-text text-sm font-semibold">{t('wizard.accessPolicy.when.appsTitle')}</h3>
          <FieldGroup label={t('wizard.accessPolicy.when.appsLabel')} htmlFor="pol-apps">
            <Input
              id="pol-apps"
              value={form.clientIds}
              placeholder="sytue-mail sytue-drive"
              class="font-mono"
              spellcheck={false}
              aria-describedby="pol-apps-hint"
              onInput={(e) => set('clientIds', e.currentTarget.value)}
            />
            <FieldHint id="pol-apps-hint">{t('wizard.accessPolicy.when.appsHint')}</FieldHint>
          </FieldGroup>
        </section>

        <section class="border-border bg-surface-raised flex flex-col gap-3 rounded-lg border p-4">
          <div>
            <h3 class="text-text text-sm font-semibold">
              {t('wizard.accessPolicy.when.timeTitle')}
            </h3>
            <p class="text-text-muted mt-0.5 text-sm leading-relaxed">
              {t('wizard.accessPolicy.when.timeBody')}
            </p>
          </div>
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text-muted text-xs font-medium">
              {t('wizard.accessPolicy.when.daysLegend')}
            </legend>
            <div class="flex flex-wrap gap-3">
              {WEEKDAYS.map((i) => (
                <Checkbox
                  key={i}
                  label={weekdayName(locale, i, 'narrow')}
                  aria-label={weekdayName(locale, i, 'long')}
                  checked={form.days.includes(i)}
                  onChange={() => set('days', toggle(form.days, i))}
                />
              ))}
            </div>
          </fieldset>
          <div class="grid gap-3 sm:grid-cols-2">
            <FieldGroup label={t('wizard.accessPolicy.when.from')} htmlFor="pol-start">
              <Input
                id="pol-start"
                type="time"
                value={form.startTime}
                invalid={Boolean(errTime)}
                onInput={(e) => set('startTime', e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup label={t('wizard.accessPolicy.when.to')} htmlFor="pol-end">
              <Input
                id="pol-end"
                type="time"
                value={form.endTime}
                invalid={Boolean(errTime)}
                onInput={(e) => set('endTime', e.currentTarget.value)}
              />
            </FieldGroup>
          </div>
          {errTime ? <FieldHint tone="danger">{errTime}</FieldHint> : null}
        </section>

        <InlineHelp tone="info" title={t('wizard.accessPolicy.when.soFarTitle')}>
          {clauses.length === 0
            ? t('wizard.accessPolicy.when.soFarNone')
            : t('wizard.accessPolicy.when.soFarSome', { when: formatList(locale, clauses) })}
        </InlineHelp>
      </div>
    ),
  };

  const stepWhat: WizardStep = {
    id: 'what',
    title: t('wizard.accessPolicy.step.what'),
    heading: t('wizard.accessPolicy.what.heading'),
    description: t('wizard.accessPolicy.what.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <div class="flex flex-col gap-3">
          {ACTIONS.map((a) => (
            <ChoiceCard
              key={a.value}
              name="sytue-policy-action"
              selected={form.action === a.value}
              title={t(a.title)}
              description={t(a.body)}
              onSelect={() => set('action', a.value)}
            />
          ))}
        </div>
        <InlineHelp tone="info" title={t('wizard.accessPolicy.sentenceTitle')}>
          {sentence}
        </InlineHelp>
        <p class="sytue-meta leading-relaxed">{t('wizard.accessPolicy.what.strictestWins')}</p>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('wizard.accessPolicy.summary.who'),
      value: whoText(form, groups, users, t, locale),
      onEdit: () => setStep(0),
    },
    {
      label: t('wizard.accessPolicy.summary.exempt'),
      value: exemptNames.length
        ? listText(exemptNames, t, locale)
        : t('wizard.accessPolicy.summary.nobody'),
      onEdit: () => setStep(0),
    },
    {
      label: t('wizard.accessPolicy.summary.when'),
      value:
        clauses.length === 0
          ? t('wizard.accessPolicy.summary.whenAlways')
          : t('wizard.accessPolicy.summary.whenSome', { when: formatList(locale, clauses) }),
      onEdit: () => setStep(1),
    },
    {
      label: t('wizard.accessPolicy.summary.what'),
      value: t(
        ACTIONS.find((a) => a.value === form.action)?.title ??
          'wizard.accessPolicy.action.mfaTitle',
      ),
      onEdit: () => setStep(2),
    },
    {
      label: t('wizard.accessPolicy.summary.state'),
      value: t(
        form.enabled
          ? 'wizard.accessPolicy.summary.stateOn'
          : 'wizard.accessPolicy.summary.stateOff',
      ),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('wizard.accessPolicy.step.review'),
    heading: t(
      editing ? 'wizard.accessPolicy.review.headingEdit' : 'wizard.accessPolicy.review.headingNew',
    ),
    description: t('wizard.accessPolicy.review.description'),
    nextLabel: t(
      editing ? 'wizard.accessPolicy.review.saveEdit' : 'wizard.accessPolicy.review.saveNew',
    ),
    canContinue: !risk.blocking || !form.enabled || understood,
    onNext: () => void save(),
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone="neutral" title={t('wizard.accessPolicy.sentenceTitle')}>
          {sentence}
        </InlineHelp>

        <ReviewSummary items={summary} />

        <InlineHelp tone={risk.tone} title={risk.title}>
          {risk.body}
        </InlineHelp>

        {risk.blocking && form.enabled ? (
          <div class="border-danger bg-danger-bg rounded-md border p-3">
            <Checkbox
              label={t('wizard.accessPolicy.review.understood')}
              checked={understood}
              onChange={(e) => setUnderstood(e.currentTarget.checked)}
            />
          </div>
        ) : null}

        <div class="border-border bg-surface-raised flex flex-col gap-4 rounded-lg border p-4">
          <FieldGroup label={t('wizard.accessPolicy.review.nameLabel')} htmlFor="pol-name">
            <Input
              id="pol-name"
              value={form.name}
              placeholder={suggested}
              aria-describedby="pol-name-hint"
              onInput={(e) => set('name', e.currentTarget.value)}
            />
            <FieldHint id="pol-name-hint">
              {t('wizard.accessPolicy.review.nameHint', { suggested })}
            </FieldHint>
          </FieldGroup>

          <FieldGroup label={t('wizard.accessPolicy.review.priorityLabel')} htmlFor="pol-prio">
            <Input
              id="pol-prio"
              type="number"
              min={0}
              value={String(form.priority)}
              class="max-w-32"
              aria-describedby="pol-prio-hint"
              onInput={(e) => set('priority', Math.trunc(Number(e.currentTarget.value) || 0))}
            />
            <FieldHint id="pol-prio-hint">{t('wizard.accessPolicy.review.priorityHint')}</FieldHint>
          </FieldGroup>

          <div>
            <Switch
              label={t('wizard.accessPolicy.review.enableLabel')}
              checked={form.enabled}
              onChange={(e) => set('enabled', e.currentTarget.checked)}
            />
            <p class="text-text-muted mt-1.5 text-sm leading-snug">
              {t(
                form.enabled
                  ? 'wizard.accessPolicy.review.enableOn'
                  : 'wizard.accessPolicy.review.enableOff',
              )}
            </p>
          </div>
        </div>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('wizard.accessPolicy.step.done'),
    heading: t(
      editing ? 'wizard.accessPolicy.done.headingEdit' : 'wizard.accessPolicy.done.headingNew',
    ),
    done: true,
    content: (
      <div class="flex flex-col gap-4">
        <InlineHelp tone={form.enabled ? 'success' : 'neutral'} title={finalName}>
          {t(
            form.enabled ? 'wizard.accessPolicy.done.bodyOn' : 'wizard.accessPolicy.done.bodyOff',
            {
              sentence,
            },
          )}
        </InlineHelp>
        {form.enabled && form.action !== 'allow' ? (
          <section class="border-border bg-surface-raised rounded-lg border p-4">
            <h3 class="text-text text-sm font-semibold">
              {t('wizard.accessPolicy.done.checkTitle')}
            </h3>
            <p class="text-text-muted mt-1 text-sm leading-relaxed">
              {exemptNames.length
                ? t('wizard.accessPolicy.done.checkBodyExempt', {
                    names: listText(exemptNames, t, locale),
                  })
                : t('wizard.accessPolicy.done.checkBody')}
            </p>
          </section>
        ) : null}
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('wizard.accessPolicy.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWho, stepWhen, stepWhat, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}
