/* SPDX-License-Identifier: Apache-2.0 */
import type { HttpClient } from './http.js';

export interface WhatsAppSignup {
  instance_id: string;
  state: string;
  app_id: string;
  config_id: string;
  graph_version: string;
}

export interface WhatsAppSignupMetadata {
  available: boolean;
  app_id?: string;
  config_id?: string;
  graph_version?: string;
}

export interface WhatsAppNumber {
  id: string;
  display_phone: string;
  verified_name: string;
  registration_required: boolean;
}

export interface WhatsAppSignupResult {
  instance_id: string;
  numbers: WhatsAppNumber[];
}

export interface WhatsAppSignupFinish {
  state: string;
  code: string;
  waba_id?: string;
  phone_number_id?: string;
  flow?: 'FINISH' | 'FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING' | 'FINISH_ONLY_WABA';
}

export function whatsAppSignupStatus(
  client: HttpClient,
  handoff?: { state: string; instance: string },
): Promise<WhatsAppSignupMetadata> {
  return client.json({ path: '/whatsapp/signup', ...(handoff ? { query: handoff } : {}) });
}

export function startWhatsAppSignup(
  client: HttpClient,
  instanceId?: string,
): Promise<WhatsAppSignup> {
  return client.postJson('/whatsapp/signup/start', instanceId ? { instance_id: instanceId } : {});
}

/** Exchange the short-lived Meta code immediately; provider tokens stay in the gateway. */
export function finishWhatsAppSignup(
  client: HttpClient,
  input: WhatsAppSignupFinish,
): Promise<WhatsAppSignupResult> {
  return client.postJson('/whatsapp/signup/finish', input);
}

export function listWhatsAppNumbers(
  client: HttpClient,
  instanceId: string,
): Promise<WhatsAppSignupResult> {
  return client.json({ path: '/whatsapp/numbers', query: { instance: instanceId } });
}
