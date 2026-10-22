import { nativeWorkerResponse } from '@sytue/offline/worker';

export const prerender = false;
export const GET = (): Response => nativeWorkerResponse();
