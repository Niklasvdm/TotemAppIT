# =============================================================================
# outputs.tf — Totem Finder dev/test LXC
# =============================================================================

output "container_vmid" {
  description = "VMID of the dev container"
  value       = proxmox_lxc.totem_dev.vmid
}

output "container_ip" {
  description = "IP address of the dev container"
  value       = local.container_ip
}

output "ssh_command" {
  description = "Connect to the box"
  value       = "ssh root@${local.ssh_host}"
}

output "sync_hint" {
  description = "Push your local working tree up for a fast edit/build loop"
  value       = "../sync.sh root@${local.ssh_host}"
}

output "rebuild_hint" {
  description = "Throw it away and get a clean box"
  value       = "terraform destroy -var-file=../env/dev.tfvars && terraform apply -var-file=../env/dev.tfvars"
}
