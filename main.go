package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"enumcloud/pkg/cloud"
	"enumcloud/pkg/cloud/aws"
	"enumcloud/pkg/cloud/azure"
	"enumcloud/pkg/cloud/gcp"
)

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}

// fatalf prints an error to stderr and exits. Errors go to stderr so that
// stdout stays clean for JSON/CSV output.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func showExamples(platform string) {
	switch strings.ToLower(platform) {
	case "aws":
		fmt.Println(`
AWS Examples:
  # Public AMIs / EBS snapshots owned by an account (works cross-account):
  cloudsnitch -platform aws -profile PROFILE -aws-account 123456789012 ami
  cloudsnitch -platform aws -profile PROFILE -aws-account 123456789012 ebs

  # Same checks across many accounts:
  cloudsnitch -platform aws -profile PROFILE -accounts-file accounts.txt ebs

  # Public S3 buckets, internet-exposed EC2, public/cross-account ECR
  # (these inspect the account your profile is authenticated to):
  cloudsnitch -platform aws -profile PROFILE -aws-account 123456789012 s3

  # Everything, as JSON on stdout:
  cloudsnitch -platform aws -profile PROFILE -aws-account 123456789012 -output-format json all | jq .

Resource types: ami, ebs, s3, ec2, ecr, all`)

	case "gcp":
		fmt.Println(`
GCP Examples:
  # Uses Application Default Credentials (gcloud auth application-default login).
  # If no project is given, the active gcloud project is used.
  cloudsnitch -platform gcp -target-project PROJECT_ID storage
  cloudsnitch -platform gcp -target-project PROJECT_ID compute
  cloudsnitch -platform gcp -target-project PROJECT_ID -output-format json all

Resource types: storage, compute, bigquery, iam, run, functions, artifacts,
secrets, sql, firestore, pubsub, vpc, build, gke, all`)

	case "azure":
		fmt.Println(`
Azure Examples (preview):
  cloudsnitch -platform azure -subscription-id SUB_ID all`)

	default:
		fmt.Println("Please specify a valid platform (aws, gcp, or azure) with -platform")
	}
}

// currentGcloudProject returns the active gcloud project, or "" if gcloud is
// unavailable or has no project set.
func currentGcloudProject() string {
	out, err := exec.Command("gcloud", "config", "get-value", "project").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readAccountsFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var accounts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			accounts = append(accounts, line)
		}
	}
	return accounts, scanner.Err()
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -platform <aws|gcp|azure> [options] [resource-type]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nResource Types:\n")
		fmt.Fprintf(os.Stderr, "  AWS:   ami, ebs, s3, ec2, ecr, all\n")
		fmt.Fprintf(os.Stderr, "  GCP:   storage, compute, bigquery, iam, run, functions, artifacts,\n")
		fmt.Fprintf(os.Stderr, "         secrets, sql, firestore, pubsub, vpc, build, gke, all\n")
		fmt.Fprintf(os.Stderr, "  Azure: all (preview)\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  AWS_PROFILE, AWS_ACCOUNT_ID, AWS_ACCOUNTS_FILE\n")
		fmt.Fprintf(os.Stderr, "  GCP_PROJECT_ID, AZURE_SUBSCRIPTION_ID\n")
	}

	platform := flag.String("platform", "", "Cloud platform to enumerate (aws, gcp, azure)")
	examples := flag.Bool("examples", false, "Show example commands for the specified platform")

	profile := flag.String("profile", getEnvOrDefault("AWS_PROFILE", ""), "AWS CLI profile name")
	accountsFile := flag.String("accounts-file", getEnvOrDefault("AWS_ACCOUNTS_FILE", ""), "Path to file containing AWS account IDs, one per line")
	awsAccount := flag.String("aws-account", getEnvOrDefault("AWS_ACCOUNT_ID", ""), "AWS account ID")

	gcpProject := flag.String("target-project", getEnvOrDefault("GCP_PROJECT_ID", ""), "GCP project ID to enumerate")
	flag.StringVar(gcpProject, "gcp-project-id", *gcpProject, "Alias for -target-project")

	account := flag.String("account", "", "Shorthand: AWS account ID or GCP project ID depending on -platform")
	azureSubscriptionID := flag.String("subscription-id", getEnvOrDefault("AZURE_SUBSCRIPTION_ID", ""), "Azure subscription ID")
	flag.StringVar(azureSubscriptionID, "subscription", *azureSubscriptionID, "Alias for -subscription-id")

	logLevel := flag.String("log-level", "INFO", "Logging level (DEBUG, INFO, WARNING, ERROR)")
	logFile := flag.String("log-file", "", "Also write logs to this file")
	outputFormat := flag.String("output-format", "text", "Output format (text, json, csv)")
	outputFile := flag.String("output-file", "", "Write results to this file instead of stdout")

	flag.Parse()

	if *examples {
		showExamples(*platform)
		return
	}

	plat := strings.ToLower(*platform)
	if plat == "" {
		flag.Usage()
		os.Exit(2)
	}

	switch *outputFormat {
	case "text", "json", "csv":
	default:
		fatalf("unsupported output format %q (use text, json, or csv)", *outputFormat)
	}

	if err := cloud.InitLogger(cloud.LogConfig{Level: *logLevel, FilePath: *logFile}); err != nil {
		fatalf("failed to initialize logger: %v", err)
	}

	resourceType := "all"
	if flag.NArg() > 0 {
		resourceType = strings.ToLower(flag.Arg(0))
	}

	if *account != "" {
		switch plat {
		case "aws":
			*awsAccount = *account
		case "gcp":
			*gcpProject = *account
		}
	}

	config := cloud.Config{
		Platform:       plat,
		Profile:        *profile,
		SubscriptionID: *azureSubscriptionID,
		OutputFormat:   *outputFormat,
		OutputFile:     *outputFile,
		ResourceTypes:  []string{resourceType},
	}

	var allResults []*cloud.Results

	switch plat {
	case "aws":
		var accounts []string
		if *accountsFile != "" {
			var err error
			accounts, err = readAccountsFile(*accountsFile)
			if err != nil {
				fatalf("failed to read accounts file: %v", err)
			}
		} else if *awsAccount != "" {
			accounts = []string{*awsAccount}
		}
		if len(accounts) == 0 {
			fatalf("AWS account ID is required. Use -aws-account, -accounts-file, or set AWS_ACCOUNT_ID")
		}
		if config.Profile != "" {
			cloud.InfoLogger.Printf("Using AWS profile: %s", config.Profile)
		}

		for _, acct := range accounts {
			config.AWSAccount = acct
			enumerator, err := aws.NewEnumerator(config)
			if err != nil {
				fatalf("failed to create AWS enumerator: %v", err)
			}
			results, err := enumerator.Enumerate()
			if err != nil {
				// One bad account shouldn't kill a multi-account run.
				cloud.ErrorLogger.Printf("AWS enumeration failed for account %s: %v", acct, err)
				continue
			}
			allResults = append(allResults, results)
		}

	case "gcp":
		project := *gcpProject
		if project == "" {
			project = currentGcloudProject()
		}
		if project == "" {
			fatalf("GCP project ID is required. Use -target-project, set GCP_PROJECT_ID, or set a gcloud default project")
		}
		config.SourceProject = project
		config.TargetProject = project

		enumerator, err := gcp.NewEnumerator(config)
		if err != nil {
			fatalf("failed to create GCP enumerator: %v", err)
		}
		results, err := enumerator.Enumerate()
		if err != nil {
			fatalf("GCP enumeration failed: %v", err)
		}
		allResults = append(allResults, results)

	case "azure":
		if config.SubscriptionID == "" {
			fatalf("Azure subscription ID is required. Use -subscription-id or set AZURE_SUBSCRIPTION_ID")
		}
		enumerator, err := azure.NewEnumerator(config)
		if err != nil {
			fatalf("failed to create Azure enumerator: %v", err)
		}
		results, err := enumerator.Enumerate()
		if err != nil {
			fatalf("Azure enumeration failed: %v", err)
		}
		allResults = append(allResults, results)

	default:
		fatalf("unsupported platform: %s", *platform)
	}

	combined := &cloud.Results{Platform: plat, Resources: make([]cloud.Resource, 0)}
	for _, r := range allResults {
		combined.Resources = append(combined.Resources, r.Resources...)
	}

	if err := cloud.WriteOutput(combined, config); err != nil {
		fatalf("failed to write output: %v", err)
	}
}
