import type { JSX } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import {
  Checkbox,
  Button,
  ChoiceCard,
  FieldGroup,
  Input,
  Wizard,
  describedBy,
  type WizardStep,
} from '@sytue/ui';
import { type Locale, plegar } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { Marco } from '../components/Marco.js';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';
import { leerFallo } from '../lib/ficha.js';
import { GaleriaPlantillas } from '../components/GaleriaPlantillas.js';
import { nombreDe, type PlantillaInfo } from '../lib/galeria.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

/** Disponibilidad del identificador: `null` = aún sin comprobar o sin poder comprobarse. */
type Disponible = boolean | null;

const aSlug = (texto: string) =>
  plegar(texto)
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');

export function NuevoSitioApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = (k: ClaveMensaje, params?: Record<string, string | number>) =>
    translate(locale, k, params);

  const [paso, setPaso] = useState(0);
  const [plantillas, setPlantillas] = useState<PlantillaInfo[]>([]);
  const [errorPlantillas, setErrorPlantillas] = useState(false);
  const [intentoPlantillas, setIntentoPlantillas] = useState(0);
  const [plantilla, setPlantilla] = useState<string>('');

  const [nombre, setNombre] = useState('');
  const [slug, setSlug] = useState('');
  const [slugManual, setSlugManual] = useState(false);
  const [slugDisponible, setSlugDisponible] = useState<Disponible>(null);
  const [comprobandoSlug, setComprobandoSlug] = useState(false);
  const [errorSlug, setErrorSlug] = useState(false);

  const [idiomaPrincipal, setIdiomaPrincipal] = useState<'es' | 'en'>(
    locale === 'es' ? 'es' : 'en',
  );
  const [bilingue, setBilingue] = useState(true);

  const [creando, setCreando] = useState(false);
  const [errorCreacion, setErrorCreacion] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setErrorPlantillas(false);
    fetch(ruta('/api/templates'), { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(String(res.status));
        return res.json() as Promise<PlantillaInfo[]>;
      })
      .then(setPlantillas)
      .catch(() => {
        if (!controller.signal.aborted) setErrorPlantillas(true);
      });
    return () => controller.abort();
  }, [intentoPlantillas]);

  // Comprobación de disponibilidad del identificador, con espera para no preguntar por cada letra.
  useEffect(() => {
    setErrorSlug(false);
    if (!slug) {
      setSlugDisponible(null);
      setComprobandoSlug(false);
      return;
    }
    setComprobandoSlug(true);
    let vigente = true;
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const res = await fetch(ruta(`/api/sites/check-slug?slug=${encodeURIComponent(slug)}`));
          if (!res.ok) throw new Error(String(res.status));
          const data = (await res.json()) as { available: boolean };
          if (vigente) setSlugDisponible(data.available);
        } catch {
          // Sin poder comprobarlo no se bloquea: el servidor lo vuelve a validar al crear.
          if (vigente) {
            setSlugDisponible(null);
            setErrorSlug(true);
          }
        } finally {
          if (vigente) setComprobandoSlug(false);
        }
      })();
    }, 300);
    return () => {
      vigente = false;
      clearTimeout(timer);
    };
  }, [slug]);

  const crear = async () => {
    setCreando(true);
    setErrorCreacion(null);
    try {
      const languages: string[] = [idiomaPrincipal];
      if (bilingue) languages.push(idiomaPrincipal === 'es' ? 'en' : 'es');
      const res = await fetch(ruta('/api/sites'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: nombre.trim(), slug, template: plantilla, languages }),
      });
      if (!res.ok) {
        const err = await leerFallo(res);
        if (err.code === 'slug_conflict') {
          setSlugDisponible(false);
          setPaso(0);
          return;
        }
        if (err.code === 'site_limit') {
          setErrorCreacion(t('new.err_site_limit'));
          return;
        }
        throw new Error(String(res.status));
      }
      const sitio = (await res.json()) as { id: string };
      window.location.assign(ruta(`/edit/${sitio.id}`));
    } catch {
      setErrorCreacion(t('new.create_error'));
    } finally {
      setCreando(false);
    }
  };

  const estadoSlug = comprobandoSlug
    ? { texto: t('new.slug_checking'), clase: 'text-text-muted' }
    : slugDisponible === true
      ? { texto: t('new.slug_available'), clase: 'text-success-text' }
      : errorSlug
        ? { texto: t('new.slug_unverified'), clase: 'text-text-muted' }
        : null;

  const pasoPlantilla: WizardStep = {
    id: 'plantilla',
    title: t('new.step_template'),
    heading: t('new.template_step'),
    canContinue: Boolean(plantilla),
    content: (
      <div>
        {errorPlantillas ? (
          <div role="alert" class="mb-4 space-y-2">
            <p class="text-danger-text text-sm">{t('new.templates_error')}</p>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setIntentoPlantillas((n) => n + 1)}
            >
              {t('sites.retry')}
            </Button>
          </div>
        ) : null}
        <GaleriaPlantillas
          plantillas={plantillas}
          seleccionada={plantilla}
          onSeleccionar={setPlantilla}
          locale={locale === 'es' ? 'es' : 'en'}
          t={t}
        />
      </div>
    ),
  };
  const pasoDatos: WizardStep = {
    id: 'datos',
    title: t('new.step_info'),
    heading: t('new.details_step'),
    description: t('new.details_help'),
    canContinue:
      nombre.trim() !== '' && slug !== '' && slugDisponible !== false && !comprobandoSlug,
    content: (
      <div class="max-w-xl space-y-5">
        <FieldGroup label={t('new.name_label')} htmlFor="sitio-nombre" required>
          <Input
            id="sitio-nombre"
            value={nombre}
            autocomplete="off"
            placeholder={t('new.name_placeholder')}
            onInput={(e) => {
              const val = (e.target as HTMLInputElement).value;
              setNombre(val);
              if (!slugManual) setSlug(aSlug(val));
            }}
          />
        </FieldGroup>

        <FieldGroup
          label={t('new.slug_label')}
          htmlFor="sitio-slug"
          required
          help={t('new.slug_help')}
          {...(slugDisponible === false ? { error: t('new.slug_unavailable') } : {})}
        >
          <Input
            id="sitio-slug"
            value={slug}
            autocomplete="off"
            spellcheck={false}
            placeholder={t('new.slug_placeholder')}
            invalid={slugDisponible === false}
            aria-describedby={describedBy(
              'sitio-slug',
              t('new.slug_help'),
              slugDisponible === false ? t('new.slug_unavailable') : undefined,
            )}
            onInput={(e) => {
              setSlugManual(true);
              setSlug(
                (e.target as HTMLInputElement).value.toLowerCase().replace(/[^a-z0-9-]/g, ''),
              );
            }}
          />
        </FieldGroup>
        <p role="status" aria-live="polite" class={`min-h-5 text-xs ${estadoSlug?.clase ?? ''}`}>
          {estadoSlug?.texto ?? ''}
        </p>
      </div>
    ),
  };
  const pasoIdiomas: WizardStep = {
    id: 'idiomas',
    title: t('new.step_languages'),
    heading: t('new.languages_label'),
    content: (
      <div class="max-w-xl space-y-5">
        <fieldset>
          <legend class="text-text mb-2 text-sm font-semibold">{t('new.main_language')}</legend>
          <div role="radiogroup" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <ChoiceCard
              name="idioma-principal"
              value="es"
              title={t('new.lang_es')}
              dense
              selected={idiomaPrincipal === 'es'}
              onSelect={() => setIdiomaPrincipal('es')}
            />
            <ChoiceCard
              name="idioma-principal"
              value="en"
              title={t('new.lang_en')}
              dense
              selected={idiomaPrincipal === 'en'}
              onSelect={() => setIdiomaPrincipal('en')}
            />
          </div>
        </fieldset>

        <Checkbox
          id="sitio-bilingue"
          label={t('new.bilingual', {
            idioma: idiomaPrincipal === 'es' ? t('new.lang_en') : t('new.lang_es'),
          })}
          checked={bilingue}
          onChange={(e) => setBilingue(e.currentTarget.checked)}
        />

        <p class="text-text-muted text-xs">{t('new.languages_notice')}</p>
      </div>
    ),
  };
  const pasos: WizardStep[] = [
    pasoDatos,
    pasoPlantilla,
    pasoIdiomas,
    {
      id: 'revisar',
      title: t('new.step_review'),
      heading: t('new.review_heading'),
      description: t('new.review_help'),
      nextLabel: t('new.submit'),
      onNext: crear,
      canContinue:
        Boolean(plantilla) && nombre.trim() !== '' && slug !== '' && slugDisponible !== false,
      content: (
        <div class="max-w-xl space-y-5">
          {errorCreacion && (
            <p role="alert" class="border-danger text-danger-text rounded-md border p-3 text-sm">
              {errorCreacion}
            </p>
          )}
          <dl class="border-border bg-bg-subtle grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 rounded-lg border p-5 text-sm">
            <dt class="text-text-muted">{t('new.name_label')}</dt>
            <dd class="font-semibold">{nombre.trim()}</dd>
            <dt class="text-text-muted">{t('new.slug_label')}</dt>
            <dd>{slug}</dd>
            <dt class="text-text-muted">{t('new.step_template')}</dt>
            <dd>
              {plantilla === 'blanco'
                ? t('new.template_blank')
                : nombreDe(
                    plantillas.find((p) => p.key === plantilla) ?? {
                      key: plantilla,
                      name: {},
                      description: {},
                      vertical: '',
                      store: false,
                    },
                    locale,
                  )}
            </dd>
            <dt class="text-text-muted">{t('new.primary_language')}</dt>
            <dd>{t(idiomaPrincipal === 'es' ? 'new.lang_es' : 'new.lang_en')}</dd>
            <dt class="text-text-muted">{t('new.step_languages')}</dt>
            <dd>
              {bilingue
                ? idiomaPrincipal === 'es'
                  ? `${t('new.lang_es')} / ${t('new.lang_en')}`
                  : `${t('new.lang_en')} / ${t('new.lang_es')}`
                : t(idiomaPrincipal === 'es' ? 'new.lang_es' : 'new.lang_en')}
            </dd>
          </dl>
          <p class="text-text-muted text-sm">{t('new.draft_notice')}</p>
        </div>
      ),
    },
  ];

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="new">
      <div class="mx-auto max-w-4xl p-4 sm:p-6">
        <h1 class="sytue-h2 text-text mb-6">{t('new.title')}</h1>
        <Wizard
          steps={pasos}
          current={paso}
          onCurrentChange={setPaso}
          busy={creando}
          onCancel={() => window.location.assign(ruta('/'))}
          labels={{
            back: t('new.back'),
            next: t('new.next'),
            cancel: t('new.cancel'),
            progress: t('new.progress'),
            goToStep: (n) => t('new.go_to_step', { n }),
            stepOf: (n, total) => t('new.step_of', { n, total }),
          }}
        />
      </div>
    </Marco>
  );
}

export default NuevoSitioApp;
