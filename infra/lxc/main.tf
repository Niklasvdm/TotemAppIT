# =============================================================================
# main.tf — Totem Finder DEV/TEST LXC on Proxmox
#
# A disposable sandbox to build and iterate on totemd. Provisions an
# unprivileged Debian LXC and installs the full toolchain (Go, Node.js, Python,
# sqlite3, build tools) via bootstrap-dev.sh. It runs NO application — you SSH in
# and build/test by hand. Destroy + re-apply to get a clean box in minutes.
#
#   terraform apply   -var-file=../env/dev.tfvars     # create
#   terraform destroy -var-file=../env/dev.tfvars     # nuke
#   (re-apply to rebuild — nothing on this box is precious)
# =============================================================================

terraform {
  required_version = ">= 1.4.0"

  required_providers {
    proxmox = {
      source  = "Telmate/proxmox"
      version = "3.0.2-rc04"
    }
  }
}

provider "proxmox" {
  pm_api_url          = local.proxmox_api_url
  pm_tls_insecure     = var.pm_tls_insecure
  pm_api_token_id     = var.pm_api_token_id
  pm_api_token_secret = var.pm_api_token_secret
}

locals {
  proxmox_api_url = var.pm_api_url != "" ? var.pm_api_url : format("https://%s:8006/api2/json", var.proxmox_host_ip)

  container_ip = var.container_ip != "" ? var.container_ip : (
    (var.ip_address != "" && lower(var.ip_address) != "dhcp")
    ? try(regex("^([0-9.]+)", var.ip_address)[0], "")
    : ""
  )

  container_cidr = (
    (var.ip_address != "" && lower(var.ip_address) != "dhcp")
    ? var.ip_address
    : (local.container_ip != "" ? format("%s/%s", local.container_ip, var.container_prefix) : "dhcp")
  )

  ssh_host   = var.ssh_host != "" ? var.ssh_host : local.container_ip
  config_dir = abspath("${path.module}/../configuration")
}

resource "proxmox_lxc" "totem_dev" {
  target_node = var.target_node
  vmid        = var.vmid
  hostname    = var.hostname

  ostemplate = var.ostemplate
  password   = var.root_password
  start      = true
  onboot     = false # a scratch box — don't auto-start it on host reboot

  # Pure-Go build needs no special capabilities, so keep it unprivileged.
  unprivileged = true

  cores  = var.cores
  memory = var.memory
  swap   = var.swap

  rootfs {
    storage = var.rootfs_storage
    size    = var.rootfs_size
  }

  network {
    name   = "eth0"
    bridge = var.bridge
    ip     = local.container_cidr
    gw     = var.gateway
  }

  ssh_public_keys = file(var.ssh_public_key_path)

  connection {
    type    = "ssh"
    host    = local.ssh_host
    user    = "root"
    agent   = true
    timeout = "15m"
  }

  # Install the dev toolchain (Go, Node, Python, sqlite3, build-essential, git).
  provisioner "file" {
    source      = "${local.config_dir}/bootstrap-dev.sh"
    destination = "/tmp/bootstrap-dev.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/bootstrap-dev.sh",
      "/tmp/bootstrap-dev.sh '${var.go_version}' '${var.node_major}' '${var.timezone}'",
      "rm -f /tmp/bootstrap-dev.sh",
    ]
  }
}
