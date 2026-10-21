/**
 * Separador «Nuevos mensajes» (Ola 8): dado el número de no leídos capturado al
 * ABRIR un canal (antes de markRead) y el total de mensajes cargados, calcula el
 * índice del primer mensaje no leído — la fila ante la que va el separador.
 *
 * Los no leídos son los `count` mensajes más recientes: el primero está en
 * `total - count`. Si no hay no leídos → null (sin separador). Si hay más no
 * leídos que mensajes cargados (una página tapó parte), se ancla arriba (0): todo
 * lo visible es nuevo.
 *
 * ponytail: se deriva del CONTADOR de no leídos, no de un last_read por mensaje
 * (el backend no expone la marca por mensaje al cliente); si el contador excluyera
 * los mensajes propios el separador podría quedar unas filas corrido. Basta para un
 * marcador visual de sesión; afinar con un last_read por-mensaje si se añade.
 */
export function unreadDividerIndex(count: number, total: number): number | null {
  if (count <= 0 || total <= 0) return null;
  if (count >= total) return 0;
  return total - count;
}
