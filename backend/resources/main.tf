terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
  # Pin the subscription so a stray default context can't provision elsewhere.
  subscription_id = var.subscription_id
}

# Reuse the existing resource group (shared with finops-app's DB VMs).
data "azurerm_resource_group" "main" {
  name = var.resource_group_name
}

# ── Networking ────────────────────────────────────────────────────────────────
# Distinct names + address space from finops-db-vnet (10.10.0.0/24) so the two
# apps' resources never collide inside the shared resource group.

resource "azurerm_virtual_network" "db" {
  name                = "purchasing-db-vnet"
  location            = data.azurerm_resource_group.main.location
  resource_group_name = data.azurerm_resource_group.main.name
  address_space       = ["10.20.0.0/24"]
}

resource "azurerm_subnet" "db" {
  name                 = "purchasing-db-subnet"
  resource_group_name  = data.azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.db.name
  address_prefixes     = ["10.20.0.0/24"]
}

# ── Network Security Group ────────────────────────────────────────────────────

resource "azurerm_network_security_group" "db" {
  name                = "purchasing-db-nsg"
  location            = data.azurerm_resource_group.main.location
  resource_group_name = data.azurerm_resource_group.main.name
}

# 5432 open only to Choreo's outbound NAT range (the US data plane egress).
resource "azurerm_network_security_rule" "allow_postgres_choreo" {
  name                        = "allow-postgres-choreo"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "5432"
  source_address_prefixes     = var.choreo_cidrs
  destination_address_prefix  = "*"
  resource_group_name         = data.azurerm_resource_group.main.name
  network_security_group_name = azurerm_network_security_group.db.name
}

# SSH only from your admin IP (used to apply db-roles.sql + migrations).
resource "azurerm_network_security_rule" "allow_ssh_admin" {
  name                        = "allow-ssh-admin"
  priority                    = 200
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "22"
  source_address_prefix       = var.admin_ip_cidr
  destination_address_prefix  = "*"
  resource_group_name         = data.azurerm_resource_group.main.name
  network_security_group_name = azurerm_network_security_group.db.name
}

resource "azurerm_subnet_network_security_group_association" "db" {
  subnet_id                 = azurerm_subnet.db.id
  network_security_group_id = azurerm_network_security_group.db.id
}

# ── Public IP ─────────────────────────────────────────────────────────────────

resource "azurerm_public_ip" "db" {
  name                = "purchasing-db-pip"
  location            = data.azurerm_resource_group.main.location
  resource_group_name = data.azurerm_resource_group.main.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

# ── Network Interface ─────────────────────────────────────────────────────────

resource "azurerm_network_interface" "db" {
  name                = "purchasing-db-nic"
  location            = data.azurerm_resource_group.main.location
  resource_group_name = data.azurerm_resource_group.main.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.db.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.db.id
  }
}

# ── Virtual Machine ───────────────────────────────────────────────────────────

resource "azurerm_linux_virtual_machine" "db" {
  name                = "purchasing-db-vm-us"
  resource_group_name = data.azurerm_resource_group.main.name
  location            = data.azurerm_resource_group.main.location
  size                = var.vm_size
  admin_username      = var.admin_username

  network_interface_ids = [azurerm_network_interface.db.id]

  admin_ssh_key {
    username   = var.admin_username
    public_key = file(var.ssh_public_key_path)
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "StandardSSD_LRS"
    disk_size_gb         = 64
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }

  # Runs on first boot: installs PostgreSQL 17 and creates the empty `purchasing`
  # DB. Application roles + migrations are provisioned as a post-apply step (see
  # references/postgres-setup.md) — nothing secret is baked into custom_data.
  custom_data = base64encode(templatefile("${path.module}/scripts/init-postgres.sh.tpl", {
    choreo_cidrs = var.choreo_cidrs
  }))

  # custom_data only ever runs on FIRST boot, so editing choreo_cidrs cannot
  # change a running VM's pg_hba.conf — but Terraform still treats the new
  # base64 blob as a replacement trigger, which would destroy the database.
  # Adding an environment is therefore two steps: the NSG rule below (Terraform)
  # and a pg_hba.conf edit over SSH (see references/postgres-setup.md).
  lifecycle {
    ignore_changes = [custom_data]
  }
}
