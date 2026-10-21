/**
 * Reenviar mensaje (Ola 8): compone el cuerpo del mensaje que se publica en el
 * canal destino citando el original. Usa el Markdown ligero de MessageBody —
 * **negrita** para la atribución, cita `>` para el cuerpo y el enlace canónico
 * (autolink) al original — de modo que se lee bien renderizado y en crudo.
 *
 * No arrastra menciones: el reenvío NO debe volver a notificar a los @mencionados
 * del original (por eso se cita como texto, sin el array de mentions).
 */
export function forwardedBody(body: string, author: string, link: string): string {
  const trimmed = body.trim();
  const quoted = trimmed
    ? trimmed
        .split('\n')
        .map((line) => `> ${line}`)
        .join('\n')
    : '> _(sin texto)_';
  return `Reenviado de **${author}**:\n${quoted}\n\n${link}`;
}
