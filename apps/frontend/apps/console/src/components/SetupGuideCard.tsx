/**
 * SetupGuideCard — los primeros pasos de una organización en la portada de Sytue
 * Admin: dominio, correo, invitar, verificación en dos pasos y, como opcionales,
 * SSO y apps conectadas. Cada paso lleva a su sección. Se esconde sola en cuanto
 * los obligatorios están hechos: una guía que se queda para siempre enseña a no
 * mirarla. El estado lo decide `lib/primeros-pasos.ts`. Solo @sytue/ui.
 */
import { Badge, Card, CardBody, CardHeader, CardTitle, Icon } from '@sytue/ui';
import type { BadgeTone, IconName } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import {
  setupComplete,
  type SetupStep,
  type SetupStepId,
  type SetupStepStatus,
} from '../lib/primeros-pasos.js';

const PASO: Record<SetupStepId, { icon: IconName; href: () => string }> = {
  // El dominio y el correo se configuran en la tarjeta de dominio de esta misma
  // portada: el ancla evita mandar a otra página a buscar lo que está debajo.
  domain: { icon: 'globe', href: () => '#dominio' },
  mail: { icon: 'info', href: () => '#dominio' },
  invite: { icon: 'userCheck', href: () => r('/users') },
  mfa: { icon: 'shield', href: () => r('/users') },
  sso: { icon: 'key', href: () => r('/sso') },
  apps: { icon: 'link', href: () => r('/apps') },
};

// Claves literales (no plantillas): así el test de claves usadas las encuentra.
const TEXTO: Record<SetupStepId, { step: ConsoleMessageKey; hint: ConsoleMessageKey }> = {
  domain: { step: 'dashboard.setup.step.domain', hint: 'dashboard.setup.hint.domain' },
  mail: { step: 'dashboard.setup.step.mail', hint: 'dashboard.setup.hint.mail' },
  invite: { step: 'dashboard.setup.step.invite', hint: 'dashboard.setup.hint.invite' },
  mfa: { step: 'dashboard.setup.step.mfa', hint: 'dashboard.setup.hint.mfa' },
  sso: { step: 'dashboard.setup.step.sso', hint: 'dashboard.setup.hint.sso' },
  apps: { step: 'dashboard.setup.step.apps', hint: 'dashboard.setup.hint.apps' },
};

const ESTADO: Record<SetupStepStatus, ConsoleMessageKey> = {
  done: 'dashboard.setup.status.done',
  pending: 'dashboard.setup.status.pending',
  optional: 'dashboard.setup.status.optional',
  unknown: 'dashboard.setup.status.unknown',
};

const TONO: Record<SetupStepStatus, BadgeTone> = {
  done: 'success',
  pending: 'warning',
  optional: 'neutral',
  unknown: 'neutral',
};

export function SetupGuideCard({ steps, locale }: { steps: SetupStep[]; locale: Locale }) {
  const t = useT(locale);
  if (setupComplete(steps)) return null;
  const obligatorios = steps.filter((p) => p.required);
  const hechos = obligatorios.filter((p) => p.status === 'done').length;
  return (
    <Card class="mb-4" data-testid="setup-guide">
      <CardHeader>
        <CardTitle>{t('dashboard.setup.title')}</CardTitle>
        <span class="text-text-muted text-sm tabular-nums">
          {t('dashboard.setup.progress', { done: hechos, total: obligatorios.length })}
        </span>
      </CardHeader>
      <CardBody class="p-0">
        <ol>
          {steps.map((p) => (
            <li
              key={p.id}
              data-step={p.id}
              data-status={p.status}
              class="border-border flex min-h-11 items-center gap-3 border-b px-4 py-2.5 last:border-0"
            >
              <Icon
                name={PASO[p.id].icon}
                size={16}
                class={
                  p.status === 'done' ? 'text-success-text shrink-0' : 'text-text-subtle shrink-0'
                }
                aria-hidden="true"
              />
              <div class="min-w-0 flex-1">
                <p class="text-text text-sm font-medium">{t(TEXTO[p.id].step)}</p>
                <p class="sytue-meta">{t(TEXTO[p.id].hint)}</p>
              </div>
              <Badge tone={TONO[p.status]}>{t(ESTADO[p.status])}</Badge>
              {p.status !== 'done' && (
                <a
                  href={PASO[p.id].href()}
                  class="text-primary-text shrink-0 text-sm font-semibold hover:underline"
                  aria-label={`${t('dashboard.setup.open')}: ${t(TEXTO[p.id].step)}`}
                >
                  {t('dashboard.setup.open')}
                </a>
              )}
            </li>
          ))}
        </ol>
      </CardBody>
    </Card>
  );
}
