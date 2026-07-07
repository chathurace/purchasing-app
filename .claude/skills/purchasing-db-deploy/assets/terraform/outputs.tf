output "vm_public_ip" {
  description = "Public IP address of the PostgreSQL VM"
  value       = azurerm_public_ip.db.ip_address
}

output "ssh_command" {
  description = "SSH command to connect to the VM (used to run db-roles.sql + migrations)"
  value       = "ssh ${var.admin_username}@${azurerm_public_ip.db.ip_address}"
}

output "runtime_database_url_template" {
  description = "Fill in the runtime password → this becomes database.url in Choreo config.yaml"
  value       = "postgres://purchasing_runtime:<runtime_pw>@${azurerm_public_ip.db.ip_address}:5432/purchasing?sslmode=require"
}

output "migration_database_url_template" {
  description = "Fill in the migrator password → use this DSN to apply backend/migrations/*.sql"
  value       = "postgres://purchasing_migrator:<migrator_pw>@${azurerm_public_ip.db.ip_address}:5432/purchasing?sslmode=require"
}
