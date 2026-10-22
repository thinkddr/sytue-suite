/**
 * PauseCard — apagar el proyecto sin borrarlo.
 *
 * Un entorno de pruebas que solo se usa en horario de oficina, una demo que ya
 * terminó, un proyecto esperando una decisión: hasta ahora las dos únicas
 * opciones eran pagarlo mientras corría o BORRARLO y perder dominio, variables y
 * servicios. Es lo que Coolify llama pausar.
 *
 * Pausado significa cero réplicas y CERO factura de cómputo —el metering excluye
 * los proyectos pausados—, y todo lo demás intacto. Por eso no vive en la zona de
 * peligro: no destruye nada y se deshace con un clic.
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
  InlineHelp,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';

interface Props {
  project: string;
  paused: boolean;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function PauseCard(props: Props) {
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

function errorTitle(t: T, err: unknown): string {
  return statusOf(err) === 403 ? t('pause.forbidden') : t('pause.fail');
}

function Inner({ project, paused, live, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [off, setOff] = useState(paused);
  const [busy, setBusy] = useState(false);

  async function toggle() {
    const next = !off;
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/project-runtime', {
        project,
        paused: next,
      });
      setOff(next);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: next ? t('pause.paused') : t('pause.resumed'),
        ...demoDesc(t, res.live, next ? t('pause.paused.desc') : t('pause.resumed.desc')),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', title: errorTitle(t, err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card class="mt-6">
      <CardHeader>
        <CardTitle as="h2">
          <span class="flex items-center gap-2">
            <Icon name="clock" size={18} />
            {t('pause.title')}
          </span>
        </CardTitle>
        {off ? <Badge tone="warning">{t('pause.badge')}</Badge> : null}
      </CardHeader>
      <CardBody class="flex flex-wrap items-center justify-between gap-4">
        <div class="min-w-0">
          <p class="text-text-muted text-sm">{off ? t('pause.desc.paused') : t('pause.desc')}</p>
        </div>
        <Button
          variant={off ? 'primary' : 'secondary'}
          class="shrink-0"
          loading={busy}
          disabled={!live}
          onClick={() => void toggle()}
        >
          {off ? t('pause.resume') : t('pause.action')}
        </Button>
      </CardBody>
      {/* Lo que NO se pierde, dicho aquí y no en la confirmación: es la duda que
          frena a alguien antes de pulsar, no después. */}
      {off ? null : <InlineHelp class="mx-4 mb-4">{t('pause.keeps')}</InlineHelp>}
    </Card>
  );
}
