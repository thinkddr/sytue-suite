/**
 * VersionList — las filas del historial de versiones de un fichero: número, la marca
 * de «actual», fecha, autor y tamaño, y a la derecha las acciones de quien la monta.
 *
 * Lo enseñan el explorador de Drive y CertiPDF sobre el MISMO historial (el de Drive,
 * que guarda cada escritura como una versión). Con dos copias de la fila, una acababa
 * con la fecha en otro formato o perdiendo el autor en el teléfono, que es lo que ya
 * le pasó a la de Drive antes de partirla en dos líneas.
 *
 * Los rótulos llegan por props (los catálogos son de cada app) y las acciones también:
 * Drive descarga y confirma en la fila, CertiPDF abre la versión en el visor del
 * navegador. Lo común es la fila, no lo que se hace con ella.
 *
 *   <VersionList
 *     versions={versiones}
 *     versionLabel={(v) => t('versions.version', { version: v.version })}
 *     currentLabel={t('versions.current')}
 *     actions={(v) => <Button …>Restaurar</Button>}
 *   />
 */
import type { ComponentChildren, JSX } from 'preact';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { Icon } from './Icon.js';
import { formatBytes } from '../lib/formatBytes.js';
import { cx } from '../lib/cx.js';

/** Lo que la fila necesita de una versión (lo cumple `FileVersion` de api-clients). */
export interface VersionListItem {
  version: number;
  isCurrent: boolean;
  sizeBytes: number;
  author?: string;
  createdAt?: string;
}

export interface VersionListProps<V extends VersionListItem> {
  versions: V[];
  /** «Versión 3». Una frase entera del catálogo: el número no va siempre detrás. */
  versionLabel: (v: V) => string;
  /** La marca de la versión vigente («Actual»). */
  currentLabel: string;
  /** Botones de la fila, a la derecha. */
  actions?: (v: V) => ComponentChildren;
  /** Lo que se despliega bajo la fila (p. ej. una confirmación en línea). */
  below?: (v: V) => ComponentChildren;
  /** El idioma de las fechas y los tamaños; por defecto, el del contexto de la suite. */
  locale?: Locale;
  class?: string;
}

/** Fecha y hora de una versión en el formato del idioma (cambian el orden y el reloj). */
export function formatVersionDate(locale: Locale, iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return formatDateTime(locale, d, {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function VersionList<V extends VersionListItem>({
  versions,
  versionLabel,
  currentLabel,
  actions,
  below,
  locale: idioma,
  class: cls,
}: VersionListProps<V>): JSX.Element {
  const contexto = useI18n();
  const locale = idioma ?? contexto.locale;
  return (
    <ul class={cx('divide-border/60 border-border divide-y rounded-md border', cls)}>
      {versions.map((v) => (
        <li key={v.version} class="px-3 py-2.5" aria-current={v.isCurrent ? 'true' : undefined}>
          <div class="flex items-center gap-3">
            <span
              class="bg-bg-subtle text-text-muted grid h-9 w-9 shrink-0 place-items-center rounded-md"
              aria-hidden="true"
            >
              <Icon name="clock" size={16} />
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-2">
                <span class="text-text truncate font-medium">{versionLabel(v)}</span>
                {v.isCurrent ? (
                  <span class="bg-selected text-primary-text rounded-full px-2 py-0.5 text-xs font-semibold">
                    {currentLabel}
                  </span>
                ) : null}
              </span>
              {/* En el teléfono, la fecha y debajo autor y tamaño: en una línea truncada a
                  137 px se perdían los dos. */}
              <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                <span class="max-md:block max-md:truncate">
                  {formatVersionDate(locale, v.createdAt) || '—'}
                </span>
                <span class="max-md:block max-md:truncate">
                  <span class="max-md:hidden"> · </span>
                  {v.author ? `${v.author} · ` : ''}
                  {formatBytes(locale, v.sizeBytes)}
                </span>
              </span>
            </span>
            {actions ? <span class="flex shrink-0 items-center gap-1">{actions(v)}</span> : null}
          </div>
          {below?.(v)}
        </li>
      ))}
    </ul>
  );
}
