package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"enumcloud/pkg/cloud"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// defaultRegion is used for global calls (DescribeRegions, ListBuckets) when
// the profile doesn't set a region.
const defaultRegion = "us-east-1"

type Enumerator struct {
	config cloud.Config
	ctx    context.Context
	cfg    aws.Config
}

func NewEnumerator(config cloud.Config) (cloud.Enumerator, error) {
	return &Enumerator{
		config: config,
		ctx:    context.Background(),
	}, nil
}

func (e *Enumerator) Enumerate() (*cloud.Results, error) {
	results := &cloud.Results{
		Platform:  "aws",
		Resources: make([]cloud.Resource, 0),
	}

	var opts []func(*config.LoadOptions) error
	if e.config.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(e.config.Profile))
	}
	cfg, err := config.LoadDefaultConfig(e.ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %v", err)
	}
	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}
	e.cfg = cfg

	regions, err := e.regions()
	if err != nil {
		return nil, fmt.Errorf("failed to list regions: %v", err)
	}

	checks := map[string]func([]string, *cloud.Results) error{
		"ami": e.enumerateAMIs,
		"ebs": e.enumerateEBSSnapshots,
		"s3":  func(_ []string, r *cloud.Results) error { return e.enumerateS3(r) },
		"ec2": e.enumerateEC2,
		"ecr": e.enumerateECR,
	}
	order := []string{"ami", "ebs", "s3", "ec2", "ecr"}

	for _, resourceType := range e.config.ResourceTypes {
		var run []string
		switch resourceType {
		case "", "all":
			run = order
		default:
			if _, ok := checks[resourceType]; !ok {
				return nil, fmt.Errorf("unsupported AWS resource type: %s", resourceType)
			}
			run = []string{resourceType}
		}
		for _, name := range run {
			if err := checks[name](regions, results); err != nil {
				cloud.ErrorLogger.Printf("Failed to enumerate %s: %v", name, err)
			}
		}
	}

	return results, nil
}

// regions returns the regions enabled for the caller's account.
func (e *Enumerator) regions() ([]string, error) {
	out, err := ec2.NewFromConfig(e.cfg).DescribeRegions(e.ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, err
	}
	regions := make([]string, 0, len(out.Regions))
	for _, r := range out.Regions {
		regions = append(regions, aws.ToString(r.RegionName))
	}
	return regions, nil
}

func (e *Enumerator) ec2In(region string) *ec2.Client {
	return ec2.NewFromConfig(e.cfg, func(o *ec2.Options) { o.Region = region })
}

// enumerateAMIs finds AMIs owned by the target account that are launchable by
// everyone. This works cross-account: public AMIs are visible to any caller.
func (e *Enumerator) enumerateAMIs(regions []string, results *cloud.Results) error {
	cloud.InfoLogger.Printf("Enumerating public AMIs owned by account %s...", e.config.AWSAccount)

	for _, region := range regions {
		out, err := e.ec2In(region).DescribeImages(e.ctx, &ec2.DescribeImagesInput{
			Owners: []string{e.config.AWSAccount},
			Filters: []types.Filter{
				{Name: aws.String("is-public"), Values: []string{"true"}},
			},
		})
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to describe AMIs in region %s: %v", region, err)
			continue
		}

		for _, image := range out.Images {
			createdAt, _ := time.Parse(time.RFC3339, aws.ToString(image.CreationDate))
			results.Resources = append(results.Resources, cloud.Resource{
				Type:      "ami",
				Name:      aws.ToString(image.Name),
				ID:        aws.ToString(image.ImageId),
				Location:  region,
				CreatedAt: createdAt,
				Tags:      ec2Tags(image.Tags),
				Properties: map[string]interface{}{
					"exposure":         "public",
					"description":      aws.ToString(image.Description),
					"platform":         aws.ToString(image.PlatformDetails),
					"architecture":     string(image.Architecture),
					"root_device_type": string(image.RootDeviceType),
					"owner_id":         aws.ToString(image.OwnerId),
					"state":            string(image.State),
				},
			})
		}
	}
	return nil
}

// enumerateEBSSnapshots finds snapshots owned by the target account that are
// restorable by all AWS accounts. Like AMIs, this works cross-account.
func (e *Enumerator) enumerateEBSSnapshots(regions []string, results *cloud.Results) error {
	cloud.InfoLogger.Printf("Enumerating public EBS snapshots owned by account %s...", e.config.AWSAccount)

	for _, region := range regions {
		paginator := ec2.NewDescribeSnapshotsPaginator(e.ec2In(region), &ec2.DescribeSnapshotsInput{
			OwnerIds:            []string{e.config.AWSAccount},
			RestorableByUserIds: []string{"all"},
		})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(e.ctx)
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to describe snapshots in region %s: %v", region, err)
				break
			}
			for _, snapshot := range out.Snapshots {
				results.Resources = append(results.Resources, cloud.Resource{
					Type:      "ebs-snapshot",
					Name:      nameTag(snapshot.Tags),
					ID:        aws.ToString(snapshot.SnapshotId),
					Location:  region,
					CreatedAt: aws.ToTime(snapshot.StartTime),
					Tags:      ec2Tags(snapshot.Tags),
					Properties: map[string]interface{}{
						"exposure":    "public",
						"description": aws.ToString(snapshot.Description),
						"volume_id":   aws.ToString(snapshot.VolumeId),
						"volume_size": aws.ToInt32(snapshot.VolumeSize),
						"encrypted":   aws.ToBool(snapshot.Encrypted),
						"owner_id":    aws.ToString(snapshot.OwnerId),
					},
				})
			}
		}
	}
	return nil
}

// enumerateS3 reports buckets in the caller's account that are public through
// their bucket policy or ACL, after accounting for bucket-level Block Public
// Access. Account-level Block Public Access is not evaluated.
func (e *Enumerator) enumerateS3(results *cloud.Results) error {
	cloud.InfoLogger.Println("Enumerating public S3 buckets in the caller's account...")

	globalClient := s3.NewFromConfig(e.cfg)
	out, err := globalClient.ListBuckets(e.ctx, &s3.ListBucketsInput{})
	if err != nil {
		return err
	}

	for _, bucket := range out.Buckets {
		name := aws.ToString(bucket.Name)

		region := defaultRegion
		if loc, err := globalClient.GetBucketLocation(e.ctx, &s3.GetBucketLocationInput{Bucket: bucket.Name}); err == nil {
			switch loc.LocationConstraint {
			case "":
			case "EU":
				region = "eu-west-1"
			default:
				region = string(loc.LocationConstraint)
			}
		}
		client := s3.NewFromConfig(e.cfg, func(o *s3.Options) { o.Region = region })

		var pab s3types.PublicAccessBlockConfiguration
		if out, err := client.GetPublicAccessBlock(e.ctx, &s3.GetPublicAccessBlockInput{Bucket: bucket.Name}); err == nil && out.PublicAccessBlockConfiguration != nil {
			pab = *out.PublicAccessBlockConfiguration
		}

		policyPublic := false
		if out, err := client.GetBucketPolicyStatus(e.ctx, &s3.GetBucketPolicyStatusInput{Bucket: bucket.Name}); err == nil && out.PolicyStatus != nil {
			policyPublic = aws.ToBool(out.PolicyStatus.IsPublic) && !aws.ToBool(pab.RestrictPublicBuckets)
		} else if err != nil && !isErrorCode(err, "NoSuchBucketPolicy") {
			cloud.DebugLogger.Printf("GetBucketPolicyStatus %s: %v", name, err)
		}

		var publicGrants []string
		if !aws.ToBool(pab.IgnorePublicAcls) {
			if acl, err := client.GetBucketAcl(e.ctx, &s3.GetBucketAclInput{Bucket: bucket.Name}); err == nil {
				publicGrants = publicACLGrants(acl.Grants)
			} else {
				cloud.DebugLogger.Printf("GetBucketAcl %s: %v", name, err)
			}
		}

		if !policyPublic && len(publicGrants) == 0 {
			continue
		}

		tags := make(map[string]string)
		if tagOut, err := client.GetBucketTagging(e.ctx, &s3.GetBucketTaggingInput{Bucket: bucket.Name}); err == nil {
			for _, tag := range tagOut.TagSet {
				tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}
		}

		results.Resources = append(results.Resources, cloud.Resource{
			Type:      "s3-bucket",
			Name:      name,
			ID:        "arn:aws:s3:::" + name,
			Location:  region,
			CreatedAt: aws.ToTime(bucket.CreationDate),
			Tags:      tags,
			Properties: map[string]interface{}{
				"exposure":          "public",
				"public_via_policy": policyPublic,
				"public_acl_grants": publicGrants,
			},
		})
	}
	return nil
}

func publicACLGrants(grants []s3types.Grant) []string {
	const (
		allUsers  = "http://acs.amazonaws.com/groups/global/AllUsers"
		authUsers = "http://acs.amazonaws.com/groups/global/AuthenticatedUsers"
	)
	var out []string
	for _, g := range grants {
		if g.Grantee == nil || g.Grantee.Type != s3types.TypeGroup {
			continue
		}
		switch aws.ToString(g.Grantee.URI) {
		case allUsers:
			out = append(out, "AllUsers:"+string(g.Permission))
		case authUsers:
			out = append(out, "AuthenticatedUsers:"+string(g.Permission))
		}
	}
	return out
}

// enumerateEC2 reports instances in the caller's account that have a public
// IP, along with any security group rules open to 0.0.0.0/0 or ::/0.
func (e *Enumerator) enumerateEC2(regions []string, results *cloud.Results) error {
	cloud.InfoLogger.Println("Enumerating internet-exposed EC2 instances in the caller's account...")

	for _, region := range regions {
		client := e.ec2In(region)

		openRules, err := openSecurityGroupRules(e.ctx, client)
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to describe security groups in region %s: %v", region, err)
		}

		paginator := ec2.NewDescribeInstancesPaginator(client, &ec2.DescribeInstancesInput{})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(e.ctx)
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to describe instances in region %s: %v", region, err)
				break
			}
			for _, reservation := range out.Reservations {
				for _, instance := range reservation.Instances {
					if aws.ToString(instance.PublicIpAddress) == "" {
						continue
					}

					var groups, open []string
					for _, sg := range instance.SecurityGroups {
						id := aws.ToString(sg.GroupId)
						groups = append(groups, id)
						open = append(open, openRules[id]...)
					}

					state := ""
					if instance.State != nil {
						state = string(instance.State.Name)
					}

					results.Resources = append(results.Resources, cloud.Resource{
						Type:      "ec2-instance",
						Name:      nameTag(instance.Tags),
						ID:        aws.ToString(instance.InstanceId),
						Location:  region,
						CreatedAt: aws.ToTime(instance.LaunchTime),
						Tags:      ec2Tags(instance.Tags),
						Properties: map[string]interface{}{
							"exposure":         "public-ip",
							"public_ip":        aws.ToString(instance.PublicIpAddress),
							"public_dns":       aws.ToString(instance.PublicDnsName),
							"private_ip":       aws.ToString(instance.PrivateIpAddress),
							"instance_type":    string(instance.InstanceType),
							"state":            state,
							"security_groups":  groups,
							"open_to_internet": open,
						},
					})
				}
			}
		}
	}
	return nil
}

// openSecurityGroupRules maps security group ID to a list of ingress rules
// (e.g. "tcp/22", "all") that allow traffic from anywhere.
func openSecurityGroupRules(ctx context.Context, client *ec2.Client) (map[string][]string, error) {
	rules := make(map[string][]string)
	paginator := ec2.NewDescribeSecurityGroupsPaginator(client, &ec2.DescribeSecurityGroupsInput{})
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return rules, err
		}
		for _, sg := range out.SecurityGroups {
			for _, perm := range sg.IpPermissions {
				if !permOpenToWorld(perm) {
					continue
				}
				rules[aws.ToString(sg.GroupId)] = append(rules[aws.ToString(sg.GroupId)], describePerm(perm))
			}
		}
	}
	return rules, nil
}

func permOpenToWorld(perm types.IpPermission) bool {
	for _, r := range perm.IpRanges {
		if aws.ToString(r.CidrIp) == "0.0.0.0/0" {
			return true
		}
	}
	for _, r := range perm.Ipv6Ranges {
		if aws.ToString(r.CidrIpv6) == "::/0" {
			return true
		}
	}
	return false
}

func describePerm(perm types.IpPermission) string {
	proto := aws.ToString(perm.IpProtocol)
	if proto == "-1" {
		return "all"
	}
	from, to := aws.ToInt32(perm.FromPort), aws.ToInt32(perm.ToPort)
	if from == to {
		return fmt.Sprintf("%s/%d", proto, from)
	}
	return fmt.Sprintf("%s/%d-%d", proto, from, to)
}

// enumerateECR reports private ECR repositories in the caller's account that
// have a repository policy attached, for manual review.
func (e *Enumerator) enumerateECR(regions []string, results *cloud.Results) error {
	cloud.InfoLogger.Println("Enumerating public and cross-account ECR repositories in the caller's account...")

	for _, region := range regions {
		client := ecr.NewFromConfig(e.cfg, func(o *ecr.Options) { o.Region = region })

		paginator := ecr.NewDescribeRepositoriesPaginator(client, &ecr.DescribeRepositoriesInput{})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(e.ctx)
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to describe repositories in region %s: %v", region, err)
				break
			}

			for _, repo := range out.Repositories {
				policyOut, err := client.GetRepositoryPolicy(e.ctx, &ecr.GetRepositoryPolicyInput{
					RepositoryName: repo.RepositoryName,
				})
				if err != nil {
					var notFound *ecrtypes.RepositoryPolicyNotFoundException
					if !errors.As(err, &notFound) {
						cloud.ErrorLogger.Printf("Failed to get policy for %s: %v", aws.ToString(repo.RepositoryName), err)
					}
					continue
				}

				// TODO: parse the policy and only report grants to "*" or
				// external accounts. For now, any repository policy is
				// surfaced for manual review.
				policyText := aws.ToString(policyOut.PolicyText)

				tags := make(map[string]string)
				if tagOut, err := client.ListTagsForResource(e.ctx, &ecr.ListTagsForResourceInput{
					ResourceArn: repo.RepositoryArn,
				}); err == nil {
					for _, tag := range tagOut.Tags {
						tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
					}
				}

				results.Resources = append(results.Resources, cloud.Resource{
					Type:      "ecr-repository",
					Name:      aws.ToString(repo.RepositoryName),
					ID:        aws.ToString(repo.RepositoryArn),
					Location:  region,
					CreatedAt: aws.ToTime(repo.CreatedAt),
					Tags:      tags,
					Properties: map[string]interface{}{
						"exposure":       "has-policy (review)",
						"repository_uri": aws.ToString(repo.RepositoryUri),
						"policy":         policyText,
					},
				})
			}
		}
	}
	return nil
}

func ec2Tags(tags []types.Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		out[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return out
}

func nameTag(tags []types.Tag) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == "Name" {
			return aws.ToString(tag.Value)
		}
	}
	return "unnamed"
}

func isErrorCode(err error, code string) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == code
}
