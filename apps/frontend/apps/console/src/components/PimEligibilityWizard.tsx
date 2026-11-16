/**
 * PimEligibilityWizard — alta guiada de una elegibilidad PIM.
 *
 * Una elegibilidad dice quién PUEDE elevarse a qué rol, durante cuánto y si alguien
 * tiene que aprobarlo. En un modal de cuatro campos era fácil hacer elegible a
 * alguien para «owner» sin pensarlo; aquí cada decisión es un paso, los roles de
 * superusuario avisan de lo que suponen y se revisa todo antes de crear.
 * Usa el BFF (POST /api/admin/pim/eligibilities); el enforcement vive en Sytue ID.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Select,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import { formatNumber, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { PimEligibility } from '../lib/types.js';

export interface PimEligibilityWizardProps {
  t: T;
  locale: Locale;
  users: { id: string; email: string; name: string }[];
  roles: { id: string; name: string }[];
  onCreated: (elig: PimEligibility) => void;
  onCancel: () => void;
  onDone: () => void;
}

/** Ventana máxima que acepta Sytue ID para una activación (24 h). */
export const PIM_MAX_MINUTES = 1440;

/** Valor del rol: 'builtin:<role>' para los integrados, o el uuid del personalizado. */
export function roleRefFromValue(value: string): { role_id?: string; builtin_role?: string } {
  return value.startsWith('builtin:')
    ? { builtin_role: value.slice('builtin:'.length) }
    : { role_id: value };
}

export default function PimEligibilityWizard({
  t,
  locale,
  users,
  roles,
  onCreated,
  onCancel,
  onDone,
}: PimEligibilityWizardProps) {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [userId, setUserId] = useState('');
  const [roleValue, setRoleValue] = useState('');
  const [approval, setApproval] = useState(false);
  const [maxMinutes, setMaxMinutes] = useState(60);

  const user = users.find((u) => u.id === userId);
  const builtin = roleValue.startsWith('builtin:');
  const roleName = builtin
    ? t(roleValue === 'builtin:owner' ? 'pim.newElig.builtinOwner' : 'pim.newElig.builtinAdmin')
    : (roles.find((r) => r.id === roleValue)?.name ?? '');
  const maxValid = maxMinutes >= 1 && maxMinutes <= PIM_MAX_MINUTES;

  async function create() {
    if (busy || !userId || !roleValue || !maxValid) return;
    setBusy(true);
    try {
      const res = await fetch('/api/admin/pim/eligibilities', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          user_id: userId,
          ...roleRefFromValue(roleValue),
          requires_approval: approval,
          max_duration_minutes: maxMinutes,
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      onCreated((await res.json()) as PimEligibility);
      setStep(4);
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.eligCreateFailed'),
        description:
          code === '409'
            ? t('pim.toast.eligDuplicate')
            : code === '400'
              ? t('pim.toast.eligCheckFields')
              : t('common.tryAgain'),
      });
    } finally {
      setBusy(false);
    }
  }

  const stepWho: WizardStep = {
    id: 'who',
    title: t('pim.wizard.step.who'),
    heading: t('pim.wizard.who.heading'),
    description: t('pim.wizard.who.description'),
    canContinue: Boolean(userId),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <FieldGroup label={t('pim.newElig.user')} htmlFor="pim-user">
          <Select
            id="pim-user"
            value={userId}
            aria-describedby="pim-user-hint"
            onChange={(e) => setUserId(e.currentTarget.value)}
          >
            <option value="">{t('pim.newElig.selectUser')}</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name} · {u.email}
              </option>
            ))}
          </Select>
          <FieldHint id="pim-user-hint">{t('pim.wizard.who.hint')}</FieldHint>
        </FieldGroup>
      </div>
    ),
  };

  const stepRole: WizardStep = {
    id: 'role',
    title: t('pim.wizard.step.role'),
    heading: t('pim.wizard.role.heading'),
    description: t('pim.wizard.role.description'),
    canContinue: Boolean(roleValue),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <fieldset class="flex flex-col gap-2">
          <legend class="text-text-muted mb-1 text-xs font-medium">
            {t('pim.newElig.customRoles')}
          </legend>
          {roles.length === 0 ? (
            <p class="sytue-meta">{t('pim.wizard.role.noCustom')}</p>
          ) : (
            roles.map((r) => (
              <ChoiceCard
                key={r.id}
                dense
                name="sytue-pim-role"
                value={r.id}
                selected={roleValue === r.id}
                title={r.name}
                icon="lock"
                onSelect={() => setRoleValue(r.id)}
              />
            ))
          )}
        </fieldset>
        <fieldset class="flex flex-col gap-2">
          <legend class="text-text-muted mb-1 text-xs font-medium">
            {t('pim.newElig.builtinRoles')}
          </legend>
          {(['admin', 'owner'] as const).map((b) => (
            <ChoiceCard
              key={b}
              dense
              name="sytue-pim-role"
              value={`builtin:${b}`}
              selected={roleValue === `builtin:${b}`}
              title={t(b === 'owner' ? 'pim.newElig.builtinOwner' : 'pim.newElig.builtinAdmin')}
              description={t('pim.wizard.role.builtinBody')}
              icon="shield"
              onSelect={() => setRoleValue(`builtin:${b}`)}
            />
          ))}
        </fieldset>
        {builtin ? (
          <InlineHelp tone="warning" title={t('pim.wizard.role.builtinWarnTitle')}>
            {t('pim.wizard.role.builtinWarnBody')}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const stepRules: WizardStep = {
    id: 'rules',
    title: t('pim.wizard.step.rules'),
    heading: t('pim.wizard.rules.heading'),
    description: t('pim.wizard.rules.description'),
    canContinue: maxValid,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ChoiceCard
          name="sytue-pim-approval"
          selected={!approval}
          title={t('pim.directActivation')}
          description={t('pim.wizard.rules.directBody')}
          icon="key"
          onSelect={() => setApproval(false)}
        />
        <ChoiceCard
          name="sytue-pim-approval"
          selected={approval}
          title={t('pim.requiresApproval')}
          description={t('pim.wizard.rules.approvalBody')}
          icon="check"
          onSelect={() => setApproval(true)}
        />
        <FieldGroup label={t('pim.newElig.maxLabel')} htmlFor="pim-max">
          <Input
            id="pim-max"
            type="number"
            min={1}
            max={PIM_MAX_MINUTES}
            value={String(maxMinutes)}
            invalid={!maxValid}
            aria-describedby="pim-max-hint"
            onInput={(e) => setMaxMinutes(Number(e.currentTarget.value) || 0)}
          />
          <FieldHint id="pim-max-hint">{t('pim.newElig.maxHelp')}</FieldHint>
        </FieldGroup>
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('pim.newElig.user'),
      value: user ? `${user.name} · ${user.email}` : '—',
      onEdit: () => setStep(0),
    },
    { label: t('pim.newElig.role'), value: roleName || '—', onEdit: () => setStep(1) },
    {
      label: t('pim.wizard.review.activation'),
      value: t(approval ? 'pim.withApproval' : 'pim.directActivationShort'),
      onEdit: () => setStep(2),
    },
    {
      label: t('pim.newElig.maxLabel'),
      value: t('pim.minutes', { minutes: formatNumber(locale, maxMinutes) }),
      onEdit: () => setStep(2),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('pim.wizard.step.review'),
    heading: t('pim.wizard.review.heading'),
    description: t('pim.wizard.review.description'),
    nextLabel: t('pim.newElig.submit'),
    canContinue: Boolean(userId && roleValue && maxValid),
    onNext: () => void create(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={summary} />
        <InlineHelp tone="neutral" title={t('pim.wizard.review.noteTitle')}>
          {t('pim.wizard.review.noteBody')}
        </InlineHelp>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('pim.wizard.step.done'),
    heading: t('pim.wizard.done.heading'),
    done: true,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <InlineHelp tone="success" title={`${roleName} · ${user?.email ?? ''}`}>
          {t(approval ? 'pim.wizard.done.bodyApproval' : 'pim.wizard.done.bodyDirect')}
        </InlineHelp>
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button variant="primary" onClick={onDone}>
            {t('pim.wizard.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepWho, stepRole, stepRules, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}
