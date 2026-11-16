// SPDX-License-Identifier: Apache-2.0
// Keep window and service-worker logout cleanup aligned.
export const OFFLINE_DATABASE = 'sytue-offline';
export const OFFLINE_CAS_DATABASE = 'sytue-offline-cas-v2';
export const OFFLINE_DATABASES = [OFFLINE_DATABASE, OFFLINE_CAS_DATABASE] as const;
