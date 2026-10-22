/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { setRoomsMatrix, RoomsMatrix } from './client.js';
import {
  captureMatrixScope,
  assertMatrixScope,
  clearMatrixMessages,
  decryptMessage,
  encryptedFiles,
  knownMessage,
  rememberUpload,
} from './messages.js';
import type { Attachment, ChatMessage } from '../../lib/chat.js';
import { uploadAttachment } from '../chatApi.js';

const attachment: Attachment = {
  fileId: 'mxc://matrix.sytue.eu/opaque',
  name: 'private.txt',
  contentType: 'text/plain',
  sizeBytes: 7,
  matrixFile: {
    url: 'mxc://matrix.sytue.eu/opaque',
    key: { kty: 'oct', k: 'fixture' },
    iv: 'fixture',
    hashes: { sha256: 'fixture' },
    v: 'v2',
  },
};
const message = {
  id: 'message-one',
  channelId: 'channel-one',
  body: '',
  author: { id: 'author-one' },
  matrix_event: { room_id: '!one:matrix.sytue.eu', event_id: '$one' },
} as ChatMessage;

function client(
  read?: RoomsMatrix['read'],
  releaseAttachment?: RoomsMatrix['releaseAttachment'],
): RoomsMatrix {
  return {
    read,
    releaseAttachment,
    bootstrap: {
      channels: [{ channel_id: 'channel-one', matrix_room_id: '!one:matrix.sytue.eu' }],
    },
  } as RoomsMatrix;
}
const receipt = { room: '!one:matrix.sytue.eu', file: attachment };

afterEach(() => {
  clearMatrixMessages();
  setRoomsMatrix(null);
});

test('attachment descriptors can only be used in their original channel and Matrix client', () => {
  setRoomsMatrix(client());
  rememberUpload(captureMatrixScope(), 'channel-one', receipt);
  expect(encryptedFiles('channel-one', [attachment.fileId])).toEqual({
    attachments: [attachment],
    room: receipt.room,
  });
  expect(() => encryptedFiles('channel-two', [attachment.fileId])).toThrow('matrix_reference');
  setRoomsMatrix(client());
  expect(() => encryptedFiles('channel-one', [attachment.fileId])).toThrow('matrix_identity');
});

test('a late upload cannot refill the cache after logout, including reuse of the same client', () => {
  const released: string[] = [];
  const original = client(undefined, (url) => released.push(url));
  setRoomsMatrix(original);
  const scope = captureMatrixScope();
  clearMatrixMessages();
  setRoomsMatrix(null);
  setRoomsMatrix(original);
  expect(() =>
    rememberUpload(scope, 'channel-one', {
      ...receipt,
      file: { ...attachment, objectUrl: 'blob:late' },
    }),
  ).toThrow('matrix_identity');
  expect(released).toEqual(['blob:late']);
  expect(() => encryptedFiles('channel-one', [attachment.fileId])).toThrow('matrix_reference');
});

test('the uploader keeps the client and channel captured before asynchronous I/O', async () => {
  let complete!: (receipt: Awaited<ReturnType<RoomsMatrix['upload']>>) => void;
  let destination = '';
  const original = client();
  original.upload = (channel) => {
    destination = channel;
    return new Promise((resolve) => {
      complete = resolve;
    });
  };
  setRoomsMatrix(original);
  const pending = uploadAttachment(new File(['private'], 'private.txt'), {
    channelId: 'channel-one',
  });
  clearMatrixMessages();
  setRoomsMatrix(client());
  complete(receipt);
  await expect(pending).rejects.toThrow('matrix_identity');
  expect(destination).toBe('channel-one');
  expect(() => encryptedFiles('channel-one', [attachment.fileId])).toThrow('matrix_reference');
});

test('an upload receipt is invalid when the channel is rebound to a different encrypted room', () => {
  const original = client();
  setRoomsMatrix(original);
  const scope = captureMatrixScope();
  rememberUpload(scope, 'channel-one', receipt);
  original.bootstrap.channels[0]!.matrix_room_id = '!replacement:matrix.sytue.eu';
  expect(() => encryptedFiles('channel-one', [attachment.fileId])).toThrow('matrix_reference');
  expect(() => rememberUpload(scope, 'channel-one', receipt)).toThrow('matrix_reference');
});

test('the final sender rejects a changed room binding before publishing an attachment key', async () => {
  const instance = Object.create(RoomsMatrix.prototype) as RoomsMatrix;
  let sent = 0;
  Object.assign(instance, {
    room: async () => '!replacement:matrix.sytue.eu',
    client: {
      sendEvent: async () => {
        sent++;
        return { event_id: '$new' };
      },
    },
    closed: false,
  });
  await expect(
    instance.send(
      'channel-one',
      { body: '', attachments: [attachment] },
      'tx',
      undefined,
      receipt.room,
    ),
  ).rejects.toThrow('matrix_reference');
  expect(sent).toBe(0);
  // The identical attachment still sends to the intended encrypted room.
  Object.assign(instance, { room: async () => receipt.room });
  await instance.send(
    'channel-one',
    { body: '', attachments: [attachment] },
    'tx',
    undefined,
    receipt.room,
  );
  expect(sent).toBe(1);
});

test('a delayed decryption cannot restore another account’s message or attachment keys', async () => {
  let complete!: (content: { body: string; attachments: Attachment[] }) => void;
  setRoomsMatrix(
    client(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    ),
  );
  const pending = decryptMessage(message);
  clearMatrixMessages();
  setRoomsMatrix(client());
  complete({ body: 'private', attachments: [attachment] });
  await expect(pending).rejects.toThrow('matrix_identity');
  expect(() => knownMessage(message.id)).toThrow('matrix_reference');
});

test('normal decryption remains available only to the same client and cache generation', async () => {
  setRoomsMatrix(client(async () => ({ body: 'private', attachments: [attachment] })));
  const decrypted = await decryptMessage(message);
  expect(knownMessage(message.id)).toEqual(decrypted);
  setRoomsMatrix(client());
  expect(() => knownMessage(message.id)).toThrow('matrix_identity');
});

test('a media response received after closing the client cannot produce a plaintext object URL', async () => {
  const previousFetch = globalThis.fetch;
  let complete!: (response: Response) => void;
  const instance = Object.create(RoomsMatrix.prototype) as RoomsMatrix;
  Object.assign(instance, {
    bootstrap: { homeserver_url: 'https://matrix.sytue.eu' },
    client: {
      mxcUrlToHttp: () => 'https://matrix.sytue.eu/_matrix/client/v1/media/download/opaque',
      getAccessToken: () => 'artificial-test-token',
    },
    closed: false,
  });
  globalThis.fetch = (() =>
    new Promise((resolve) => {
      complete = resolve;
    })) as unknown as typeof fetch;
  try {
    const pending = instance.download(attachment);
    Object.assign(instance, { closed: true });
    complete(new Response('ciphertext'));
    await expect(pending).rejects.toThrow('matrix_device');
  } finally {
    globalThis.fetch = previousFetch;
  }
});

// Provider meeting writes can finish after logout; their encrypted share scope must not.
test('a meeting sender scope rejects logout generation even if the Matrix client is reused', () => {
  const original = client();
  setRoomsMatrix(original);
  const scope = captureMatrixScope();
  assertMatrixScope(scope);
  clearMatrixMessages();
  setRoomsMatrix(original);
  expect(() => assertMatrixScope(scope)).toThrow('matrix_identity');
});
