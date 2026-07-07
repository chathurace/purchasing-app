-- IdPs (e.g. WSO2 IS) don't always release email/name claims. Allow them to be
-- NULL so first-time login never fails on a missing profile field. The UNIQUE
-- constraint on email still holds (Postgres treats NULLs as distinct).
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN name  DROP NOT NULL;
