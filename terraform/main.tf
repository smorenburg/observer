# ---------------------------------------------------------------------------------------------------------------------
# TERRAFORM CONFIGURATION
# ---------------------------------------------------------------------------------------------------------------------

terraform {
  required_version = "~> 1.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 4.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 4.0"
    }
  }

  backend "gcs" {}
}

# ---------------------------------------------------------------------------------------------------------------------
# NETWORK, SERVICE ACCOUNT, CLUSTER, AND NODE POOLS
# ---------------------------------------------------------------------------------------------------------------------

module "network" {
  source = "github.com/incentro-cloud/terraform-google-network"

  project_id = var.project_id
  name       = "vpc-network"

  subnets = [
    {
      name                     = "nodes"
      ip_cidr_range            = "10.0.2.0/23"
      region                   = "europe-west1"
      private_ip_google_access = true

      log_config = {
        aggregation_interval = "INTERVAL_5_SEC"
        flow_sampling        = "0.5"
        metadata             = "INCLUDE_ALL_METADATA"
      }

      secondary_ip_ranges = [
        {
          range_name    = "pods"
          ip_cidr_range = "10.0.16.0/20"
        },
        {
          range_name    = "services"
          ip_cidr_range = "10.0.32.0/20"
        }
      ]
    }
  ]

  rules = [
    {
      name        = "allow-iap-ingress"
      direction   = "INGRESS"
      ranges      = ["35.235.240.0/20"]
      target_tags = ["iap"]

      allow = [
        {
          protocol = "tcp"
          ports    = ["22", "3389"]
        }
      ]
    },
    {
      name      = "allow-internal-ingress"
      direction = "INGRESS"
      priority  = 65534
      ranges    = ["10.0.2.0/23"]

      allow = [
        {
          protocol = "icmp"
        },
        {
          protocol = "tcp"
        },
        {
          protocol = "udp"
        }
      ]
    }
  ]

  routers = [
    {
      name       = "vpc-router"
      region     = "europe-west1"
      create_nat = true
    }
  ]
}

module "kubernetes" {
  source = "github.com/incentro-cloud/terraform-google-kubernetes"

  project_id            = var.project_id
  name                  = "cluster-01"
  location              = "europe-west1-b"
  node_locations        = ["europe-west1-c", "europe-west1-d"]
  network               = module.network.vpc_id
  subnetwork            = "nodes"
  service_account_roles = ["roles/compute.securityAdmin"]

  monitoring_config = {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }

  private_cluster_config = {
    master_ipv4_cidr_block = "172.16.0.0/28"
  }

  ip_allocation_policy = {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  node_pools = [
    {
      name               = "node-pool-01"
      initial_node_count = 1

      autoscaling = {
        min_node_count = 1
        max_node_count = 3
      }

      node_config = {
        preemptible  = true
        machine_type = "e2-medium"
        tags         = ["iap"]
      }
    }
  ]

  depends_on = [module.network]
}
