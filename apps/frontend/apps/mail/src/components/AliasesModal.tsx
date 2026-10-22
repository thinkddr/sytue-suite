/**
 * AliasesModal — alias de correo: direcciones adicionales que entregan a la MISMA
 * cuenta. Explica en llano qué es un alias, valida el formato en vivo (y avisa de
 * duplicados antes de llamar al servicio) y lista las direcciones con la principal
 * arriba. El servicio valida de nuevo contra los dominios que servimos; los alias
 * alimentan además el selector «De» del redactor (enviar-como). Aquí solo el CRUD
 * vía BFF. docs/CONVENTIONS.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  Icon,
  SettingsSection,
  Spinner,
  useToast,
} from '@sytue/ui';
import {
  listAliases,
  createAlias,
  deleteAlias,
  MailHttpError,
  type Alias,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  /** Se creó/borró un alias: el padre refresca el selector «De» del redactor. */
  onChanged?: () => void;
  onClose: () => void;
}

// Validación básica de dirección (el servidor es la autoridad; esto solo evita
// llamadas obvias inválidas y explica el fallo antes de intentarlo).
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function AliasesModal({ locale, open, onChanged, onClose }: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [primary, setPrimary] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError(false);
    listAliases()
      .then(({ aliases: list, primary: prim }) => {
        if (!active) return;
        setAliases(list);
        setPrimary(prim);
      })
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  async function handleAdd(): Promise<void> {
    const address = draft.trim();
    if (!canAdd) return;
    setSaving(true);
    try {
      await createAlias(address);
      setDraft('');
      setReload((n) => n + 1);
      onChanged?.();
      toast.notify({
        tone: 'success',
        title: t('aliases.added'),
        description: t('aliases.addedDescription', { address }),
      });
    } catch (err) {
      const code = err instanceof MailHttpError ? err.code : undefined;
      toast.notify({
        tone: 'danger',
        title: t('aliases.createFailed'),
        description: t(
          code === 'alias_reserved'
            ? 'aliases.reservedDescription'
            : code === 'alias_limit'
              ? 'aliases.limitDescription'
              : 'aliases.createFailedDescription',
        ),
      });
    } finally {
      setSaving(false);
    }
  }

  function handleDelete(a: Alias): void {
    const prev = aliases;
    setAliases((list) => list.filter((x) => x.id !== a.id));
    void deleteAlias(a.id)
      .then(() => onChanged?.())
      .catch(() => {
        setAliases(prev);
        toast.notify({ tone: 'danger', title: t('aliases.deleteFailed') });
      });
  }

  // Validación en vivo: el fallo típico es repetir una dirección que ya existe o
  // escribirla a medias. Se dice qué falla, no «dirección inválida».
  const trimmed = draft.trim();
  const looksValid = EMAIL_RE.test(trimmed);
  const duplicate =
    trimmed !== '' &&
    (trimmed.toLowerCase() === primary.toLowerCase() ||
      aliases.some((a) => a.address.toLowerCase() === trimmed.toLowerCase()));
  const canAdd = looksValid && !duplicate && !saving;
  const draftHint: { tone: 'neutral' | 'danger' | 'success'; text: string } = duplicate
    ? { tone: 'danger', text: t('aliases.hintDuplicate') }
    : trimmed === ''
      ? { tone: 'neutral', text: t('aliases.hintEmpty') }
      : !trimmed.includes('@')
        ? { tone: 'danger', text: t('aliases.hintNoDomain') }
        : looksValid
          ? { tone: 'success', text: t('aliases.hintValid') }
          : { tone: 'danger', text: t('aliases.hintInvalid') };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('aliases.title')}
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-6">
        <InlineHelp tone="info" title={t('aliases.whatTitle')}>
          {t('aliases.whatBody')}
        </InlineHelp>

        <SettingsSection
          plain
          title={t('aliases.yoursTitle')}
          description={t('aliases.yoursDescription')}
        >
          {loading ? (
            <div class="text-text-subtle flex items-center gap-2 py-4">
              <Spinner size={16} label={t('aliases.loading')} />
              <span class="text-sm" aria-hidden="true">
                {t('common.loading')}
              </span>
            </div>
          ) : error ? (
            <div class="flex flex-col items-start gap-2 py-2" role="alert">
              <p class="text-text flex items-start gap-2 text-sm">
                <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
                {t('aliases.loadError')}
              </p>
              <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
                {t('common.retry')}
              </Button>
            </div>
          ) : (
            <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
              {/* La dirección primaria, no borrable, para dar contexto. */}
              {primary ? (
                <li class="flex items-center justify-between gap-3 px-3 py-2.5">
                  <span class="text-text truncate text-sm font-medium">{primary}</span>
                  <Badge tone="neutral" class="shrink-0">
                    {t('aliases.primary')}
                  </Badge>
                </li>
              ) : null}
              {aliases.map((a) => (
                <li key={a.id} class="flex items-center justify-between gap-3 px-3 py-2.5">
                  <span class="text-text truncate text-sm">{a.address}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    class="text-text-subtle hover:text-danger-text shrink-0"
                    aria-label={t('aliases.deleteAlias', { address: a.address })}
                    title={t('aliases.deleteShort')}
                    onClick={() => handleDelete(a)}
                  >
                    <Icon name="close" size={16} />
                  </Button>
                </li>
              ))}
              {aliases.length === 0 ? (
                <li>
                  <EmptyState
                    title={t('aliases.emptyTitle')}
                    description={t('aliases.emptyDescription')}
                    illustration={<Icon name="user" size={32} />}
                    action={
                      <Button
                        variant="secondary"
                        onClick={() => document.getElementById('alias-address')?.focus()}
                      >
                        {t('aliases.emptyAction')}
                      </Button>
                    }
                  />
                </li>
              ) : null}
            </ul>
          )}
        </SettingsSection>

        <SettingsSection
          plain
          title={t('aliases.addTitle')}
          description={t('aliases.addDescription')}
          class="border-border border-t pt-5"
        >
          <FieldGroup label={t('common.address')} htmlFor="alias-address">
            <div class="flex items-start gap-2">
              <div class="min-w-0 flex-1">
                <Input
                  id="alias-address"
                  type="email"
                  value={draft}
                  placeholder={t('aliases.placeholder')}
                  invalid={draftHint.tone === 'danger'}
                  aria-describedby="alias-address-hint"
                  onInput={(e) => setDraft(e.currentTarget.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && canAdd) void handleAdd();
                  }}
                />
              </div>
              <Button
                variant="primary"
                class="shrink-0"
                leadingIcon={<Icon name="plus" />}
                loading={saving}
                disabled={!canAdd}
                onClick={() => void handleAdd()}
              >
                {t('common.add')}
              </Button>
            </div>
            <FieldHint id="alias-address-hint" tone={draftHint.tone}>
              {draftHint.text}
            </FieldHint>
          </FieldGroup>
        </SettingsSection>
      </div>
    </Modal>
  );
}
