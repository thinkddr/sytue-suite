/**
 * AccionesDeMensaje — piezas de interacción con un mensaje que comparten el
 * histórico del canal (`MessageView`) y el panel de hilo (`ThreadPanel`): las
 * reacciones rápidas de la barra flotante, la fila de chips de reacción, las
 * entradas del menú «…» y la caja de edición inline.
 *
 * Viven aparte para que una respuesta de hilo se maneje EXACTAMENTE igual que un
 * mensaje del canal (mismo menú, mismas etiquetas accesibles, mismo orden): antes el
 * hilo solo pintaba el cuerpo y no se podía reaccionar, editar ni borrar desde él.
 * Cada vista sigue siendo dueña de su estado (qué se edita, qué se confirma borrar);
 * aquí solo está lo que se pinta y qué entrada llama a qué.
 */
import type { JSX } from 'preact';
import { Button, Icon, cx, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { messageLink, type ChatMessage } from '../client/chatApi.js';
import { appUrl, orgActual } from '@sytue/config';
import type { T } from '../i18n/index.js';
import { copyToClipboard } from '../lib/clipboard.js';
import { reactedByLabel } from '../lib/reactions.js';
import {
  PinGlyph,
  BookmarkGlyph,
  CopyGlyph,
  LinkGlyph,
  UnreadGlyph,
  ForwardGlyph,
  EditGlyph,
  TrashGlyph,
  ThreadsGlyph,
  SeenGlyph,
  NoteGlyph,
} from './glyphs.js';
import type { MenuEntry } from './MessageMenu.js';
import { EmojiToken } from './MessageBody.js';
import { EmojiPickerButton } from './EmojiPicker.js';

/** Reacciones rápidas de la barra de acciones (paridad Mattermost). */
export const QUICK_REACTIONS = ['👍', '❤️', '😄', '🎉'] as const;

/** Copia un texto al portapapeles y avisa con un toast del resultado. */
export function useCopiarConAviso(t: T): (texto: string, okTitle: string) => void {
  const toast = useToast();
  return (texto, okTitle) => {
    void copyToClipboard(texto).then((ok) =>
      toast.notify(
        ok
          ? { tone: 'success', title: okTitle }
          : { tone: 'danger', title: t('message.copyFailed') },
      ),
    );
  };
}

/** Qué sabe la vista del mensaje y qué acciones ofrece. Una acción ausente no sale
 *  en el menú: el hilo, p. ej., no ofrece «Marcar como no leído» (el no leído es
 *  del canal, no de una respuesta). */
export interface ContextoDelMenu {
  t: T;
  copiar: (texto: string, okTitle: string) => void;
  pinned?: boolean | undefined;
  saved?: boolean | undefined;
  following?: boolean | undefined;
  blocked?: boolean | undefined;
  onTogglePin?: ((m: ChatMessage, pinned: boolean) => void) | undefined;
  onToggleSave?: ((m: ChatMessage, saved: boolean) => void) | undefined;
  onToggleFollow?: ((m: ChatMessage, following: boolean) => void) | undefined;
  onForward?: ((m: ChatMessage) => void) | undefined;
  onMarkUnread?: ((m: ChatMessage) => void) | undefined;
  /** Abre «Visto por» (solo se ofrece en los mensajes propios). */
  onReaders?: ((m: ChatMessage) => void) | undefined;
  /** Abre la edición inline del mensaje propio. */
  onEdit?: ((m: ChatMessage) => void) | undefined;
  /** Pide confirmar el borrado del mensaje propio. */
  onDelete?: ((m: ChatMessage) => void) | undefined;
  onReport?: ((m: ChatMessage) => void) | undefined;
  onToggleBlock?: ((m: ChatMessage, blocked: boolean) => void) | undefined;
}

/** Construye las entradas del menú «…» de un mensaje (según estado y permisos). */
export function entradasDelMenu(m: ChatMessage, c: ContextoDelMenu): MenuEntry[] {
  const { t } = c;
  const items: MenuEntry[] = [];
  if (c.onTogglePin) {
    const pinned = c.pinned ?? false;
    const onTogglePin = c.onTogglePin;
    items.push({
      key: 'pin',
      label: pinned ? t('message.unpin') : t('message.pin'),
      icon: <PinGlyph size={16} />,
      onSelect: () => onTogglePin(m, pinned),
    });
  }
  if (c.onToggleSave) {
    const saved = c.saved ?? false;
    const onToggleSave = c.onToggleSave;
    items.push({
      key: 'save',
      label: saved ? t('message.unsave') : t('action.save'),
      icon: <BookmarkGlyph size={16} />,
      onSelect: () => onToggleSave(m, saved),
    });
  }
  // «Seguir mensaje» (paridad Mattermost): solo en mensajes RAÍZ — el servicio
  // rechaza seguir una respuesta (exige parent_id IS NULL).
  if (c.onToggleFollow && !m.parentId) {
    const following = c.following ?? false;
    const onToggleFollow = c.onToggleFollow;
    items.push({
      key: 'follow',
      label: following ? t('thread.unfollowShort') : t('message.follow'),
      icon: <ThreadsGlyph size={16} />,
      onSelect: () => onToggleFollow(m, following),
    });
  }
  items.push({
    key: 'copy',
    label: t('message.copyText'),
    icon: <CopyGlyph size={16} />,
    onSelect: () => c.copiar(m.body, t('message.copied')),
  });
  items.push({
    key: 'link',
    label: t('message.copyLink'),
    icon: <LinkGlyph size={16} />,
    onSelect: () => c.copiar(messageLink(m.channelId, m.id), t('message.linkCopied')),
  });
  // «Guardar en Notes»: abre Sytue Notepads con una nota nueva ya vinculada a este
  // mensaje. NO se copia el texto al otro lado — se guarda la REFERENCIA, así
  // que quien abra la nota sigue pasando por el permiso del canal.
  //
  // El host sale de `appDomains` (@sytue/config), que ya lo deriva del emisor
  // OIDC del entorno: es lo mismo que hace el conmutador de aplicaciones, y
  // ahorra bajar una URL más por props hasta aquí. La organización (ADR 0056)
  // es la de la URL vista (`orgActual`): la referencia apunta al mensaje de
  // ESTA organización, y en el navegador el evento siempre la tiene a mano.
  items.push({
    key: 'to-notes',
    label: t('message.saveToNotes'),
    icon: <NoteGlyph size={16} />,
    onSelect: () => {
      const rotulo = m.body.trim().slice(0, 120);
      window.open(
        `${appUrl('notes', '', orgActual('/rooms'))}/?link=room_message:${encodeURIComponent(m.id)}&label=${encodeURIComponent(rotulo)}`,
        '_blank',
        'noopener,noreferrer',
      );
    },
  });
  if (c.onForward) {
    const onForward = c.onForward;
    items.push({
      key: 'forward',
      label: t('message.forward'),
      icon: <ForwardGlyph size={16} />,
      onSelect: () => onForward(m),
    });
  }
  if (c.onMarkUnread) {
    const onMarkUnread = c.onMarkUnread;
    items.push({
      key: 'unread',
      label: t('message.markUnread'),
      icon: <UnreadGlyph size={16} />,
      onSelect: () => onMarkUnread(m),
    });
  }
  // «Visto por» solo en los PROPIOS: saber quién ha leído lo que escribió otro
  // no ayuda a nadie a trabajar y sí convierte el chat en un panel de control
  // sobre los compañeros.
  if (m.mine && c.onReaders) {
    const onReaders = c.onReaders;
    items.push({
      key: 'readers',
      label: t('message.readersTitle'),
      icon: <SeenGlyph size={16} />,
      onSelect: () => onReaders(m),
    });
  }
  if (m.mine && (c.onEdit || c.onDelete)) {
    items.push('separator');
    if (c.onEdit) {
      const onEdit = c.onEdit;
      items.push({
        key: 'edit',
        label: t('action.edit'),
        icon: <EditGlyph size={16} />,
        onSelect: () => onEdit(m),
      });
    }
    if (c.onDelete) {
      const onDelete = c.onDelete;
      items.push({
        key: 'delete',
        label: t('action.delete'),
        icon: <TrashGlyph size={16} />,
        danger: true,
        onSelect: () => onDelete(m),
      });
    }
  }
  // Denunciar y bloquear: solo sobre lo AJENO y de una persona (un bot no se
  // bloquea: se quita la integración).
  if (!m.mine && !m.isBot && (c.onReport || c.onToggleBlock)) {
    const blocked = c.blocked ?? false;
    items.push('separator');
    if (c.onReport) {
      const onReport = c.onReport;
      items.push({
        key: 'report',
        label: t('report.message'),
        icon: <Icon name="warning" size={16} />,
        danger: true,
        onSelect: () => onReport(m),
      });
    }
    if (c.onToggleBlock) {
      const onToggleBlock = c.onToggleBlock;
      items.push({
        key: 'block',
        label: t(blocked ? 'block.unblock' : 'block.block', { name: m.author.name }),
        danger: !blocked,
        onSelect: () => onToggleBlock(m, blocked),
      });
    }
  }
  return items;
}

/** Clases de los botones de la barra flotante de acciones (28 px, glifo centrado). */
export const BOTON_DE_BARRA =
  'flex h-7 w-7 items-center justify-center rounded text-text-muted transition-colors duration-150 hover:bg-bg-subtle hover:text-text sytue-focus-ring';

/** Reacciones rápidas + selector de emoji completo, para la barra flotante. */
export function ReaccionesRapidas({
  m,
  onReact,
  t,
}: {
  m: ChatMessage;
  onReact: (messageId: string, emoji: string) => void;
  t: T;
}): JSX.Element {
  return (
    <>
      {QUICK_REACTIONS.map((emoji) => (
        <button
          key={emoji}
          type="button"
          aria-label={t('reaction.with', { emoji })}
          onClick={() => onReact(m.id, emoji)}
          class="hover:bg-bg-subtle flex h-7 w-7 items-center justify-center rounded text-sm transition-colors duration-150"
        >
          {emoji}
        </button>
      ))}
      {/* Selector de emoji completo: reaccionar con cualquiera, no solo el set rápido. */}
      <EmojiPickerButton
        onPick={(glyph) => onReact(m.id, glyph)}
        t={t}
        label={t('reaction.withAny')}
        title={t('reaction.react')}
        size={16}
        triggerClass={BOTON_DE_BARRA}
      />
    </>
  );
}

/** Chips de reacción de un mensaje + «+» para añadir otra. Sin contenedor: la vista
 *  decide con qué más comparten fila (indicador de hilo, «Siguiendo»…). */
export function FilaDeReacciones({
  m,
  onReact,
  memberNames,
  locale,
  t,
}: {
  m: ChatMessage;
  onReact?: ((messageId: string, emoji: string) => void) | undefined;
  memberNames?: Record<string, string> | undefined;
  locale: Locale;
  t: T;
}): JSX.Element {
  return (
    <>
      {(m.reactions ?? []).map((r) => {
        // «Quién reaccionó»: nombres de los reactores (tooltip nativo + etiqueta
        // accesible). Sin `subs` del servicio cae al emoji + conteo de siempre.
        const who = reactedByLabel(locale, t, r.emoji, r.subs, memberNames);
        return (
          <button
            key={r.emoji}
            type="button"
            aria-pressed={r.mine ? 'true' : 'false'}
            aria-label={t(r.mine ? 'reaction.removeMine' : 'reaction.add', {
              what: who ?? `${r.emoji} ${r.count}`,
            })}
            {...(who ? { title: who } : {})}
            onClick={() => onReact?.(m.id, r.emoji)}
            class={cx(
              'rounded-control sytue-focus-ring inline-flex items-center gap-1 border px-2 py-0.5 text-xs font-medium transition-colors duration-150',
              r.mine
                ? // Texto coloreado con el rol -text (AA sobre su tinte, §1.6);
                  // el hover del borde tambien remapea bien en oscuro.
                  'border-primary bg-selected text-primary-text hover:border-primary-text'
                : 'border-border bg-surface text-text-muted hover:border-border-strong hover:bg-bg-subtle',
            )}
          >
            <span aria-hidden="true">
              <EmojiToken token={r.emoji} />
            </span>
            <span>{r.count}</span>
          </button>
        );
      })}
      {/* «+» al final de las reacciones: añadir otra con cualquier emoji. */}
      {onReact && (m.reactions?.length ?? 0) > 0 ? (
        <EmojiPickerButton
          onPick={(glyph) => onReact(m.id, glyph)}
          t={t}
          label={t('reaction.addOne')}
          title={t('reaction.addOne')}
          triggerClass="inline-flex h-6 items-center justify-center rounded-control border border-border bg-surface px-2 text-text-muted transition-colors duration-150 hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring"
        >
          <span class="text-sm leading-none font-semibold" aria-hidden="true">
            +
          </span>
        </EmojiPickerButton>
      ) : null}
    </>
  );
}

/** Caja de edición inline de un mensaje propio: Intro guarda, Mayús+Intro salta de
 *  línea y Esc cancela, como en el compositor. */
export function CajaDeEdicion({
  valor,
  onInput,
  onGuardar,
  onCancelar,
  cajaRef,
  t,
}: {
  valor: string;
  onInput: (valor: string) => void;
  onGuardar: () => void;
  onCancelar: () => void;
  /** Ref de la caja: quien la monta decide cuándo y cómo enfocarla. */
  cajaRef?: ((el: HTMLTextAreaElement | null) => void) | undefined;
  t: T;
}): JSX.Element {
  return (
    <div class="mt-0.5">
      <textarea
        {...(cajaRef ? { ref: cajaRef } : {})}
        aria-label={t('message.edit')}
        value={valor}
        autofocus
        rows={Math.min(6, valor.split('\n').length)}
        onInput={(e) => onInput((e.target as HTMLTextAreaElement).value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            onGuardar();
          } else if (e.key === 'Escape') {
            e.preventDefault();
            onCancelar();
          }
        }}
        class="border-border bg-surface text-text sytue-focus-ring w-full resize-none rounded-md border px-2.5 py-1.5 text-base outline-none"
      />
      <div class="mt-1 flex items-center gap-2">
        <Button size="sm" onClick={onGuardar}>
          {t('action.save')}
        </Button>
        <Button size="sm" variant="secondary" onClick={onCancelar}>
          {t('action.cancel')}
        </Button>
        <span class="sytue-meta">{t('message.editHint')}</span>
      </div>
    </div>
  );
}

/** Aviso de envío fallido con «Reintentar»: icono + texto (nunca solo color), igual
 *  en el canal que en el hilo. */
export function AvisoDeEnvioFallido({
  m,
  onResend,
  t,
}: {
  m: ChatMessage;
  onResend?: ((m: ChatMessage) => void) | undefined;
  t: T;
}): JSX.Element {
  return (
    <p class="text-danger-text mt-0.5 flex items-center gap-1.5 text-xs">
      <Icon name="warning" size={12} />
      <span>{t('message.sendFailed')}</span>
      {onResend ? (
        <button
          type="button"
          class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2 transition-colors duration-150"
          onClick={() => onResend(m)}
        >
          {t('action.retry')}
        </button>
      ) : null}
    </p>
  );
}
