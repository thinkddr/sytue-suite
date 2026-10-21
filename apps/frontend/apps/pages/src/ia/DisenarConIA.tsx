/**
 * Diseñar con IA: describe el negocio y la IA propone las secciones de la página (§9.3, §14.1).
 */
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Button, FieldGroup, Select, Textarea } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { ClaveMensaje } from '../i18n/index.js';
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

export interface DisenarConIAProps {
  siteId: string;
  ir?: unknown;
  /** Idioma de los textos del sitio que se está editando (no el de la interfaz). */
  idioma?: string;
  /** Idioma de la interfaz; si falta, el del documento. */
  locale?: Locale;
  /** Confirma el último borrador guardado antes de crear el trabajo. */
  prepararBorrador?: () => Promise<boolean>;
  onAplicar?: (secciones: unknown[]) => void;
  onCerrar?: () => void;
  /** ¿Administra la organización quien mira? Si no, no se pide la vista de la clave. */
  administra?: boolean;
}

const IDEAS: ClaveMensaje[] = [
  'ai.design.idea_1',
  'ai.design.idea_2',
  'ai.design.idea_3',
  'ai.design.idea_4',
];

// El estilo viaja DENTRO del prompt: la API rechaza campos que no conoce (`style` daba 400) y
// no tiene uno para esto. Va en inglés fijo porque lo lee el modelo, no la persona.
const ESTILOS: { id: string; etiqueta: ClaveMensaje; pista: string }[] = [
  { id: 'modern', etiqueta: 'ai.design.style_modern', pista: 'modern and clean' },
  { id: 'elegant', etiqueta: 'ai.design.style_elegant', pista: 'elegant and refined' },
  { id: 'bold', etiqueta: 'ai.design.style_bold', pista: 'bold and vibrant' },
  { id: 'minimal', etiqueta: 'ai.design.style_minimal', pista: 'minimalist' },
];

const LENGUAS = ['es', 'en', 'ca', 'gl', 'eu', 'fr', 'de', 'it', 'pt'];

export function DisenarConIA({
  siteId,
  idioma = 'es',
  locale: localeProp,
  prepararBorrador,
  onAplicar,
  onCerrar,
  administra,
}: DisenarConIAProps): JSX.Element {
  const { locale, t } = useTraduccion(localeProp);
  const ctx = useContextoIA(administra);

  const [prompt, setPrompt] = useState('');
  const [estilo, setEstilo] = useState('modern');
  const [lang, setLang] = useState(LENGUAS.includes(idioma) ? idioma : 'es');
  const [enviando, setEnviando] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [secciones, setSecciones] = useState<unknown[] | null>(null);

  const lanzar = async () => {
    const pista = ESTILOS.find((e) => e.id === estilo)?.pista ?? '';
    setEnviando(true);
    setError(null);
    setSecciones(null);
    const r = await lanzarTrabajoIA(
      `/api/sites/${siteId}/ia/design`,
      {
        prompt: `${prompt.trim()}\n\nVisual style: ${pista}.`,
        language: lang,
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
    const r = resultadoDe(job.result) as { sections?: unknown[] } | unknown[] | null;
    const lista = Array.isArray(r) ? r : r?.sections;
    setSecciones(Array.isArray(lista) ? lista : []);
  };

  const volver = () => {
    setJobId(null);
    setSecciones(null);
    setError(null);
  };

  return (
    <div class="space-y-5">
      {/* El título lo pone el diálogo que lo abre (`Modal` en EditorApp). */}
      <p class="text-text-muted text-sm">{t('ai.design.desc')}</p>

      <PanelContextoIA ctx={ctx} t={t} locale={locale} />

      {error ? <AvisoErrorIA texto={error} /> : null}

      {!jobId ? (
        <form
          class="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (prompt.trim() && !ctx.bloqueado && !enviando) void lanzar();
          }}
        >
          <FieldGroup
            label={t('ai.design.prompt_label')}
            htmlFor="ai-prompt"
            help={t('ai.design.prompt_help')}
          >
            <Textarea
              id="ai-prompt"
              rows={4}
              value={prompt}
              onInput={(e) => setPrompt(e.currentTarget.value)}
              placeholder={t('ai.design.prompt_placeholder')}
            />
          </FieldGroup>

          <div>
            <p class="text-text-muted text-xs font-semibold">{t('ai.design.ideas_label')}</p>
            <ul class="mt-1 space-y-0.5">
              {IDEAS.map((clave) => (
                <li key={clave}>
                  <button
                    type="button"
                    onClick={() => setPrompt(t(clave))}
                    class="sytue-focus-ring text-primary-text min-h-[24px] rounded-sm text-left text-sm hover:underline pointer-coarse:min-h-[max(44px,2.75rem)]"
                  >
                    {t(clave)}
                  </button>
                </li>
              ))}
            </ul>
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FieldGroup label={t('ai.design.style_label')} htmlFor="ai-style">
              <Select
                id="ai-style"
                value={estilo}
                onChange={(e) => setEstilo(e.currentTarget.value)}
              >
                {ESTILOS.map((e) => (
                  <option key={e.id} value={e.id}>
                    {t(e.etiqueta)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup label={t('ai.design.lang_label')} htmlFor="ai-lang">
              <Select id="ai-lang" value={lang} onChange={(e) => setLang(e.currentTarget.value)}>
                {LENGUAS.map((l) => (
                  <option key={l} value={l}>
                    {nombreIdioma(locale, l)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          </div>

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
              disabled={!prompt.trim() || ctx.bloqueado}
            >
              {t('ai.design.button_generate')}
            </Button>
          </PieAcciones>
        </form>
      ) : (
        <div class="space-y-4">
          <EstadoTrabajoIA
            jobId={jobId}
            locale={locale}
            titulo={t('ai.design.job_title')}
            onCompletado={alCompletar}
            onReintentar={() => void lanzar()}
            onCancelar={volver}
          />

          {secciones && secciones.length === 0 ? (
            <>
              <AvisoErrorIA texto={t('ai.design.result_empty')} />
              <PieAcciones>
                <Button type="button" variant="secondary" onClick={volver}>
                  {t('ai.design.button_again')}
                </Button>
              </PieAcciones>
            </>
          ) : null}

          {secciones && secciones.length > 0 ? (
            <TarjetaResultadoIA
              titulo={t('ai.design.result_title')}
              acciones={
                <>
                  <Button type="button" variant="secondary" onClick={volver}>
                    {t('ai.design.button_again')}
                  </Button>
                  <Button type="button" variant="primary" onClick={() => onAplicar?.(secciones)}>
                    {t('ai.design.button_apply')}
                  </Button>
                </>
              }
            >
              <p class="text-text text-sm">
                {t('ai.design.result_sections', { count: secciones.length })}
              </p>
              <p class="text-text-muted text-xs">{t('ai.design.result_help')}</p>
            </TarjetaResultadoIA>
          ) : null}
        </div>
      )}
    </div>
  );
}

export default DisenarConIA;
