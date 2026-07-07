-- ============================================================================
-- purchasing-app — least-privilege PostgreSQL roles
-- ============================================================================
-- One-time DBA provisioning script. Run as the postgres superuser against the
-- `purchasing` database, BEFORE applying migrations:
--   sudo -u postgres psql -d purchasing \
--     -v migrator_pw='<strong-random-pw>' \
--     -v runtime_pw='<strong-random-pw>' \
--     -f db-roles.sql
--
-- It creates two login roles:
--   * purchasing_migrator — owns the schema, may run DDL. Used to apply
--     backend/migrations/*.sql (MIGRATION DSN).
--   * purchasing_runtime  — DML only (SELECT/INSERT/UPDATE/DELETE + sequences),
--     no DDL. This is the DSN the app puts in config.yaml `database.url`.
--
-- The purchasing-app runs NO migrations at startup (the pool just connects), so
-- the runtime role never needs CREATE/ALTER/DROP.
--
-- Idempotent: safe to re-run. Run this BEFORE migrations so that migrator owns
-- the tables it creates and default privileges auto-grant DML to runtime.
--
-- Generate strong passwords with e.g. `openssl rand -base64 24`.
--
-- After running + applying migrations, configure the app:
--   database.url: postgres://purchasing_runtime:<runtime_pw>@<host>:5432/purchasing?sslmode=require
-- and apply migrations with:
--   postgres://purchasing_migrator:<migrator_pw>@<host>:5432/purchasing?sslmode=require
-- ============================================================================

\set ON_ERROR_STOP on

-- Refuse to run unless both passwords were supplied.
\if :{?migrator_pw}
\else
  \warn '!! migrator_pw not supplied. Re-run with: -v migrator_pw=... -v runtime_pw=...'
  \quit
\endif
\if :{?runtime_pw}
\else
  \warn '!! runtime_pw not supplied. Re-run with: -v migrator_pw=... -v runtime_pw=...'
  \quit
\endif

-- --- 1. Roles ---------------------------------------------------------------
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'purchasing_migrator') AS _create_migrator \gset
\if :_create_migrator
  CREATE ROLE purchasing_migrator LOGIN PASSWORD :'migrator_pw';
\endif
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'purchasing_runtime') AS _create_runtime \gset
\if :_create_runtime
  CREATE ROLE purchasing_runtime LOGIN PASSWORD :'runtime_pw';
\endif

-- If re-running to rotate passwords, uncomment:
-- ALTER ROLE purchasing_migrator PASSWORD :'migrator_pw';
-- ALTER ROLE purchasing_runtime  PASSWORD :'runtime_pw';

-- --- 2. Schema ownership / DDL rights for the migrator ----------------------
GRANT CONNECT ON DATABASE purchasing TO purchasing_migrator;
GRANT ALL ON SCHEMA public TO purchasing_migrator;
-- Reassign any pre-existing objects to the migrator (no-op on a fresh DB).
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' LOOP
    EXECUTE format('ALTER TABLE public.%I OWNER TO purchasing_migrator', r.tablename);
  END LOOP;
  FOR r IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
    EXECUTE format('ALTER SEQUENCE public.%I OWNER TO purchasing_migrator', r.sequencename);
  END LOOP;
END
$$;

-- --- 3. DML-only rights for the runtime role --------------------------------
GRANT CONNECT ON DATABASE purchasing TO purchasing_runtime;
GRANT USAGE ON SCHEMA public TO purchasing_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO purchasing_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO purchasing_runtime;
-- Explicitly ensure the runtime role can never DDL.
REVOKE CREATE ON SCHEMA public FROM purchasing_runtime;

-- --- 4. Default privileges for future migration-created objects -------------
-- Granted BY the migrator (the role that will own future tables) so newly
-- created objects are automatically reachable by the runtime role.
ALTER DEFAULT PRIVILEGES FOR ROLE purchasing_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO purchasing_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE purchasing_migrator IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO purchasing_runtime;

-- --- 5. Verify (optional) ---------------------------------------------------
-- SELECT rolname FROM pg_roles WHERE rolname LIKE 'purchasing_%';
