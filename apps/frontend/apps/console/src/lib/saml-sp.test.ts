/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las reglas del alta manual de un SP SAML: las mismas que Sytue ID o más estrictas,
 * nunca más laxas, y la clave privada pegada en lugar del certificado no pasa NUNCA
 * (el campo es público). Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import {
  NAMEID_FORMATS,
  altaDesdeCuerpo,
  altaSamlSp,
  falloAcs,
  falloCertificado,
  fallosSamlSp,
  type BorradorSamlSp,
} from './saml-sp.js';

const CERT = `-----BEGIN CERTIFICATE-----
MIIBszCCAVmgAwIBAgIUQ2xhdmUtZGUtcHJ1ZWJhLXNvbG8tZm9ybWEwCgYIKoZI
zj0EAwIwFTETMBEGA1UEAwwKc3AuZWplbXBsbzAeFw0yNjAxMDEwMDAwMDBaFw0y
-----END CERTIFICATE-----`;
// Solo la forma de una clave privada (cabecera y pie): el campo es público y se
// rechaza por la cabecera. Un cuerpo que parezca real lo marca gitleaks al
// exportar a sytue-suite.
const CLAVE = `-----BEGIN PRIVATE KEY-----
AAAA
-----END PRIVATE KEY-----`;

const borrador = (cambios: Partial<BorradorSamlSp> = {}): BorradorSamlSp => ({
  name: 'Gastos',
  entityId: 'https://gastos.acme.es/saml',
  acsUrl: 'https://gastos.acme.es/saml/acs',
  certPem: '',
  nameIdFormat: NAMEID_FORMATS[0],
  attrs: {},
  idpInitiated: false,
  relayState: '',
  ...cambios,
});

test('el ACS: https siempre, http solo en localhost, sin fragmento', () => {
  expect(falloAcs('https://app.acme.es/acs')).toBeNull();
  expect(falloAcs('http://localhost:8080/acs')).toBeNull();
  expect(falloAcs('http://app.acme.es/acs')).toBe('acsInsecure');
  expect(falloAcs('https://app.acme.es/acs#x')).toBe('acsNotUrl');
  expect(falloAcs('ftp://app.acme.es/acs')).toBe('acsNotUrl');
  expect(falloAcs('no es una url')).toBe('acsNotUrl');
  expect(falloAcs('   ')).toBe('acsRequired');
});

test('el certificado: opcional, PEM CERTIFICATE y nunca una clave privada', () => {
  expect(falloCertificado('')).toBeNull();
  expect(falloCertificado(CERT)).toBeNull();
  expect(falloCertificado(`${CERT}\n${CERT}`)).toBeNull();
  expect(falloCertificado(CLAVE)).toBe('certPrivateKey');
  // Juntos también: la clave no se cuela detrás de un certificado bueno.
  expect(falloCertificado(`${CERT}\n${CLAVE}`)).toBe('certPrivateKey');
  expect(
    falloCertificado('-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----'),
  ).toBe('certPrivateKey');
  expect(falloCertificado('MIIBszCCAVmgAwIBAgIU')).toBe('certNotPem');
  expect(falloCertificado(`${CERT}\nbasura`)).toBe('certNotPem');
  expect(falloCertificado(CERT.repeat(200))).toBe('certTooLarge');
});

test('cada fallo va a su campo y un borrador bueno no tiene ninguno', () => {
  expect(fallosSamlSp(borrador())).toEqual({});
  expect(
    fallosSamlSp(
      borrador({
        name: ' ',
        entityId: 'https://a b',
        acsUrl: '',
        certPem: CLAVE,
        nameIdFormat: 'urn:inventado',
        attrs: { email: 'correo electronico' },
      }),
    ),
  ).toEqual({
    name: 'nameRequired',
    entityId: 'entityIdSpaces',
    acsUrl: 'acsRequired',
    certPem: 'certPrivateKey',
    nameIdFormat: 'nameIdUnknown',
    attrs: 'attrSpaces',
  });
});

test('el alta va recortada, sin atributos vacíos, encendida y sin RelayState si no se inicia desde Sytue', () => {
  expect(
    altaSamlSp(
      borrador({
        name: '  Gastos ',
        certPem: `\n${CERT}\n`,
        attrs: { email: ' mail ', name: '', groups: 'memberOf' },
        relayState: '/inicio',
      }),
    ),
  ).toEqual({
    name: 'Gastos',
    entity_id: 'https://gastos.acme.es/saml',
    acs_url: 'https://gastos.acme.es/saml/acs',
    cert_pem: CERT,
    name_id_format: NAMEID_FORMATS[0],
    attribute_mapping: { email: 'mail', groups: 'memberOf' },
    idp_initiated: false,
    relay_state_default: '',
    enabled: true,
  });
  expect(
    altaSamlSp(borrador({ idpInitiated: true, relayState: ' /inicio ' })).relay_state_default,
  ).toBe('/inicio');
});

test('la BFF no se fía de los tipos del cuerpo', () => {
  expect(altaDesdeCuerpo(null)).toEqual({ ok: false, fallo: 'nameRequired' });
  expect(
    altaDesdeCuerpo({
      name: 'Gastos',
      entity_id: 'urn:gastos',
      acs_url: 'https://gastos.acme.es/acs',
      idp_initiated: 'sí',
      attribute_mapping: { email: 7, otro: 'x' },
    }),
  ).toEqual({
    ok: true,
    input: {
      name: 'Gastos',
      entity_id: 'urn:gastos',
      acs_url: 'https://gastos.acme.es/acs',
      cert_pem: '',
      name_id_format: NAMEID_FORMATS[0],
      attribute_mapping: {},
      idp_initiated: false,
      relay_state_default: '',
      enabled: true,
    },
  });
  expect(
    altaDesdeCuerpo({
      name: 'G',
      entity_id: 'urn:g',
      acs_url: 'https://g.es/acs',
      cert_pem: CLAVE,
    }),
  ).toEqual({ ok: false, fallo: 'certPrivateKey' });
});
