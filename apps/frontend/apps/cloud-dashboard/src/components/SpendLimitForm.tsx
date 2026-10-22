/**
 * SpendLimitForm — gasto del período vs límite mensual del tenant (isla).
 * Barra de proporción honesta (nunca >100 %) + input numérico nativo para fijar
 * el tope, vía POST /api/billing/spend-limit (proxy al control-plane). Solo
 * @sytue/ui; foco doble-anillo AA heredado de los primitivos. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  Button,
  FieldGroup,
  Input,
  describedBy,
  useToast,
  ToastProvider,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import { useT } from '../i18n/index.js';
import { formatEur } from '../lib/format.js';

interface Props {
  currentSpendEur: number;
  spendLimitEur: number | null;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function SpendLimitForm(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ currentSpendEur, spendLimitEur, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [limit, setLimit] = useState<number | null>(spendLimitEur);
  const [draft, setDraft] = useState(spendLimitEur != null ? String(spendLimitEur) : '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Proporción honesta: cap a 1 para que la barra nunca mienta (>100 %).
  const pct = limit && limit > 0 ? Math.min(1, currentSpendEur / limit) : 0;
  const over = limit != null && limit > 0 && currentSpendEur > limit;

  async function save(e: Event) {
    e.preventDefault();
    const value = Number(draft);
    if (!Number.isFinite(value) || value < 0) {
      setError(t('spend.invalid'));
      toast.notify({ tone: 'danger', title: t('spend.invalid') });
      return;
    }
    setError(null);
    setBusy(true);
    try {
      const { live } = await postBff('/api/billing/spend-limit', { spend_limit_eur: value });
      setLimit(value);
      toast.notify({
        tone: 'success',
        title: t('spend.saved'),
        ...demoDesc(t, live, t('spend.saved.desc', { limit: formatEur(locale, value) })),
      });
      notifyDemoIfNeeded(t, toast, live);
    } catch {
      toast.notify({ tone: 'danger', title: t('spend.save.fail') });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle as="h2">{t('spend.title')}</CardTitle>
        {over ? (
          <Badge tone="danger">{t('spend.overLimit')}</Badge>
        ) : (
          <Badge tone="neutral">{limit != null ? t('spend.withLimit') : t('spend.noLimit')}</Badge>
        )}
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <div>
          <p class="text-text sytue-h3 tabular-nums">{formatEur(locale, currentSpendEur)}</p>
          <p class="text-text-muted mt-0.5 text-sm">
            {limit != null && limit > 0
              ? t('spend.ofLimit', { limit: formatEur(locale, limit) })
              : t('spend.noLimitSet')}
          </p>
        </div>

        {limit != null && limit > 0 ? (
          <span
            class="bg-bg-subtle block h-2 w-full overflow-hidden rounded-full"
            role="img"
            aria-label={t('spend.barLabel', { percent: Math.round(pct * 100) })}
          >
            <span
              class={'block h-full rounded-full ' + (over ? 'bg-danger' : 'bg-primary-600')}
              style={`width:${Math.max(2, Math.round(pct * 100))}%`}
            />
          </span>
        ) : null}

        <form class="flex items-end gap-2" onSubmit={(e) => void save(e)}>
          <FieldGroup
            label={t('spend.field')}
            htmlFor="spend-limit"
            help={t('spend.field.help')}
            {...(error ? { error } : {})}
            class="flex-1"
          >
            <Input
              id="spend-limit"
              type="number"
              min="0"
              step="1"
              inputMode="decimal"
              value={draft}
              invalid={error != null}
              aria-describedby={describedBy('spend-limit', 'help', error ?? undefined)}
              onInput={(e) => {
                setDraft((e.target as HTMLInputElement).value);
                if (error) setError(null);
              }}
            />
          </FieldGroup>
          <Button type="submit" variant="primary" loading={busy} disabled={busy}>
            {t('common.save')}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
