# PostgreSQL 17 Setup on Azure VM — purchasing-app

## Terraform provisioning

Files live in `assets/terraform/` (copy into `backend/resources/`). Defaults:

```hcl
subscription_id     = "754affde-02df-4fa8-8219-2be190c1330c"  # iam-cs-general
resource_group_name = "rg-perftest-chathura"                  # shared with finops
location            = "eastus"
vm_size             = "Standard_D2as_v5"
choreo_cidrs        = ["20.22.170.144/28"]                    # Choreo US egress
```

Required per-deploy inputs in `terraform.tfvars` (never commit):
```hcl
admin_ip_cidr       = "<your-public-ip>/32"   # curl -s ifconfig.me
ssh_public_key_path = "~/.ssh/id_rsa.pub"
```

Resource names are prefixed `purchasing-` (vnet/subnet/nsg/pip/nic + VM `purchasing-db-vm-us`)
and the VNet uses `10.20.0.0/24` — distinct from finops (`finops-db-*`, `10.10.0.0/24`) so the
two apps coexist safely in the shared resource group.

```bash
cd backend/resources && terraform init && terraform apply
```

### Confirming the Choreo egress CIDR
The `20.22.170.144/28` default is the observed US data-plane egress. Before relying on it,
confirm in the Choreo console for the purchasing-app component (DevOps → the component's
data plane / egress IPs). If it differs, update `choreo_cidrs` and re-apply — Terraform will
only touch the NSG rule and the VM's `pg_hba.conf` is regenerated on the next boot (or edit
`pg_hba.conf` on the VM directly and `systemctl reload postgresql`).

## What cloud-init does (`init-postgres.sh.tpl`)

1. Adds the PGDG apt repo and installs `postgresql-17` + client.
2. Sets `listen_addresses = '*'` in `postgresql.conf`.
3. Appends a `pg_hba.conf` line **per Choreo CIDR**, SSL required:
   ```
   hostssl  purchasing  purchasing_runtime  20.22.170.144/28  scram-sha-256
   ```
   (The role is created later by `db-roles.sql`; the line is inert until then.)
4. Creates the **empty** `purchasing` database (idempotent). No app roles, no secrets baked in.

Log on the VM: `/var/log/init-postgres.log`.

## Step 3 — provision roles (`db-roles.sql`)

Generate passwords locally:
```bash
migrator_pw=$(openssl rand -base64 24); runtime_pw=$(openssl rand -base64 24)
echo "migrator=$migrator_pw"; echo "runtime=$runtime_pw"   # save to private/ (gitignored)
```

**Transport A — SSH (admin IP is already allowed on port 22):**
```bash
IP=$(terraform -chdir=backend/resources output -raw vm_public_ip)
scp backend/scripts/db-roles.sql azureuser@$IP:/tmp/
ssh azureuser@$IP "sudo -u postgres psql -d purchasing \
  -v migrator_pw=\"$migrator_pw\" -v runtime_pw=\"$runtime_pw\" -f /tmp/db-roles.sql && rm /tmp/db-roles.sql"
```

**Transport B — `az vm run-command` (no SSH; matches finops' established pattern):**
base64 the file in so the passwords are passed as psql vars, never interpolated into SQL text.
```bash
SQL_B64=$(base64 -i backend/scripts/db-roles.sql)
az vm run-command invoke -g rg-perftest-chathura -n purchasing-db-vm-us \
  --command-id RunShellScript --scripts \
  "echo $SQL_B64 | base64 -d > /tmp/db-roles.sql
   sudo -u postgres psql -d purchasing -v migrator_pw='$migrator_pw' -v runtime_pw='$runtime_pw' -f /tmp/db-roles.sql
   rm -f /tmp/db-roles.sql"
```

Run `db-roles.sql` **before** migrations so `purchasing_migrator` owns the tables it creates
and default privileges auto-grant DML to `purchasing_runtime`.

## Step 4 — apply migrations as the migrator

The purchasing-app does **not** run migrations at startup (it just opens a pgxpool). Apply the
33 files in `backend/migrations/` in numeric order, as `purchasing_migrator`.

**Via SSH (run on the VM against localhost):**
```bash
IP=$(terraform -chdir=backend/resources output -raw vm_public_ip)
scp -r backend/migrations azureuser@$IP:/tmp/pmig
ssh azureuser@$IP "set -e
  for f in /tmp/pmig/*.sql; do
    echo \"applying \$f\"
    PGPASSWORD='$migrator_pw' psql -h localhost -U purchasing_migrator -d purchasing -v ON_ERROR_STOP=1 -f \"\$f\"
  done
  rm -rf /tmp/pmig"
```

**Or from your laptop** — only if you temporarily add your admin IP to `choreo_cidrs`
(or a dedicated NSG rule) so 5432 is reachable, then apply with the migration DSN:
```bash
for f in backend/migrations/*.sql; do
  psql "postgres://purchasing_migrator:$migrator_pw@$IP:5432/purchasing?sslmode=require" -v ON_ERROR_STOP=1 -f "$f"
done
```
Remove the temporary 5432 rule afterwards.

Idempotency: these migrations are apply-once (numbered files, no tracking table in the app).
If you need to re-provision a fresh VM later, apply the full 001→033 sequence against the new
empty DB — do not partially re-apply against an existing schema.

## Connection strings

```
# app config.yaml (runtime, DML only)
database.url: postgres://purchasing_runtime:<runtime_pw>@<VM_IP>:5432/purchasing?sslmode=require

# migrations only (DDL)
postgres://purchasing_migrator:<migrator_pw>@<VM_IP>:5432/purchasing?sslmode=require
```
If a URL parser chokes on special characters in a password, URL-encode them (e.g. `$`→`%24`).
pgx/v5 handles the raw string fine.

## Verifying

From the VM (`ssh azureuser@<ip>`):
```bash
sudo -u postgres psql -c '\l'                       # purchasing DB exists
sudo -u postgres psql -c '\du'                       # purchasing_migrator + purchasing_runtime
sudo -u postgres psql -d purchasing -c '\dt'         # tables after migrations
```
End-to-end from the deployed app: check the Choreo component logs for a successful pool
connect and hit an authenticated endpoint that reads the DB.
