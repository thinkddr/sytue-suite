/*
 * Frontera única para los comandos de edición nativos heredados.
 *
 * `execCommand` sigue siendo necesario en los editores contenteditable porque
 * conserva la pila de deshacer del navegador; Clipboard API / Selection / Range
 * son los caminos modernos alrededor de esta frontera. Mantener el cast aquí
 * evita repartir una API deprecada por todas las aplicaciones y permite retirar
 * el fallback en un solo sitio cuando los motores ofrezcan un reemplazo completo.
 */
interface LegacyEditingDocument {
  execCommand: (commandId: string, showUI?: boolean, value?: string) => boolean;
  queryCommandEnabled: (commandId: string) => boolean;
  queryCommandState: (commandId: string) => boolean;
  queryCommandValue: (commandId: string) => string;
}

function legacyEditingDocument(documentRef: Document): LegacyEditingDocument {
  return documentRef;
}

export function executeEditingCommand(
  commandId: string,
  value?: string,
  documentRef: Document = document,
): boolean {
  try {
    return legacyEditingDocument(documentRef).execCommand(commandId, false, value);
  } catch {
    return false;
  }
}

export function editingCommandState(commandId: string, documentRef: Document = document): boolean {
  try {
    return legacyEditingDocument(documentRef).queryCommandState(commandId);
  } catch {
    return false;
  }
}

export function editingCommandValue(commandId: string, documentRef: Document = document): string {
  try {
    return legacyEditingDocument(documentRef).queryCommandValue(commandId) || '';
  } catch {
    return '';
  }
}

export function editingCommandEnabled(
  commandId: string,
  documentRef: Document = document,
): boolean {
  try {
    return legacyEditingDocument(documentRef).queryCommandEnabled(commandId);
  } catch {
    return false;
  }
}
