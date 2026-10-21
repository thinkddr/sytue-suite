/**
 * Exportar con IA: elegir la plataforma de destino, generar el paquete y descargar el ZIP (§9.3, §9.4).
 */
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Badge, Button, ChoiceCard, Icon, withBase } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { ClaveMensaje } from '../i18n/index.js';
import { EstadoTrabajoIA } from './EstadoTrabajoIA';
import {
  AvisoErrorIA,
  PanelContextoIA,
  PieAcciones,
  TarjetaResultadoIA,
  lanzarTrabajoIA,
  textoError,
  useContextoIA,
  useTraduccion,
} from './comun.js';

export interface ExportarConIAProps {
  siteId: string;
  /** Idioma de la interfaz; si falta, el del documento. */
  locale?: Locale;
  onCerrar?: () => void;
}

interface DestinoExport {
  id: string;
  nombre: ClaveMensaje;
  descripcion: ClaveMensaje;
  disponible: boolean;
}

const DESTINOS: DestinoExport[] = [
  {
    id: 'shopify',
    nombre: 'ai.export.dest_shopify',
    descripcion: 'ai.export.dest_shopify_desc',
    disponible: true,
  },
  {
    id: 'react',
    nombre: 'ai.export.dest_react',
    descripcion: 'ai.export.dest_react_desc',
    disponible: false,
  },
  {
    id: 'nextjs',
    nombre: 'ai.export.dest_nextjs',
    descripcion: 'ai.export.dest_nextjs_desc',
    disponible: false,
  },
  {
    id: 'woocommerce',
    nombre: 'ai.export.dest_woocommerce',
    descripcion: 'ai.export.dest_woocommerce_desc',
    disponible: false,
  },
];

export function ExportarConIA({
  siteId,
  locale: localeProp,
  onCerrar,
}: ExportarConIAProps): JSX.Element {
  const { locale, t } = useTraduccion(localeProp);
  const ctx = useContextoIA();

  const [destino, setDestino] = useState('shopify');
  const [enviando, setEnviando] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [listo, setListo] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const elegido = DESTINOS.find((d) => d.id === destino) ?? DESTINOS[0]!;

  const lanzar = async () => {
    setEnviando(true);
    setError(null);
    setListo(false);
    const r = await lanzarTrabajoIA(`/api/exportar-ia/${siteId}`, { destination: destino });
    setEnviando(false);
    if ('error' in r) {
      setError(textoError(t, locale, r.error));
      return;
    }
    setJobId(r.jobId);
  };

  const volver = () => {
    setJobId(null);
    setListo(false);
    setError(null);
  };

  return (
    <div class="space-y-5">
      <div>
        <h3 class="sytue-h3 text-text">{t('ai.export.title')}</h3>
        <p class="text-text-muted mt-1 text-sm">{t('ai.export.desc')}</p>
      </div>

      <PanelContextoIA ctx={ctx} t={t} locale={locale} />

      {error ? <AvisoErrorIA texto={error} /> : null}

      {!jobId ? (
        <form
          class="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!ctx.bloqueado && !enviando) void lanzar();
          }}
        >
          <fieldset>
            <legend class="text-text text-sm font-semibold">
              {t('ai.export.destination_label')}
            </legend>
            <div class="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
              {DESTINOS.map((d) => (
                <ChoiceCard
                  key={d.id}
                  name="ai-export-destino"
                  value={d.id}
                  title={t(d.nombre)}
                  description={t(d.descripcion)}
                  badge={
                    <Badge tone={d.disponible ? 'success' : 'neutral'}>
                      {d.disponible ? t('ai.export.badge_available') : t('ai.export.badge_soon')}
                    </Badge>
                  }
                  selected={destino === d.id}
                  disabled={!d.disponible}
                  onSelect={() => setDestino(d.id)}
                />
              ))}
            </div>
          </fieldset>

          <PieAcciones>
            {onCerrar ? (
              <Button type="button" variant="secondary" onClick={onCerrar}>
                {t('ai.common.cancel')}
              </Button>
            ) : null}
            <Button type="submit" variant="primary" loading={enviando} disabled={ctx.bloqueado}>
              {t('ai.export.button_export')}
            </Button>
          </PieAcciones>
        </form>
      ) : (
        <div class="space-y-4">
          <EstadoTrabajoIA
            jobId={jobId}
            locale={locale}
            titulo={t('ai.export.job_title', { destination: t(elegido.nombre) })}
            onCompletado={() => setListo(true)}
            onReintentar={() => void lanzar()}
            onCancelar={volver}
          />

          {listo ? (
            <TarjetaResultadoIA
              titulo={t('ai.export.result_title')}
              acciones={
                <>
                  <Button type="button" variant="secondary" onClick={volver}>
                    {t('ai.export.button_again')}
                  </Button>
                  <Button
                    variant="primary"
                    href={withBase(import.meta.env.BASE_URL, `/api/ia/jobs/${jobId}/zip`)}
                    leadingIcon={<Icon name="download" size={16} />}
                  >
                    {t('ai.export.button_download')}
                  </Button>
                </>
              }
            >
              <p class="text-text-muted text-sm">{t('ai.export.result_help')}</p>
              <div class="bg-bg-subtle rounded-md p-3">
                <p class="text-text text-sm font-semibold">{t('ai.export.steps_title')}</p>
                <ol class="text-text-muted mt-1 list-decimal space-y-1 pl-5 text-sm">
                  <li>{t('ai.export.step_1')}</li>
                  <li>{t('ai.export.step_2')}</li>
                  <li>{t('ai.export.step_3')}</li>
                </ol>
              </div>
            </TarjetaResultadoIA>
          ) : null}
        </div>
      )}
    </div>
  );
}

export default ExportarConIA;
