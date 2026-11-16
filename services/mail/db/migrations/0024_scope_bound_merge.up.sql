-- SPDX-License-Identifier: Apache-2.0
-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('private_ingress_key_plans'::regclass);
SELECT sytue_scope_merge_register('private_ingress_devices'::regclass);
SELECT sytue_scope_merge_register('private_ingress_tasks'::regclass);
SELECT sytue_scope_merge_register('private_ingress_actions'::regclass);
