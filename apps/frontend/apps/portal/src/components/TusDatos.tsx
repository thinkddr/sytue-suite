/**
 * TusDatos — «Descarga una copia de todo lo tuyo» (RGPD art. 20).
 *
 * Una decisión y un botón: en qué formato van las LISTAS (JSON, CSV o XML). Lo
 * demás no se elige porque no hay nada que elegir: lo que tiene formato estándar
 * va en él (.eml, .ics, .vcf, los archivos tal cual) y la copia lleva todo.
 *
 * La descarga es un enlace normal al BFF, que la pasa en streaming desde Sytue Hub:
 * así la guarda el gestor de descargas del navegador, que sigue aunque se cierre la
 * pestaña y enseña su propio progreso, en vez de una barra que habría que inventar.
 *
 * Cada copia es de UNA organización: el tenant va firmado en el token. Con varias,
 * elegir otra es entrar en ella (el mismo cambio que el menú de la cuenta) y volver
 * aquí con ?descargar=<formato>, que empieza la descarga sola.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { adminUrl } from '@sytue/config';
import { Button, ChoiceCard, InlineHelp, SettingsSection, SettingsShell } from '@sytue/ui';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';

type Formato = 'json' | 'csv' | 'xml';

const FORMATOS: {
  value: Formato;
  title: string;
  key: 'data.export.json' | 'data.export.csv' | 'data.export.xml';
}[] = [
  // CSV primero: es el que abre cualquiera sin instalar nada.
  { value: 'csv', title: 'CSV', key: 'data.export.csv' },
  { value: 'json', title: 'JSON', key: 'data.export.json' },
  { value: 'xml', title: 'XML', key: 'data.export.xml' },
];

/** Una de mis organizaciones, tal y como la sirve `/auth/organizations`. */
interface MiOrg {
  id: string;
  name: string;
  active: boolean;
}

function esFormato(v: string | null): v is Formato {
  return v === 'json' || v === 'csv' || v === 'xml';
}

export function TusDatos({ locale }: { locale: Locale }): JSX.Element {
  const t = useT(locale);
  const [formato, setFormato] = useState<Formato>('csv');
  const [empezada, setEmpezada] = useState(false);
  const [orgs, setOrgs] = useState<MiOrg[]>([]);
  const [org, setOrg] = useState('');
  const [fallo, setFallo] = useState(false);

  useEffect(() => {
    const pedida = new URLSearchParams(window.location.search).get('descargar');
    if (esFormato(pedida)) {
      setFormato(pedida);
      setEmpezada(true);
      window.history.replaceState(null, '', window.location.pathname);
      window.location.assign(r(`/api/export?format=${pedida}`));
    }
    void fetch(r('/auth/organizations'))
      .then((res) => (res.ok ? res.json() : null))
      .then((body: { organizations?: MiOrg[] } | null) => {
        const lista = Array.isArray(body?.organizations) ? body.organizations : [];
        setOrgs(lista);
        setOrg(lista.find((o) => o.active)?.id ?? '');
      })
      .catch(() => undefined);
  }, []);

  const otra = orgs.find((o) => o.id === org && !o.active);

  async function entrarYDescargar(destino: MiOrg) {
    setFallo(false);
    try {
      const res = await fetch(r('/auth/organizations'), {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ org_id: destino.id }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const vuelta = `${window.location.pathname}?descargar=${formato}`;
      window.location.assign(r(`/auth/login?returnTo=${encodeURIComponent(vuelta)}`));
    } catch {
      setFallo(true);
    }
  }

  return (
    <SettingsShell title={t('data.title')} description={t('data.lead')}>
      <SettingsSection title={t('data.export.section')}>
        <div class="flex max-w-2xl flex-col gap-4">
          <p class="text-text-muted text-sm leading-relaxed">{t('data.export.body')}</p>
          {orgs.length > 1 ? (
            <fieldset class="flex flex-col gap-2">
              <legend class="text-text mb-1 text-sm font-semibold">{t('ob.sum.org')}</legend>
              <p class="text-text-muted mb-1 text-xs">{t('data.export.orgHelp')}</p>
              {orgs.map((o) => (
                <ChoiceCard
                  key={o.id}
                  dense
                  name="sytue-export-org"
                  value={o.id}
                  title={o.name}
                  selected={org === o.id}
                  onSelect={() => setOrg(o.id)}
                />
              ))}
            </fieldset>
          ) : null}
          <fieldset class="flex flex-col gap-2">
            <legend class="text-text mb-1 text-sm font-semibold">
              {t('data.export.formatLegend')}
            </legend>
            <p class="text-text-muted mb-1 text-xs">{t('data.export.formatHelp')}</p>
            {FORMATOS.map((f) => (
              <ChoiceCard
                key={f.value}
                dense
                name="sytue-export-format"
                value={f.value}
                title={f.title}
                description={t(f.key)}
                selected={formato === f.value}
                onSelect={() => setFormato(f.value)}
              />
            ))}
          </fieldset>
          {otra ? (
            <Button
              variant="primary"
              class="self-start"
              onClick={() => void entrarYDescargar(otra)}
            >
              {t('data.export.action')}
            </Button>
          ) : (
            <a
              href={r(`/api/export?format=${formato}`)}
              download
              onClick={() => setEmpezada(true)}
              class="bg-primary sytue-focus-ring-inset rounded-control inline-flex min-h-11 items-center justify-center gap-2 self-start px-5 py-2.5 text-sm font-semibold text-[var(--sytue-on-primary)] transition-colors duration-150 hover:bg-[var(--sytue-primary-hover)]"
            >
              {t('data.export.action')}
            </a>
          )}
          {fallo ? (
            <p class="text-danger-text text-sm" role="alert">
              {t('ob.error.generic')}
            </p>
          ) : null}
          {empezada ? (
            <InlineHelp tone="success" title={t('data.export.startedTitle')}>
              {t('data.export.startedBody')}
            </InlineHelp>
          ) : null}
          <p class="text-text-muted text-xs leading-relaxed">{t('data.export.scope')}</p>
        </div>
      </SettingsSection>
      <SettingsSection title={t('data.move.section')}>
        <div class="flex max-w-2xl flex-col gap-3">
          <p class="text-text-muted text-sm leading-relaxed">{t('data.move.body')}</p>
          <a
            href={adminUrl('migrations')}
            class="text-primary-text sytue-focus-ring self-start rounded text-sm font-semibold hover:underline"
          >
            {t('data.move.action')}
          </a>
        </div>
      </SettingsSection>
    </SettingsShell>
  );
}
