/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ALTA MANUAL DE UN SP SAML: qué se acepta y qué se le dice a quien lo rellena.
 *
 * Lo usan el asistente (para decir el fallo ANTES de enviar, en el campo que toca) y
 * la BFF (para no mandar a Sytue ID algo que no tiene forma de alta). La regla de
 * verdad sigue siendo la de Sytue ID —`validateSAMLSP` y `validarCertificadoSP` en
 * `services/id/internal/http/admin_apps.go`—, y estas son las mismas o más
 * estrictas, nunca más laxas: si aquí pasara algo que allí no, el admin vería un 400
 * genérico después de rellenar cinco campos.
 */
import type { SamlSpInput } from './types.js';

/** Formatos de NameID que Sytue ID sabe emitir (`saml/idp.go`, `resolveNameID`). */
export const NAMEID_FORMATS = [
  'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
  'urn:oasis:names:tc:SAML:2.0:nameid-format:persistent',
  'urn:oasis:names:tc:SAML:2.0:nameid-format:transient',
  'urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified',
] as const;
export type NameIdFormat = (typeof NAMEID_FORMATS)[number];

/** Los datos que Sytue ID manda como atributos: solo estos tres admite el mapeo. */
export const SAML_ATTRS = ['email', 'name', 'groups'] as const;
export type SamlAttr = (typeof SAML_ATTRS)[number];

/** El mismo tope que Sytue ID (`maxCertificadoSP`). */
export const MAX_CERT_PEM = 16 * 1024;

/** Qué le pasa a un campo, como clave estable que el asistente traduce. */
export type SamlSpFallo =
  | 'nameRequired'
  | 'entityIdRequired'
  | 'entityIdSpaces'
  | 'acsRequired'
  | 'acsNotUrl'
  | 'acsInsecure'
  | 'certPrivateKey'
  | 'certNotPem'
  | 'certTooLarge'
  | 'nameIdUnknown'
  | 'attrSpaces';

/**
 * El ACS recibe la respuesta firmada con la identidad de la persona: https siempre, y
 * http solo en localhost (lo mismo que `validRegistrationRedirect` en Sytue ID).
 */
export function falloAcs(raw: string): SamlSpFallo | null {
  const v = raw.trim();
  if (!v) return 'acsRequired';
  let u: URL;
  try {
    u = new URL(v);
  } catch {
    return 'acsNotUrl';
  }
  if (u.hash) return 'acsNotUrl';
  const local = ['localhost', '127.0.0.1', '[::1]'].includes(u.hostname);
  if (u.protocol === 'https:') return null;
  if (u.protocol === 'http:' && local) return null;
  return u.protocol === 'http:' ? 'acsInsecure' : 'acsNotUrl';
}

/**
 * El certificado es opcional y PÚBLICO: se devuelve en cada lectura. Lo que importa es
 * no aceptar una CLAVE PRIVADA pegada en su lugar (dos bloques PEM que se parecen
 * mucho). La comprobación de que es X.509 la hace Sytue ID; aquí, la forma.
 */
export function falloCertificado(raw: string): SamlSpFallo | null {
  const v = raw.trim();
  if (!v) return null;
  if (v.length > MAX_CERT_PEM) return 'certTooLarge';
  if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(v)) return 'certPrivateKey';
  const bloque = /-----BEGIN CERTIFICATE-----\s*([A-Za-z0-9+/=\s]+?)\s*-----END CERTIFICATE-----/g;
  const resto = v.replace(bloque, '').trim();
  if (resto !== '' || !/-----BEGIN CERTIFICATE-----/.test(v)) return 'certNotPem';
  return null;
}

export interface BorradorSamlSp {
  name: string;
  entityId: string;
  acsUrl: string;
  certPem: string;
  nameIdFormat: string;
  attrs: Partial<Record<SamlAttr, string>>;
  idpInitiated: boolean;
  relayState: string;
}

/** Los fallos de cada campo del borrador, vacío si se puede enviar. */
export function fallosSamlSp(
  b: BorradorSamlSp,
): Partial<Record<keyof BorradorSamlSp, SamlSpFallo>> {
  const out: Partial<Record<keyof BorradorSamlSp, SamlSpFallo>> = {};
  if (!b.name.trim()) out.name = 'nameRequired';
  const entity = b.entityId.trim();
  if (!entity) out.entityId = 'entityIdRequired';
  else if (/\s/.test(entity)) out.entityId = 'entityIdSpaces';
  const acs = falloAcs(b.acsUrl);
  if (acs) out.acsUrl = acs;
  const cert = falloCertificado(b.certPem);
  if (cert) out.certPem = cert;
  if (b.nameIdFormat && !(NAMEID_FORMATS as readonly string[]).includes(b.nameIdFormat)) {
    out.nameIdFormat = 'nameIdUnknown';
  }
  if (SAML_ATTRS.some((k) => /\s/.test((b.attrs[k] ?? '').trim()))) out.attrs = 'attrSpaces';
  return out;
}

/** Lo que se envía: recortado, sin los atributos vacíos (Sytue ID usa el nombre lógico). */
export function altaSamlSp(b: BorradorSamlSp): SamlSpInput {
  const attribute_mapping: Partial<Record<SamlAttr, string>> = {};
  for (const k of SAML_ATTRS) {
    const v = (b.attrs[k] ?? '').trim();
    if (v) attribute_mapping[k] = v;
  }
  return {
    name: b.name.trim(),
    entity_id: b.entityId.trim(),
    acs_url: b.acsUrl.trim(),
    cert_pem: b.certPem.trim(),
    name_id_format: b.nameIdFormat || NAMEID_FORMATS[0],
    attribute_mapping,
    idp_initiated: b.idpInitiated,
    relay_state_default: b.idpInitiated ? b.relayState.trim() : '',
    // Un alta manual nace encendida: quien la rellena acaba de pegar los datos en la
    // app y va a probar el inicio de sesión. Apagarla está a un clic en la lista.
    enabled: true,
  };
}

/**
 * Lo que la BFF acepta de un cuerpo cualquiera: el mismo borrador que el asistente,
 * sin confiar en tipos. Devuelve el alta o el primer fallo.
 */
export function altaDesdeCuerpo(
  body: unknown,
): { ok: true; input: SamlSpInput } | { ok: false; fallo: SamlSpFallo } {
  const o = (body && typeof body === 'object' ? body : {}) as Record<string, unknown>;
  const str = (v: unknown) => (typeof v === 'string' ? v : '');
  const mapping = (
    o.attribute_mapping && typeof o.attribute_mapping === 'object' ? o.attribute_mapping : {}
  ) as Record<string, unknown>;
  const borrador: BorradorSamlSp = {
    name: str(o.name),
    entityId: str(o.entity_id),
    acsUrl: str(o.acs_url),
    certPem: str(o.cert_pem),
    nameIdFormat: str(o.name_id_format),
    attrs: Object.fromEntries(SAML_ATTRS.map((k) => [k, str(mapping[k])])),
    idpInitiated: o.idp_initiated === true,
    relayState: str(o.relay_state_default),
  };
  const fallos = Object.values(fallosSamlSp(borrador));
  if (fallos.length > 0) return { ok: false, fallo: fallos[0] as SamlSpFallo };
  return { ok: true, input: altaSamlSp(borrador) };
}
