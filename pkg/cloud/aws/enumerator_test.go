package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestPermOpenToWorld(t *testing.T) {
	open := types.IpPermission{
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22),
		IpRanges: []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
	}
	if !permOpenToWorld(open) || describePerm(open) != "tcp/22" {
		t.Errorf("expected tcp/22 open to world, got %q", describePerm(open))
	}

	v6 := types.IpPermission{
		IpProtocol: aws.String("-1"),
		Ipv6Ranges: []types.Ipv6Range{{CidrIpv6: aws.String("::/0")}},
	}
	if !permOpenToWorld(v6) || describePerm(v6) != "all" {
		t.Errorf("expected all-traffic open over IPv6")
	}

	internal := types.IpPermission{
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(1024), ToPort: aws.Int32(2048),
		IpRanges: []types.IpRange{{CidrIp: aws.String("10.0.0.0/8")}},
	}
	if permOpenToWorld(internal) {
		t.Errorf("10.0.0.0/8 should not be open to world")
	}
	if describePerm(internal) != "tcp/1024-2048" {
		t.Errorf("unexpected port range %q", describePerm(internal))
	}
}

func TestPublicACLGrants(t *testing.T) {
	grants := []s3types.Grant{
		{Grantee: &s3types.Grantee{Type: s3types.TypeGroup, URI: aws.String("http://acs.amazonaws.com/groups/global/AllUsers")}, Permission: s3types.PermissionRead},
		{Grantee: &s3types.Grantee{Type: s3types.TypeGroup, URI: aws.String("http://acs.amazonaws.com/groups/s3/LogDelivery")}, Permission: s3types.PermissionWrite},
		{Grantee: &s3types.Grantee{Type: s3types.TypeCanonicalUser, ID: aws.String("abc")}, Permission: s3types.PermissionFullControl},
	}
	got := publicACLGrants(grants)
	if len(got) != 1 || got[0] != "AllUsers:READ" {
		t.Errorf("unexpected grants: %v", got)
	}
}
