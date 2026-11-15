# AWS Emulator

This doc states the AWS Emulator version that CI uses.
It also states the fidelity scores and the limits of the emulator.

## Pinned version

CI pins AWS Emulator `1.0.385`.
The git tag is `v1.0.385`, and the tag date is 2026-11-14.
The `emulator` job in `.github/workflows/ci.yml` sets this version in `EMULATOR_VERSION`.
A change to `EMULATOR_VERSION` must also change this doc.

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
The job uses dummy credentials only, and it reads no secret.

## Fidelity scores

| Service | Fidelity score                                      |
| ------- | --------------------------------------------------- |
| EC2     | Not available. The v1.0.385 release shows no score. |
| S3      | Not available. The v1.0.385 release shows no score. |
| IAM     | Not available. The v1.0.385 release shows no score. |
| ECS     | Not available. The v1.0.385 release shows no score. |
| ELBv2   | Not available. The v1.0.385 release shows no score. |
| RDS     | Not available. The v1.0.385 release shows no score. |

The emulator README says that each release publishes the fidelity scores.
The v1.0.385 release page shows no scores, so this table has no scores.
Fill each score in this table from the pinned release notes before the merge.

## Limits

The emulator does not evaluate IAM policies or KMS policies like AWS does.
Swale can read and grade IAM documents in CI.
CI cannot prove that a policy works.
The [Swale MVP spec][mvp-spec] states this limit.

## Sources

- [AWS Emulator README](https://github.com/aws/aws-emulator)
- [AWS Emulator v1.0.385 release](https://github.com/aws/aws-emulator/releases/tag/v1.0.385)
- [Swale MVP spec (Linear)](https://linear.app/swale/document/swale-mvp-spec-2b3eee09e97d)

[mvp-spec]: https://linear.app/swale/document/swale-mvp-spec-2b3eee09e97d
