/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * El ancla S/MIME de la organización, bajada con su nombre. Devuelve por qué no se
 * pudo —`missing` si la organización aún no tiene autoridad (404: se crea con el
 * primer correo firmado), `failed` si no— para que quien llama lo diga con un aviso.
 */
export async function bajarAncla(): Promise<'ok' | 'missing' | 'failed'> {
  let res: Response;
  try {
    res = await fetch('/api/smime-anchor');
  } catch {
    return 'failed';
  }
  if (res.status === 404) return 'missing';
  if (!res.ok) return 'failed';
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement('a');
  a.href = url;
  a.download = 'sytue-smime-ca.pem';
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  return 'ok';
}
