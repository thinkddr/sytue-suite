/**
 * Las TRES direcciones de un mismo webhook entrante de Sytue Rooms.
 *
 * El servicio expone el mismo token en tres rutas (`server.go:194-196`):
 *
 *   POST /hooks/{token}         autodetecta el formato del cuerpo
 *   POST /hooks/slack/{token}   fuerza el dialecto Slack
 *   POST /hooks/teams/{token}   fuerza el dialecto Microsoft Teams
 *
 * Los alias forzados existen para los emisores que mandan solo `{"text":"…"}`, que la
 * autodetección no puede distinguir de un envío nativo. Sin enseñarlos, la
 * compatibilidad con Slack y Teams es invisible: quien tiene una herramienta que solo
 * sabe hablar el webhook de Slack no descubre que puede pegar la URL y funcionar.
 */

/** Una variante con su etiqueta de proveedor (marca: no se traduce). */
export interface HookVariant {
  label: string;
  url: string;
  /**
   * Clave de traducción para la etiqueta. Solo la lleva la del correo: «Slack» o
   * «Microsoft Teams» son marcas y se escriben igual en los cinco idiomas, pero
   * «Dirección de correo» no.
   */
  labelKey?: string;
}

/**
 * hookVariants deriva las tres direcciones de la URL que devuelve el servicio al crear o
 * rotar un entrante (`{base}/hooks/{token}`).
 *
 * Se inserta el segmento en la ÚLTIMA aparición de `/hooks/`, no en la primera: el host
 * de la organización podría contener esa cadena (`https://hooks.example.eu/...`) y
 * sustituir a ciegas produciría una URL que no existe.
 *
 * Si la URL no tiene la forma esperada se devuelve SOLO la original: es preferible
 * ofrecer una dirección que funciona a inventar dos que quizá no.
 */
export function hookVariants(url: string, mailDomain = '', channelName = ''): HookVariant[] {
  const marca = '/hooks/';
  const i = url.lastIndexOf(marca);
  if (i < 0) return [{ label: 'Sytue', url }];
  const antes = url.slice(0, i);
  const token = url.slice(i + marca.length);
  if (!token) return [{ label: 'Sytue', url }];
  const out: HookVariant[] = [
    { label: 'Sytue', url },
    { label: 'Slack', url: `${antes}${marca}slack/${token}` },
    { label: 'Microsoft Teams', url: `${antes}${marca}teams/${token}` },
    { label: 'Google Chat', url: `${antes}${marca}googlechat/${token}` },
  ];
  // La dirección de correo es la cuarta forma del MISMO token: media herramienta del
  // mercado sabe mandar un email y nada más.
  const email = channelEmail(url, mailDomain, channelName);
  if (email) out.push({ label: 'Email', labelKey: 'integrations.channelEmail', url: email });
  return out;
}

/**
 * Dirección de CORREO del mismo webhook: `<token>@<dominio de canales>`.
 *
 * Media herramienta del mercado sabe mandar un email y nada más. Con esta dirección se
 * escribe al canal sin programar nada — y es el mismo token, así que rotarlo la cambia
 * igual que cambia las URLs.
 *
 * Devuelve '' si no hay dominio configurado (el correo a canal está apagado en ese
 * despliegue) o si la URL no tiene la forma esperada: enseñar una dirección que rebota
 * sería peor que no enseñar ninguna.
 */
export function channelEmail(url: string, domain: string, channelName = ''): string {
  const d = domain.trim().replace(/^[@.]+|[@.]+$/g, '');
  if (!d) return '';
  const marca = '/hooks/';
  const i = url.lastIndexOf(marca);
  if (i < 0) return '';
  const token = url.slice(i + marca.length);
  if (!token) return '';
  // El nombre del canal delante del token. Sin él la dirección es un token de cuarenta
  // y tres caracteres: imposible de dictar, de teclear o de reconocer en una lista de
  // destinatarios. Lo que autoriza sigue siendo el token, que va detrás del «+»; el
  // nombre es para quien lee. Se sanea porque acaba en una dirección de correo.
  const alias = channelName
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 32);
  return alias ? `${alias}+${token}@${d}` : `${token}@${d}`;
}
