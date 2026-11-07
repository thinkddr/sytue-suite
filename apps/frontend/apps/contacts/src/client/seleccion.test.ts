/** «Enviar correo a todos»: qué correos van al redactor de Mail y cómo. */
import { expect, test } from 'bun:test';
import type { Contact } from '@sytue/api-clients';
import { correosDe, enlaceCorreoATodos, TOPE_DESTINATARIOS } from './seleccion.js';

function contacto(uid: string, vcardEmails: string[], emails: string[] = []): Contact {
  const vcard = ['BEGIN:VCARD', 'VERSION:4.0', `FN:${uid}`, ...vcardEmails, 'END:VCARD'].join(
    '\r\n',
  );
  return { href: `/${uid}`, uid, fullName: uid, emails, phones: [], vcard };
}

test('el principal de cada uno, sin repetir y sin quien no tiene correo', () => {
  const ana = contacto('ana', [
    'EMAIL;TYPE=home:ana@casa.eu',
    'EMAIL;TYPE=work;PREF=1:ana@obra.eu',
  ]);
  const ana2 = contacto('ana2', ['EMAIL:ANA@obra.eu']);
  const luis = contacto('luis', [], ['luis@x.eu']);
  const sin = contacto('sin', []);
  expect(correosDe([ana, ana2, luis, sin])).toEqual({
    correos: ['ana@obra.eu', 'luis@x.eu'],
    recortado: false,
  });
});

test('hasta el tope, y lo dice', () => {
  const muchos = Array.from({ length: TOPE_DESTINATARIOS + 5 }, (_, i) =>
    contacto(`c${i}`, [`EMAIL:c${i}@x.eu`]),
  );
  const { correos, recortado } = correosDe(muchos);
  expect(correos.length).toBe(TOPE_DESTINATARIOS);
  expect(recortado).toBe(true);
});

test('el enlace abre el redactor con todos en «Para»', () => {
  const u = new URL(enlaceCorreoATodos('https://acme.sytue.eu/mail', ['a@x.eu', 'b+1@x.eu']));
  expect(u.pathname).toBe('/mail/');
  expect(u.searchParams.get('action')).toBe('compose');
  expect(u.searchParams.get('to')).toBe('a@x.eu, b+1@x.eu');
});
