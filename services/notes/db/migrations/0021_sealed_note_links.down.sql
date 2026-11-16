-- SPDX-License-Identifier: Apache-2.0
-- Once any client seals a link, dropping the fence could let old writers lose it.
DO $$ BEGIN
    RAISE EXCEPTION 'Notes sealed-link fence cannot be rolled back safely';
END $$;
