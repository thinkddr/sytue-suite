/**
 * FoldersModal — gestión de carpetas propias de Sytue Mail. Crear ya se podía;
 * lo que no había por ninguna parte era RENOMBRAR ni BORRAR, aunque el servicio
 * lo acepta desde el principio (Mailbox/set update/destroy). El resultado era
 * que una carpeta creada con una errata se quedaba así para siempre y el lateral
 * solo crecía.
 *
 * Solo se listan las carpetas del usuario: Recibidos, Enviados, Papelera y
 * compañía llevan un `role` de sistema y el servidor las protege. Aparecerían
 * aquí únicamente para decir que no, así que no aparecen.
 *
 * Las carpetas parten de `mailboxes`, que la isla raíz ya tiene cargadas: pedir
 * otra vez lo mismo al abrir el diálogo sería un viaje de más y una lista que
 * podría discrepar de la del lateral.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  Icon,
  SettingsSection,
  useToast,
} from '@sytue/ui';
import {
  createMailbox,
  renameMailbox,
  deleteMailbox,
  type FolderResult,
  type Mailbox,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT, type MailMessageKey, type T } from '../i18n/index.js';
import { Glyph } from './glyphs.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  /** Carpetas de la cuenta tal como las tiene el lateral (sistema incluidas). */
  mailboxes: Mailbox[];
  onClose: () => void;
  /** Aviso al padre de que las carpetas cambiaron (para recargar el lateral). */
  onChanged: () => void;
}

/** Motivo del rechazo → texto que dice qué hacer con él. */
const FAIL_KEYS: Record<Exclude<FolderResult, 'ok'>, MailMessageKey> = {
  exists: 'folderNew.exists',
  invalid: 'folderNew.invalid',
  system: 'folders.systemFolder',
  notEmpty: 'folders.notEmpty',
};

export function FoldersModal({
  locale,
  open,
  mailboxes,
  onClose,
  onChanged,
}: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [newName, setNewName] = useState('');
  const [saving, setSaving] = useState(false);

  // Las de sistema no se tocan: el servidor las rechaza y listarlas solo serviría
  // para ofrecer un botón que siempre falla.
  const propias = mailboxes.filter((m) => !m.role);

  useEffect(() => {
    if (open) setNewName('');
  }, [open]);

  /** Avisa del rechazo con su motivo; `ok` no dice nada (el cambio ya se ve). */
  function reportar(res: FolderResult, okTitle: string): boolean {
    if (res === 'ok') {
      toast.notify({ tone: 'success', title: okTitle });
      onChanged();
      return true;
    }
    toast.notify({ tone: 'danger', title: t(FAIL_KEYS[res]) });
    return false;
  }

  /** Avería (red, servicio caído): no hay motivo que explicar, solo el fallo. */
  function fallo(title: string): void {
    toast.notify({ tone: 'danger', title });
  }

  const trimmed = newName.trim();
  const duplicado = mailboxes.some((m) => m.name.toLowerCase() === trimmed.toLowerCase());
  const puedeCrear = trimmed !== '' && !duplicado && !saving;

  async function crear(): Promise<void> {
    if (!puedeCrear) return;
    setSaving(true);
    try {
      const res = await createMailbox(trimmed);
      if (reportar(res, t('folderNew.created', { name: trimmed }))) setNewName('');
    } catch {
      toast.notify({ tone: 'danger', title: t('folderNew.failed') });
    } finally {
      setSaving(false);
    }
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('folders.title')}
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-6">
        <InlineHelp tone="info" title={t('folders.whatTitle')}>
          {t('folders.whatBody')}
        </InlineHelp>

        <SettingsSection
          plain
          title={t('folders.yoursTitle')}
          description={t('folders.yoursDescription')}
        >
          {propias.length === 0 ? (
            <EmptyState
              title={t('folders.emptyTitle')}
              description={t('folders.emptyDescription')}
              illustration={<Glyph name="folder" size={32} />}
              action={
                <Button
                  variant="secondary"
                  onClick={() => document.getElementById('fld-name')?.focus()}
                >
                  {t('folders.emptyAction')}
                </Button>
              }
            />
          ) : (
            <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
              {propias.map((m) => (
                <FolderRow key={m.id} t={t} folder={m} onReport={reportar} onFail={fallo} />
              ))}
            </ul>
          )}
        </SettingsSection>

        <SettingsSection
          plain
          title={t('folders.newTitle')}
          description={t('folders.newDescription')}
          class="border-border border-t pt-5"
        >
          <FieldGroup label={t('common.name')} htmlFor="fld-name">
            <Input
              id="fld-name"
              type="text"
              value={newName}
              maxLength={255}
              placeholder={t('folderNew.placeholder')}
              invalid={duplicado}
              aria-describedby="fld-name-hint"
              onInput={(e) => setNewName(e.currentTarget.value)}
              onKeyDown={(e: KeyboardEvent) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void crear();
                }
              }}
            />
            <FieldHint id="fld-name-hint" tone={duplicado ? 'danger' : 'neutral'}>
              {duplicado ? t('folderNew.exists') : t('folderNew.help')}
            </FieldHint>
          </FieldGroup>
          <div class="flex justify-end">
            <Button
              variant="primary"
              leadingIcon={<Icon name="plus" />}
              loading={saving}
              disabled={!puedeCrear}
              onClick={() => void crear()}
            >
              {t('folderNew.create')}
            </Button>
          </div>
        </SettingsSection>
      </div>
    </Modal>
  );
}

/**
 * Fila de una carpeta: nombre editable (guarda al salir del campo) y borrado con
 * confirmación en línea, porque una carpeta se borra una vez y para siempre.
 */
function FolderRow({
  t,
  folder,
  onReport,
  onFail,
}: {
  t: T;
  folder: Mailbox;
  onReport: (res: FolderResult, okTitle: string) => boolean;
  onFail: (title: string) => void;
}): JSX.Element {
  const [name, setName] = useState(folder.name);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const confirmId = `fld-del-${folder.id}`;

  // El nombre viene del padre: si el renombrado falla y el lateral se recarga,
  // el campo tiene que volver a lo que hay de verdad en el servidor.
  useEffect(() => setName(folder.name), [folder.name]);
  useEffect(() => {
    if (confirming) document.getElementById(confirmId)?.focus();
  }, [confirming, confirmId]);

  async function guardarNombre(): Promise<void> {
    const nuevo = name.trim();
    if (busy || !nuevo || nuevo === folder.name) {
      setName(folder.name);
      return;
    }
    setBusy(true);
    try {
      if (!onReport(await renameMailbox(folder.id, nuevo), t('folders.renamed'))) {
        setName(folder.name);
      }
    } catch {
      setName(folder.name);
      onFail(t('folders.renameFailed'));
    } finally {
      setBusy(false);
    }
  }

  async function borrar(): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      onReport(await deleteMailbox(folder.id), t('folders.deleted'));
    } catch {
      onFail(t('folders.deleteFailed'));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  }

  return (
    // En el teléfono, la confirmación baja a su línea: al lado, el campo se quedaba en
    // 26 px y no se veía qué carpeta se borraba.
    <li class="flex items-center gap-3 px-3 py-2.5 max-md:flex-wrap">
      <Glyph name="folder" size={16} class="text-text-subtle shrink-0" />
      <Input
        class="min-w-0 flex-1"
        type="text"
        value={name}
        maxLength={255}
        disabled={busy}
        aria-label={t('folders.rowName', { folder: folder.name })}
        onInput={(e) => setName(e.currentTarget.value)}
        onBlur={() => void guardarNombre()}
        onKeyDown={(e: KeyboardEvent) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            (e.currentTarget as HTMLInputElement).blur();
          }
        }}
      />
      {/* El recuento explica por qué el borrado puede negarse: una carpeta con
          correo dentro no se borra hasta vaciarla. */}
      <span class="text-text-subtle shrink-0 text-xs tabular-nums">
        {t('folders.messageCount', { count: folder.totalEmails })}
      </span>
      {confirming ? (
        <div class="flex shrink-0 items-center gap-1.5 max-md:basis-full max-md:justify-end">
          <span class="text-text-muted text-xs">{t('folders.confirmDelete')}</span>
          <Button
            id={confirmId}
            variant="danger"
            size="sm"
            loading={busy}
            onClick={() => void borrar()}
          >
            {t('common.yes')}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={busy}
            onClick={() => setConfirming(false)}
          >
            {t('common.no')}
          </Button>
        </div>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          iconOnly
          class="text-text-subtle hover:text-danger-text shrink-0"
          aria-label={t('folders.deleteFolder', { folder: folder.name })}
          title={t('folders.deleteShort')}
          disabled={busy}
          onClick={() => setConfirming(true)}
        >
          <Icon name="close" size={16} />
        </Button>
      )}
    </li>
  );
}
