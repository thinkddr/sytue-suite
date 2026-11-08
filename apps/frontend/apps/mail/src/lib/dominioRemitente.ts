/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DE QUÉ DOMINIO SALE EL LOGO DE UN REMITENTE.
 *
 * Puro y sin dependencias de Node: lo usa el navegador para construir la URL
 * (`/api/sender-logo?d=…`) y el BFF para validar lo que le llega. El servicio Go,
 * que es quien busca el logo, decide lo mismo con su propia copia
 * (services/mail/internal/logo): las dos recorren su testdata/dominios.json.
 */

/**
 * Buzones gratuitos: detrás de `ana@gmail.com` hay una PERSONA, no Google. Poner el
 * logo de Gmail en su fila sería mentir sobre quién escribe, así que estos dominios
 * se quedan con las iniciales.
 */
const BUZONES_PERSONALES = new Set([
  'gmail.com',
  'googlemail.com',
  'outlook.com',
  'outlook.es',
  'hotmail.com',
  'hotmail.es',
  'live.com',
  'msn.com',
  'yahoo.com',
  'yahoo.es',
  'ymail.com',
  'icloud.com',
  'me.com',
  'mac.com',
  'aol.com',
  'proton.me',
  'protonmail.com',
  'pm.me',
  'gmx.com',
  'gmx.es',
  'gmx.de',
  'gmx.net',
  'web.de',
  'mail.com',
  'zoho.com',
  'yandex.com',
  'yandex.ru',
  'mail.ru',
  'tutanota.com',
  'tuta.io',
  'telefonica.net',
  'sytue.eu',
]);

/**
 * Sufijos públicos de dos niveles más comunes. Sin ellos, `news.bbc.co.uk` daría
 * `co.uk`, que no es de nadie. No es la Public Suffix List entera —son 9 000
 * entradas para una heurística de presentación—: si un sufijo raro se cuela, lo
 * peor que pasa es que no hay logo y salen las iniciales.
 */
const SUFIJOS_DOBLES = new Set([
  'co.uk',
  'org.uk',
  'ac.uk',
  'gov.uk',
  'com.es',
  'org.es',
  'gob.es',
  'edu.es',
  'nom.es',
  'com.mx',
  'com.ar',
  'com.br',
  'com.co',
  'com.pe',
  'com.au',
  'co.nz',
  'co.jp',
  'co.za',
  'com.tr',
  'com.cn',
  'co.in',
]);

const ETIQUETA = /^(?!-)[a-z0-9-]{1,63}(?<!-)$/;

/**
 * Normaliza un dominio a su parte registrable (`corp.vodafone.es` → `vodafone.es`)
 * o devuelve `null` si no es un dominio público válido. Los subdominios de envío
 * (`email.`, `news.`, `corp.`) casi nunca tienen web; la marca vive en el registrable.
 */
export function dominioRegistrable(raw: string): string | null {
  const host = raw.trim().toLowerCase().replace(/\.$/, '');
  if (!host || host.length > 253) return null;
  const etiquetas = host.split('.');
  if (etiquetas.length < 2 || !etiquetas.every((e) => ETIQUETA.test(e))) return null;
  // Un TLD numérico es una IP escrita como dominio: aquí no hay logo que buscar.
  if (/^\d+$/.test(etiquetas[etiquetas.length - 1]!)) return null;
  const ultimasDos = etiquetas.slice(-2).join('.');
  const n = SUFIJOS_DOBLES.has(ultimasDos) ? 3 : 2;
  if (etiquetas.length < n) return null;
  return etiquetas.slice(-n).join('.');
}

/**
 * El dominio del que buscar el logo de `email`, o `null` si la fila debe quedarse
 * con las iniciales (dirección mal formada, buzón personal gratuito, o un
 * subdominio de Sytue).
 */
export function dominioDeLogo(email: string | undefined): string | null {
  if (!email) return null;
  const arroba = email.lastIndexOf('@');
  if (arroba < 1) return null;
  const dominio = dominioRegistrable(email.slice(arroba + 1));
  if (!dominio || BUZONES_PERSONALES.has(dominio)) return null;
  return dominio;
}
