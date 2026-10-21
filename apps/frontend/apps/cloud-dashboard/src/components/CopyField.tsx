/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CopyField — un valor con acción a la derecha, reutilizable entre paneles
 * (Sytue DB Studio, Almacenamiento). Para valores públicos (URL REST, endpoint
 * S3, access key) la acción es «Copiar» (copia real + confirmación solo si tuvo
 * éxito). Para secretos el valor mostrado es un PREVIEW enmascarado/truncado por
 * diseño, así que NO se ofrece copiar (copiaría una clave rota): en su lugar se
 * enlaza a «Variables y secretos», donde viven los valores reales. Requiere un
 * ToastProvider ancestro (usa useToast). docs/CONVENTIONS.md §10, §14.
 */
import { Badge, Button, useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';

export function CopyField({
  t,
  label,
  value,
  secret,
  manageHref,
}: {
  /** `t` ya atada al locale de la página (llega por props desde la isla padre). */
  t: T;
  label: string;
  value: string;
  secret?: boolean;
  manageHref?: string;
}) {
  const toast = useToast();
  return (
    <div class="flex flex-col gap-1">
      <span class="text-text text-sm font-semibold">{label}</span>
      <div class="flex items-center gap-2">
        <code
          class="border-border bg-surface-sunken text-text-muted min-w-0 flex-1 truncate rounded-md border px-3 py-2 font-mono text-sm"
          title={secret ? undefined : value}
        >
          {value}
        </code>
        {secret ? <Badge tone="warning">{t('copy.secret')}</Badge> : null}
        {secret ? (
          <a
            href={manageHref}
            class="text-primary-text sytue-focus-ring-inset shrink-0 rounded-sm text-sm font-semibold hover:underline"
            aria-label={t('copy.manageLabel', { label })}
          >
            {t('copy.manage')}
          </a>
        ) : (
          <Button
            variant="secondary"
            size="sm"
            class="shrink-0"
            aria-label={t('copy.copyLabel', { label })}
            onClick={() => {
              // El toast solo tras copiar de verdad: clipboard puede no existir
              // (contexto no seguro) o rechazar (permisos).
              void (async () => {
                try {
                  if (!navigator.clipboard) throw new Error('clipboard no disponible');
                  await navigator.clipboard.writeText(value);
                  toast.notify({
                    tone: 'info',
                    title: t('copy.copied'),
                    description: t('copy.copied.desc', { label }),
                  });
                } catch {
                  toast.notify({
                    tone: 'danger',
                    title: t('copy.fail'),
                    description: t('copy.fail.desc'),
                  });
                }
              })();
            }}
          >
            {t('copy.copy')}
          </Button>
        )}
      </div>
    </div>
  );
}
