-- SPDX-License-Identifier: Apache-2.0
-- The shared v1 mechanism is backward compatible with the existing 0035 source.
-- Retain it on downgrade rather than weakening the active writer fence.
SELECT 1;
