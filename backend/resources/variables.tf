variable "subscription_id" {
  description = "Azure subscription ID (iam-cs-general)"
  type        = string
  default     = "754affde-02df-4fa8-8219-2be190c1330c"
}

variable "resource_group_name" {
  description = "Name of the existing Azure resource group (shared with finops-app)"
  type        = string
  default     = "rg-perftest-chathura"
}

variable "location" {
  description = "Azure region — US to co-locate with the Choreo US data plane"
  type        = string
  default     = "eastus"
}

variable "choreo_cidrs" {
  description = "Choreo outbound NAT IP CIDR blocks allowed to reach PostgreSQL — one entry per environment the backend is deployed to. Confirm the exact ranges in the Choreo console before apply."
  type        = list(string)
  default     = ["20.0.0.0/8", "203.94.95.136/32", "203.94.95.137/32"]
}

variable "admin_ip_cidr" {
  description = "Your admin public IP in CIDR notation (allowed SSH access)"
  type        = string
  # e.g. "112.134.207.3/32"
}

variable "admin_username" {
  description = "Linux admin username for the VM"
  type        = string
  default     = "azureuser"
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key file for VM access"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "vm_size" {
  description = "Azure VM size"
  type        = string
  default     = "Standard_D2as_v5"
}
