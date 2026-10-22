/**
 * ConfirmDialog — confirmar una acción, o pedir el dato que le falta.
 *
 * Existe porque la suite tiene VEINTIÚN `window.confirm`/`window.prompt` repartidos,
 * y cada uno es el mismo problema: el navegador pinta un diálogo que no es nuestro,
 * con botones en el idioma del SISTEMA operativo y no en el de la sesión, sin
 * relación visual con la aplicación y sin ningún sitio donde explicar qué va a pasar.
 * El peor caso de la suite pedía con `prompt()` el motivo obligatorio de suspender
 * una organización: un campo sin validar, sin contexto y sin poder marcarlo como
 * requerido.
 *
 * No es un Modal distinto: es el Modal de siempre con el pie que se repetía en cada
 * sitio —cancelar a la izquierda, la acción a la derecha— y el foco puesto en el
 * botón que confirma. Lo que va DENTRO lo pone quien llama: un párrafo para una
 * confirmación, o un campo cuando además hay que recoger un dato.
 *
 * `tone="danger"` para lo que destruye o desconecta algo. No es decoración: es lo
 * que distingue «guardar» de «desconectar el dominio de correo de la organización».
 */
import type { ComponentChildren, JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { Button } from './Button.js';
import { Modal } from './Modal.js';

export interface ConfirmDialogProps {
  open: boolean;
  /** Cerrar sin hacer nada (cancelar, Escape, click fuera). */
  onClose: () => void;
  /** Ejecutar la acción. Quien llama decide si además cierra. */
  onConfirm: () => void;
  title: string;
  /** Rótulo del botón que confirma. Un verbo, no «Aceptar»: quien lee tiene que
   *  saber qué va a pasar sin volver al título. */
  confirmLabel: string;
  /** `danger` para lo que destruye, desconecta o suspende. */
  tone?: 'primary' | 'danger';
  /** Deshabilita el botón de confirmar (p. ej. falta un campo obligatorio). */
  confirmDisabled?: boolean;
  busy?: boolean;
  /** El cuerpo: un párrafo, o los campos que hagan falta. */
  children: ComponentChildren;
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  confirmLabel,
  tone = 'primary',
  confirmDisabled = false,
  busy = false,
  children,
}: ConfirmDialogProps): JSX.Element {
  const t = useT();
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      size="sm"
      // Botones del tamaño por defecto, como el pie de cualquier otro diálogo de la
      // suite: con `sm` (28 px, texto de 11,4) el «¿Mover a la papelera?» se veía más
      // pequeño que el «Nuevo cuaderno» de al lado. Medido en producción, 2026-09-21:
      // 31,5 en Calendar, Contacts, Mail, Notes y Docs; 28 aquí y en Rooms.
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('ui.cancel')}
          </Button>
          <Button
            variant={tone === 'danger' ? 'danger' : 'primary'}
            onClick={onConfirm}
            disabled={confirmDisabled || busy}
            loading={busy}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {/* `gap-4` entre lo que se pase: un párrafo y dos campos salían pegados (≈5 px
          entre un campo y la etiqueta del siguiente). Con un hijo no cambia nada. */}
      <div class="flex flex-col gap-4">{children}</div>
    </Modal>
  );
}
