/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import {
  descargarImagenRemota,
  esIpPrivadaOReservada,
  neutralizarProxyEnCuerpo,
  reescribirImagenesRemotas,
  validarUrlImagenRemota,
} from './remoteImage.js';

describe('esIpPrivadaOReservada y validarUrlImagenRemota (anti-SSRF)', () => {
  test('bloquea loopback, privadas RFC1918, metadata cloud, CGNAT e IPv6 locales/mapeadas', () => {
    for (const ip of [
      '127.0.0.1',
      '10.0.0.5',
      '172.16.0.1',
      '172.31.255.254',
      '192.168.1.1',
      '169.254.169.254',
      '100.64.0.1',
      '0.0.0.0',
      '::1',
      '::',
      'fe80::1',
      'fc00::1',
      'fd12:3456::1',
      '::ffff:127.0.0.1',
      '::ffff:169.254.169.254',
      '::ffff:7f00:0001',
    ]) {
      expect(esIpPrivadaOReservada(ip), `debería bloquear ${ip}`).toBe(true);
    }
    expect(esIpPrivadaOReservada('93.184.216.34')).toBe(false);
    expect(esIpPrivadaOReservada('2606:2800:220:1:248:1893:25c8:1946')).toBe(false);
  });

  test('rechaza esquemas no https, puertos distintos de 443 y dominios que resuelven a IP privada', async () => {
    await expect(
      validarUrlImagenRemota('http://cdn.ejemplo.com/a.png', async () => ['93.184.216.34']),
    ).rejects.toThrow('https_required');
    await expect(
      validarUrlImagenRemota('https://cdn.ejemplo.com:8443/a.png', async () => ['93.184.216.34']),
    ).rejects.toThrow('port_not_allowed');
    await expect(
      validarUrlImagenRemota('https://interno.svc.cluster.local/a.png', async () => [
        '93.184.216.34',
      ]),
    ).rejects.toThrow('ssrf_blocked');
    await expect(
      validarUrlImagenRemota('https://rebind.ejemplo.com/a.png', async () => [
        '93.184.216.34',
        '169.254.169.254',
      ]),
    ).rejects.toThrow('ssrf_blocked');
  });
});

describe('descargarImagenRemota', () => {
  const dnsPublico = async () => ['93.184.216.34'] as const;

  test('sirve una imagen rasterizada dentro del tope y rechaza SVG, HTML o exceso de tamaño', async () => {
    const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47]);
    const okRes = await descargarImagenRemota('https://cdn.ejemplo.com/logo.png', {
      resolverDns: dnsPublico,
      fetchImpl: (async () =>
        new Response(png, {
          status: 200,
          headers: { 'content-type': 'image/png; charset=binary' },
        })) as unknown as typeof fetch,
    });
    expect(okRes).toEqual({ ok: true, bytes: png, contentType: 'image/png' });

    const svgRes = await descargarImagenRemota('https://cdn.ejemplo.com/xss.svg', {
      resolverDns: dnsPublico,
      fetchImpl: (async () =>
        new Response('<svg onload="alert(1)"/>', {
          status: 200,
          headers: { 'content-type': 'image/svg+xml' },
        })) as unknown as typeof fetch,
    });
    expect(svgRes).toEqual({ ok: false, status: 415, error: 'unsupported_media_type' });

    const grandeRes = await descargarImagenRemota('https://cdn.ejemplo.com/huge.png', {
      resolverDns: dnsPublico,
      topeBytes: 8,
      fetchImpl: (async () =>
        new Response(new Uint8Array(16), {
          status: 200,
          headers: { 'content-type': 'image/png' },
        })) as unknown as typeof fetch,
    });
    expect(grandeRes).toEqual({ ok: false, status: 413, error: 'image_too_large' });
  });

  test('revalida cada salto de redirección contra SSRF', async () => {
    const redirRes = await descargarImagenRemota('https://cdn.ejemplo.com/salto.png', {
      resolverDns: async (host) =>
        host === 'metadata.interno.ejemplo.com' ? ['169.254.169.254'] : ['93.184.216.34'],
      fetchImpl: (async () =>
        new Response(null, {
          status: 302,
          headers: { location: 'https://metadata.interno.ejemplo.com/latest/meta-data/' },
        })) as unknown as typeof fetch,
    });
    expect(redirRes).toEqual({ ok: false, status: 403, error: 'ssrf_blocked' });
  });
});

describe('reescribirImagenesRemotas y neutralizarProxyEnCuerpo', () => {
  test('reescribe src, background, poster, srcset y url() hacia el proxy y neutraliza un proxy preinyectado', () => {
    const crudo =
      '<img src="/mail/api/remote-image?url=https://tracker.evil/1.png">' +
      '<img src="https://cdn.ejemplo.com/a.png" srcset="https://cdn.ejemplo.com/a-2x.png 2x">' +
      '<div style="background-image: url(https://cdn.ejemplo.com/bg.webp)"></div>';
    const seguro = neutralizarProxyEnCuerpo(crudo);
    expect(seguro).not.toContain('/api/remote-image');

    const reescrito = reescribirImagenesRemotas(
      seguro,
      (u) => `/mail/api/remote-image?url=${encodeURIComponent(u)}`,
    );
    expect(reescrito).toContain(
      'src="/mail/api/remote-image?url=https%3A%2F%2Fcdn.ejemplo.com%2Fa.png"',
    );
    expect(reescrito).toContain(
      'srcset="/mail/api/remote-image?url=https%3A%2F%2Fcdn.ejemplo.com%2Fa-2x.png 2x"',
    );
    expect(reescrito).toContain(
      'url("/mail/api/remote-image?url=https%3A%2F%2Fcdn.ejemplo.com%2Fbg.webp")',
    );
  });
});
