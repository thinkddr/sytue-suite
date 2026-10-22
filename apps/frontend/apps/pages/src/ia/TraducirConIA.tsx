/**
 * Traducir con IA: traduce los textos del sitio a otros idiomas sin tocar el diseño (§9.3).
 */
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Button, Checkbox, FieldGroup, Select } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { IDIOMAS_VISITANTE } from '@sytue/pages-render';
import { EstadoTrabajoIA, type AIJob } from './EstadoTrabajoIA';
import {
  AvisoErrorIA,
  PanelContextoIA,
  PieAcciones,
  TarjetaResultadoIA,
  lanzarTrabajoIA,
  nombreIdioma,
  resultadoDe,
  textoError,
  useContextoIA,
  useTraduccion,
} from './comun.js';

export interface TraducirConIAProps {
  siteId: string;
  ir?: unknown;
  /** Página seleccionada en el editor; el alcance se envía como page:<id>. */
  pageId?: string;
  /** Idioma predeterminado e idiomas habilitados del sitio. */
  idioma?: string;
  idiomas?: readonly string[];
  /** Idioma de la interfaz; si falta, el del documento. */
  locale?: Locale;
  /** Confirma el último borrador guardado antes de crear el trabajo. */
  prepararBorrador?: () => Promise<boolean>;
  onAplicar?: (parche: unknown, sobrescribir: boolean, destinos: readonly string[]) => void;
  onCerrar?: () => void;
  /** ¿Administra la organización quien mira? Si no, no se pide la vista de la clave. */
  administra?: boolean;
}

const LENGUAS = IDIOMAS_VISITANTE;

export function TraducirConIA({
  siteId,
  pageId,
  idioma,
  idiomas,
  locale: localeProp,
  prepararBorrador,
  onAplicar,
  onCerrar,
  administra,
}: TraducirConIAProps): JSX.Element {
  const { locale, t } = useTraduccion(localeProp);
  const ctx = useContextoIA(administra);

  const lenguasOrigen = LENGUAS.filter((lengua) => !idiomas || idiomas.includes(lengua));
  const inicial =
    idioma && lenguasOrigen.some((lengua) => lengua === idioma)
      ? idioma
      : lenguasOrigen.includes('es')
        ? 'es'
        : (lenguasOrigen[0] ?? 'es');
  const [origen, setOrigen] = useState(inicial);
  const [destinos, setDestinos] = useState<string[]>([inicial === 'es' ? 'en' : 'es']);
  const [alcance, setAlcance] = useState<'site' | 'page'>('site');
  const [sobrescribir, setSobrescribir] = useState(false);
  const [sobrescribirResultado, setSobrescribirResultado] = useState(false);
  const [destinosResultado, setDestinosResultado] = useState<readonly string[]>([]);

  const [enviando, setEnviando] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [parche, setParche] = useState<Record<string, unknown> | null>(null);

  const cambiarOrigen = (nuevo: string) => {
    setOrigen(nuevo);
    setDestinos((d) => d.filter((l) => l !== nuevo));
  };
  const alternar = (l: string) =>
    setDestinos((d) => (d.includes(l) ? d.filter((x) => x !== l) : [...d, l]));

  const lanzar = async () => {
    if (alcance === 'page' && !pageId) {
      setError(t('ai.translate.scope_invalid'));
      return;
    }
    setEnviando(true);
    setError(null);
    setParche(null);
    setSobrescribirResultado(sobrescribir);
    setDestinosResultado([...destinos]);
    const r = await lanzarTrabajoIA(
      `/api/sites/${siteId}/ia/translate`,
      {
        from: origen,
        to: destinos,
        scope: alcance === 'page' ? `page:${pageId}` : 'site',
        overwrite: sobrescribir,
      },
      prepararBorrador,
    );
    setEnviando(false);
    if ('error' in r) {
      setError(textoError(t, locale, r.error));
      return;
    }
    setJobId(r.jobId);
  };

  const alCompletar = (job: AIJob) => {
    const r = resultadoDe(job.result);
    setParche(
      r && typeof r === 'object' && !Array.isArray(r) ? (r as Record<string, unknown>) : {},
    );
  };

  const volver = () => {
    setJobId(null);
    setParche(null);
    setError(null);
  };

  const total = parche ? Object.keys(parche).length : 0;

  return (
    <div class="space-y-5">
      {/* El título lo pone el diálogo que lo abre (`Modal` en EditorApp). */}
      <p class="text-text-muted text-sm">{t('ai.translate.desc')}</p>

      <PanelContextoIA ctx={ctx} t={t} locale={locale} />

      {error ? <AvisoErrorIA texto={error} /> : null}

      {!jobId ? (
        <form
          class="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (destinos.length > 0 && !ctx.bloqueado && !enviando) void lanzar();
          }}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FieldGroup label={t('ai.translate.from_label')} htmlFor="ai-trans-from">
              <Select
                id="ai-trans-from"
                value={origen}
                onChange={(e) => cambiarOrigen(e.currentTarget.value)}
              >
                {lenguasOrigen.map((l) => (
                  <option key={l} value={l}>
                    {nombreIdioma(locale, l)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup label={t('ai.translate.scope_label')} htmlFor="ai-trans-scope">
              <Select
                id="ai-trans-scope"
                value={alcance}
                aria-describedby="ai-trans-scope-help"
                onChange={(e) => {
                  const value = e.currentTarget.value;
                  if (value === 'site' || (value === 'page' && pageId)) setAlcance(value);
                }}
              >
                <option value="site">{t('ai.translate.scope_site')}</option>
                <option value="page" disabled={!pageId}>
                  {t('ai.translate.scope_page')}
                </option>
                <option value="store" disabled>
                  {t('ai.translate.scope_store')}
                </option>
              </Select>
              <p id="ai-trans-scope-help" class="text-text-muted mt-1 text-xs">
                {t('ai.translate.store_unavailable')}
              </p>
            </FieldGroup>
          </div>

          <fieldset>
            <legend class="text-text text-sm font-semibold">{t('ai.translate.to_label')}</legend>
            <p class="text-text-muted mt-1 text-xs">{t('ai.translate.to_help')}</p>
            <div class="mt-2 grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
              {LENGUAS.map((l) => (
                <Checkbox
                  key={l}
                  id={`ai-trans-to-${l}`}
                  label={nombreIdioma(locale, l)}
                  checked={destinos.includes(l)}
                  disabled={l === origen}
                  onChange={() => alternar(l)}
                  class="min-h-[24px] pointer-coarse:min-h-[max(44px,2.75rem)]"
                />
              ))}
            </div>
          </fieldset>

          <Checkbox
            id="ai-trans-overwrite"
            label={t('ai.translate.overwrite_label')}
            checked={sobrescribir}
            onChange={(e) => setSobrescribir(e.currentTarget.checked)}
          />

          <PieAcciones>
            {onCerrar ? (
              <Button type="button" variant="secondary" onClick={onCerrar}>
                {t('ai.common.cancel')}
              </Button>
            ) : null}
            <Button
              type="submit"
              variant="primary"
              loading={enviando}
              disabled={destinos.length === 0 || ctx.bloqueado}
            >
              {t('ai.translate.button_translate')}
            </Button>
          </PieAcciones>
        </form>
      ) : (
        <div class="space-y-4">
          <EstadoTrabajoIA
            jobId={jobId}
            locale={locale}
            titulo={t('ai.translate.job_title')}
            onCompletado={alCompletar}
            onReintentar={() => void lanzar()}
            onCancelar={volver}
          />

          {parche && total === 0 ? (
            <>
              <AvisoErrorIA texto={t('ai.translate.result_empty')} />
              <PieAcciones>
                <Button type="button" variant="secondary" onClick={volver}>
                  {t('ai.translate.button_again')}
                </Button>
              </PieAcciones>
            </>
          ) : null}

          {parche && total > 0 ? (
            <TarjetaResultadoIA
              titulo={t('ai.translate.result_title')}
              acciones={
                <>
                  <Button type="button" variant="secondary" onClick={volver}>
                    {t('ai.translate.button_again')}
                  </Button>
                  <Button
                    type="button"
                    variant="primary"
                    onClick={() => onAplicar?.(parche, sobrescribirResultado, destinosResultado)}
                  >
                    {t('ai.translate.button_apply')}
                  </Button>
                </>
              }
            >
              <p class="text-text text-sm">{t('ai.translate.result_texts', { count: total })}</p>
              <p class="text-text-muted text-xs">{t('ai.translate.result_help')}</p>
            </TarjetaResultadoIA>
          ) : null}
        </div>
      )}
    </div>
  );
}

export default TraducirConIA;
