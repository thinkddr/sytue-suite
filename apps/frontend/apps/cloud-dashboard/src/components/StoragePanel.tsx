/**
 * StoragePanel — Sytue Storage del proyecto (bucket real vía API).
 *
 * Listaba objetos y enseñaba las credenciales, y ahí se acababa: para subir un
 * fichero había que sacar un cliente S3 aparte, aunque el control-plane firma URLs
 * de subida y descarga desde la Ola E+F. Ahora se sube, se descarga y se borra
 * desde aquí.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Button,
  Badge,
  Icon,
  Input,
  Modal,
  Table,
  SectionHeader,
  EmptyState,
  ToastProvider,
  useToast,
  formatBytes,
} from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { StorageView } from '../lib/cloud.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import { CopyField } from './CopyField.js';
import DemoBanner from './DemoBanner.js';

/** Mismo tope que el BFF (lib/cloud.ts MAX_UPLOAD_BYTES), para decirlo antes. */
const MAX_UPLOAD_BYTES = 25 * 1024 * 1024;

interface Row {
  key: string;
  size: string;
  modified: string;
}

interface Props {
  project: string;
  storage: StorageView;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function StoragePanel(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

/** Código HTTP embebido por postBff en el mensaje del error (bff_<status>). */
function statusOf(e: unknown): number {
  const m = /bff_(\d+)/.exec(e instanceof Error ? e.message : String(e));
  return m ? Number(m[1]) : 0;
}

function Inner({ project, storage, live, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [objects, setObjects] = useState<Row[]>(storage.objects);
  const [prefix, setPrefix] = useState('');
  const [uploading, setUploading] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<Row | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function upload(file: File) {
    if (file.size > MAX_UPLOAD_BYTES) {
      toast.notify({ tone: 'danger', title: t('storage.upload.tooLarge') });
      return;
    }
    setUploading(true);
    try {
      const form = new FormData();
      form.set('project', project);
      form.set('file', file);
      if (prefix.trim()) form.set('prefix', prefix.trim());
      // FormData va con fetch directo: postBff serializa JSON, y aquí lo que
      // viaja es el fichero.
      const res = await fetch('/api/storage', { method: 'POST', body: form });
      if (!res.ok) throw new Error(`bff_${res.status}`);
      const data = (await res.json()) as { live: boolean; key: string };
      // La fila se pinta con lo que sabemos ahora; el tamaño y la fecha exactos
      // los trae la recarga, y fingirlos aquí sería inventar el estado del bucket.
      setObjects((prev) => [
        { key: data.key, size: formatBytes(locale, file.size), modified: t('storage.justNow') },
        ...prev.filter((o) => o.key !== data.key),
      ]);
      notifyDemoIfNeeded(t, toast, data.live);
      toast.notify({
        tone: 'success',
        title: t('storage.upload.done', { key: data.key }),
        ...demoDesc(t, data.live),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: statusOf(err) === 413 ? t('storage.upload.tooLarge') : t('storage.upload.fail'),
      });
    } finally {
      setUploading(false);
    }
  }

  async function download(row: Row) {
    try {
      const res = await postBff<{ ok: boolean; live: boolean; url?: string }>('/api/storage', {
        project,
        key: row.key,
        download: true,
      });
      if (!res.url) {
        notifyDemoIfNeeded(t, toast, res.live);
        return;
      }
      // Enlace prefirmado (15 min): lo abre el navegador y el fichero va directo
      // del almacenamiento a su disco, sin pasar por el panel.
      window.open(res.url, '_blank', 'noopener');
    } catch {
      toast.notify({ tone: 'danger', title: t('storage.download.fail') });
    }
  }

  async function confirmDelete() {
    const target = pendingDelete;
    if (!target) return;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/storage', {
        project,
        key: target.key,
        delete: true,
      });
      setObjects((prev) => prev.filter((o) => o.key !== target.key));
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('storage.delete.done', { key: target.key }),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: statusOf(err) === 403 ? t('storage.delete.forbidden') : t('storage.delete.fail'),
      });
    } finally {
      setDeleting(false);
    }
  }

  const columns: TableColumn<Row>[] = [
    {
      key: 'key',
      header: t('storage.col.object'),
      cell: (o) => <span class="font-mono text-sm">{o.key}</span>,
    },
    { key: 'size', header: t('storage.col.size'), align: 'end', cell: (o) => o.size },
    { key: 'modified', header: t('storage.col.modified'), cell: (o) => o.modified },
    {
      key: 'actions',
      header: t('storage.col.actions'),
      align: 'end',
      cell: (o) => (
        <span class="flex justify-end gap-1">
          <Button
            variant="ghost"
            class="text-sm"
            onClick={() => void download(o)}
            aria-label={t('storage.download.aria', { key: o.key })}
          >
            {t('storage.download')}
          </Button>
          <Button
            variant="ghost"
            class="text-danger-text text-sm"
            disabled={deleting}
            onClick={() => setPendingDelete(o)}
            aria-label={t('storage.delete.aria', { key: o.key })}
          >
            {t('common.delete')}
          </Button>
        </span>
      ),
    },
  ];

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="bucket" size={20} />}
        title={t('nav.storage')}
        subtitle={t('storage.subtitle', { bucket: storage.bucket, region: storage.region })}
      />
      <div class="cloud-content">
        <Card class="mb-4">
          <CardHeader>
            <CardTitle as="h2">{t('storage.access')}</CardTitle>
            <Badge tone="accent">{t('storage.injected')}</Badge>
          </CardHeader>
          <CardBody class="grid gap-4 sm:grid-cols-2">
            <CopyField t={t} label={t('storage.endpoint')} value={storage.endpoint} />
            <CopyField t={t} label={t('storage.accessKey')} value={storage.accessKey} />
          </CardBody>
        </Card>

        <Card class="mb-4">
          <CardHeader>
            <CardTitle as="h2">{t('storage.upload.title')}</CardTitle>
          </CardHeader>
          <CardBody class="flex flex-wrap items-end gap-3">
            <div class="w-56">
              <label for="storage-prefix" class="text-text mb-1 block text-sm font-semibold">
                {t('storage.upload.prefix')}
              </label>
              <Input
                id="storage-prefix"
                value={prefix}
                placeholder="assets/img"
                class="font-mono"
                onInput={(e) => setPrefix(e.currentTarget.value)}
              />
            </div>
            <div class="min-w-0 flex-1">
              <label for="storage-file" class="text-text mb-1 block text-sm font-semibold">
                {t('storage.upload.file')}
              </label>
              {/* Input nativo: el navegador ya sabe elegir ficheros, y envolverlo
                  en un botón propio solo quita el teclado y el arrastrar-soltar
                  que trae de fábrica. */}
              <input
                id="storage-file"
                type="file"
                disabled={uploading}
                class="text-text file:border-border file:bg-bg-subtle file:text-text hover:file:bg-bg-subtle block w-full text-sm file:mr-3 file:rounded-md file:border file:px-3 file:py-1.5 file:text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--sytue-focus-ring)]"
                onChange={(e) => {
                  const f = e.currentTarget.files?.[0];
                  // Se limpia el input para poder volver a elegir EL MISMO
                  // fichero: sin esto, corregir y reintentar no dispara el evento.
                  e.currentTarget.value = '';
                  if (f) void upload(f);
                }}
              />
            </div>
            <p class="text-text-subtle w-full text-sm">{t('storage.upload.limit')}</p>
          </CardBody>
        </Card>

        {objects.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                illustration={<Icon name="bucket" size={24} />}
                title={t('storage.empty.title')}
                description={t('storage.empty.desc')}
              />
            </CardBody>
          </Card>
        ) : (
          <Table
            columns={columns}
            rows={objects}
            rowKey={(o) => o.key}
            caption={t('storage.caption')}
          />
        )}
      </div>

      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('storage.delete.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={deleting}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={deleting} onClick={() => void confirmDelete()}>
              {t('common.delete')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('storage.delete.body', { key: pendingDelete?.key ?? '' })}
        </p>
      </Modal>
    </>
  );
}
