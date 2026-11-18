# AWS Emulator

This doc states the AWS Emulator version that CI uses.
It also states the fidelity scores and the limits of the emulator.

## Pinned version

CI pins AWS Emulator `1.0.385`.
The git tag is `v1.0.385`, and the tag date is 2026-11-14.
The `emulator` job in `.github/workflows/ci.yml` sets this version in `EMULATOR_VERSION`.
The job also sets `EMULATOR_DEB_SHA256` to the SHA-256 checksum of that package.
A change to `EMULATOR_VERSION` must also change this doc and `EMULATOR_DEB_SHA256`.

## How CI runs the emulator

The `emulator` job in `.github/workflows/ci.yml` runs the emulator.
The job starts the emulator with `aws-emulator --telemetry off --auth off`.
Auth is off because the smoke check uses dummy credentials.
The emulator refuses dummy credentials when auth is on.

The emulator serves HTTPS with a self-signed certificate.

- Endpoint: `https://127.0.0.1:9297`
- CA file: `$HOME/.aws/emulator/9297/ca.pem`
- Health path: `/__admin/health`

The script `scripts/emulator-smoke.sh` runs the smoke check.
The smoke check runs `aws sts get-caller-identity` and `aws ec2 describe-vpcs`.
Both calls are read-only.
The job also runs `go test -tags emulator ./internal/awsread/`.
That test calls `sts:GetCallerIdentity` through the AWS SDK for Go v2.
The job uses dummy credentials only, and it reads no secret.

## Fidelity scores

AWS defines a fidelity score for each service.
The score is the number of fidelity tests that pass out of the total for that service.
The [Fidelity in AWS Emulator][fidelity] page gives this definition.

The source file is `scores.json.gz` from AWS Emulator release `v1.0.385`.
The `schemaVersion` value in the file is 1.
The file gives the emulator commit `813af480bf7f16d5418b64e4f5644a74a7da11c5`.
Tag `v1.0.385` points to that commit.
The file does not store the score.
This table counts the units in that file.
One unit is one fidelity test.
A unit counts for a service when its `services` list contains the service id.
The total includes units with the outcome `pass`, `fail`, or `skipped`.
The first number in each score is the number of `pass` units.
The second number is the total.
The percent shows that ratio to one decimal place.

| Service | Fidelity score       |
| ------- | -------------------- |
| EC2     | 1246 of 2499 (49.9%) |
| S3      | 2061 of 2845 (72.4%) |
| IAM     | 830 of 1238 (67.0%)  |
| ECS     | 551 of 965 (57.1%)   |
| ELBv2   | 424 of 724 (58.6%)   |
| RDS     | 374 of 775 (48.3%)   |

The service ids in the file are these:

- EC2: `com.amazonaws.ec2#AmazonEC2`
- S3: `com.amazonaws.s3#AmazonS3`
- IAM: `com.amazonaws.iam#AWSIdentityManagementV20100508`
- ECS: `com.amazonaws.ecs#AmazonEC2ContainerServiceV20141113`
- ELBv2: `com.amazonaws.elasticloadbalancingv2#ElasticLoadBalancing_v10`
- RDS: `com.amazonaws.rds#AmazonRDSv19`

The EC2 score counts every unit for the EC2 service.
That service includes VPC and networking.

## Limits

The emulator does not evaluate IAM policies or KMS policies like AWS does.
Swale can read and grade IAM documents in CI.
CI cannot prove that a policy works.
The [Swale MVP spec][mvp-spec] states this limit.

## Sources

- [AWS Emulator README](https://github.com/aws/aws-emulator)
- [AWS Emulator v1.0.385 release](https://github.com/aws/aws-emulator/releases/tag/v1.0.385)
- [scores.json.gz for v1.0.385](https://github.com/aws/aws-emulator/releases/download/v1.0.385/scores.json.gz)
- [Fidelity in AWS Emulator](https://docs.aws.amazon.com/emulator/latest/userguide/fidelity.html)
- [Swale MVP spec (Linear)](https://linear.app/swale/document/swale-mvp-spec-2b3eee09e97d)

[fidelity]: https://docs.aws.amazon.com/emulator/latest/userguide/fidelity.html
[mvp-spec]: https://linear.app/swale/document/swale-mvp-spec-2b3eee09e97d
