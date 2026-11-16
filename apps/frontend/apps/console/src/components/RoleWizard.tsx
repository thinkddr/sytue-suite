/**
 * RoleWizard — alta guiada de un rol personalizado.
 *
 * Antes era un modal con un nombre y ocho casillas, y se podía crear un rol sin
 * permisos sin enterarse. Ahora son tres preguntas —cómo se llama, qué puede hacer y
 * un resumen antes de crear— y, al terminar, el siguiente paso natural: asignarlo a
 * alguien. Usa el BFF (POST /api/admin/roles); el enforcement vive en Sytue ID.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  ReviewSummary,
  Wizard,
  useToast,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { PERMISSION_CATALOG, togglePerm } from '../lib/roles.js';
import type { CustomRole } from '../lib/types.js';
import { PermissionChecklist } from './PermissionChecklist.js';

export interface RoleWizardProps {
  t: T;
  onCreated: (role: CustomRole) => void;
  /** Abre la ficha del rol recién creado para asignarlo. */
  onAssign: (role: CustomRole) => void;
  onCancel: () => void;
  onDone: () => void;
}

export default function RoleWizard({ t, onCreated, onAssign, onCancel, onDone }: RoleWizardProps) {
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState('');
  const [perms, setPerms] = useState<string[]>([]);
  const [created, setCreated] = useState<CustomRole | null>(null);
  const trimmed = name.trim();

  // En el orden del catálogo, no en el de los clics: el resumen se lee igual siempre.
  const chosen = PERMISSION_CATALOG.filter((p) => perms.includes(p.key));

  async function create() {
    if (busy || !trimmed) return;
    setBusy(true);
    try {
      const res = await fetch('/api/admin/roles', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: trimmed, permissions: chosen.map((p) => p.key) }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const role = (await res.json()) as CustomRole;
      const full = { ...role, memberCount: role.memberCount ?? 0 };
      setCreated(full);
      onCreated(full);
      setStep(3);
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('roles.toast.createFailed'),
        description:
          code === '409'
            ? t('roles.toast.nameTaken')
            : code === '400'
              ? t('roles.toast.checkNameAndPerms')
              : t('common.tryAgain'),
      });
      // Un nombre repetido se arregla en su paso: se lleva allí al admin.
      if (code === '409') setStep(0);
    } finally {
      setBusy(false);
    }
  }

  const stepName: WizardStep = {
    id: 'name',
    title: t('roles.wizard.step.name'),
    heading: t('roles.wizard.name.heading'),
    description: t('roles.wizard.name.description'),
    canContinue: trimmed.length > 0,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <FieldGroup label={t('tenants.form.name')} htmlFor="new-role-name">
          <Input
            id="new-role-name"
            value={name}
            placeholder={t('roles.namePlaceholder')}
            aria-describedby="new-role-name-hint"
            onInput={(e) => setName(e.currentTarget.value)}
          />
          <FieldHint id="new-role-name-hint">{t('roles.wizard.name.hint')}</FieldHint>
        </FieldGroup>
      </div>
    ),
  };

  const stepPerms: WizardStep = {
    id: 'permissions',
    title: t('roles.wizard.step.permissions'),
    heading: t('roles.wizard.permissions.heading'),
    description: t('roles.wizard.permissions.description'),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <PermissionChecklist
          selected={perms}
          onToggle={(key) => setPerms((prev) => togglePerm(prev, key))}
          t={t}
        />
        {perms.length === 0 ? (
          <InlineHelp tone="warning" title={t('roles.wizard.permissions.noneTitle')}>
            {t('roles.wizard.permissions.noneBody')}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    { label: t('tenants.form.name'), value: trimmed || '—', onEdit: () => setStep(0) },
    {
      label: t('roles.permissions'),
      value: chosen.length ? chosen.map((p) => t(p.label)).join(' · ') : t('roles.noPermissions'),
      onEdit: () => setStep(1),
    },
  ];

  const stepReview: WizardStep = {
    id: 'review',
    title: t('roles.wizard.step.review'),
    heading: t('roles.wizard.review.heading'),
    description: t('roles.wizard.review.description'),
    nextLabel: t('roles.create'),
    canContinue: trimmed.length > 0,
    onNext: () => void create(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={summary} />
        <InlineHelp tone="neutral" title={t('roles.wizard.review.noteTitle')}>
          {t('roles.wizard.review.noteBody')}
        </InlineHelp>
      </div>
    ),
  };

  const stepDone: WizardStep = {
    id: 'done',
    title: t('roles.wizard.step.done'),
    heading: t('roles.wizard.done.heading'),
    done: true,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <InlineHelp tone="success" title={created?.name ?? trimmed}>
          {t('roles.wizard.done.body')}
        </InlineHelp>
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          <Button
            variant="primary"
            disabled={!created}
            onClick={() => created && onAssign(created)}
          >
            {t('roles.wizard.done.assign')}
          </Button>
          <Button variant="ghost" onClick={onDone}>
            {t('roles.wizard.done.back')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepName, stepPerms, stepReview, stepDone]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onCancel}
    />
  );
}
