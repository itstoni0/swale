# Fixture unit for tests.
# This file holds no remote state and no account data.

include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../modules/empty"
}
