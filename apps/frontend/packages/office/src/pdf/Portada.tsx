/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Portada de Sytue CertiPDF: lo mismo que tienen Docus, Calcus y Diapos en la
 * suya —crear en blanco, abrir un fichero, los recientes con filtro— y además
 * la zona de arrastrar y soltar (o escanear) que ya era la puerta de CertiPDF,
 * porque un PDF casi siempre viene de fuera. Los recientes salen de /PDF del
 * Drive (api/pdf-recientes.ts): un PDF no es un documento de Docs.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Badge, EmptyState, Input, Spinner, cx } from '@sytue/ui';
import { contiene, formatDate, type Locale } from '@sytue/i18n';
import { productColor } from '@sytue/config';
import { docsT } from '../i18n';
import { r } from '../lib/rutas';
import { AbrirPdf } from './Abrir';
import { crearPdfEnBlanco } from './crear';
import type { PdfReciente } from '../pages/api/pdf-recientes';
import { PdfGlyph } from '../components/DocList';
import {
  firmas,
  nombreDePeticion,
  quienFirma,
  type PeticionFirma,
  type PeticionesFirma,
} from './api';
import { mensajeDeError } from './errores';

/**
 * Las peticiones de firma, arriba del todo cuando las hay: las que me hacen
 * (pendientes) y cómo van las que he hecho. Sin ninguna no ocupa sitio.
 */
function PeticionesDeFirma({ t, locale }: { t: ReturnType<typeof docsT>; locale: Locale }) {
  const [ps, setPs] = useState<PeticionesFirma | null>(null);
  useEffect(() => {
    firmas
      .listar()
      .then(setPs)
      .catch(() => setPs(null));
  }, []);
  const [error, setError] = useState('');
  // Un paso a la vez: dos «abrir» seguidos dejarían dos copias en la unidad.
  const [ocupado, setOcupado] = useState(false);
  const recibidas = (ps?.received ?? []).filter((p) => p.status === 'pendiente');
  const pedidas = ps?.sent ?? [];
  const bloqueados = ps?.blocked ?? [];
  if (recibidas.length === 0 && pedidas.length === 0 && bloqueados.length === 0) return null;
  // Los ajustes de quien recibe (ADR 0059): a quién ha bloqueado, y quitarlo.
  const desbloquear = (sub: string) => {
    if (ocupado) return;
    setOcupado(true);
    setError('');
    void firmas
      .desbloquear(sub)
      .then(() =>
        setPs((x) => (x ? { ...x, blocked: (x.blocked ?? []).filter((b) => b.sub !== sub) } : x)),
      )
      .catch((e: unknown) => setError(mensajeDeError(e, t, 'pdf.error.apply')))
      .finally(() => setOcupado(false));
  };
  const abrirRuta = (ruta: string | undefined) => {
    if (ruta) window.location.assign(r(`/pdf?path=${encodeURIComponent(ruta)}`));
  };
  // De otra organización (ADR 0057) el PDF aún no está en mi unidad: se trae y se abre.
  const traerYAbrir = (
    paso: () => Promise<PeticionFirma>,
    ruta: (p: PeticionFirma) => string | undefined,
  ) => {
    if (ocupado) return;
    setOcupado(true);
    setError('');
    void paso()
      .then((x) => abrirRuta(ruta(x)))
      .catch((e: unknown) => setError(mensajeDeError(e, t, 'pdf.error.apply')))
      .finally(() => setOcupado(false));
  };
  const fila = (
    destino: string | (() => void),
    nombre: string,
    quien: string,
    extra: string,
    key: string,
  ) => {
    const clase =
      'border-border bg-surface hover:border-border-strong sytue-focus-ring-inset flex w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left text-sm';
    const dentro = (
      <>
        <span class="text-text min-w-0 truncate font-medium">{nombre.replace(/\.pdf$/i, '')}</span>
        <span class="text-text-muted shrink-0 text-xs">
          {quien}
          {extra ? ` · ${extra}` : ''}
        </span>
      </>
    );
    return (
      <li key={key}>
        {typeof destino === 'string' ? (
          <a href={destino} class={clase}>
            {dentro}
          </a>
        ) : (
          <button
            type="button"
            class={clase}
            onClick={destino}
            disabled={ocupado}
            aria-busy={ocupado}
          >
            {dentro}
          </button>
        )}
      </li>
    );
  };
  return (
    <section class="mb-10 grid shrink-0 grid-cols-1 gap-6 lg:grid-cols-2">
      {error && (
        <p role="alert" class="text-danger-text min-w-0 text-xs lg:col-span-2">
          {error}
        </p>
      )}
      {recibidas.length > 0 && (
        <div>
          <h2 class="sytue-overline text-text-subtle mb-3">{t('pdf.home.requests.received')}</h2>
          <ul class="flex flex-col gap-2">
            {recibidas.map((p) =>
              fila(
                p.path
                  ? r(`/pdf?path=${encodeURIComponent(p.path)}`)
                  : () =>
                      traerYAbrir(
                        () => firmas.abrir(p.id),
                        (x) => x.path,
                      ),
                nombreDePeticion(p),
                quienFirma(p, 'received'),
                p.external && !p.path
                  ? `${formatDate(locale, p.created_at)} · ${t('pdf.home.requests.external')}`
                  : formatDate(locale, p.created_at),
                p.id,
              ),
            )}
          </ul>
        </div>
      )}
      {pedidas.length > 0 && (
        <div>
          <h2 class="sytue-overline text-text-subtle mb-3">{t('pdf.home.requests.sent')}</h2>
          <ul class="flex flex-col gap-2">
            {pedidas.slice(0, 10).map((p) =>
              fila(
                p.external && p.status === 'firmada' && !p.signed_path
                  ? () =>
                      traerYAbrir(
                        () => firmas.traer(p.id),
                        (x) => x.signed_path,
                      )
                  : r(`/pdf?path=${encodeURIComponent(p.signed_path ?? p.path)}`),
                nombreDePeticion(p),
                quienFirma(p, 'sent'),
                t(`pdf.signatureRequest.status.${p.status}`),
                p.id,
              ),
            )}
          </ul>
        </div>
      )}
      {bloqueados.length > 0 && (
        <div class="min-w-0">
          <h2 class="sytue-overline text-text-subtle mb-1">{t('pdf.home.blocked.title')}</h2>
          <p class="text-text-muted mb-3 text-xs text-pretty">{t('pdf.home.blocked.help')}</p>
          <ul class="flex flex-col gap-2">
            {bloqueados.map((b) => (
              <li
                key={b.sub}
                class="border-border bg-surface flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm"
              >
                <span class="text-text min-w-0 truncate">{b.name || b.sub}</span>
                <button
                  type="button"
                  class="sytue-focus-ring shrink-0 rounded text-xs underline"
                  disabled={ocupado}
                  onClick={() => desbloquear(b.sub)}
                >
                  {t('pdf.home.blocked.unblock')}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

export function PortadaPdf({ locale }: { locale: Locale }) {
  const t = docsT(locale);
  const [lista, setLista] = useState<PdfReciente[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const [query, setQuery] = useState('');
  const [creando, setCreando] = useState(false);
  const [errorCrear, setErrorCrear] = useState(false);

  useEffect(() => {
    fetch('/api/pdf-recientes', { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error(String(res.status)))))
      .then((d: { items: PdfReciente[] }) => setLista(d.items))
      .catch(() => setFallo(true));
  }, []);

  const filtrados = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (lista ?? []).filter((p) => !q || contiene(p.name, q));
  }, [lista, query]);

  const crear = async () => {
    if (creando) return;
    setCreando(true);
    setErrorCrear(false);
    try {
      const path = await crearPdfEnBlanco(t('pdf.new.filename'));
      window.location.assign(r(`/pdf?path=${encodeURIComponent(path)}`));
    } catch {
      setErrorCrear(true);
      setCreando(false);
    }
  };

  return (
    <div
      data-suelo-tactil
      class="mx-auto flex h-full min-h-0 w-full max-w-[var(--container-content,1280px)] flex-col overflow-y-auto px-6 pt-8 pb-10"
    >
      <header class="mb-8 shrink-0">
        <h1 class="sytue-h2 text-text">{t('pdf.list.heading')}</h1>
        <p class="text-text-muted mt-1 max-w-2xl text-base">{t('pdf.list.tagline')}</p>
      </header>

      {/* Crear nuevo + abrir: la tarjeta de «PDF en blanco» como en las otras
          portadas, y al lado la zona de soltar/elegir/escanear de siempre. */}
      <section aria-labelledby="pdf-new-heading" class="mb-10 shrink-0">
        <h2 id="pdf-new-heading" class="sytue-overline text-text-subtle mb-3">
          {t('list.new.heading')}
        </h2>
        <div class="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
          <button
            type="button"
            onClick={() => void crear()}
            disabled={creando}
            aria-busy={creando}
            class="border-border bg-surface hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring-inset flex items-center gap-3 self-start rounded-lg border p-4 text-left transition-colors duration-150 disabled:opacity-60"
          >
            <span
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-white"
              style={{ background: productColor.pdf }}
              aria-hidden="true"
            >
              {creando ? <Spinner size={18} /> : <PdfGlyph size={22} />}
            </span>
            <span class="flex min-w-0 flex-col">
              <span class="text-text truncate font-semibold">{t('kind.pdf')}</span>
              <span class="text-text-muted truncate text-sm">
                {creando ? t('list.new.creating') : t('list.new.blank')}
              </span>
            </span>
          </button>
          <AbrirPdf locale={locale} embebido />
        </div>
        {errorCrear ? (
          <p class="text-danger-text mt-2 text-sm" role="alert">
            {t('create.error.description')}
          </p>
        ) : null}
      </section>

      <PeticionesDeFirma t={t} locale={locale} />

      {/* Recientes: la lista de /PDF del Drive, con filtro por nombre. */}
      <section aria-labelledby="pdf-recent-heading" class="flex min-h-0 flex-col">
        <div class="mb-3 flex shrink-0 items-center gap-2">
          <h2 id="pdf-recent-heading" class="sytue-overline text-text-subtle">
            {t('list.recent.heading')}
          </h2>
          {lista && lista.length > 0 ? (
            <Badge tone="neutral" aria-label={t('list.count', { count: lista.length })}>
              {lista.length}
            </Badge>
          ) : null}
          <div class="ml-auto w-64 max-w-[60%]">
            <Input
              type="search"
              value={query}
              onInput={(e) => setQuery(e.currentTarget.value)}
              placeholder={t('list.filter.placeholder')}
              aria-label={t('list.filter.label')}
            />
          </div>
        </div>
        {fallo ? (
          <p class="text-danger-text text-sm" role="alert">
            {t('pdf.list.loadFailed')}
          </p>
        ) : lista === null ? (
          <div class="flex justify-center py-10" aria-busy="true">
            <Spinner size={22} />
          </div>
        ) : lista.length === 0 ? (
          <EmptyState title={t('pdf.list.heading')} description={t('pdf.list.empty')} />
        ) : filtrados.length === 0 ? (
          <EmptyState
            title={t('list.empty.search.title', { query })}
            description={t('list.empty.search.description')}
          />
        ) : (
          <ul class="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-4">
            {filtrados.map((p) => (
              <li key={p.path}>
                <a
                  href={r(`/pdf?path=${encodeURIComponent(p.path)}`)}
                  class={cx(
                    'border-border bg-surface hover:border-border-strong sytue-focus-ring-inset flex h-full flex-col overflow-hidden rounded-lg border transition-colors duration-150',
                  )}
                >
                  <span
                    class="bg-bg-subtle flex h-24 items-center justify-center"
                    style={{ color: productColor.pdf }}
                    aria-hidden="true"
                  >
                    <PdfGlyph size={32} />
                  </span>
                  <span class="flex min-w-0 flex-col gap-0.5 p-3">
                    <span class="text-text truncate font-semibold" title={p.name}>
                      {p.name.replace(/\.pdf$/i, '')}
                    </span>
                    <span class="text-text-muted truncate text-sm">
                      {t('kind.pdf')}
                      {p.modifiedAt ? ` · ${formatDate(locale, p.modifiedAt)}` : ''}
                    </span>
                  </span>
                </a>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
