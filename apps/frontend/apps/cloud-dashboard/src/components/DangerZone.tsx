/**
 * DangerZone — dar de baja el proyecto entero.
 *
 * El control-plane lo permite desde el primer día (`DELETE /projects/{id}`, con
 * rol admin/owner) y el panel no tenía por dónde: un proyecto creado por error se
 * quedaba en la lista para siempre, con sus servicios y su factura.
 *
 * Escribir el nombre no es fricción decorativa: es lo único que distingue «quiero
 * borrar ESTE» de un clic en la fila equivocada, y aquí no hay deshacer. Mismo
 * criterio que la confirmación del restore de base de datos.
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
  Icon,
  Input,
  Modal,
  InlineHelp,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';

interface Props {
  project: string;
  /** Nombre legible; el que hay que teclear es el slug, que es el identificador. */
  name: string;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function DangerZone(props: Props) {
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

function deleteErrorTitle(t: T, err: unknown): string {
  return statusOf(err) === 403 ? t('danger.delete.forbidden') : t('danger.delete.fail');
}

function Inner({ project, name, live, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);

  const confirmed = typed.trim() === project;

  async function remove() {
    if (!confirmed) return;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/projects/delete', {
        project,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      if (res.live) {
        // Ya no existe: quedarse en su pantalla solo produciría 404 al recargar.
        // Con `r()`, porque Cloud vive bajo `/cloud`: una barra suelta sacaba al
        // usuario de la aplicación justo después de borrar su proyecto.
        window.location.href = r('/');
        return;
      }
      setOpen(false);
      setTyped('');
    } catch (err) {
      toast.notify({ tone: 'danger', title: deleteErrorTitle(t, err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {/* `border-danger` y no el token `danger-border`, que no existe en
          theme.css, así que la clase no llegaba a generarse y la zona de
          peligro se quedaba con el pelo gris de cualquier otra tarjeta. */}
      <Card class="border-danger mt-6">
        <CardHeader>
          <CardTitle as="h2">
            <span class="flex items-center gap-2">
              <Icon name="warning" size={18} />
              {t('danger.title')}
            </span>
          </CardTitle>
        </CardHeader>
        <CardBody class="flex flex-wrap items-center justify-between gap-4">
          <div class="min-w-0">
            <p class="text-text font-medium">{t('danger.delete.title', { name })}</p>
            <p class="text-text-muted mt-1 text-sm">{t('danger.delete.desc')}</p>
          </div>
          <Button variant="danger" class="shrink-0" onClick={() => setOpen(true)} disabled={!live}>
            {t('danger.delete.action')}
          </Button>
        </CardBody>
      </Card>

      <Modal
        open={open}
        onClose={() => {
          setOpen(false);
          setTyped('');
        }}
        title={t('danger.confirm.title')}
        size="sm"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setOpen(false);
                setTyped('');
              }}
              disabled={busy}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              loading={busy}
              disabled={!confirmed || busy}
              onClick={() => void remove()}
            >
              {t('danger.confirm.action')}
            </Button>
          </>
        }
      >
        <InlineHelp tone="danger" title={t('danger.confirm.warning')}>
          {t('danger.confirm.body')}
        </InlineHelp>
        <label for="danger-confirm" class="text-text mt-4 mb-1 block text-sm font-semibold">
          {t('danger.confirm.label', { slug: project })}
        </label>
        <Input
          id="danger-confirm"
          value={typed}
          class="font-mono"
          autocomplete="off"
          placeholder={project}
          onInput={(e) => setTyped(e.currentTarget.value)}
        />
      </Modal>
    </>
  );
}
