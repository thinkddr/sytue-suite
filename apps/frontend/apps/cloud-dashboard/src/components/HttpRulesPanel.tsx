/**
 * HttpRulesPanel — redirecciones y cabeceras del proyecto.
 *
 * Es lo que en Vercel se escribe en `vercel.json`: mover una URL que cambió sin
 * perder los enlaces de nadie, y poner las cabeceras que la app no pone. Aquí no
 * existía, así que la única salida era resolverlo dentro del código de cada
 * aplicación — y en un sitio estático, ni eso.
 *
 * Se aplican en el borde (Traefik), antes de llegar a la app.
 *
 * # No se escriben expresiones regulares
 *
 * El campo pide una RUTA y admite un `*` final. Lo traduce la plataforma
 * escapando el resto: una regex ajena en el borde es un problema de rendimiento
 * y de seguridad, y quien la necesita de verdad es una minoría diminuta.
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
  Select,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';

export interface Redirect {
  from: string;
  to: string;
  status?: number;
}

export interface HttpRules {
  redirects?: Redirect[];
  headers?: Record<string, string>;
}

/** Mismos topes que el dominio (MaxRedirects / MaxHeaders). */
const MAX = 20;

interface Props {
  project: string;
  rules: HttpRules;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function HttpRulesPanel(props: Props) {
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
  switch (statusOf(err)) {
    case 400:
      return t('rules.invalid');
    case 403:
      return t('rules.forbidden');
    default:
      return t('rules.fail');
  }
}

/** Par clave/valor editable, para que las cabeceras no sean un textarea de JSON. */
interface HeaderRow {
  name: string;
  value: string;
}

function Inner({ project, rules, live, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [redirects, setRedirects] = useState<Redirect[]>(rules.redirects ?? []);
  const [headers, setHeaders] = useState<HeaderRow[]>(
    Object.entries(rules.headers ?? {}).map(([name, value]) => ({ name, value })),
  );
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      // Las filas vacías no viajan: son el hueco que deja «añadir» antes de
      // escribir, no una regla que el usuario quiera guardar.
      const payload: HttpRules = {
        redirects: redirects.filter((r) => r.from.trim() && r.to.trim()),
        headers: Object.fromEntries(
          headers
            .filter((h) => h.name.trim() && h.value.trim())
            .map((h) => [h.name.trim(), h.value.trim()]),
        ),
      };
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/project-runtime', {
        project,
        http_rules: payload,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('rules.saved'),
        ...demoDesc(t, res.live, t('rules.saved.desc')),
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
        <div class="min-w-0">
          <CardTitle as="h2">{t('rules.title')}</CardTitle>
          <p class="text-text-muted mt-0.5 text-sm">{t('rules.subtitle')}</p>
        </div>
      </CardHeader>
      <CardBody class="flex flex-col gap-6">
        <section aria-labelledby="rules-redirects">
          <h3 id="rules-redirects" class="text-text mb-2 text-sm font-semibold">
            {t('rules.redirects')}
          </h3>
          <ul class="flex flex-col gap-2">
            {redirects.map((r, i) => (
              <li key={`r${i}`} class="flex flex-wrap items-end gap-2">
                <div class="min-w-[12rem] flex-1">
                  <label class="text-text-muted mb-1 block text-xs" for={`rule-from-${i}`}>
                    {t('rules.from')}
                  </label>
                  <Input
                    id={`rule-from-${i}`}
                    value={r.from}
                    placeholder="/blog/*"
                    class="font-mono"
                    onInput={(e) =>
                      setRedirects((prev) =>
                        prev.map((x, j) => (j === i ? { ...x, from: e.currentTarget.value } : x)),
                      )
                    }
                  />
                </div>
                <div class="min-w-[12rem] flex-1">
                  <label class="text-text-muted mb-1 block text-xs" for={`rule-to-${i}`}>
                    {t('rules.to')}
                  </label>
                  <Input
                    id={`rule-to-${i}`}
                    value={r.to}
                    placeholder="/noticias"
                    class="font-mono"
                    onInput={(e) =>
                      setRedirects((prev) =>
                        prev.map((x, j) => (j === i ? { ...x, to: e.currentTarget.value } : x)),
                      )
                    }
                  />
                </div>
                <div class="w-28">
                  <label class="text-text-muted mb-1 block text-xs" for={`rule-status-${i}`}>
                    {t('rules.status')}
                  </label>
                  <Select
                    id={`rule-status-${i}`}
                    value={String(r.status ?? 308)}
                    onChange={(e) =>
                      setRedirects((prev) =>
                        prev.map((x, j) =>
                          j === i ? { ...x, status: Number(e.currentTarget.value) } : x,
                        ),
                      )
                    }
                  >
                    <option value="308">308</option>
                    <option value="301">301</option>
                    <option value="307">307</option>
                    <option value="302">302</option>
                  </Select>
                </div>
                <Button
                  variant="ghost"
                  class="text-danger-text"
                  aria-label={t('rules.remove')}
                  onClick={() => setRedirects((prev) => prev.filter((_, j) => j !== i))}
                >
                  {t('common.delete')}
                </Button>
              </li>
            ))}
          </ul>
          <Button
            variant="secondary"
            size="sm"
            class="mt-2"
            leadingIcon={<Icon name="plus" size={16} />}
            disabled={redirects.length >= MAX}
            onClick={() => setRedirects((prev) => [...prev, { from: '', to: '', status: 308 }])}
          >
            {t('rules.addRedirect')}
          </Button>
        </section>

        <section aria-labelledby="rules-headers">
          <h3 id="rules-headers" class="text-text mb-2 text-sm font-semibold">
            {t('rules.headers')}
          </h3>
          <ul class="flex flex-col gap-2">
            {headers.map((h, i) => (
              <li key={`h${i}`} class="flex flex-wrap items-end gap-2">
                <div class="min-w-[12rem] flex-1">
                  <label class="text-text-muted mb-1 block text-xs" for={`hdr-name-${i}`}>
                    {t('rules.header.name')}
                  </label>
                  <Input
                    id={`hdr-name-${i}`}
                    value={h.name}
                    placeholder="X-Frame-Options"
                    class="font-mono"
                    onInput={(e) =>
                      setHeaders((prev) =>
                        prev.map((x, j) => (j === i ? { ...x, name: e.currentTarget.value } : x)),
                      )
                    }
                  />
                </div>
                <div class="min-w-[12rem] flex-1">
                  <label class="text-text-muted mb-1 block text-xs" for={`hdr-value-${i}`}>
                    {t('rules.header.value')}
                  </label>
                  <Input
                    id={`hdr-value-${i}`}
                    value={h.value}
                    placeholder="DENY"
                    class="font-mono"
                    onInput={(e) =>
                      setHeaders((prev) =>
                        prev.map((x, j) => (j === i ? { ...x, value: e.currentTarget.value } : x)),
                      )
                    }
                  />
                </div>
                <Button
                  variant="ghost"
                  class="text-danger-text"
                  aria-label={t('rules.remove')}
                  onClick={() => setHeaders((prev) => prev.filter((_, j) => j !== i))}
                >
                  {t('common.delete')}
                </Button>
              </li>
            ))}
          </ul>
          <Button
            variant="secondary"
            size="sm"
            class="mt-2"
            leadingIcon={<Icon name="plus" size={16} />}
            disabled={headers.length >= MAX}
            onClick={() => setHeaders((prev) => [...prev, { name: '', value: '' }])}
          >
            {t('rules.addHeader')}
          </Button>
        </section>

        <div class="flex items-center gap-3">
          <Button variant="primary" loading={busy} disabled={!live} onClick={() => void save()}>
            {t('common.save')}
          </Button>
          <span class="text-text-subtle text-sm">{t('rules.applyNote')}</span>
        </div>
      </CardBody>
    </Card>
  );
}
