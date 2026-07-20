-- Rename the `finance` / `finance_admin` roles to `procurement` / `procurement_admin`
-- and rebrand the Finance team as Procurement. The role values live in `roles.name`
-- (referenced by `teams.member_role` and, via id, by `user_roles`); renaming in place
-- keeps `roles.id` stable so existing `user_roles` grants are preserved.
--
-- This is the SINGLE source of the finance->procurement rename: migrations 001/038
-- still seed the original `finance` names, so this runs as a real rename on every DB
-- (fresh or already-deployed) and applies cleanly regardless of migration level.

-- teams.member_role has a plain FK to roles(name) (no ON UPDATE CASCADE), so drop it
-- for the rename and re-add it afterwards.
ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_member_role_fkey;

UPDATE roles SET name = 'procurement'       WHERE name = 'finance';
UPDATE roles SET name = 'procurement_admin' WHERE name = 'finance_admin';

UPDATE teams
   SET key         = 'procurement',
       name        = 'Procurement',
       member_role = 'procurement'
 WHERE member_role = 'finance';

ALTER TABLE teams
    ADD CONSTRAINT teams_member_role_fkey
    FOREIGN KEY (member_role) REFERENCES roles(name);
