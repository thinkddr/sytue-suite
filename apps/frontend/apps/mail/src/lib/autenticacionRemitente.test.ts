/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import { detalleRemitente, resumenRemitente } from './autenticacionRemitente.js';
import { toEmailSummary } from './mailrest.js';

describe('resumenRemitente', () => {
  test('DMARC pass es lo único que da «Verificado»', () => {
    expect(resumenRemitente({ spf: 'pass', dkim: 'pass', dmarc: 'pass' })).toBe('verificado');
    expect(resumenRemitente({ spf: 'pass', dkim: 'pass' })).toBeNull();
  });
  test('DMARC fail avisa aunque SPF pase', () => {
    expect(resumenRemitente({ spf: 'pass', dmarc: 'fail' })).toBe('aviso');
  });
  test('sin DMARC, SPF fallido y sin firma DKIM avisa', () => {
    expect(resumenRemitente({ spf: 'softfail' })).toBe('aviso');
    expect(resumenRemitente({ spf: 'fail', dkim: 'fail' })).toBe('aviso');
    expect(resumenRemitente({ spf: 'fail', dkim: 'pass' })).toBeNull();
  });
  test('sin veredictos no dice nada', () => {
    expect(resumenRemitente(undefined)).toBeNull();
    expect(resumenRemitente({})).toBeNull();
  });
  test('el detalle nombra solo lo evaluado', () => {
    expect(detalleRemitente({ spf: 'pass', dmarc: 'fail' })).toBe('SPF pass · DMARC fail');
  });
});

describe('toEmailSummary', () => {
  test('lleva los auth_* del sobre a senderAuth', () => {
    const e = toEmailSummary({ id: 'm1', auth_spf: 'pass', auth_dmarc: 'pass' });
    expect(e.senderAuth).toEqual({ spf: 'pass', dmarc: 'pass' });
    expect(toEmailSummary({ id: 'm2' }).senderAuth).toBeUndefined();
  });
});
