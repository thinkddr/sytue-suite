/**
 * BatchActionBar — barra de acciones en lote para la multiselección de la lista
 * (patrón Gmail). Sobria: iconos densos, sin relleno de color,
 * con etiquetas accesibles. Anuncia el conteo por aria-live (no roba foco). Las
 * acciones (leído/no leído, archivar, papelera, spam, etiquetar) operan sobre TODA
 * la selección en una sola llamada en lote del backend (id = ANY). docs/design-system §3.17.
 */
import type { JSX } from 'preact';
import {
  Button,
  Dropdown,
  MASCARA_SCROLL_X,
  MenuItem,
  Icon,
  Toolbar,
  cx,
  useScrollRestante,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import type { Label } from '../client/mailApi.js';
import { labelDot } from './utils.js';
import { Glyph } from './glyphs.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  count: number;
  onRead: () => void;
  onUnread: () => void;
  /** null oculta la acción (no hay carpeta destino o ya estamos en ella). */
  onArchive: (() => void) | null;
  onTrash: (() => void) | null;
  onSpam: (() => void) | null;
  /** Borrado definitivo (solo en papelera/spam): pide confirmación al padre. */
  onPurge: (() => void) | null;
  onClear: () => void;
  labels: Label[];
  /** Añade la etiqueta a toda la selección. */
  onLabel: (labelId: string) => void;
}

/** Botón de acción de la barra: icon-only, denso, con etiqueta accesible. */
function BarAction({
  label,
  onAct,
  children,
}: {
  label: string;
  onAct: () => void;
  children: JSX.Element;
}): JSX.Element {
  return (
    <Button
      size="sm"
      variant="ghost"
      iconOnly
      aria-label={label}
      title={label}
      leadingIcon={children}
      onClick={onAct}
    />
  );
}

export function BatchActionBar({
  locale,
  count,
  onRead,
  onUnread,
  onArchive,
  onTrash,
  onSpam,
  onPurge,
  onClear,
  labels,
  onLabel,
}: Props): JSX.Element {
  const t = useT(locale);
  // NUNCA envuelve y nada se encoge: con nueve acciones y el recuento, en un
  // teléfono la fila entera se desplaza de lado (ver scrollX.ts) — sin scroll,
  // el botón que no cabía simplemente desaparecía.
  const barra = useScrollRestante<HTMLDivElement>();
  return (
    <Toolbar
      aria-label={t('batch.toolbar')}
      innerRef={barra.ref}
      data-scroll-x
      class={cx(
        'flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto [&>*]:shrink-0',
        barra.queda && MASCARA_SCROLL_X,
      )}
    >
      <span class="text-text mr-1 text-sm font-medium tabular-nums" aria-live="polite">
        {t('batch.selected', { count })}
      </span>
      <span class="bg-border mx-1 h-5 w-px shrink-0" aria-hidden="true" />
      <BarAction label={t('action.markRead')} onAct={onRead}>
        <Glyph name="read" />
      </BarAction>
      <BarAction label={t('action.markUnread')} onAct={onUnread}>
        <Glyph name="unread" />
      </BarAction>
      {onArchive ? (
        <BarAction label={t('action.archive')} onAct={onArchive}>
          <Glyph name="archive" />
        </BarAction>
      ) : null}
      {onTrash ? (
        <BarAction label={t('action.trash')} onAct={onTrash}>
          <Glyph name="trash" />
        </BarAction>
      ) : null}
      {onSpam ? (
        <BarAction label={t('action.spam')} onAct={onSpam}>
          <Glyph name="spam" />
        </BarAction>
      ) : null}
      {/* En la papelera «mover a la papelera» no significa nada: lo que hace
          falta ahí es sacarlo de en medio de verdad, que además es lo único que
          devuelve espacio. La confirmación la pide el padre. */}
      {onPurge ? (
        <BarAction label={t('action.purge')} onAct={onPurge}>
          <Glyph name="trash" />
        </BarAction>
      ) : null}
      {labels.length > 0 ? (
        <Dropdown
          label={t('batch.labelSelection')}
          align="end"
          trigger={(p) => (
            <Button
              size="sm"
              variant="ghost"
              iconOnly
              aria-label={t('batch.labelSelection')}
              title={t('action.label')}
              leadingIcon={<Glyph name="tag" />}
              {...p}
            />
          )}
        >
          <div class="max-h-72 min-w-[220px] overflow-y-auto">
            {labels.map((l) => (
              <MenuItem key={l.id} onClick={() => onLabel(l.id)}>
                <span class="flex flex-1 items-center gap-2">
                  <span
                    class="h-2.5 w-2.5 shrink-0 rounded-full"
                    style={{ background: labelDot(l.color) }}
                    aria-hidden="true"
                  />
                  <span class="flex-1 truncate">{l.name}</span>
                </span>
              </MenuItem>
            ))}
          </div>
        </Dropdown>
      ) : null}
      <span class="flex-1" />
      <Button
        size="sm"
        variant="ghost"
        iconOnly
        aria-label={t('batch.clear')}
        title={t('batch.clear')}
        leadingIcon={<Icon name="close" size={16} />}
        onClick={onClear}
      />
    </Toolbar>
  );
}
