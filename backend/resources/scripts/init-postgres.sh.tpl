#!/bin/bash
set -e
exec > /var/log/init-postgres.log 2>&1

echo "=== Starting PostgreSQL 17 setup ==="

# Add official PGDG apt repository
apt-get install -y curl ca-certificates
install -d /usr/share/postgresql-common/pgdg
curl -s -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
  --fail https://www.postgresql.org/media/keys/ACCC4CF8.asc
sh -c "echo \"deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] \
  https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main\" \
  > /etc/apt/sources.list.d/pgdg.list"

apt-get update -qq
apt-get install -y postgresql-17 postgresql-client-17

systemctl enable postgresql
systemctl start postgresql

# Allow remote connections
sed -i "s/#listen_addresses = .*/listen_addresses = '*'/" \
  /etc/postgresql/17/main/postgresql.conf

# Restrict access: the app runtime role, on the `purchasing` DB, from Choreo NAT
# CIDRs only, SSL required. The role itself is created later by db-roles.sql — a
# pg_hba line for a not-yet-existing role is harmless (it just won't match yet).
cat >> /etc/postgresql/17/main/pg_hba.conf <<HBAEOF

# Allow purchasing_runtime from Choreo NAT IPs only (SSL required)
%{ for cidr in choreo_cidrs ~}
hostssl  purchasing  purchasing_runtime  ${cidr}  scram-sha-256
%{ endfor ~}
HBAEOF

systemctl restart postgresql

# Create the empty application database. Roles (purchasing_migrator /
# purchasing_runtime) and schema/grants are provisioned post-apply by db-roles.sql,
# then migrations are applied as purchasing_migrator. Nothing secret lives here.
sudo -u postgres psql <<SQLEOF
SELECT 'CREATE DATABASE purchasing'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'purchasing')\gexec
SQLEOF

echo "=== PostgreSQL 17 setup complete ==="
