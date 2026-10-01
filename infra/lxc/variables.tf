# =============================================================================
# variables.tf — Totem Finder dev/test LXC
# =============================================================================

# --- Proxmox connection ------------------------------------------------------
variable "pm_api_url" {
  type        = string
  default     = ""
  description = "Proxmox API endpoint URL. Leave empty to derive from proxmox_host_ip."
  validation {
    condition     = var.pm_api_url != "" || var.proxmox_host_ip != ""
    error_message = "Set pm_api_url explicitly or provide proxmox_host_ip."
  }
}

variable "proxmox_host_ip" {
  type        = string
  default     = ""
  description = "IPv4 of the Proxmox host (used to derive pm_api_url when not set)."
}

variable "pm_tls_insecure" {
  type        = bool
  default     = true
  description = "Skip TLS verification for the Proxmox API (true for self-signed certs)."
}

variable "pm_api_token_id" {
  type        = string
  sensitive   = true
  description = "Proxmox API token ID (user@realm!token-name)."
}

variable "pm_api_token_secret" {
  type        = string
  sensitive   = true
  description = "Proxmox API token secret."
}

# --- Container identity -------------------------------------------------------
variable "vmid" {
  type        = number
  description = "Unique numeric identifier. Bump it to stand up a second parallel box."
}

variable "target_node" {
  type        = string
  description = "Proxmox node that hosts the container."
}

variable "hostname" {
  type        = string
  default     = "totem-dev"
  description = "Container hostname."
}

variable "ostemplate" {
  type        = string
  description = "Proxmox LXC template path (e.g. local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst)."
}

variable "root_password" {
  type        = string
  sensitive   = true
  description = "Root password for the container (scratch box — still keep it out of git)."
}

# --- SSH (used by provisioners) ----------------------------------------------
variable "ssh_public_key_path" {
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
  description = "Public key injected into the container at creation."
}

variable "ssh_private_key_path" {
  type        = string
  default     = "~/.ssh/id_ed25519"
  description = "Matching private key (must be loaded in your SSH agent)."
}

# --- Resources (sized for Go/Node builds, not production) ---------------------
variable "cores" {
  type        = number
  default     = 1
  description = "vCPU cores. 1 is plenty for Go builds + Python tests; bump to 2 if the Phase-3 Vite build feels slow."
}

variable "memory" {
  type        = number
  default     = 1024
  description = "RAM in MB. 1024 covers Go builds and a small React/Vite build; bump to 2048 only if a Vite build OOMs."
}

variable "swap" {
  type        = number
  default     = 512
  description = "Swap in MB — a cushion for transient npm/Vite spikes without reserving the RAM."
}

variable "rootfs_storage" {
  type        = string
  default     = "local"
  description = "Proxmox storage pool for the root filesystem."
}

variable "rootfs_size" {
  type        = string
  default     = "8G"
  description = "Root filesystem size. ~3G used (OS + Go cache + node_modules); 8G leaves margin."
}

# --- Network ------------------------------------------------------------------
variable "bridge" {
  type        = string
  default     = "vmbr0"
  description = "Proxmox bridge to attach to."
}

variable "container_ip" {
  type        = string
  default     = ""
  description = "Bare IPv4. Takes precedence over the IP derived from ip_address."
}

variable "container_prefix" {
  type        = string
  default     = "24"
  description = "CIDR prefix length used with container_ip."
}

variable "ip_address" {
  type        = string
  default     = "dhcp"
  description = "Static IP in CIDR form (e.g. 192.168.10.7/24) or 'dhcp'."
  validation {
    condition     = !(lower(var.ip_address) == "dhcp" && var.container_ip == "" && var.ssh_host == "")
    error_message = "Provide ssh_host or container_ip when using DHCP so provisioners can connect."
  }
}

variable "gateway" {
  type        = string
  default     = ""
  description = "Default gateway (leave empty when using DHCP)."
}

variable "ssh_host" {
  type        = string
  default     = ""
  description = "Host used for SSH provisioning. Defaults to the resolved container IP."
}

# --- Toolchain versions -------------------------------------------------------
variable "go_version" {
  type        = string
  default     = "1.26.8"
  description = "Go version installed from go.dev. ncruces/go-sqlite3 requires >= 1.26, so don't drop below that."
}

variable "node_major" {
  type        = string
  default     = "22"
  description = "Node.js major version (NodeSource), for the Phase-3 React build."
}

variable "timezone" {
  type        = string
  default     = "Europe/Brussels"
  description = "System timezone (IANA)."
}
