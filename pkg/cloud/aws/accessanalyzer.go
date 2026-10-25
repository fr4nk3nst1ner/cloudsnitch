package aws

import (
	"fmt"
	"sort"
	"strings"

	"enumcloud/pkg/cloud"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/accessanalyzer"
	aatypes "github.com/aws/aws-sdk-go-v2/service/accessanalyzer/types"
)

// enumerateAccessAnalyzer pulls active external-access findings from IAM
// Access Analyzer in every region. Access Analyzer evaluates resource policies
// for S3, IAM roles, KMS, Lambda, SQS, Secrets Manager, EBS/RDS snapshots,
// ECR, EFS, DynamoDB and more, so this one check covers far more resource
// types than the hand-written ones.
//
// It needs an external-access analyzer (type ACCOUNT or ORGANIZATION) in each
// region to be scanned. With an ORGANIZATION analyzer in the management or
// delegated admin account, findings for any member account can be pulled
// by passing that account to -aws-account.
func (e *Enumerator) enumerateAccessAnalyzer(regions []string, results *cloud.Results) error {
	cloud.InfoLogger.Printf("Pulling IAM Access Analyzer findings for account %s...", e.config.AWSAccount)

	seen := make(map[string]bool)
	regionsWithAnalyzer := 0

	for _, region := range regions {
		client := accessanalyzer.NewFromConfig(e.cfg, func(o *accessanalyzer.Options) { o.Region = region })

		var analyzers []aatypes.AnalyzerSummary
		paginator := accessanalyzer.NewListAnalyzersPaginator(client, &accessanalyzer.ListAnalyzersInput{})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(e.ctx)
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to list analyzers in region %s: %v", region, err)
				break
			}
			analyzers = append(analyzers, out.Analyzers...)
		}

		analyzer, ok := selectAnalyzer(analyzers)
		if !ok {
			cloud.DebugLogger.Printf("No active external-access analyzer in region %s", region)
			continue
		}
		regionsWithAnalyzer++
		analyzerArn := aws.ToString(analyzer.Arn)

		findings := accessanalyzer.NewListFindingsPaginator(client, &accessanalyzer.ListFindingsInput{
			AnalyzerArn: analyzer.Arn,
			Filter: map[string]aatypes.Criterion{
				"status":               {Eq: []string{string(aatypes.FindingStatusActive)}},
				"resourceOwnerAccount": {Eq: []string{e.config.AWSAccount}},
			},
		})
		for findings.HasMorePages() {
			out, err := findings.NextPage(e.ctx)
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to list findings in region %s: %v", region, err)
				break
			}
			for _, f := range out.Findings {
				if f.Error != nil {
					cloud.DebugLogger.Printf("Skipping finding %s with error: %s", aws.ToString(f.Id), aws.ToString(f.Error))
					continue
				}
				// Global resources such as IAM roles are reported by the
				// analyzer in every region; keep one copy.
				key := findingKey(f)
				if seen[key] {
					continue
				}
				seen[key] = true
				results.Resources = append(results.Resources, findingToResource(f, region, analyzerArn))
			}
		}
	}

	if regionsWithAnalyzer == 0 {
		return fmt.Errorf("no active external-access analyzer found in any region; create one with " +
			"`aws accessanalyzer create-analyzer --analyzer-name cloudsnitch --type ACCOUNT` " +
			"(or ORGANIZATION from the management/delegated admin account) in each region to scan")
	}
	return nil
}

// selectAnalyzer picks an active external-access analyzer, preferring an
// ORGANIZATION analyzer because it covers member accounts too. Unused-access
// analyzers are ignored: they report unused permissions, not exposure.
func selectAnalyzer(analyzers []aatypes.AnalyzerSummary) (aatypes.AnalyzerSummary, bool) {
	var account *aatypes.AnalyzerSummary
	for i := range analyzers {
		a := analyzers[i]
		if a.Status != aatypes.AnalyzerStatusActive {
			continue
		}
		switch a.Type {
		case aatypes.TypeOrganization:
			return a, true
		case aatypes.TypeAccount:
			if account == nil {
				account = &analyzers[i]
			}
		}
	}
	if account != nil {
		return *account, true
	}
	return aatypes.AnalyzerSummary{}, false
}

func findingKey(f aatypes.FindingSummary) string {
	return strings.Join([]string{
		aws.ToString(f.Resource),
		fmt.Sprint(aws.ToBool(f.IsPublic)),
		sortedPairs(f.Principal),
		sortedPairs(f.Condition),
	}, "|")
}

func sortedPairs(m map[string]string) string {
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func findingToResource(f aatypes.FindingSummary, region, analyzerArn string) cloud.Resource {
	exposure := "cross-account"
	if aws.ToBool(f.IsPublic) {
		exposure = "public"
	}

	resource := aws.ToString(f.Resource)
	return cloud.Resource{
		Type:      "access-analyzer/" + string(f.ResourceType),
		Name:      resourceName(resource),
		ID:        resource,
		Location:  region,
		CreatedAt: aws.ToTime(f.CreatedAt),
		Properties: map[string]interface{}{
			"exposure":       exposure,
			"principal":      f.Principal,
			"actions":        f.Action,
			"condition":      f.Condition,
			"owner_id":       aws.ToString(f.ResourceOwnerAccount),
			"finding_id":     aws.ToString(f.Id),
			"analyzer_arn":   analyzerArn,
			"last_evaluated": aws.ToTime(f.AnalyzedAt),
		},
	}
}

// resourceName returns the trailing name segment of an ARN, or the input if
// it has none.
func resourceName(arn string) string {
	if i := strings.LastIndexAny(arn, "/:"); i >= 0 && i < len(arn)-1 {
		return arn[i+1:]
	}
	return arn
}
