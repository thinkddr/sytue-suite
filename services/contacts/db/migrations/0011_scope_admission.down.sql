-- SPDX-License-Identifier: Apache-2.0
SELECT sytue_scope_admission_remove();
DROP TABLE private_scope_admission_control;
DROP FUNCTION sytue_scope_admission_remove();
DROP FUNCTION sytue_scope_admission_register_existing();
DROP FUNCTION sytue_scope_admission_tables();
DROP FUNCTION sytue_scope_admission_transition(uuid,text,text,text);
DROP FUNCTION sytue_scope_admission_register(regclass,boolean);
DROP FUNCTION sytue_scope_admission_data_guard();
DROP FUNCTION sytue_scope_admission_check(text,boolean);
DROP FUNCTION sytue_scope_admission_control_guard();
