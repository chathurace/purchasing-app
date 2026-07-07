# Deployment prompt — purchasing-app database

Paste the block below into a fresh Claude Code session **run from the purchasing-app repo root**
(`/Users/chathura/work/projects/cs/dt/purchasing-app`). It drives the `purchasing-db-deploy`
skill end-to-end.

---

Deploy the PostgreSQL database for purchasing-app on Azure, in the US region (`eastus`), using
the same method we used for the finops-app database. Follow the `purchasing-db-deploy` skill in
`.claude/skills/purchasing-db-deploy/`.

Target: a PostgreSQL 17 VM named `purchasing-db-vm-us` in resource group `rg-perftest-chathura`
(subscription `iam-cs-general`), reachable only from the Choreo US egress range and my admin IP.
Database `purchasing`, with a `purchasing_migrator` (DDL) / `purchasing_runtime` (DML) role split.

Do this in order, pausing for my confirmation before `terraform apply` and before pointing the
app at the new DB:

1. Stage the skill's `assets/terraform/*` into `backend/resources/` and `assets/db-roles.sql`
   into `backend/scripts/`. Create `backend/resources/terraform.tfvars` from the example, set
   `admin_ip_cidr` to my current public IP (`curl -s ifconfig.me`)/32, and confirm
   `choreo_cidrs`. Make sure `terraform.tfvars`, `*.tfstate*`, and `config.yaml` are gitignored.
2. Confirm the Choreo US data-plane egress CIDR is `20.22.170.144/28` (tell me if you can't
   verify it and proceed with that default only after I confirm).
3. `terraform init` + `apply` (show me the plan first). Report the VM public IP and confirm
   cloud-init finished (`/var/log/init-postgres.log`) and the empty `purchasing` DB exists.
4. Generate two strong passwords, run `db-roles.sql` as the `postgres` superuser on the VM
   (SSH or `az vm run-command`), then apply all of `backend/migrations/*.sql` in numeric order
   as `purchasing_migrator`. Verify the tables exist and are owned by the migrator.
5. Give me the two connection strings. Put the `purchasing_runtime` DSN in `config.yaml`'s
   `database.url` (sslmode=require); keep the `purchasing_migrator` DSN only for migrations.
   Save both passwords to the gitignored `private/` dir — never commit them.
6. Do NOT commit anything to git unless I ask.

Assume `az` is already logged in to `iam-cs-general`. If any Choreo-side detail (egress CIDR,
whether the component is deployed yet) is unknown, ask me rather than guessing.

---

## Notes for whoever runs this

- purchasing-app configures the DB in **`backend/config.yaml`** (`database.url`), not env vars.
- The app does **not** run migrations at startup — they're applied manually (step 4). That's
  why the deploy includes an explicit migration step against the remote DB.
- This reuses the finops resource group but with `purchasing-`-prefixed resources and a
  `10.20.0.0/24` VNet, so nothing collides with the finops DB VMs.
- Full detail, alternate transports, and backups are in the skill's `references/`.
