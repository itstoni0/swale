# Read-only IAM policy

This document gives the IAM policy for discovery.
Discovery reads resources in one AWS account and one Region.
The policy allows only read actions.

Attach the policy to the IAM principal that runs discovery.
The policy does not grant a write action.

The services are EC2, ELB, RDS, ECS, S3, and IAM.
The IAM prefix for ELB is `elasticloadbalancing`.
The action `sts:GetCallerIdentity` records the account ID.

Many `Describe` actions do not support a resource ARN.
The policy sets `Resource` to `*`.

The policy does not allow `s3:GetObject`.
Discovery does not read S3 object bytes.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DiscoveryRead",
      "Effect": "Allow",
      "Action": [
        "ec2:Describe*",
        "elasticloadbalancing:Describe*",
        "rds:Describe*",
        "ecs:Describe*",
        "ecs:List*",
        "iam:Get*",
        "iam:List*",
        "s3:List*",
        "s3:GetBucket*",
        "s3:GetEncryptionConfiguration",
        "s3:GetLifecycleConfiguration",
        "s3:GetAccelerateConfiguration",
        "s3:GetReplicationConfiguration",
        "s3:GetAccountPublicAccessBlock",
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    }
  ]
}
```
