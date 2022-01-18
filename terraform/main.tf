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

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}

locals {
  apis = [
    "compute.googleapis.com",
    "container.googleapis.com",
    "secretmanager.googleapis.com",
    "cloudkms.googleapis.com"
  ]
  cluster_01_service_account_roles = [
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
    "roles/monitoring.viewer",
    "roles/stackdriver.resourceMetadata.writer",
    "roles/compute.securityAdmin"
  ]
  cluster_01_observer_service_account_roles = []
}

# Enable the APIs
resource "google_project_service" "apis" {
  for_each           = toset(local.apis)
  service            = each.value
  disable_on_destroy = false
}

# Create the service accounts and IAM bindings.
resource "google_service_account" "cluster_01" {
  account_id = "cluster-01"

  depends_on = [google_project_service.apis]
}

resource "google_service_account" "cluster_01_observer" {
  account_id = "cluster-01-observer"

  depends_on = [google_project_service.apis]
}

resource "google_project_iam_member" "cluster_01_service_account_roles" {
  for_each = toset(local.cluster_01_service_account_roles)
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.cluster_01.email}"
}

resource "google_project_iam_member" "cluster_01_observer_service_account_roles" {
  for_each = toset(local.cluster_01_observer_service_account_roles)
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.cluster_01_observer.email}"
}

resource "google_service_account_iam_binding" "cluster_01_observer_workload_identity_user" {
  service_account_id = google_service_account.cluster_01_observer.name
  role               = "roles/iam.workloadIdentityUser"
  members            = ["serviceAccount:${var.project_id}.svc.id.goog[observer/observer]"]
}

# Create the keyring, encryption keys, and IAM bindings.
resource "google_kms_key_ring" "cluster_01" {
  name     = "cluster-01"
  location = "europe-west4"

  depends_on = [google_project_service.apis]
}

resource "google_kms_crypto_key" "secrets" {
  name     = "secrets"
  key_ring = google_kms_key_ring.cluster_01.id
}

resource "google_kms_crypto_key" "observer" {
  name     = "observer"
  key_ring = google_kms_key_ring.cluster_01.id
}

resource "google_kms_crypto_key_iam_member" "secrets" {
  crypto_key_id = google_kms_crypto_key.secrets.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:service-743723366099@container-engine-robot.iam.gserviceaccount.com"
}

# Create network and subnetwork, including secondary ranges and firewall rules.
resource "google_compute_network" "vpc_network" {
  name                    = "vpc-network"
  auto_create_subnetworks = false

  depends_on = [google_project_service.apis]
}

resource "google_compute_subnetwork" "nodes" {
  name                     = "nodes"
  ip_cidr_range            = "10.0.2.0/23"
  network                  = google_compute_network.vpc_network.id
  private_ip_google_access = true

  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = "0.5"
    metadata             = "INCLUDE_ALL_METADATA"
  }

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = "10.0.16.0/20"
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = "10.0.32.0/20"
  }
}

resource "google_compute_firewall" "allow_iap_ingress" {
  name          = "allow-iap-ingress"
  network       = google_compute_network.vpc_network.id
  direction     = "INGRESS"
  source_ranges = ["35.235.240.0/20"]

  allow {
    protocol = "tcp"
    ports    = ["22", "3389"]
  }

  target_tags = ["nodes"]
}

resource "google_compute_firewall" "allow_metrics_ingress" {
  name          = "allow-metrics-ingress"
  network       = google_compute_network.vpc_network.id
  direction     = "INGRESS"
  source_ranges = [google_container_cluster.cluster_01.private_cluster_config[0].master_ipv4_cidr_block]

  allow {
    protocol = "tcp"
    ports    = ["9090"]
  }

  target_tags = ["nodes"]
}

resource "google_compute_firewall" "allow_internal_ingress" {
  name          = "allow-internal-ingress"
  network       = google_compute_network.vpc_network.id
  direction     = "INGRESS"
  priority      = 65534
  source_ranges = [google_compute_subnetwork.nodes.ip_cidr_range]

  allow {
    protocol = "tcp"
  }
  allow {
    protocol = "udp"
  }
  allow {
    protocol = "icmp"
  }
}

# Create the router and router NAT.
resource "google_compute_router" "vpc_router" {
  name    = "vpc-router"
  network = google_compute_network.vpc_network.id
}

resource "google_compute_router_nat" "vpc_router_nat" {
  name                               = "vpc-router-nat"
  router                             = google_compute_router.vpc_router.name
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}

# Create the public IP for the observer ingress resource.
resource "google_compute_global_address" "cluster_01_observer_ingress" {
  name = "cluster-01-observer-ingress"

  depends_on = [google_project_service.apis]
}

# Create the GKE cluster and node pool.
resource "google_container_cluster" "cluster_01" {
  provider = google-beta

  name                        = "cluster-01"
  location                    = var.region
  node_locations              = ["europe-west4-a", "europe-west4-b", "europe-west4-c"]
  remove_default_node_pool    = true
  initial_node_count          = 1
  network                     = google_compute_network.vpc_network.id
  subnetwork                  = google_compute_subnetwork.nodes.id
  networking_mode             = "VPC_NATIVE"
  enable_intranode_visibility = true

  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = "192.168.0.0/28"
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  database_encryption {
    state    = "ENCRYPTED"
    key_name = google_kms_crypto_key.secrets.id
  }

  depends_on = [google_kms_crypto_key_iam_member.secrets]
}

resource "google_container_node_pool" "node_pool_01" {
  name               = "node-pool-01"
  cluster            = google_container_cluster.cluster_01.name
  location           = google_container_cluster.cluster_01.location
  node_locations     = google_container_cluster.cluster_01.node_locations
  initial_node_count = 1

  autoscaling {
    min_node_count = 1
    max_node_count = 3
  }

  node_config {
    preemptible     = true
    machine_type    = "e2-medium"
    tags            = ["nodes"]
    service_account = google_service_account.cluster_01.email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }
  }
}
