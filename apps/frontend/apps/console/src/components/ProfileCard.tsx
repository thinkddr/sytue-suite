/**
 * ProfileCard — edición del perfil propio (self-service). Solo el display name:
 * email verificado, rol y tenant se gestionan en otros flujos. PATCH vía el BFF
 * dedicado (/api/me/profile → Sytue ID PATCH /api/v1/me). Solo @sytue/ui, sobrio,
 * foco AA, toast de resultado. docs/CONVENTIONS.md §8.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Input,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { MeProfile } from '../lib/admin.js';

// ponytail: tope de UI; Sytue ID es quien valida/sanea de verdad (SanitizeDisplayName).
const MAX = 200;

function Inner({ name: initialName, email, t }: { name: string; email: string; t: T }) {
  const toast = useToast();
  const [name, setName] = useState(initialName);
  const [saved, setSaved] = useState(initialName);
  const [saving, setSaving] = useState(false);

  const trimmed = name.trim();
  const invalid = trimmed.length === 0 || trimmed.length > MAX;
  const dirty = trimmed !== saved;

  async function save() {
    if (invalid || !dirty || saving) return;
    setSaving(true);
    try {
      const res = await fetch('/api/me/profile', {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: trimmed }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const profile = (await res.json()) as MeProfile;
      setName(profile.name);
      setSaved(profile.name);
      toast.notify({ tone: 'success', title: t('profile.saved') });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description: code === '400' ? t('profile.invalidName') : t('common.tryAgain'),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        {/* h2: el h1 de la pagina lo pone account.astro. */}
        <CardTitle as="h2">{t('profile.title')}</CardTitle>
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <div class="flex max-w-sm flex-col gap-1.5">
          <label class="text-text text-sm font-medium" for="profile-name">
            {t('profile.displayName')}
          </label>
          <Input
            id="profile-name"
            value={name}
            maxLength={MAX}
            invalid={invalid}
            autoComplete="name"
            onInput={(e) => setName(e.currentTarget.value)}
          />
          {invalid ? <p class="text-danger-text text-xs">{t('profile.nameEmpty')}</p> : null}
        </div>
        <div class="flex max-w-sm flex-col gap-1.5">
          <label class="text-text text-sm font-medium" for="profile-email">
            {t('profile.email')}
          </label>
          <Input id="profile-email" value={email} disabled />
          <p class="text-text-muted text-xs">{t('profile.emailHint')}</p>
        </div>
        <div class="flex">
          <Button
            variant="primary"
            size="sm"
            loading={saving}
            disabled={invalid || !dirty}
            onClick={() => void save()}
          >
            {t('common.save')}
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

export default function ProfileCard({
  locale,
  ...props
}: {
  name: string;
  email: string;
  locale: Locale;
}) {
  return (
    <ToastProvider>
      <Inner {...props} t={useT(locale)} />
    </ToastProvider>
  );
}
