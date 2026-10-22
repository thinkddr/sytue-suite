/**
 * SensitivityLabelsPanel — etiquetas de sensibilidad (paridad Purview / Google Drive
 * labels) como página de AJUSTES.
 *
 * La lista es la escala de la organización, de menos a más sensible, con lo que impide
 * cada etiqueta dicho en llano. Crear o cambiar una abre el asistente corto
 * (SensitivityLabelWizard) en vez de un formulario.
 *
 * Alcance real de esta pieza, sin adornos: definir la taxonomía y que Sytue Drive
 * bloquee los enlaces públicos de lo etiquetado. NO hay cifrado por etiqueta ni
 * clasificación automática: etiquetar es manual.
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Icon,
  InlineHelp,
  SettingsSection,
  SettingsShell,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import SensitivityLabelWizard from './SensitivityLabelWizard.js';
import { useT } from '../i18n/index.js';
import type { SensitivityLabel } from '../lib/types.js';

interface SensitivityLabelsPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  labels: SensitivityLabel[];
  /** false = Sytue ID no accesible; el panel avisa y no edita. */
  live: boolean;
}

function Inner({ locale, labels, live }: SensitivityLabelsPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<SensitivityLabel[]>(
    [...labels].sort((a, b) => a.rank - b.rank || a.name.localeCompare(b.name)),
  );
  const [wizard, setWizard] = useState<{ open: boolean; label?: SensitivityLabel }>({
    open: false,
  });
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmDel, setConfirmDel] = useState<SensitivityLabel | null>(null);

  function upsert(l: SensitivityLabel) {
    setItems((prev) => {
      const i = prev.findIndex((x) => x.id === l.id);
      const next = i < 0 ? [...prev, l] : prev.map((x) => (x.id === l.id ? l : x));
      return next.sort((a, b) => a.rank - b.rank || a.name.localeCompare(b.name));
    });
  }

  async function remove(l: SensitivityLabel) {
    setBusyId(l.id);
    try {
      const res = await fetch(`/api/admin/sensitivity-labels/${encodeURIComponent(l.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((x) => x.id !== l.id));
      setConfirmDel(null);
      toast.notify({ tone: 'success', title: t('labels.toast.deleted'), description: l.name });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('labels.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  if (wizard.open) {
    return (
      <SettingsShell
        title={
          wizard.label ? t('labels.wizard.editTitle', { name: wizard.label.name }) : t('labels.new')
        }
        description={t('labels.wizard.description')}
      >
        <SensitivityLabelWizard
          t={t}
          editing={wizard.label}
          labels={items}
          onSaved={upsert}
          onCancel={() => setWizard({ open: false })}
          onDone={() => setWizard({ open: false })}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('page.sensitivityLabels')}
      description={t('labels.description')}
      actions={
        <Button
          variant="primary"
          disabled={!live}
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setWizard({ open: true })}
        >
          {t('labels.new')}
        </Button>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineIdTitle')}>
            {t('labels.offlineBody')}
          </InlineHelp>
        )
      }
    >
      <SettingsSection
        plain
        title={t('labels.scale.title')}
        description={t('labels.scale.description')}
      >
        {items.length === 0 ? (
          <Card>
            <EmptyState
              title={t('labels.empty.title')}
              description={t('labels.empty.description')}
              illustration={<Icon name="lock" size={28} />}
              action={
                <Button
                  variant="primary"
                  disabled={!live}
                  onClick={() => setWizard({ open: true })}
                >
                  {t('labels.empty.action')}
                </Button>
              }
            />
          </Card>
        ) : (
          <ul class="border-border bg-surface divide-border-subtle divide-y overflow-hidden rounded-lg border">
            {items.map((l, i) => (
              <li key={l.id} class="hover:bg-bg-subtle flex flex-wrap items-center gap-3 px-3 py-3">
                <span
                  aria-hidden="true"
                  class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold tabular-nums"
                >
                  {i + 1}
                </span>
                <span
                  aria-hidden="true"
                  class="border-border-subtle h-3 w-3 shrink-0 rounded-full border"
                  style={l.color ? { backgroundColor: l.color } : undefined}
                />
                <span class="min-w-0 flex-1 basis-64">
                  <span class="text-text block truncate font-medium">{l.name}</span>
                  <span class="sytue-meta block truncate">
                    {l.description || t('labels.noDescription')}
                  </span>
                </span>
                <span class="shrink-0">
                  {l.blockExternalShare ? (
                    <Badge tone="danger">{t('labels.blocksExternal')}</Badge>
                  ) : (
                    <Badge tone="neutral">{t('labels.informational')}</Badge>
                  )}
                </span>
                <span class="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live}
                    aria-label={t('common.editName', { name: l.name })}
                    onClick={() => setWizard({ open: true, label: l })}
                  >
                    {t('common.edit')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live || busyId === l.id}
                    aria-label={t('common.deleteName', { name: l.name })}
                    onClick={() => setConfirmDel(l)}
                  >
                    {t('common.delete')}
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        )}
        {items.length > 0 ? (
          <p class="sytue-meta leading-relaxed">{t('labels.manualNote')}</p>
        ) : null}
      </SettingsSection>

      <ConfirmDialog
        open={Boolean(confirmDel)}
        onClose={() => setConfirmDel(null)}
        onConfirm={() => {
          if (confirmDel) void remove(confirmDel);
        }}
        title={t('labels.delete.title')}
        confirmLabel={t('common.delete')}
        tone="danger"
        busy={confirmDel ? busyId === confirmDel.id : false}
      >
        {confirmDel ? (
          <>
            <p class="text-text-muted text-sm leading-relaxed">
              {t('labels.delete.intro', { name: confirmDel.name })}
            </p>
            <p class="text-text-muted text-sm leading-relaxed">
              {confirmDel.blockExternalShare
                ? t('labels.delete.keepBlocking')
                : t('labels.delete.keep')}
            </p>
          </>
        ) : null}
      </ConfirmDialog>
    </SettingsShell>
  );
}

export default function SensitivityLabelsPanel(props: SensitivityLabelsPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
