-- SPDX-License-Identifier: Apache-2.0
-- A verified identity may already receive Calendar collaboration in another
-- tenant. Compatibility checks report a blocker; they never assign historical
-- rows to the current holder of an email address or return that content.
CREATE POLICY attendees_private_preflight ON attendees FOR SELECT
 USING(current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_shares_private_preflight ON calendar_shares FOR SELECT
 USING(current_setting('app.maintenance',true)='on');
CREATE POLICY calendar_requests_private_preflight ON calendar_peticiones_edicion FOR SELECT
 USING(current_setting('app.maintenance',true)='on');
