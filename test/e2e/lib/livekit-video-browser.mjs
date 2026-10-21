/* SPDX-License-Identifier: Apache-2.0 */
import { Room, RoomEvent, ExternalE2EEKeyProvider, setLogLevel } from '/livekit-client.mjs';
setLogLevel('silent');
const bytes = (value) => Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
const base64 = (value) => {
  let result = '';
  for (const byte of value) result += String.fromCharCode(byte);
  return btoa(result);
};
function within(promise, label) {
  let timer;
  return Promise.race([
    promise,
    new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(label)), 12000);
    }),
  ]).finally(() => clearTimeout(timer));
}
async function gather(peer) {
  if (peer.iceGatheringState === 'complete') return;
  await within(
    new Promise((resolve) => {
      const changed = () => {
        if (peer.iceGatheringState === 'complete') {
          peer.removeEventListener('icegatheringstatechange', changed);
          resolve();
        }
      };
      peer.addEventListener('icegatheringstatechange', changed);
      changed();
    }),
    'Local ICE gathering did not complete',
  );
}
async function transform(fixture, source, operation, tampering) {
  const expected = fixture.frames.filter(
    (frame) => frame.source === source && (!tampering || frame.type === 'key'),
  );
  if (expected.length === 0) throw new Error('Missing artificial VP8 frames');
  const worker = new Worker('/livekit-worker.mjs', { type: 'module' });
  const provider = new ExternalE2EEKeyProvider();
  const room = new Room({
    disconnectOnPageLeave: false,
    encryption: { keyProvider: provider, worker },
  });
  const identity = 'ARTIFICIAL-VIDEO-INTEROP';
  room.localParticipant.identity = identity;
  const local = new RTCPeerConnection({ encodedInsertableStreams: true });
  const remote = new RTCPeerConnection();
  const canvas = document.createElement('canvas');
  canvas.width = source === 'camera' ? 640 : 1280;
  canvas.height = source === 'camera' ? 360 : 720;
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas unavailable');
  let tick = 0;
  const draw = () => {
    context.fillStyle = `rgb(${tick++ % 255},70,120)`;
    context.fillRect(0, 0, canvas.width, canvas.height);
  };
  draw();
  const stream = canvas.captureStream(10);
  const interval = setInterval(draw, 100);
  try {
    await provider.setKey(fixture.key_b64);
    const enabled = within(
      new Promise((resolve, reject) => {
        room.once(RoomEvent.ParticipantEncryptionStatusChanged, (value) =>
          value ? resolve() : reject(new Error('Worker encryption was disabled')),
        );
      }),
      'Worker did not become ready',
    );
    await room.setE2EEEnabled(true);
    await enabled;
    const sender = local.addTrack(stream.getVideoTracks()[0], stream);
    const transceiver = local.getTransceivers()[0];
    transceiver.setCodecPreferences(
      RTCRtpSender.getCapabilities('video').codecs.filter(
        (codec) => codec.mimeType.toLowerCase() === 'video/vp8',
      ),
    );
    const encoded = sender.createEncodedStreams();
    const seen = new Set();
    let finish;
    const completed = within(
      new Promise((resolve, reject) => {
        finish = { resolve, reject };
      }),
      'Worker VP8 frame did not complete',
    );
    const output = [];
    const selected = encoded.readable.pipeThrough(
      new TransformStream({
        transform(frame, controller) {
          const wanted = expected.find(
            (entry) => entry.type === frame.type && !seen.has(entry.type),
          );
          if (!wanted) return;
          seen.add(wanted.type);
          const data = bytes(operation === 'encode' ? wanted.plaintext : wanted.ciphertext);
          if (tampering === 'header') data[0] ^= 1;
          if (tampering === 'iv') data[data.length - 3] ^= 1;
          // Only replace bytes on a real RTCEncodedVideoFrame. No encryption is reimplemented here.
          frame.data = data.buffer;
          controller.enqueue(frame);
        },
      }),
    );
    const collected = new WritableStream({
      write(frame) {
        if (tampering) {
          finish.reject(new Error('Modified VP8 ciphertext was accepted'));
          return;
        }
        output.push({ type: frame.type, data: base64(new Uint8Array(frame.data)) });
        if (output.length === expected.length) finish.resolve(output);
      },
    });
    worker.addEventListener('message', ({ data }) => {
      if (data.kind !== 'error') return;
      if (tampering) finish.resolve([]);
      else finish.reject(new Error('VP8 worker rejected a valid frame'));
    });
    worker.addEventListener('error', () => finish.reject(new Error('VP8 worker crashed')));
    worker.postMessage(
      {
        kind: operation,
        data: {
          participantIdentity: identity,
          trackId: `artificial-${source}`,
          readableStream: selected,
          writableStream: collected,
          codec: 'vp8',
          hasPacketTrailer: false,
        },
      },
      [selected, collected],
    );
    await local.setLocalDescription(await local.createOffer());
    await gather(local);
    await remote.setRemoteDescription(local.localDescription);
    await remote.setLocalDescription(await remote.createAnswer());
    await gather(remote);
    await local.setRemoteDescription(remote.localDescription);
    return await completed;
  } finally {
    clearInterval(interval);
    for (const track of stream.getTracks()) track.stop();
    local.close();
    remote.close();
    worker.terminate();
    await room.disconnect();
  }
}
window.runLiveKitVideoInterop = async (fixture) => {
  if (!fixture.artificial || fixture.frames.length !== 4)
    throw new Error('Artificial four-frame fixture required');
  const frames = [];
  for (const source of ['camera', 'screen']) {
    const decoded = await transform(fixture, source, 'decode');
    const encoded = await transform(fixture, source, 'encode');
    for (const expected of fixture.frames.filter((frame) => frame.source === source)) {
      if (decoded.find((frame) => frame.type === expected.type)?.data !== expected.plaintext)
        throw new Error('Go-to-JS VP8 bytes differ');
      const ciphertext = encoded.find((frame) => frame.type === expected.type)?.data;
      if (!ciphertext || ciphertext === expected.plaintext)
        throw new Error('JS VP8 output is not encrypted');
      frames.push({ source, type: expected.type, plaintext: expected.plaintext, ciphertext });
    }
  }
  await transform(fixture, 'camera', 'decode', 'header');
  await transform(fixture, 'camera', 'decode', 'iv');
  return {
    artificial: true,
    generator: 'livekit-client 2.22.3 official Worker and real RTCEncodedVideoFrame',
    key_b64: fixture.key_b64,
    frames,
  };
};
