/**
 * DraftsPanel — panel lateral (RHS) de BORRADORES: lo escrito y no enviado, en
 * cualquier canal o hilo. Es la puerta que faltaba a algo que antes solo existía si
 * te acordabas de volver al canal donde lo dejaste (y solo en ESE dispositivo).
 *
 * Los datos vienen del almacenamiento local (lib/drafts.ts), que a su vez se
 * sincroniza con el servidor: por eso el panel no tiene estado de carga ni de error
 * —no hay petición que esperar— y el contador del sidebar reacciona a la misma
 * pulsación que escribe el texto.
 *
 * Mismo patrón visual que ScheduledPanel: cabecera sólida + cuerpo desplazable, y la
 * fila es un BOTÓN que lleva al sitio donde se estaba redactando; «Descartar» vive
 * fuera de ese botón, nunca anidado dentro.
 */
import type { JSX } from 'preact';
import { EmptyState, Illustration, Icon, IconButton } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { LocalDraft } from '../lib/drafts.js';
import { TrashGlyph } from './glyphs.js';

interface Props {
  drafts: LocalDraft[];
  /** channelId -> nombre de canal, para decir DÓNDE se estaba escribiendo. */
  channelNames?: Record<string, string>;
  /** Abre el canal (y el hilo, si el borrador es de uno) con el texto pendiente. */
  onSelect: (d: LocalDraft) => void;
  onDiscard: (d: LocalDraft) => void;
  onClose: () => void;
  t: T;
}

export function DraftsPanel({
  drafts,
  channelNames,
  onSelect,
  onDiscard,
  onClose,
  t,
}: Props): JSX.Element {
  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('drafts.aria')}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <h2 class="text-md text-header-fg leading-tight font-semibold">{t('drafts.title')}</h2>
        <IconButton
          size="lg"
          tone="header"
          label={t('action.closeNamed', { name: t('drafts.title') })}
          showTitle={false}
          title={t('action.close')}
          onClick={onClose}
        >
          <Icon name="close" size={18} />
        </IconButton>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto py-2">
        {drafts.length === 0 ? (
          <EmptyState
            class="py-4"
            title={t('drafts.empty')}
            illustration={<Illustration name="emptyChat" size={96} />}
          />
        ) : (
          drafts.map((d) => {
            const nombre = channelNames?.[d.channelId];
            return (
              <div
                key={`${d.channelId}:${d.parentId ?? ''}`}
                class="border-border flex items-start gap-2 border-b px-2 py-1 last:border-b-0"
              >
                <button
                  type="button"
                  onClick={() => onSelect(d)}
                  class="hover:bg-bg-subtle sytue-focus-ring rounded-control min-w-0 flex-1 px-2 py-2 text-left"
                >
                  <div class="flex min-w-0 flex-wrap items-baseline gap-x-2">
                    <span class="text-text truncate text-sm font-semibold">
                      {nombre ? `#${nombre}` : t('drafts.unknownChannel')}
                    </span>
                    {d.parentId ? (
                      <span class="sytue-meta shrink-0">{t('drafts.inThread')}</span>
                    ) : null}
                  </div>
                  {/* Sin MessageBody: un borrador es TEXTO a medias, y renderizarlo
                      como mensaje pintaría menciones y emoji a medio escribir. */}
                  <p class="text-text-muted mt-0.5 line-clamp-2 text-sm break-words whitespace-pre-wrap">
                    {d.body}
                  </p>
                </button>
                <IconButton
                  size="sm"
                  label={t('drafts.discard')}
                  showTitle={false}
                  title={t('drafts.discard')}
                  class="mt-2"
                  onClick={() => onDiscard(d)}
                >
                  <TrashGlyph size={14} />
                </IconButton>
              </div>
            );
          })
        )}
      </div>
    </aside>
  );
}
