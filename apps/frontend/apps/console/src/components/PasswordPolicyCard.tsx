/**
 * PasswordPolicyCard — ajustes de seguridad de la organización: política de
 * contraseñas (longitud mínima + exigir complejidad). Sytue ID la guarda y la
 * aplica en TODOS los puntos donde se fija/cambia una contraseña. GET/PATCH vía
 * el BFF (/api/admin/org/password-policy). Solo @sytue/ui. docs/CONVENTIONS.md §8.
 */
import { useState } from 'preact/hooks';
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Input,
  Switch,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { PasswordPolicy } from '../lib/types.js';

const MIN = 8;
const MAX = 128;

function Inner({ policy, live, t }: { policy: PasswordPolicy; live: boolean; t: T }) {
  const toast = useToast();
  const [minLength, setMinLength] = useState(policy.min_length);
  const [complexity, setComplexity] = useState(policy.require_complexity);
  const [saving, setSaving] = useState(false);
  // La última política guardada, que es la base contra la que se mide `dirty`. En
  // estado y no escribiendo sobre la prop: esa es del padre.
  const [base, setBase] = useState(policy);

  const invalid = !Number.isInteger(minLength) || minLength < MIN || minLength > MAX;
  const dirty = minLength !== base.min_length || complexity !== base.require_complexity;

  async function save() {
    if (invalid || !dirty || saving) return;
    setSaving(true);
    try {
      const res = await fetch('/api/admin/org/password-policy', {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ min_length: minLength, require_complexity: complexity }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const saved = (await res.json()) as PasswordPolicy;
      setMinLength(saved.min_length);
      setComplexity(saved.require_complexity);
      // La política guardada pasa a ser la nueva base: refleja el estado del servicio.
      setBase(saved);
      toast.notify({
        tone: 'success',
        title: t('passwordPolicy.saved'),
        description: t('passwordPolicy.savedHint'),
      });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('passwordPolicy.saveFailed'),
        description: code === '403' ? t('passwordPolicy.forbidden') : t('common.tryAgain'),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('passwordPolicy.title')}</CardTitle>
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('passwordPolicy.description')}</p>
        {!live ? <p class="text-warning-text text-xs">{t('passwordPolicy.offline')}</p> : null}
        <div class="flex max-w-xs flex-col gap-1.5">
          <label class="text-text text-sm font-medium" for="pw-min-length">
            {t('passwordPolicy.minLength')}
          </label>
          <Input
            id="pw-min-length"
            type="number"
            min={MIN}
            max={MAX}
            value={String(minLength)}
            invalid={invalid}
            disabled={!live}
            class="w-28 tabular-nums"
            onInput={(e) => setMinLength(Math.trunc(Number(e.currentTarget.value)))}
          />
          {invalid ? (
            <p class="text-danger-text text-xs">
              {t('passwordPolicy.range', { min: MIN, count: MAX })}
            </p>
          ) : null}
        </div>
        <div class="flex flex-col gap-1">
          <Switch
            label={t('passwordPolicy.complexity')}
            checked={complexity}
            disabled={!live}
            onChange={(e) => setComplexity(e.currentTarget.checked)}
          />
          <p class="text-text-muted text-xs">{t('passwordPolicy.complexityHint')}</p>
        </div>
        <div class="flex">
          <Button
            variant="primary"
            size="sm"
            loading={saving}
            disabled={!live || invalid || !dirty}
            onClick={() => void save()}
          >
            {t('passwordPolicy.save')}
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

export default function PasswordPolicyCard({
  locale,
  ...props
}: {
  policy: PasswordPolicy;
  live: boolean;
  locale: Locale;
}) {
  return (
    <ToastProvider>
      <Inner {...props} t={useT(locale)} />
    </ToastProvider>
  );
}
