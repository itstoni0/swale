# Swale

Read the anbu skills before you change this repository.
The skills live at `https://github.com/itstoni0/anbu` under `skills/`.
Add the anbu marketplace with `/plugin marketplace add itstoni0/anbu`.
Install the plugin with `/plugin install anbu@anbu`.

Swale is a Go CLI.
Swale applies infrastructure-as-code standards in a team repository.
Swale generates Terragrunt units and stacks.
The units and the stacks reference modules from a Terragrunt module catalog.
The user chooses the catalog.
The Gruntwork IaC Library is one optional source.

Swale tests real configurations against AWS Emulator.
AWS Emulator lives at `https://github.com/aws/aws-emulator`.
Swale writes files only.
The Go code does not import the AWS SDK.
The Go code does not read AWS credentials.

## Project facts

Stack: Go 1.25.x, Cobra, Terragrunt, Terraform, and AWS Emulator for tests.
Lint: `go vet ./...`
Test: `go test ./...`
Branch pattern: `itstoni0/swale-<number>-<short-description>`.
Environments: Do not put AWS credentials in the Go code.
Do not commit a `.env` file.

## License rule

Do not copy the Gruntwork IaC Library.
Do not vendor the Gruntwork IaC Library.
Do not redistribute the Gruntwork IaC Library.
Do not copy terraform-aws-service-catalog.
Do not vendor terraform-aws-service-catalog.
Do not redistribute terraform-aws-service-catalog.

Write a module source reference only.

You may depend on Terragrunt.
Terragrunt uses the MIT license.
You may depend on Boilerplate.
Boilerplate uses the MPL-2.0 license.
You may depend on Terratest.
Terratest uses the Apache-2.0 license.

## Exceptions

None.
