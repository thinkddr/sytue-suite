-- SPDX-License-Identifier: Apache-2.0
-- Identity revocation retires pending native consent across every tenant, in
-- the same maintenance transaction that persists its durable denial.
CREATE POLICY matrix_native_consents_maintenance ON matrix_native_consents
    USING(current_setting('app.maintenance',true)='on')
    WITH CHECK(current_setting('app.maintenance',true)='on');
