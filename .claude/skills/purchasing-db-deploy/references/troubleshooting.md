# DB Deployment Troubleshooting — purchasing-app

## 1. App can't connect from Choreo (connection refused / timeout / SSL error)
Checklist:
1. `sslmode=require` in `database.url` — the VM's `pg_hba.conf` uses `hostssl` only.
2. The exact Choreo **US egress CIDR** is in the NSG rule *and* in `pg_hba.conf`
   (`hostssl purchasing purchasing_runtime <cidr> scram-sha-256`). Confirm the range in the
   Choreo console — data-plane egress can differ from the `20.22.170.144/28` default.
3. NSG rule `allow-postgres-choreo` allows 5432 from that CIDR.
4. `listen_addresses = '*'` in `postgresql.conf`.
5. Role/password match: the DSN uses `purchasing_runtime` and the password from `db-roles.sql`.

After editing `pg_hba.conf` / `postgresql.conf` on the VM:
`sudo systemctl reload postgresql` (or `restart` for `listen_addresses`).

## 2. `password authentication failed for user "purchasing_runtime"`
Either the role wasn't created (db-roles.sql not yet run — do step 3 before pointing the app
at it), or the password in `config.yaml` doesn't match. Rotate by re-running `db-roles.sql`
with the `ALTER ROLE ... PASSWORD` lines uncommented.

## 3. Migrations fail: `permission denied for schema public` / tables owned by wrong role
Run `db-roles.sql` **before** migrations, and apply migrations **as `purchasing_migrator`**
(not `postgres`, not `purchasing_runtime`). If tables were created by the wrong role, re-run
`db-roles.sql` — its `DO` block reassigns existing table/sequence ownership to the migrator.

## 4. Runtime role gets `permission denied` on a table added by a later migration
Default privileges only apply to objects created **after** the `ALTER DEFAULT PRIVILEGES` ran,
and only when created **by** `purchasing_migrator`. If a migration was applied as another role,
re-grant: `GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO purchasing_runtime;`
as the table owner, then re-run `db-roles.sql` to fix ownership + defaults going forward.

## 5. Terraform: resource name / VNet collision with finops
This skill's resources are all prefixed `purchasing-` and use `10.20.0.0/24`. If you see a
conflict, something was renamed — check nothing points at `finops-db-*` or `10.10.0.0/24`.

## 6. `az` provisioning into the wrong subscription
The Terraform provider pins `subscription_id` to `iam-cs-general`. If apply fails on auth,
`az login` and `az account set -s 754affde-02df-4fa8-8219-2be190c1330c`.

## 7. Finding the VM later / running commands without SSH
```bash
az vm run-command invoke -g rg-perftest-chathura -n purchasing-db-vm-us \
  --command-id RunShellScript --scripts "sudo -u postgres psql -d purchasing -c '\dt'"
```
Only the Choreo egress + your admin IP reach the VM over the network, so direct psql from an
un-allowlisted machine will time out — use `az vm run-command` or SSH from your admin IP.
