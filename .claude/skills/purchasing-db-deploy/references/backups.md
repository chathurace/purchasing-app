# Backups — pgBackRest PITR to Azure Blob (purchasing-app)

Mirrors the finops-app backup setup (pgBackRest → Azure Blob, WAL archiving, PITR). Optional
but recommended before the DB holds real data. Enabling `archive_mode` requires a PostgreSQL
restart (brief downtime), so do it during the initial deploy window.

Keep the backup **storage account in the same region (`eastus`)** as the VM to avoid
cross-region egress, and separate from finops' account.

## One-time setup (on the VM)

```bash
sudo apt-get install -y pgbackrest

# Create an eastus storage account + container for backups, e.g.:
#   az storage account create -g rg-perftest-chathura -n purchasingdbbkpeus001 \
#     -l eastus --sku Standard_LRS
#   az storage container create --account-name purchasingdbbkpeus001 -n pgbackrest
```

`/etc/pgbackrest/pgbackrest.conf` (shared-key auth; keep the key out of git):
```ini
[global]
repo1-type=azure
repo1-azure-account=purchasingdbbkpeus001
repo1-azure-container=pgbackrest
repo1-azure-key=<storage-account-key>
repo1-retention-full=2
repo1-retention-full-type=time
repo1-retention-archive=14
start-fast=y

[purchasing]
pg1-path=/var/lib/postgresql/17/main
```

Enable WAL archiving in `postgresql.conf`:
```
archive_mode = on
archive_command = 'pgbackrest --stanza=purchasing archive-push %p'
wal_level = replica
```
Then `sudo systemctl restart postgresql`.

Initialize + first full backup:
```bash
sudo -u postgres pgbackrest --stanza=purchasing stanza-create
sudo -u postgres pgbackrest --stanza=purchasing --type=full backup
```

## Schedule (postgres crontab)
```
0 2 * * 0  pgbackrest --stanza=purchasing --type=full backup     # weekly full, Sun 02:00 UTC
0 2 * * 1-6 pgbackrest --stanza=purchasing --type=incr backup    # daily incr, Mon–Sat
```

## Operate
```bash
sudo -u postgres pgbackrest --stanza=purchasing info      # list backups / PITR window
sudo -u postgres pgbackrest --stanza=purchasing backup    # manual backup
# restore: stop postgres, then pgbackrest --stanza=purchasing --type=time \
#   "--target=2026-07-06 12:00:00+00" restore ; start postgres
```

14-day PITR retention. Verify a restore into a throwaway VM at least once.
