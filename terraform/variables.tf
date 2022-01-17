variable "project_id" {
  type        = string
  description = "The identifier of the project to deploy all resources to."
}

variable "region" {
  type        = string
  description = "The region to run the resources in."
  default     = "europe-west4"
}
