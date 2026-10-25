package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	aatypes "github.com/aws/aws-sdk-go-v2/service/accessanalyzer/types"
)

func TestSelectAnalyzer(t *testing.T) {
	account := aatypes.AnalyzerSummary{Arn: aws.String("acct"), Type: aatypes.TypeAccount, Status: aatypes.AnalyzerStatusActive}
	org := aatypes.AnalyzerSummary{Arn: aws.String("org"), Type: aatypes.TypeOrganization, Status: aatypes.AnalyzerStatusActive}
	unused := aatypes.AnalyzerSummary{Arn: aws.String("unused"), Type: aatypes.TypeAccountUnusedAccess, Status: aatypes.AnalyzerStatusActive}
	creating := aatypes.AnalyzerSummary{Arn: aws.String("creating"), Type: aatypes.TypeOrganization, Status: aatypes.AnalyzerStatusCreating}

	tests := []struct {
		name string
		in   []aatypes.AnalyzerSummary
		want string
	}{
		{"prefers organization", []aatypes.AnalyzerSummary{account, org}, "org"},
		{"falls back to account", []aatypes.AnalyzerSummary{unused, account, creating}, "acct"},
		{"ignores unused-access and inactive", []aatypes.AnalyzerSummary{unused, creating}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		got, ok := selectAnalyzer(tt.in)
		if tt.want == "" {
			if ok {
				t.Errorf("%s: expected no analyzer, got %s", tt.name, aws.ToString(got.Arn))
			}
			continue
		}
		if !ok || aws.ToString(got.Arn) != tt.want {
			t.Errorf("%s: got %q (ok=%v), want %q", tt.name, aws.ToString(got.Arn), ok, tt.want)
		}
	}
}

func TestFindingKeyDedupesAcrossRegions(t *testing.T) {
	a := aatypes.FindingSummary{
		Id:        aws.String("id-us-east-1"),
		Resource:  aws.String("arn:aws:iam::111111111111:role/deploy"),
		IsPublic:  aws.Bool(false),
		Principal: map[string]string{"AWS": "222222222222"},
	}
	b := a
	b.Id = aws.String("id-eu-west-1")
	if findingKey(a) != findingKey(b) {
		t.Errorf("same resource and principal in two regions should dedupe")
	}

	c := a
	c.Principal = map[string]string{"AWS": "333333333333"}
	if findingKey(a) == findingKey(c) {
		t.Errorf("different principals should not dedupe")
	}
}

func TestFindingToResource(t *testing.T) {
	f := aatypes.FindingSummary{
		Id:                   aws.String("abc"),
		Resource:             aws.String("arn:aws:s3:::open-bucket"),
		ResourceType:         aatypes.ResourceTypeAwsS3Bucket,
		ResourceOwnerAccount: aws.String("111111111111"),
		IsPublic:             aws.Bool(true),
		Principal:            map[string]string{"AWS": "*"},
		Action:               []string{"s3:GetObject"},
	}
	r := findingToResource(f, "us-east-1", "arn:analyzer")
	if r.Type != "access-analyzer/AWS::S3::Bucket" {
		t.Errorf("unexpected type %q", r.Type)
	}
	if r.Name != "open-bucket" {
		t.Errorf("unexpected name %q", r.Name)
	}
	if r.Properties["exposure"] != "public" {
		t.Errorf("expected public exposure, got %v", r.Properties["exposure"])
	}

	f.IsPublic = aws.Bool(false)
	if got := findingToResource(f, "us-east-1", "").Properties["exposure"]; got != "cross-account" {
		t.Errorf("expected cross-account exposure, got %v", got)
	}
}

func TestResourceName(t *testing.T) {
	cases := map[string]string{
		"arn:aws:s3:::my-bucket":                         "my-bucket",
		"arn:aws:iam::111111111111:role/path/deploy":     "deploy",
		"arn:aws:kms:us-east-1:111111111111:key/abc-123": "abc-123",
		"plain": "plain",
	}
	for in, want := range cases {
		if got := resourceName(in); got != want {
			t.Errorf("resourceName(%q) = %q, want %q", in, got, want)
		}
	}
}
