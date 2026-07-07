#!/usr/bin/env bash
#
# reset-transactional-data.sh — wipe all transactional data + associated files.
#
# Deletes every purchase-request-derived row (purchase requests, quotations,
# contracts, GRNs, invoices, approvals, recommendations, documents, …) and the
# uploaded files that belong to them, then resets the identity sequences so IDs
# restart at 1.
#
# PRESERVES master / reference data: users, roles, user_roles, vendors,
# cost_centers (+ secondary owners), and config_options (dropdown lists).
#
# Usage:
#   scripts/reset-transactional-data.sh [--yes] [--config PATH] [--files-root DIR]
#
#   --yes          skip the interactive confirmation prompt
#   --config PATH  path to config.yaml (default: backend/config.yaml)
#   --files-root   override the files root (default: read from config, else ../files)
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$REPO_ROOT/backend/config.yaml"
FILES_ROOT=""
ASSUME_YES=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes|-y)       ASSUME_YES=1; shift ;;
    --config)       CONFIG="$2"; shift 2 ;;
    --files-root)   FILES_ROOT="$2"; shift 2 ;;
    -h|--help)      grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)              echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ ! -f "$CONFIG" ]]; then
  echo "config not found: $CONFIG" >&2
  exit 1
fi

# --- Resolve the database URL from config.yaml (database.url) -----------------
DB_URL="$(grep -E '^[[:space:]]*url:' "$CONFIG" | head -n1 | sed -E 's/^[[:space:]]*url:[[:space:]]*"?([^"]*)"?[[:space:]]*$/\1/')"
if [[ -z "$DB_URL" ]]; then
  echo "could not read database.url from $CONFIG" >&2
  exit 1
fi

# --- Resolve the files root ---------------------------------------------------
# Precedence: --files-root flag > uncommented storage.files_root in config >
# the backend default (../files, i.e. $REPO_ROOT/files).
if [[ -z "$FILES_ROOT" ]]; then
  FILES_ROOT="$(grep -E '^[[:space:]]*files_root:' "$CONFIG" | head -n1 | sed -E 's/^[[:space:]]*files_root:[[:space:]]*"?([^"]*)"?.*$/\1/' || true)"
fi
if [[ -z "$FILES_ROOT" ]]; then
  FILES_ROOT="$REPO_ROOT/files"
elif [[ "$FILES_ROOT" != /* ]]; then
  # config paths are relative to the backend working dir
  FILES_ROOT="$REPO_ROOT/backend/$FILES_ROOT"
fi
PR_FILES_DIR="$FILES_ROOT/purchase-requests"

# --- Transactional tables, children first (TRUNCATE ... CASCADE would suffice
#     from purchase_requests alone, but listing them is explicit and safe). ----
TABLES=(
  invoice_cost_allocations
  invoice_items
  invoices
  grn_items
  grns
  contracts
  quotation_items
  quotations
  pr_recommendation_comments
  pr_recommendation_approvals
  pr_recommendations
  pr_approvals
  documents
  pr_reference_sequences
  purchase_requests
)

echo "About to PERMANENTLY DELETE all transactional data and files:"
echo "  Database : $DB_URL"
echo "  Tables   : ${TABLES[*]}"
echo "  Files    : $PR_FILES_DIR/*"
echo
echo "Preserved: users, roles, user_roles, vendors, cost_centers, config_options."
echo

if [[ "$ASSUME_YES" -ne 1 ]]; then
  read -r -p "Type 'DELETE' to proceed: " confirm
  if [[ "$confirm" != "DELETE" ]]; then
    echo "Aborted."
    exit 1
  fi
fi

# --- Wipe the database in a single transaction --------------------------------
JOINED="$(IFS=,; echo "${TABLES[*]}")"
echo "Truncating tables..."
psql "$DB_URL" -v ON_ERROR_STOP=1 <<SQL
BEGIN;
TRUNCATE TABLE $JOINED RESTART IDENTITY CASCADE;
COMMIT;
SQL

# --- Wipe the associated files ------------------------------------------------
if [[ -d "$PR_FILES_DIR" ]]; then
  echo "Removing files under $PR_FILES_DIR ..."
  # Remove per-PR subdirectories but keep the purchase-requests/ dir itself.
  find "$PR_FILES_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
else
  echo "Files dir $PR_FILES_DIR does not exist; nothing to remove."
fi

echo "Done. Transactional data and associated files have been reset."
