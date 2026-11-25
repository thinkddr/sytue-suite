/* SPDX-License-Identifier: Apache-2.0 */
/**
 * La rama de Matrix de las puertas, en su propio trozo: la puerta de dispositivos y el
 * SDK (matrix-js-sdk, ~1 MB) que hay debajo. Solo la pide `matrix-lazy.tsx` con
 * `import()`; nadie la importa de forma estática, así que el `import()` siempre parte
 * el trozo, también donde Matrix ya llega al JS inicial por otro lado (Rooms).
 */
export { MatrixDeviceGate } from './MatrixDeviceGate.js';
export { SuiteMatrix } from '@sytue/offline/matrix/client';
