/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Colaboración» del ribbon (BLUEPRINT.md §6.3 + encargo B5):
 * comentarios (nuevo sobre la selección + mostrar/ocultar panel), control de
 * cambios (marca la edición con autoría —<ins>/<del> data-track— en el HTML del
 * folio; solo kind document), versiones (guardar + historial) y compartir
 * (mismo diálogo que la cabecera: copiar enlace + toast).
 */
import type { JSX } from 'preact';
import { Dropdown, MenuItem, useConfirm, usePrompt, useToast } from '@sytue/ui';
import { formatDateTime } from '@sytue/i18n';
import {
  getEditorRoot,
  removeCommentMark,
  resolveTrackedChanges,
  wrapSelectionForComment,
} from '../../canvas/exec';
import { quitarTabulaciones } from '../../canvas/tabulaciones';
import { withTrackFlag } from '../../trackchanges';
import { ShareDialog } from '../../dialogs/ShareDialog';
import { EditorIcon } from '../../icons';
import type { EditorContext } from '../../types';
import { RibbonButton, RibbonGroup, RibbonSeparator } from '../primitives';

/** Receta de foco visible del blueprint (§0.6): nunca outline:none sin reemplazo. */
// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Botón-icono 28×28 con la misma receta que las primitivas (§5); aquí en local
 *  porque el disparador del diálogo necesita aria-haspopup/aria-expanded, que
 *  RibbonButton no expone (semántica de diálogo, no de toggle). */
const ICON_BTN =
  'inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text ' +
  FOCUS_RING;

/** Botón compacto con texto (disparador del historial), a juego con RibbonSelect. */
const CHIP_BTN =
  'inline-flex h-7 cursor-pointer items-center gap-1 rounded-sm border border-border bg-surface px-2 text-xs text-text hover:bg-bg-subtle ' +
  // Abierto (el menú de Historial, el panel de Actividad) se dice como en los botones de la
  // cinta: relleno `selected`. Sin esto, con el panel abierto el botón no cambiaba nada.
  'aria-expanded:bg-selected aria-pressed:bg-selected ' +
  'disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

/** «Actividad»: abre el panel de auditoría del documento (solo el propietario). */
function ActividadBoton({ ctx }: { ctx: EditorContext }): JSX.Element {
  const abierto = ctx.ui.panel === 'actividad';
  return (
    <button
      type="button"
      class={CHIP_BTN}
      title={ctx.t('audit.history')}
      aria-pressed={abierto}
      disabled={ctx.connection === 'offline'}
      onClick={() => ctx.setUi({ panel: abierto ? null : 'actividad' })}
    >
      {ctx.t('audit.history')}
    </button>
  );
}

export function ColaboracionTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  // Comentarios y versiones pedían su dato con el diálogo del navegador.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const toast = useToast();
  const t = ctx.t;
  const offline = ctx.connection === 'offline';
  const comentariosVisibles = ctx.ui.panel === 'comentarios';

  /** Comentario anclado a la selección: se envuelve lo seleccionado en un
   *  `<mark data-comment-id>` (resaltado sutil que persiste en el HTML del folio)
   *  ANTES del prompt —que colapsa la selección— y el ancla `{ mark, text }` viaja
   *  en el comentario. Sin selección → comentario de documento (ancla null). */
  const nuevoComentario = async (): Promise<void> => {
    const markId = crypto.randomUUID().replace(/-/g, '').slice(0, 12);
    const cita = wrapSelectionForComment(markId); // null si no hay selección de texto
    const body = await preguntar({ mensaje: t('comments.newPrompt') });
    if (body === null || !body.trim()) {
      if (cita !== null) removeCommentMark(markId); // cancelar: deshacer el marcado
      return;
    }
    const anchor = cita !== null ? { mark: markId, text: cita } : null;
    void ctx
      .addComment(body.trim(), anchor)
      .then(() => toast.notify({ tone: 'success', title: t('comments.added') }))
      .catch(() => {
        if (cita !== null) removeCommentMark(markId); // backend falló: sin ancla huérfana
        toast.notify({ tone: 'danger', title: t('comments.addError') });
      });
    // Abrir el panel para ver el hilo recién creado.
    ctx.setUi({ panel: 'comentarios' });
  };

  /** Conmuta el control de cambios y PERSISTE el modo en el HTML del folio (la
   *  sentinela al frente del contenido, que ES el modelo): así reabrir el doc lo
   *  restaura, aunque no se llegue a teclear tras conmutar. */
  const conmutarControlCambios = (): void => {
    const on = !ctx.ui.track;
    ctx.setUi({ track: on });
    const root = getEditorRoot();
    if (root) ctx.onContentInput(withTrackFlag(quitarTabulaciones(root.innerHTML), on));
  };

  /** Consolida (aceptar) o descarta (rechazar) TODAS las marcas rastreadas. Pide
   *  confirmación: es masivo y no entra en el deshacer nativo (swap de innerHTML). */
  const resolverCambios = async (accept: boolean): Promise<void> => {
    const ok = await confirmar({
      mensaje: t(accept ? 'review.acceptAll.confirm' : 'review.rejectAll.confirm'),
      confirmLabel: t(accept ? 'review.acceptAll' : 'review.rejectAll'),
      ...(accept ? {} : { tone: 'danger' as const }),
    });
    if (!ok) return;
    if (resolveTrackedChanges(accept))
      toast.notify({
        tone: 'success',
        title: t(accept ? 'review.accepted' : 'review.rejected'),
      });
    else toast.notify({ tone: 'info', title: t('review.nothing') });
  };

  const guardarVersion = async (): Promise<void> => {
    // Mismo flujo que el menú Archivo (§6.3): etiqueta OPCIONAL —por eso
    // `permitirVacio`—; cancelar aborta.
    const label = await preguntar({ mensaje: t('versions.labelPrompt'), permitirVacio: true });
    if (label === null) return;
    void ctx
      .createVersion(label || undefined)
      .then(() => toast.notify({ tone: 'success', title: t('versions.saved') }))
      .catch(() => toast.notify({ tone: 'danger', title: t('versions.saveError') }));
  };

  const restaurarVersion = async (id: string): Promise<void> => {
    const ok = await confirmar({
      titulo: t('versions.restoreTitle'),
      mensaje: t('versions.restoreConfirm'),
      confirmLabel: t('versions.restoreAction'),
    });
    if (!ok) return;
    void ctx
      .restoreVersion(id)
      .then(() => toast.notify({ tone: 'success', title: t('versions.restored') }))
      .catch(() => toast.notify({ tone: 'danger', title: t('versions.restoreError') }));
  };

  return (
    <>
      <RibbonGroup label={t('group.comments')}>
        <RibbonButton
          commandId="commentNew"
          icon="commentAdd"
          disabled={offline}
          onAction={() => void nuevoComentario()}
        />
        <RibbonButton
          commandId="commentsPanel"
          icon="commentList"
          label={t(comentariosVisibles ? 'comments.hide' : 'comments.show')}
          active={comentariosVisibles}
          onAction={() => ctx.setUi({ panel: comentariosVisibles ? null : 'comentarios' })}
        />
      </RibbonGroup>

      <RibbonSeparator />

      {/* Control de cambios: solo el folio (kind document) rastrea la edición. Al
          activarlo, lo tecleado/pegado se marca como <ins> y lo borrado como <del>
          con autoría; las marcas viven en el HTML y persisten con el autosave. */}
      {ctx.kind === 'document' && (
        <>
          <RibbonGroup label={t('group.review')}>
            <RibbonButton
              commandId="trackChanges"
              icon="trackChanges"
              active={ctx.ui.track}
              onAction={conmutarControlCambios}
            />
            {/* Abre el panel de revisión (aceptar/rechazar cambio a cambio + navegar). */}
            <RibbonButton
              commandId="reviewPanel"
              icon="panelRight"
              label={t(ctx.ui.panel === 'revision' ? 'review.panelHide' : 'review.panel')}
              active={ctx.ui.panel === 'revision'}
              onAction={() => ctx.setUi({ panel: ctx.ui.panel === 'revision' ? null : 'revision' })}
            />
            <RibbonButton
              commandId="acceptAllChanges"
              icon="check"
              onAction={() => void resolverCambios(true)}
            />
            <RibbonButton
              commandId="rejectAllChanges"
              icon="close"
              onAction={() => void resolverCambios(false)}
            />
          </RibbonGroup>

          <RibbonSeparator />
        </>
      )}

      <RibbonGroup label={t('group.versions')}>
        <RibbonButton
          commandId="versionSave"
          icon="history"
          disabled={offline}
          onAction={() => void guardarVersion()}
        />
        <Dropdown
          label={t('versions.history')}
          align="start"
          trigger={(trigger) => (
            <button
              type="button"
              class={CHIP_BTN}
              title={t('versions.history')}
              {...trigger}
              disabled={offline}
              onClick={() => {
                // Al abrir, refrescar la lista (§6.3); reabrir también refresca.
                if (!trigger['aria-expanded']) void ctx.loadVersions();
                trigger.onClick();
              }}
            >
              {t('versions.historyShort')}
              <EditorIcon name="chevronDown" size="sm" />
            </button>
          )}
        >
          {ctx.versions.length === 0 ? (
            /* Texto estático (sin role=menuitem): no debe recibir foco del menú. */
            <p class="text-text-muted px-2.5 py-2 text-sm">{t('versions.empty')}</p>
          ) : (
            ctx.versions.map((v) => (
              <MenuItem key={v.id} onClick={() => void restaurarVersion(v.id)}>
                {`${v.label || t('versions.untitled')} — ${formatDateTime(
                  ctx.locale,
                  v.created_at,
                  {
                    dateStyle: 'short',
                    timeStyle: 'short',
                  },
                )}`}
              </MenuItem>
            ))
          )}
        </Dropdown>
        {ctx.esPropietario && <ActividadBoton ctx={ctx} />}
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.share')}>
        {/* Mismo popover que la cabecera (§6.1.6); aquí el disparador es el icono del ribbon. */}
        <ShareDialog
          ctx={ctx}
          align="start"
          trigger={(trigger) => (
            <button
              type="button"
              class={ICON_BTN}
              aria-label={t('share.action')}
              title={t('share.action')}
              {...trigger}
              disabled={offline}
            >
              <EditorIcon name="share" />
            </button>
          )}
        />
      </RibbonGroup>
      {dialogoConfirmar}
      {dialogoPreguntar}
    </>
  );
}
