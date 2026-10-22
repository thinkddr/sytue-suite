-- SPDX-License-Identifier: Apache-2.0
-- «Borrar el chat» de un mensaje directo SOLO para quien lo pide, como en WhatsApp.
-- hidden_at lo saca de su barra lateral hasta que alguien vuelva a escribir;
-- cleared_before esconde para él lo anterior. Solo se amplía: nada lo exige aún.
ALTER TABLE channel_members
  ADD COLUMN hidden_at timestamptz,
  ADD COLUMN cleared_before timestamptz;
