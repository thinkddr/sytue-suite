/**
 * ServiceLogsModal — el registro de un servicio gestionado, donde se usa.
 *
 * Una función que no arranca era un callejón sin salida: la pantalla la enseñaba
 * en «provisioning» y el motivo solo existía dentro del clúster. Lo mismo con una
 * tarea programada que falla cada noche. Los logs de la APP del proyecto sí se
 * veían; los de sus servicios, no.
 *
 * Se pide al abrir y con un botón de recargar: sin stream. Un SSE por cada
 * función abierta cuesta una conexión viva por pestaña, y aquí lo que se busca
 * casi siempre es «qué dijo al fallar», que ya ocurrió. Si algún día hace falta
 * seguirlo en vivo, el añadido es el mismo SSE que ya usa el visor de la app.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import { Button, Modal, Icon, EmptyState, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { ServiceLogLine } from '../lib/cloud.js';
import { useT } from '../i18n/index.js';

interface Props {
  serviceId: string;
  serviceName: string;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function ServiceLogsModal({ serviceId, serviceName, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [lines, setLines] = useState<ServiceLogLine[]>([]);
  const [live, setLive] = useState(true);

  async function load() {
    setLoading(true);
    try {
      const res = await fetch(
        `/api/services/logs?service=${encodeURIComponent(serviceId)}&lines=200`,
      );
      if (!res.ok) throw new Error(`bff_${res.status}`);
      const data = (await res.json()) as { lines: ServiceLogLine[]; live: boolean };
      setLines(data.lines);
      setLive(data.live);
    } catch {
      toast.notify({ tone: 'danger', title: t('svclogs.fail') });
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Button
        variant="ghost"
        class="text-sm"
        onClick={() => {
          setOpen(true);
          void load();
        }}
        aria-label={t('svclogs.aria', { name: serviceName })}
      >
        {t('svclogs.action')}
      </Button>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t('svclogs.title', { name: serviceName })}
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              {t('common.close')}
            </Button>
            <Button variant="secondary" loading={loading} onClick={() => void load()}>
              {t('svclogs.refresh')}
            </Button>
          </>
        }
      >
        {loading && lines.length === 0 ? (
          <p class="text-text-muted py-8 text-center text-base">{t('svclogs.loading')}</p>
        ) : lines.length === 0 ? (
          <EmptyState
            illustration={<Icon name="audit" size={24} />}
            title={t('svclogs.empty.title')}
            description={live ? t('svclogs.empty.desc') : t('svclogs.empty.demo')}
          />
        ) : (
          // Monoespaciado y con scroll: es un registro, no una tabla. La hora va
          // recortada al segundo —el nanosegundo de Kubernetes no aporta nada a
          // quien está buscando el error— y en su propia columna para poder leer
          // el texto en vertical.
          <pre class="border-border bg-surface-sunken text-text max-h-[60dvh] overflow-auto rounded-md border p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">
            {lines.map((l) => (
              <div key={`${l.ts}-${l.text}`}>
                <span class="text-text-subtle mr-2 tabular-nums">{l.ts.slice(11, 19)}</span>
                {l.text}
              </div>
            ))}
          </pre>
        )}
      </Modal>
    </>
  );
}
