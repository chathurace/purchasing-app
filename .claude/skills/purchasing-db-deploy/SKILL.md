---
name: purchasing-db-deploy
description: Use this skill when the user asks to deploy, provision, or set up the PostgreSQL database for purchasing-app on Azure (US region) for the Choreo deployment, to rotate its DB credentials, apply migrations to the remote DB, configure backups, or troubleshoot the deployed DB connection. Mirrors the finops-app DB deployment method.
version: 1.0.0
---

# Purchasing-App Database Deployment (Azure + Choreo US)

Provisions a **PostgreSQL 17** VM on Azure in the **US region** (`eastus`) to back the
purchasing-app when it is deployed on **Choreo (US data plane)**. Same method as the
finops-app DB: a Terraform-provisioned Ubuntu VM running PostgreSQL, reachable only from
Choreo's egress NAT range and your admin IP.

```
purchasing-app (Choreo, US)  ──5432/sslmode=require──▶  Azure VM PostgreSQL 17 (eastus)
                                                        exposed only to Choreo egress + admin SSH
```

## What differs from finops-app (read this first)

- **Config is YAML, not env vars.** The app reads `backend/config.yaml` → `database.url`.
  There is no `DATABASE_URL` env var and no separate migration URL in config.
- **Migrations are NOT auto-applied at startup.** The server just opens a pool. The 33
  numbered files in `backend/migrations/` must be applied to the remote DB as an explicit
  deploy step (see step 4).
- **Role split maps to two DSNs you use by hand:** `purchasing_runtime` (DML) → `config.yaml`;
  `purchasing_migrator` (DDL) → applying migrations.

## Fixed facts (do not re-derive)

| Thing | Value |
|-------|-------|
| Subscription | `iam-cs-general` (`754affde-02df-4fa8-8219-2be190c1330c`) |
| Resource group | `rg-perftest-chathura` (shared with finops; distinct resource names) |
| Region | `eastus` |
| Choreo US egress CIDR | `20.22.170.144/28` — **confirm in the Choreo console before apply** |
| VM name | `purchasing-db-vm-us` |
| DB name | `purchasing` |
| Roles | `purchasing_migrator` (DDL), `purchasing_runtime` (DML) |
| PostgreSQL | 17 (PGDG apt), data dir `/var/lib/postgresql/17/main` |

## Assets in this skill

| File | Purpose |
|------|---------|
| `assets/terraform/` | VM + VNet + NSG + public IP + cloud-init. Copy to `backend/resources/`. |
| `assets/terraform/scripts/init-postgres.sh.tpl` | First-boot: installs PG 17, creates the empty `purchasing` DB, opens pg_hba to Choreo. |
| `assets/db-roles.sql` | Creates the migrator/runtime roles + grants. Copy to `backend/scripts/`. |

## Deployment steps

> Prereqs: `az login` (subscription `iam-cs-general`), `terraform`, an SSH keypair at
> `~/.ssh/id_rsa.pub`, and `psql` locally (or use the VM's psql over SSH).

### 1. Stage the Terraform + scripts into the repo
```bash
cd /Users/chathura/work/projects/cs/dt/purchasing-app
mkdir -p backend/resources backend/scripts
cp -R .claude/skills/purchasing-db-deploy/assets/terraform/* backend/resources/
cp .claude/skills/purchasing-db-deploy/assets/db-roles.sql   backend/scripts/
cp backend/resources/terraform.tfvars.example backend/resources/terraform.tfvars
# edit terraform.tfvars: set admin_ip_cidr to "$(curl -s ifconfig.me)/32", confirm choreo_cidrs
```
Ensure `backend/resources/terraform.tfvars`, `*.tfstate*`, and `backend/config.yaml` are gitignored.

### 2. Provision the VM
```bash
cd backend/resources
terraform init
terraform apply          # review the plan, then approve
terraform output vm_public_ip
```
Cloud-init installs PostgreSQL 17 and creates the empty `purchasing` DB (~1–2 min after
the VM reports ready). Verify: `ssh azureuser@<ip> 'sudo tail -n5 /var/log/init-postgres.log'`.

### 3. Provision the least-privilege roles
Generate two strong passwords and run `db-roles.sql` as the `postgres` superuser on the VM.
Choose ONE transport — SSH (needs your admin IP on port 22, which the NSG opened) or
`az vm run-command` (no SSH needed — matches how finops did it). See
[references/postgres-setup.md](references/postgres-setup.md#step-3) for both.

### 4. Apply migrations as the migrator
The app will not do this. Push `backend/migrations/*.sql` to the VM and apply them as
`purchasing_migrator` against the local DB (order matters — run 001→033). Commands in
[references/postgres-setup.md](references/postgres-setup.md#step-4).

### 5. Point the app at the runtime role
In the Choreo deployment's `config.yaml`:
```yaml
database:
  url: "postgres://purchasing_runtime:<runtime_pw>@<vm_public_ip>:5432/purchasing?sslmode=require"
```
Keep the migrator DSN only where you run migrations — never in the app config. Store both
passwords in the repo's gitignored `private/` (as finops does), not in git.

### 6. (Recommended) Enable backups
pgBackRest PITR to Azure Blob — see [references/backups.md](references/backups.md).

## Reference docs

| Topic | File |
|-------|------|
| VM provisioning, roles, applying migrations, verifying | [references/postgres-setup.md](references/postgres-setup.md) |
| pgBackRest PITR backups | [references/backups.md](references/backups.md) |
| Connectivity / SSL / firewall gotchas | [references/troubleshooting.md](references/troubleshooting.md) |
