/**
 * Los orígenes cuyos avisos van FIJOS en «Qué quieres recibir». Los de Sytue ID son
 * los avisos de seguridad de la cuenta (un inicio de sesión nuevo, la contraseña o la
 * verificación en dos pasos que cambian): quien los silenciara no se enteraría de que
 * le han entrado en la cuenta, que es justo cuando más falta hace. No están en
 * `ORIGENES_DE_AVISOS` (no se ofrece callarlos) y el Hub rechaza silenciarlos
 * (`domain.Silenciable`); la bandeja los enseña encendidos y sin poder tocarse, para
 * que se sepa que llegan y por qué.
 */
import type { NotificationSource } from '@sytue/api-clients';
import { TIPOS_CRITICOS } from '@sytue/shell';

/** Los orígenes que nunca se silencian. Es la lista del Hub, en el mismo orden. */
export const ORIGENES_FIJOS: readonly NotificationSource[] = ['id'];

/** Si un origen admite silencio. Lo que no está en `ORIGENES_FIJOS`, sí. */
export function silenciable(source: NotificationSource): boolean {
  return !ORIGENES_FIJOS.includes(source);
}

/**
 * Si los avisos de un origen están silenciados según las reglas. Un origen fijo nunca
 * lo está, aunque quede una fila antigua de antes de que el Hub lo impidiera: es lo
 * mismo que decide el Hub al entregar, y la pantalla no debe decir otra cosa.
 */
export function silenciado(
  prefs: readonly { source: string; kind?: string | undefined; muted: boolean }[],
  source: NotificationSource,
): boolean {
  if (!silenciable(source)) return false;
  return prefs.some((p) => p.source === source && !p.kind && p.muted);
}

/**
 * Si los avisos de un tipo están silenciados, con la misma regla que aplica el Hub al
 * entregar (`domain.Muted`): la que nombra origen y tipo manda; si no la hay, la del
 * origen entero. Así «callar todo el correo y dejar las menciones» se ve como es. Un
 * tipo crítico o de un origen fijo nunca lo está. Un tipo partido en varios hereda la
 * regla del de antes (`TIPOS_PARTIDOS`), como en el Hub.
 */
export function tipoSilenciado(
  prefs: readonly { source: string; kind?: string | undefined; muted: boolean }[],
  source: NotificationSource,
  tipo: string,
): boolean {
  if (!silenciable(source) || TIPOS_CRITICOS.has(tipo)) return false;
  const exacta = prefs.find((p) => p.source === source && p.kind === tipo);
  if (exacta) return exacta.muted;
  const anterior = TIPOS_PARTIDOS[tipo];
  const heredada = anterior && prefs.find((p) => p.source === source && p.kind === anterior);
  return heredada ? heredada.muted : silenciado(prefs, source);
}

/** De qué tipo salió cada uno de los partidos: `tiposPartidos` del `domain.go` del Hub. */
export const TIPOS_PARTIDOS: Readonly<Record<string, string>> = {
  'cloud.deploy.ready': 'cloud.deploy',
  'cloud.deploy.failed': 'cloud.deploy',
};

/**
 * Una fila de tipos (que puede reunir varios del Hub) sale encendida mientras llegue
 * alguno: apagada solo si están todos callados.
 */
export function filaSilenciada(
  prefs: readonly { source: string; kind?: string | undefined; muted: boolean }[],
  source: NotificationSource,
  tipos: readonly string[],
): boolean {
  return tipos.every((t) => tipoSilenciado(prefs, source, t));
}
