import type { ChatMessage } from './chat.js';

/**
 * «↑ en el compositor vacío edita tu último mensaje» (Slack, Teams, Mattermost): el
 * mensaje que se abre es el último PROPIO del canal que de verdad se puede editar.
 *
 * Fuera quedan los que aún no existen en el servidor (en vuelo o fallidos: no hay id
 * que mandar al PATCH), las tarjetas de llamada (son del sistema aunque las firme
 * quien llamó) y las encuestas y tareas, cuyo texto no es lo que se edita. Devuelve
 * null si no hay ninguno, y entonces la flecha hace lo de siempre.
 *
 * Gemelo en Fyne: `UltimoEditable` (apps/fyne/apps/rooms/historial.go).
 */
export function ultimoEditable(messages: readonly ChatMessage[]): ChatMessage | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]!;
    if (m.mine && !m.pending && !m.failed && m.type !== 'call' && !m.card) return m;
  }
  return null;
}

/** Lo que la decisión necesita de un `KeyboardEvent`. */
export interface TeclaEditar {
  key: string;
  shiftKey: boolean;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
}

/**
 * ¿Esta tecla del compositor abre la edición del último mensaje? Solo ↑ a secas y con
 * la caja VACÍA —sin texto ni adjuntos—: con algo escrito la flecha mueve el cursor
 * entre líneas, que es lo que se espera de un área de texto.
 */
export function abreEdicionUltimo(e: TeclaEditar, texto: string, adjuntos: number): boolean {
  return (
    e.key === 'ArrowUp' &&
    !e.shiftKey &&
    !e.altKey &&
    !e.ctrlKey &&
    !e.metaKey &&
    texto === '' &&
    adjuntos === 0
  );
}

/**
 * ¿El mensaje pedido con ↑ es el que ya está en edición? Entonces solo se devuelve el
 * foco a su caja: reabrirla repondría el texto original y se perdería lo que se llevaba
 * escrito. Gemelo en Fyne: `EditandoYa` (apps/fyne/apps/rooms/historial.go).
 */
export function editandoYa(editando: string | null, pedido: string): boolean {
  return editando !== null && editando === pedido;
}
