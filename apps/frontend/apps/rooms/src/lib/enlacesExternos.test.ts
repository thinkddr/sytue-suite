/**
 * Tests de enlacesExternos — reconocer reuniones de otros proveedores y componer
 * los enlaces de compartir. Lo que puede salir mal es abrir en una pestaña nueva
 * un enlace que NO es de un proveedor (phishing con un host parecido), o
 * componer un `wa.me` sin codificar y perder la mitad del texto en el primer «&».
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { enlaceParaCompartir, proveedorDeReunion } from './enlacesExternos.js';

test('reconoce los cuatro proveedores por su host y su forma', () => {
  expect(proveedorDeReunion('https://us02web.zoom.us/j/1234567890?pwd=abc')).toBe('zoom');
  expect(proveedorDeReunion('https://zoom.us/my/sala')).toBe('zoom');
  expect(proveedorDeReunion('https://meet.google.com/abc-defg-hij')).toBe('meet');
  expect(
    proveedorDeReunion(
      'https://teams.microsoft.com/l/meetup-join/19%3ameeting_x%40thread.v2/0?context=%7b%7d',
    ),
  ).toBe('teams');
  expect(proveedorDeReunion('https://teams.live.com/meet/9876543210')).toBe('teams');
  expect(proveedorDeReunion('https://acme.webex.com/meet/ana')).toBe('webex');
  expect(proveedorDeReunion('https://acme.webex.com/acme/j.php?MTID=m1')).toBe('webex');
});

test('un host parecido, otro esquema o una ruta que no es de reunión NO se abre', () => {
  expect(proveedorDeReunion('https://zoom.us.evil.com/j/1')).toBeNull();
  expect(proveedorDeReunion('https://meet.google.com/landing')).toBeNull();
  expect(proveedorDeReunion('http://meet.google.com/abc-defg-hij')).toBeNull();
  expect(proveedorDeReunion('https://teams.microsoft.com/')).toBeNull();
  expect(proveedorDeReunion('rooms.sytue.eu/?rooms=1')).toBeNull();
  expect(proveedorDeReunion('')).toBeNull();
});

test('los enlaces de compartir codifican el texto y la URL', () => {
  const url = 'https://rooms.sytue.eu/?rooms=abc';
  expect(enlaceParaCompartir('whatsapp', 'Únete & ven', url)).toBe(
    'https://wa.me/?text=' + encodeURIComponent('Únete & ven https://rooms.sytue.eu/?rooms=abc'),
  );
  expect(enlaceParaCompartir('email', 'Hola', url, 'Reunión')).toBe(
    'mailto:?subject=Reuni%C3%B3n&body=' +
      encodeURIComponent('Hola\nhttps://rooms.sytue.eu/?rooms=abc'),
  );
  expect(enlaceParaCompartir('teams', 'Hola', url)).toContain('teams.microsoft.com/share?href=');
  expect(enlaceParaCompartir('teams', 'Hola', url)).not.toContain('?rooms=abc');
});
