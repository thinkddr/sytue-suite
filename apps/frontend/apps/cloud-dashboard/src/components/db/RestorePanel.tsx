/**
 * RestorePanel — restaura el Postgres gestionado del proyecto desde un backup.
 * Acción DESTRUCTIVA (sobrescribe la base de datos), así que nunca es de un solo
 * clic: (1) elegir UN backup de la lista, (2) confirmación explícita escribiendo
 * el nombre del servicio con banner de peligro. Solo se ofrece para kind=postgres
 * (esta vista es el Postgres del proyecto).
 *
 * Y al lado, la copia A DEMANDA: la automática es diaria, y antes de una
 * migración o un borrado masivo nadie quiere conformarse con la de ayer. Sin ella
 * «restaurar» servía a medias, porque el punto al que volver no se podía crear.
 * Solo @sytue/ui. §14.
 */
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  Modal,
  Icon,
  Badge,
  EmptyState,
  Input,
  useToast,
  formatBytes,
  Radio,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../../lib/bff-client.js';
import { useT } from '../../i18n/index.js';
import { formatDate } from '../../lib/format.js';

interface Backup {
  key: string;
  size: number;
  lastModified: string;
}

export default function RestorePanel({
  serviceId,
  serviceName,
  locale,
}: {
  serviceId: string;
  serviceName: string;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}) {
  const t = useT(locale);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [backups, setBackups] = useState<Backup[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [phase, setPhase] = useState<'select' | 'confirm'>('select');
  const [confirmText, setConfirmText] = useState('');
  const [restoring, setRestoring] = useState(false);
  const [backingUp, setBackingUp] = useState(false);
  // Cuál se está descargando: el spinner tiene que salir en SU fila, no en todas.
  const [downloading, setDownloading] = useState<string | null>(null);

  // Reubica el foco al Input de confirmación al entrar en la fase 'confirm' (el Modal
  // solo enfoca al abrir): así el usuario de teclado aterriza en el control que debe
  // usar, sin caer al <body> tras desmontarse el botón 'Continuar'. Por id para no
  // depender del ref-forwarding del primitivo Input.
  useEffect(() => {
    if (open && phase === 'confirm') {
      document.getElementById('restore-confirm')?.focus();
    }
  }, [open, phase]);

  function openModal() {
    setOpen(true);
    setPhase('select');
    setSelected(null);
    setConfirmText('');
    void loadBackups();
  }

  async function loadBackups() {
    // Sin service id (modo demostración sin backend): estado vacío honesto, no error.
    if (!serviceId) {
      setBackups([]);
      return;
    }
    setLoading(true);
    try {
      const res = await fetch(`/api/services/backups?service=${encodeURIComponent(serviceId)}`);
      if (!res.ok) throw new Error(`bff_${res.status}`);
      const data = (await res.json()) as { backups?: Backup[] };
      setBackups(data.backups ?? []);
    } catch {
      setBackups([]);
      toast.notify({ tone: 'danger', title: t('db.restore.loadFail') });
    } finally {
      setLoading(false);
    }
  }

  async function confirmRestore() {
    if (!selected) return;
    setRestoring(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/backups', {
        service: serviceId,
        backup: selected,
      });
      setOpen(false);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('db.restore.started'),
        ...demoDesc(t, res.live, t('db.restore.started.desc', { backup: selected })),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('db.restore.fail') });
    } finally {
      setRestoring(false);
    }
  }

  /**
   * Descarga una copia. Se podía restaurar, pero no LLEVÁRSELA: un dump que solo
   * existe dentro de la plataforma que lo hizo no sirve para guardarlo en otro
   * sitio ni para marcharse con los datos.
   *
   * `window.open` con la URL firmada y no un `<a download>`: el fichero vive en
   * el almacenamiento de objetos, en otro origen, y el atributo `download` se
   * ignora entre orígenes. La cabecera `Content-Disposition` que firma el
   * servidor es la que hace que el navegador lo guarde.
   */
  async function download(key: string) {
    setDownloading(key);
    try {
      const res = await fetch(
        `/api/services/backups?service=${encodeURIComponent(serviceId)}&download=${encodeURIComponent(key)}`,
      );
      if (!res.ok) throw new Error(`bff_${res.status}`);
      const data = (await res.json()) as { url?: string };
      if (!data.url) throw new Error('sin_url');
      window.open(data.url, '_blank', 'noopener');
    } catch {
      toast.notify({ tone: 'danger', title: t('db.backup.download.fail') });
    } finally {
      setDownloading(null);
    }
  }

  const canConfirm = confirmText.trim() === serviceName;

  const footer =
    phase === 'select' ? (
      <>
        <Button variant="ghost" onClick={() => setOpen(false)}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" disabled={!selected} onClick={() => setPhase('confirm')}>
          {t('common.continue')}
        </Button>
      </>
    ) : (
      <>
        <Button variant="ghost" disabled={restoring} onClick={() => setPhase('select')}>
          {t('common.back')}
        </Button>
        <Button
          variant="danger"
          disabled={!canConfirm}
          loading={restoring}
          onClick={() => void confirmRestore()}
        >
          {t('db.restore.confirmCta')}
        </Button>
      </>
    );

  /**
   * Pide una copia ahora. No espera a que termine —el Job puede tardar minutos con
   * una base grande—: se avisa de que está en marcha y aparecerá en la lista.
   */
  async function backupNow() {
    setBackingUp(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/services/backups', {
        service: serviceId,
        backup_now: true,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('db.backup.started'),
        ...demoDesc(t, res.live, t('db.backup.started.desc')),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('db.backup.fail') });
    } finally {
      setBackingUp(false);
    }
  }

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        leadingIcon={<Icon name="database" size={16} />}
        loading={backingUp}
        onClick={() => void backupNow()}
      >
        {t('db.backup.action')}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        leadingIcon={<Icon name="database" size={16} />}
        onClick={openModal}
      >
        {t('db.restore.action')}
      </Button>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={phase === 'select' ? t('db.restore.select.title') : t('db.restore.confirm.title')}
        size="md"
        footer={footer}
      >
        {phase === 'select' ? (
          loading ? (
            <p class="text-text-muted py-8 text-center text-base">{t('db.restore.loading')}</p>
          ) : backups.length === 0 ? (
            <EmptyState
              illustration={<Icon name="database" size={24} />}
              title={t('db.restore.empty.title')}
              description={t('db.restore.empty.desc')}
            />
          ) : (
            <fieldset>
              <legend class="text-text-muted mb-3 text-sm">{t('db.restore.legend')}</legend>
              <ul class="border-border divide-border divide-y rounded-md border">
                {backups.map((b) => (
                  // El botón de descarga va FUERA del <label>: dentro, pulsarlo
                  // marcaría además el radio, que es justo lo que no se pidió.
                  <li key={b.key} class="hover:bg-bg-subtle flex items-center gap-2 pr-2">
                    <label class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 px-3 py-2.5">
                      <Radio
                        name="backup"
                        value={b.key}
                        checked={selected === b.key}
                        onChange={() => setSelected(b.key)}
                        class="accent-primary h-4 w-4 shrink-0"
                      />
                      <span class="min-w-0 flex-1">
                        <span class="text-text block font-medium">
                          {formatDate(locale, b.lastModified)}
                        </span>
                        <code
                          class="text-text-muted block truncate font-mono text-xs"
                          title={b.key}
                        >
                          {b.key}
                        </code>
                      </span>
                      <Badge tone="neutral">{formatBytes(locale, b.size)}</Badge>
                    </label>
                    <Button
                      variant="ghost"
                      size="sm"
                      class="shrink-0"
                      loading={downloading === b.key}
                      disabled={downloading !== null}
                      aria-label={t('db.backup.download.aria', { backup: b.key })}
                      onClick={() => void download(b.key)}
                    >
                      {t('db.backup.download')}
                    </Button>
                  </li>
                ))}
              </ul>
            </fieldset>
          )
        ) : (
          <div class="space-y-4">
            <div
              class="border-danger bg-danger-bg text-danger-text flex gap-3 rounded-md border p-3"
              role="alert"
            >
              <Icon name="warning" size={18} class="mt-0.5 shrink-0" />
              {/* El aviso entero es UNA clave: partirlo para poner la mitad en
                  negrita obliga a concatenar, y el orden cambia entre idiomas. */}
              <p class="text-sm font-medium">{t('db.restore.warning')}</p>
            </div>
            <p class="text-text-muted text-sm break-words">
              {t('db.restore.from', { backup: selected ?? '' })}
            </p>
            <div>
              <label for="restore-confirm" class="text-text mb-1 block text-sm font-semibold">
                {t('db.restore.confirmLabel', { name: serviceName })}
              </label>
              <Input
                id="restore-confirm"
                value={confirmText}
                autocomplete="off"
                class="font-mono"
                placeholder={serviceName}
                onInput={(e) => setConfirmText(e.currentTarget.value)}
              />
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}
