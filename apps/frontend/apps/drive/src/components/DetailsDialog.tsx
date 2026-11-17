/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DetailsDialog — el «Detalles» de Google Drive o el «Obtener información» del Finder
 * para un fichero o una carpeta de Sytue Drive: qué es, dónde está, de quién es,
 * cuándo y quién lo cambió, en qué versión va, cómo está clasificado y cuánta gente
 * más entra. Hasta ahora nada de eso se podía ver sin abrir tres diálogos distintos,
 * y parte (el propietario, la fecha de creación) no se veía en ninguno.
 *
 * Panel lateral (Modal `placement="right"`): se consulta junto a la lista, no la tapa.
 * Lo reúne el BFF `/api/details` en una sola petición; mientras llega se pinta el
 * esqueleto de la propia lista, y si falla, el aviso con «Reintentar».
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, Modal, Skeleton, formatBytes, formatVersionDate, withBase } from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import type { DriveResource } from '@sytue/api-clients';
import { useT, type T } from '../i18n';
import { Glyph } from './glyphs';
import { carpetaPadre } from '../lib/ubicacion';
import { extensionOf } from '../lib/wopi';
import type { Detalles, PersonaDetalle } from '../lib/detalles';

/** URL del BFF para `item`; `null` si no tiene id (no hay a quién preguntar). */
export function urlDeDetalles(item: DriveResource): string | null {
  if (!item.id) return null;
  const kind = item.isDirectory ? 'folder' : 'file';
  return withBase(
    import.meta.env.BASE_URL,
    `/api/details?id=${encodeURIComponent(item.id)}&kind=${kind}`,
  );
}

/** «Mi unidad › Proyectos › 2026»: la carpeta que contiene `path`, legible. */
export function ubicacionLegible(t: T, path: string): string {
  const carpeta = carpetaPadre(path);
  const partes = carpeta
    .split('/')
    .filter(Boolean)
    .map((p) => {
      try {
        return decodeURIComponent(p);
      } catch {
        return p;
      }
    });
  return [t('nav.myDrive'), ...partes].join(' › ');
}

/** Nombre, si no el correo, si no «Alguien»: nunca el `sub`. */
export function nombreDe(t: T, p: PersonaDetalle): string {
  return p.name || p.email || t('details.someone');
}

function tipoDe(t: T, item: DriveResource, d: Detalles | null): string {
  if (item.isDirectory) return t('details.folder');
  const ext = extensionOf(item.name);
  if (ext) return t('details.fileOfType', { ext: ext.toUpperCase() });
  return d?.contentType || t('details.file');
}

export function DetailsDialog({
  item,
  onClose,
  onVersions,
  onShare,
}: {
  item: DriveResource;
  onClose: () => void;
  /** Abre el historial; ausente = el panel no lo ofrece (carpetas, sin conexión). */
  onVersions?: ((item: DriveResource) => void) | undefined;
  /** Abre «Compartir»; ausente = el panel no lo ofrece. */
  onShare?: ((item: DriveResource) => void) | undefined;
}) {
  const { locale } = useI18n();
  const t = useT(locale);
  const [details, setDetails] = useState<Detalles | null>(null);
  const [error, setError] = useState(false);
  // Cada carga lleva su número: un «Reintentar» pulsado dos veces no debe pintar la
  // respuesta vieja encima de la nueva.
  const [intento, setIntento] = useState(0);

  useEffect(() => {
    const url = urlDeDetalles(item);
    if (!url) {
      setError(true);
      return;
    }
    const controller = new AbortController();
    setDetails(null);
    setError(false);
    void (async () => {
      try {
        const res = await fetch(url, { credentials: 'include', signal: controller.signal });
        if (!res.ok) throw new Error(String(res.status));
        const body = (await res.json()) as { details?: Detalles };
        if (!body.details) throw new Error('sin detalles');
        if (!controller.signal.aborted) setDetails(body.details);
      } catch {
        if (!controller.signal.aborted) setError(true);
      }
    })();
    return () => controller.abort();
  }, [item, intento]);

  const reintentar = useCallback(() => setIntento((n) => n + 1), []);

  const filas: [string, string][] = [];
  if (details) {
    const vacio = t('details.unavailable');
    filas.push([t('details.type'), tipoDe(t, item, details)]);
    if (!item.isDirectory) {
      const size = details.size ?? item.size;
      filas.push([t('details.size'), size === undefined ? vacio : formatBytes(locale, size)]);
    }
    filas.push([t('details.location'), ubicacionLegible(t, details.path ?? item.path)]);
    filas.push([t('details.owner'), details.owner ? nombreDe(t, details.owner) : vacio]);
    filas.push([t('details.created'), formatVersionDate(locale, details.createdAt) || vacio]);
    const modificado = formatVersionDate(locale, details.modifiedAt ?? item.modifiedAt);
    filas.push([
      t('details.modified'),
      modificado && details.modifiedBy
        ? t('details.modifiedBy', { date: modificado, name: nombreDe(t, details.modifiedBy) })
        : modificado || vacio,
    ]);
    if (!item.isDirectory) {
      filas.push([
        t('details.version'),
        details.version !== undefined ? String(details.version) : vacio,
      ]);
      filas.push([
        t('details.label'),
        details.label
          ? details.label.blockExternalShare
            ? t('details.labelBlocks', { name: details.label.name })
            : details.label.name
          : t('details.noLabel'),
      ]);
    }
    const a = details.access;
    filas.push([t('details.links'), a ? String(a.links) : vacio]);
    filas.push([t('details.people'), a ? String(a.people) : vacio]);
    filas.push([t('details.groups'), a ? String(a.groups) : vacio]);
  }

  const puedeVersiones = !!onVersions && !item.isDirectory && !!item.id;

  return (
    <Modal
      open
      onClose={onClose}
      placement="right"
      size="md"
      title={t('details.title', { name: item.name })}
      // Las acciones van sueltas: el pie del Modal ya las reparte (y las apila en el
      // teléfono); un envoltorio propio se saltaba ese reparto (pies-al-comun.test.ts).
      footer={
        <>
          {puedeVersiones ? (
            <Button
              variant="secondary"
              leadingIcon={<Glyph name="clock" size={16} />}
              onClick={() => onVersions?.(item)}
            >
              {t('details.versions')}
            </Button>
          ) : null}
          {onShare ? (
            <Button
              variant="secondary"
              leadingIcon={<Glyph name="link" size={16} />}
              onClick={() => onShare(item)}
            >
              {t('details.share')}
            </Button>
          ) : null}
          <Button onClick={onClose}>{t('details.close')}</Button>
        </>
      }
    >
      {error ? (
        <div class="grid min-h-40 place-items-center p-4 text-center" role="alert">
          <div>
            <p class="text-text-muted text-base">{t('details.loadError')}</p>
            <Button variant="secondary" size="sm" class="mt-2" onClick={reintentar}>
              {t('details.retry')}
            </Button>
          </div>
        </div>
      ) : details === null ? (
        <div aria-busy="true" data-detalles-esqueleto="">
          <span class="sr-only" role="status">
            {t('details.loading')}
          </span>
          <div class="flex flex-col gap-3">
            {Array.from({ length: 8 }, (_, i) => (
              <div key={i} class="flex items-center gap-4">
                <Skeleton width="35%" height="0.875rem" />
                <Skeleton width="45%" height="0.875rem" />
              </div>
            ))}
          </div>
        </div>
      ) : (
        <dl class="divide-border grid grid-cols-[minmax(8rem,auto)_1fr] divide-y text-sm tabular-nums">
          {filas.map(([dt, dd]) => (
            <div key={dt} class="col-span-2 grid min-w-0 grid-cols-subgrid gap-x-4 py-2">
              <dt class="text-text-muted">{dt}</dt>
              <dd class="text-text min-w-0 break-words">{dd}</dd>
            </div>
          ))}
        </dl>
      )}
    </Modal>
  );
}
