import { describe, expect, it } from 'bun:test';
import {
  calDavUrl,
  toClient,
  toSharedClient,
  type RestCalendar,
  type SharedRestCalendar,
} from './restCalendar.js';

describe('restCalendar.toClient', () => {
  it('mapea un calendario normal (editable, sin suscripción)', () => {
    const rest: RestCalendar = {
      id: 'cal-1',
      name: 'trabajo',
      display_name: 'Trabajo',
      color: '#1E50A8',
      is_default: true,
    };
    const c = toClient('user-1', rest);
    expect(c.url).toBe('/dav/calendars/user-1/trabajo/');
    expect(c.readOnly).toBe(false);
    expect(c.isDefault).toBe(true);
    expect(c.subscription).toBeUndefined();
  });

  it('mapea una suscripción ICS de solo lectura con su estado', () => {
    const rest: RestCalendar = {
      id: 'cal-2',
      name: 'ics-festivos',
      display_name: 'Festivos',
      color: '#0F7052',
      is_default: false,
      read_only: true,
      subscription: {
        url: 'https://example.com/holidays.ics',
        last_status: 'ok',
        last_fetched_at: '2026-07-18T10:00:00Z',
      },
    };
    const c = toClient('user-1', rest);
    expect(c.readOnly).toBe(true);
    expect(c.subscription?.url).toBe('https://example.com/holidays.ics');
    expect(c.subscription?.lastStatus).toBe('ok');
    expect(c.subscription?.lastFetchedAt).toBe('2026-07-18T10:00:00Z');
  });

  it('escapa el sub en la ruta CalDAV (urlSafe: "/" y espacio -> "_")', () => {
    expect(calDavUrl('a/b c', 'x')).toBe('/dav/calendars/a_b_c/x/');
  });
});

describe('restCalendar.toSharedClient', () => {
  const base: SharedRestCalendar = {
    id: 'cal-9',
    owner_sub: 'owner-9',
    name: 'trabajo',
    display_name: 'Trabajo de Ana',
    color: '#6B3E9E',
    permission: 'reader',
    shared_by: 'owner-9',
  };

  it('un share «reader» es readOnly y usa la URL del DUEÑO', () => {
    const c = toSharedClient(base);
    // La coleccion apunta al sub del dueño, no al del usuario que accede.
    expect(c.url).toBe('/dav/calendars/owner-9/trabajo/');
    expect(c.readOnly).toBe(true); // reader → no editable (bloquea arrastrar/editar)
    expect(c.shared).toBe(true);
    expect(c.permission).toBe('reader');
    expect(c.sharedBy).toBe('owner-9');
    expect(c.isDefault).toBe(false);
  });

  it('un share «writer» es editable (readOnly=false)', () => {
    const c = toSharedClient({ ...base, permission: 'writer' });
    expect(c.readOnly).toBe(false);
    expect(c.permission).toBe('writer');
  });

  it('un calendario de suscripcion compartido sigue siendo readOnly aun como writer', () => {
    const c = toSharedClient({ ...base, permission: 'writer', read_only: true });
    expect(c.readOnly).toBe(true);
  });
});
