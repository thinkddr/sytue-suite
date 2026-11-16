/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import {
  primerosPasos,
  revisionDeCuenta,
  type DatosPrimerosPasos,
  type DatosRevision,
} from './revisionCuenta.js';
import vectores from './revisionCuenta.vectores.json';

const ahora = new Date('2026-11-10T12:00:00Z');
const hace = (dias: number): string => new Date(ahora.getTime() - dias * 86_400_000).toISOString();

const todoBien: DatosRevision = {
  me: { mfa_enabled: true, recovery_email: 'otra@example.eu' },
  passkeys: [],
  sesiones: [{ current: true }],
  dispositivos: [{ trusted: true, last_seen: hace(3) }],
  contrasenasApp: [{ created_at: hace(400), last_used_at: hace(10) }],
  ahora,
};

const estados = (d: DatosRevision) =>
  Object.fromEntries(revisionDeCuenta(d).map((p) => [p.clave, [p.estado, p.cuantos]]));

describe('revisionDeCuenta', () => {
  test('todo en orden: cinco puntos y ninguno en aviso', () => {
    const r = revisionDeCuenta(todoBien);
    expect(r.map((p) => p.clave)).toEqual([
      'dos_pasos',
      'recuperacion',
      'sesiones',
      'contrasenas_app',
      'dispositivos',
    ]);
    expect(r.every((p) => p.estado === 'ok')).toBe(true);
  });

  test('sin verificación ni llave: aviso que lleva a #seguridad; una passkey basta', () => {
    const sin = revisionDeCuenta({ ...todoBien, me: { ...todoBien.me, mfa_enabled: false } });
    expect(sin[0]).toEqual({
      clave: 'dos_pasos',
      estado: 'aviso',
      ancla: '#seguridad',
      cuantos: 1,
    });
    const conLlave = estados({
      ...todoBien,
      me: { ...todoBien.me, mfa_enabled: false },
      passkeys: [{ id: 'k' }],
    });
    expect(conLlave.dos_pasos).toEqual(['ok', 0]);
  });

  test('sin correo ni móvil de recuperación: aviso; con solo móvil, bien', () => {
    expect(
      estados({ ...todoBien, me: { mfa_enabled: true, recovery_email: ' ' } }).recuperacion,
    ).toEqual(['aviso', 1]);
    expect(
      estados({ ...todoBien, me: { mfa_enabled: true, phone: '+34600000000' } }).recuperacion,
    ).toEqual(['ok', 0]);
  });

  test('cuenta las sesiones de otros equipos, las contraseñas sin uso y los dispositivos dormidos', () => {
    const e = estados({
      ...todoBien,
      sesiones: [{ current: true }, { current: false }, {}],
      contrasenasApp: [
        { created_at: hace(200), last_used_at: hace(91) },
        { created_at: hace(120), last_used_at: null },
        { created_at: hace(5), last_used_at: null },
      ],
      dispositivos: [
        { trusted: true, last_seen: hace(100) },
        { trusted: true, current: true, last_seen: hace(100) },
        { trusted: false, last_seen: hace(300) },
      ],
    });
    expect(e.sesiones).toEqual(['aviso', 2]);
    expect(e.contrasenas_app).toEqual(['aviso', 2]);
    expect(e.dispositivos).toEqual(['aviso', 1]);
  });

  test('lo que no ha llegado no sale: sin datos, sin avisos inventados', () => {
    expect(revisionDeCuenta({ ahora })).toEqual([]);
    expect(
      revisionDeCuenta({ ...todoBien, me: null, dispositivos: null }).map((p) => p.clave),
    ).toEqual(['sesiones', 'contrasenas_app']);
    // Sin verificación y sin saber de las passkeys: no se avisa (podría tener llave)…
    expect(
      estados({ ...todoBien, me: { ...todoBien.me, mfa_enabled: false }, passkeys: null })
        .dos_pasos,
    ).toBeUndefined();
    // …pero con la verificación encendida las passkeys dan igual.
    expect(estados({ ...todoBien, passkeys: null }).dos_pasos).toEqual(['ok', 0]);
  });
});

describe('primerosPasos', () => {
  const lista: DatosPrimerosPasos = {
    me: { mfa_enabled: true, recovery_email: 'otra@example.eu', has_avatar: true },
    passkeys: [],
    contrasenasApp: [{ id: 'thunderbird' }],
    instalada: true,
  };
  const claves = (d: DatosPrimerosPasos) => primerosPasos(d).map((p) => p.clave);

  test('con verificación, recuperación, foto, un dispositivo e instalada: no queda nada', () => {
    expect(primerosPasos(lista)).toEqual([]);
  });

  test('cuenta recién creada: los cinco, en orden y cada uno con dónde se arregla', () => {
    expect(
      primerosPasos({
        me: { mfa_enabled: false, has_avatar: false },
        passkeys: [],
        contrasenasApp: [],
        instalada: false,
      }),
    ).toEqual([
      { clave: 'dos_pasos', destino: 'cuenta', ruta: '#seguridad' },
      { clave: 'recuperacion', destino: 'cuenta', ruta: '#recuperacion' },
      { clave: 'foto', destino: 'cuenta', ruta: '#perfil' },
      { clave: 'dispositivos', destino: 'portal', ruta: '/dispositivos' },
      { clave: 'instalar', destino: 'portal', ruta: '/descargas' },
    ]);
  });

  test('el criterio de la verificación es el de la revisión: una passkey la cumple', () => {
    const sinCodigo = { ...lista, me: { ...lista.me, mfa_enabled: false } };
    expect(claves(sinCodigo)).toEqual(['dos_pasos']);
    expect(claves({ ...sinCodigo, passkeys: [{ id: 'k' }] })).toEqual([]);
    // Con solo el móvil de recuperación también está cumplida.
    expect(
      claves({ ...lista, me: { mfa_enabled: true, phone: '+34600000000', has_avatar: true } }),
    ).toEqual([]);
  });

  test('lo que no ha llegado no pide nada: ni /me, ni passkeys, ni contraseñas', () => {
    expect(claves({ me: null, passkeys: null, contrasenasApp: null, instalada: true })).toEqual([]);
    // Sin saber de las passkeys no se pide la verificación (podría tener llave).
    expect(claves({ ...lista, me: { ...lista.me, mfa_enabled: false }, passkeys: null })).toEqual(
      [],
    );
    // Un /me sin `has_avatar` no es «sin foto».
    expect(claves({ ...lista, me: { mfa_enabled: true, recovery_email: 'a@b.eu' } })).toEqual([]);
  });

  test('foto, dispositivos e instalar desaparecen cada uno al cumplirse', () => {
    const pendiente: DatosPrimerosPasos = {
      ...lista,
      me: { ...lista.me, has_avatar: false },
      contrasenasApp: [],
      instalada: false,
    };
    expect(claves(pendiente)).toEqual(['foto', 'dispositivos', 'instalar']);
    expect(claves({ ...pendiente, contrasenasApp: [{ id: 'x' }] })).toEqual(['foto', 'instalar']);
    expect(claves({ ...pendiente, instalada: true })).toEqual(['foto', 'dispositivos']);
  });

  test('con verificación, recuperación y foto no hay tarjeta, aunque falte conectar o instalar', () => {
    expect(claves({ ...lista, contrasenasApp: [], instalada: false })).toEqual([]);
    expect(claves({ ...lista, me: null, contrasenasApp: [], instalada: false })).toEqual([]);
  });
});

/*
 * Los vectores compartidos con el espejo Go del Hub nativo
 * (apps/nucleo/api/revision_cuenta_test.go): la misma entrada tiene que dar la misma
 * salida en los dos lados, o la tarjeta de la portada y la del Hub dirían cosas
 * distintas a la misma persona.
 */
describe('vectores compartidos con el Hub nativo', () => {
  test.each(vectores.revision.map((c) => [c.nombre, c] as const))('revisión: %s', (_, c) => {
    const { ahora, ...datos } = c.datos;
    expect(
      revisionDeCuenta({ ...(datos as Omit<DatosRevision, 'ahora'>), ahora: new Date(ahora) }),
    ).toEqual(c.esperado as ReturnType<typeof revisionDeCuenta>);
  });

  test.each(vectores.primerosPasos.map((c) => [c.nombre, c] as const))(
    'primeros pasos: %s',
    (_, c) => {
      expect(primerosPasos(c.datos as DatosPrimerosPasos)).toEqual(
        c.esperado as ReturnType<typeof primerosPasos>,
      );
    },
  );
});
