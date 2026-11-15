#!/usr/bin/env bash
# Run a read-only smoke check against a running AWS Emulator.
# Start the emulator with "--telemetry off --auth off" before you run this script.
# The script uses dummy credentials, so the emulator must not require SigV4 keys.
set -euo pipefail

endpoint="https://127.0.0.1:9297"
ca_file="${HOME}/.aws/emulator/9297/ca.pem"
wait_seconds="${EMULATOR_WAIT_SECONDS:-60}"

# The emulator writes ca.pem when it starts and removes the file when it stops.
for ((i = 0; ; i++)); do
  if curl -fsS --cacert "$ca_file" -o /dev/null "$endpoint/__admin/health" 2>/dev/null; then
    break
  fi
  if ((i >= wait_seconds)); then
    echo "The emulator did not answer at $endpoint in $wait_seconds seconds." >&2
    exit 1
  fi
  sleep 1
done
echo "The emulator answers at $endpoint."

# Use only dummy credentials. Do not read a real profile or a real credentials file.
unset AWS_PROFILE AWS_SESSION_TOKEN
export AWS_ENDPOINT_URL="$endpoint"
export AWS_CA_BUNDLE="$ca_file"
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export AWS_DEFAULT_REGION=us-east-1
export AWS_CONFIG_FILE=/dev/null
export AWS_SHARED_CREDENTIALS_FILE=/dev/null
export AWS_EC2_METADATA_DISABLED=true
export AWS_PAGER=""

# Make only read calls.
aws sts get-caller-identity --query Arn --output text
aws ec2 describe-vpcs --query 'length(Vpcs)' --output text
echo "The emulator smoke check passed."
