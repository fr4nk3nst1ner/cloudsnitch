# cloudsnitch

A tool for enumerating publicly accessible or cross-account shared cloud resources across AWS, GCP, and Azure cloud platforms.

## Features

- Multi-cloud resource enumeration:
  - AWS resources (S3 buckets, EC2 instances, ECR repositories, public AMIs, public EBS snapshots)
  - GCP resources (Storage buckets, Compute Engine resources, BigQuery datasets, and more)
  - Azure resources (coming soon)
- Identifies resources that are:
  - Publicly accessible
  - Shared across accounts/projects
  - Potentially misconfigured
- Detailed resource information including:
  - Resource type and name
  - Creation time and location
  - Access policies and permissions
  - Resource-specific properties
  - Tags and labels
- User-friendly error handling with clear messages
- Multiple output formats (text, JSON, CSV)

## Prerequisites

- Go 1.24 or later
- For AWS:
  - AWS CLI configured with credentials
  - AWS profile with necessary permissions
  - Target account ID(s) for cross-account enumeration
- For GCP:
  - `gcloud` CLI tool installed and configured
  - Active gcloud configuration with necessary permissions
  - Target project ID for cross-project enumeration
- For Azure (coming soon):
  - Azure CLI configured with credentials
  - Subscription ID with necessary permissions

## Installation

```bash
git clone https://github.com/fr4nk3nst1ner/cloudsnitch.git
cd cloudsnitch
go build
```

## Usage

Basic syntax:
```bash
go run . -platform PLATFORM [flags] RESOURCE_TYPE
```

Logs are written to stderr and results to stdout, so JSON and CSV output can be piped (for example, `-output-format json | jq`).

Common flags:
```
-platform        Cloud platform to enumerate (aws/gcp/azure)
-output-format   Output format (text/json/csv)
-output-file     File to write output to
-examples        Show example usage
```

To see example usage and supported resource types:
```bash
go run . -examples
```

### AWS Usage

AWS-specific flags:
```
-profile         AWS profile to use
-aws-account     Target AWS account ID
-accounts-file   File containing list of AWS account IDs
```

Example commands:
```bash
# List public AMIs owned by an account
go run . -platform aws -profile myprofile -aws-account 123456789012 ami

# List public EBS snapshots in specific account
go run . -platform aws -profile myprofile -aws-account 123456789012 ebs

# List public EBS snapshots across multiple accounts
go run . -platform aws -profile myprofile -accounts-file accounts.txt ebs

# List all public resources in an account
go run . -platform aws -profile myprofile -aws-account 123456789012 all

# IAM Access Analyzer findings for an account
go run . -platform aws -profile myprofile -aws-account 123456789012 access-analyzer
```

Supported AWS resource types:
- `ami`: AMIs owned by the target account that are public (works cross-account)
- `ebs`: EBS snapshots owned by the target account that are restorable by everyone (works cross-account)
- `s3`: Buckets public via bucket policy or ACL, after bucket-level Block Public Access (caller's account)
- `ec2`: Instances with a public IP, plus security group rules open to 0.0.0.0/0 or ::/0 (caller's account)
- `ecr`: Repositories with a repository policy attached, for review (caller's account)
- `access-analyzer`: Active external-access findings from IAM Access Analyzer, covering S3, IAM roles, KMS, Lambda, SQS, Secrets Manager, EBS/RDS snapshots, ECR, EFS, DynamoDB and more. Needs an `ACCOUNT` or `ORGANIZATION` analyzer in each region you want covered; an `ORGANIZATION` analyzer (in the management or delegated admin account) lets you scan any member account by passing it to `-aws-account`
- `all`: All resource types

`-aws-account` (or `-accounts-file`) is required. The `s3`, `ec2`, and `ecr` checks inspect the account your profile is authenticated to.

### GCP Usage

GCP-specific flags:
```
-target-project   Target GCP project ID
-gcp-scope        Scope for the assets check: projects/ID, folders/ID, or organizations/ID
```

Example commands:
```bash
# Enumerate public storage buckets
go run . -platform gcp -target-project TARGET_PROJECT_ID storage

# Enumerate public compute resources
go run . -platform gcp -target-project TARGET_PROJECT_ID compute

# Enumerate public BigQuery datasets
go run . -platform gcp -target-project TARGET_PROJECT_ID bigquery

# Enumerate all public resources
go run . -platform gcp -target-project TARGET_PROJECT_ID all

# Public IAM bindings across an entire organization
go run . -platform gcp -target-project QUOTA_PROJECT_ID -gcp-scope organizations/123456789 assets
```

Supported GCP resource types:
- `storage`: Cloud Storage buckets
- `compute`: Compute Engine snapshots and images
- `bigquery`: BigQuery datasets
- `iam`: IAM policies
- `run`: Cloud Run services
- `functions`: Cloud Functions
- `artifacts`: Artifact Registry repositories
- `secrets`: Secret Manager secrets
- `sql`: Cloud SQL instances
- `firestore`: Firestore collections
- `pubsub`: Pub/Sub topics
- `vpc`: VPC firewall rules
- `build`: Cloud Build artifacts
- `gke`: GKE clusters
- `assets`: Every IAM policy that grants a role to `allUsers` or `allAuthenticatedUsers`, found with one Cloud Asset Inventory search. Covers all resource types that support IAM policies. Use `-gcp-scope folders/ID` or `-gcp-scope organizations/ID` to search a whole folder or org. Needs the Cloud Asset API enabled on the target project and `roles/cloudasset.viewer` on the scope
- `all`: All resource types

### Azure Usage (Coming Soon)

Azure-specific flags:
```
-subscription    Azure subscription ID
```

Example commands (preview):
```bash
# List public storage accounts
go run . -platform azure -subscription SUB_ID storage

# List public VMs
go run . -platform azure -subscription SUB_ID vm

# List all public resources
go run . -platform azure -subscription SUB_ID all
```

Planned Azure resource types:
- `storage`: Storage accounts
- `vm`: Virtual machines
- `acr`: Container registries
- `aks`: Kubernetes services
- `all`: All resource types

## Output Formats

### Text (default)
```
Platform: aws
Total Resources: 1

Type: s3-bucket
Name: my-public-bucket
ID: my-public-bucket
Location: us-east-1
Created: 2024-02-22T10:00:00Z
Properties:
  public_access: true
  versioning_enabled: true
Tags:
  Environment: Production
----------------------------------------
```

### JSON
```json
{
  "platform": "aws",
  "resources": [
    {
      "type": "s3-bucket",
      "name": "my-public-bucket",
      "id": "my-public-bucket",
      "location": "us-east-1",
      "created_at": "2024-02-22T10:00:00Z",
      "properties": {
        "public_access": true,
        "versioning_enabled": true
      },
      "tags": {
        "Environment": "Production"
      }
    }
  ]
}
```

### CSV
```csv
Type,Name,ID,Location,Properties,Tags,CreatedAt
s3-bucket,my-public-bucket,my-public-bucket,us-east-1,"{""public_access"":true}","{""Environment"":""Production""}","2024-02-22T10:00:00Z"
```

## Error Handling

The tool provides clear error messages for common issues:

1. API/Service Errors:
```
Skipped APIs (not enabled):
- Compute Engine API is not enabled. Visit the Google Cloud Console to enable it.
```

2. Permission Errors:
```
Skipped Resources (insufficient permissions):
- Insufficient permissions to access Cloud Storage. Ensure necessary IAM roles.
```

3. Configuration Errors:
```
Error: AWS credentials not found. Configure AWS CLI or provide credentials.
Error: Target project ID required. Use -target-project flag.
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Disclaimer

This tool is for legitimate security auditing and resource management purposes only. Always ensure you have proper authorization before scanning cloud resources. 
