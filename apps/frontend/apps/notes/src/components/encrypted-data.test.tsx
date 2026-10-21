import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard } from './NoteCard.js';

const hosts: HTMLElement[] = [];
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});
function note(extra: Partial<Note>): Note {
  return {
    id: 'n',
    notebook_id: 'nb',
    owner_sub: 'owner',
    kind: 'text',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    visibility: 'personal',
    indexable: false,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
    ...extra,
  };
}
function card(
  note: Note,
  props: {
    bloqueado?: boolean;
    claro?: { title: string; content: string; data?: Record<string, unknown> };
  },
) {
  const host = document.createElement('div');
  hosts.push(host);
  document.body.append(host);
  render(
    <NoteCard
      t={((k: string) => k) as never}
      note={note}
      seleccionada={false}
      onSelect={() => {}}
      onChange={() => {}}
      onDelete={() => {}}
      onDuplicate={() => {}}
      onTogglePin={() => {}}
      onShare={() => {}}
      onTags={() => {}}
      onBacklinks={() => {}}
      {...props}
    />,
    host,
  );
  return host;
}
test('decrypted Data renders attachments from envelope, not an external plaintext copy', () => {
  const raw = note({
    id: 'n',
    kind: 'text',
    content_cipher: 'encv1:cipher',
    data: { client_id: 'opaque' },
  });
  const host = card(raw, {
    claro: {
      title: 'private title',
      content: '<p>private body</p>',
      data: {
        attachments: [
          {
            id: 'a',
            name: 'private.pdf',
            size: 4,
            type: 'application/pdf',
            dataUrl: 'data:application/pdf;base64,c2VjcmV0',
          },
        ],
      },
    },
  });
  expect(host.textContent).toContain('private.pdf');
  expect(host.textContent).toContain('private title');
  expect(raw.data).toEqual({ client_id: 'opaque' });
});
test('locked legacy notebook hides title, body and auxiliary data without deleting originals', () => {
  const raw = note({
    id: 'legacy',
    kind: 'text',
    title: 'historic title',
    content: '<p>historic body</p>',
    data: {
      attachments: [
        { id: 'a', name: 'historic.pdf', size: 4, type: 'application/pdf', dataUrl: 'data:legacy' },
      ],
    },
  });
  const host = card(raw, { bloqueado: true });
  expect(host.textContent).not.toContain('historic');
  expect(host.innerHTML).not.toContain('data:legacy');
  expect(raw.title).toBe('historic title');
  expect(raw.data?.attachments).toHaveLength(1);
});

test('decrypted attachment Data cannot introduce an executable link', () => {
  const raw = note({ id: 'n', kind: 'text', content_cipher: 'encv1:cipher', data: {} });
  const host = card(raw, {
    claro: {
      title: 't',
      content: 'body',
      data: {
        attachments: [
          { id: 'a', name: 'bad.txt', size: 4, type: 'text/plain', dataUrl: 'javascript:alert(1)' },
        ],
      },
    },
  });
  expect(host.querySelector('a[href^="javascript:"]')).toBeNull();
  expect(host.textContent).toContain('bad.txt');
});
