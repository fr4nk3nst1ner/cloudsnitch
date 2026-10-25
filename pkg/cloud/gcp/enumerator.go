package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"enumcloud/pkg/cloud"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/storage/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/run/v1"
	"google.golang.org/api/cloudfunctions/v1"
	artifactregistry "google.golang.org/api/artifactregistry/v1"
	secretmanager "google.golang.org/api/secretmanager/v1"
	sqladmin "google.golang.org/api/sqladmin/v1"
	firestore "google.golang.org/api/firestore/v1"
	pubsub "google.golang.org/api/pubsub/v1"
	cloudbuild "google.golang.org/api/cloudbuild/v1"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
)

// Required OAuth scopes for GCP services
var requiredScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/compute",
	"https://www.googleapis.com/auth/compute.readonly",
	"https://www.googleapis.com/auth/devstorage.read_only",
	"https://www.googleapis.com/auth/cloudplatformprojects.readonly",
	"https://www.googleapis.com/auth/container.readonly",
	"https://www.googleapis.com/auth/bigquery.readonly",
	"https://www.googleapis.com/auth/cloud-platform.read-only",
	"https://www.googleapis.com/auth/datastore",
	"https://www.googleapis.com/auth/sqlservice.admin",
	"https://www.googleapis.com/auth/pubsub",
}

type Enumerator struct {
	config cloud.Config
	ctx    context.Context
}

func NewEnumerator(config cloud.Config) (cloud.Enumerator, error) {
	return &Enumerator{
		config: config,
		ctx:    context.Background(),
	}, nil
}

// Helper function to check if error is due to API being disabled
func isAPIDisabledError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "API has not been used") || 
		strings.Contains(errStr, "is disabled") ||
		strings.Contains(errStr, "SERVICE_DISABLED")
}

// Helper function to check if error is due to insufficient permissions
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "permission") || 
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "401")
}

// Helper function to get a user-friendly error message
func getUserFriendlyError(err error, service string) string {
	if isAPIDisabledError(err) {
		return fmt.Sprintf("%s API is not enabled. To enable it, visit the Google Cloud Console and enable the %s API.", service, service)
	}
	if isPermissionError(err) {
		return fmt.Sprintf("Insufficient permissions to access %s. Please ensure you have the necessary IAM roles.", service)
	}
	return err.Error()
}

func (e *Enumerator) Enumerate() (*cloud.Results, error) {
	results := &cloud.Results{
		Platform:  "gcp",
		Resources: make([]cloud.Resource, 0),
	}

	// Initialize services with default credentials and project
	sourceOpts := []option.ClientOption{
		option.WithScopes(requiredScopes...),
		option.WithQuotaProject(e.config.SourceProject),
	}

	// Log enumeration context
	cloud.InfoLogger.Printf("Starting enumeration from project %s to project %s", e.config.SourceProject, e.config.TargetProject)

	// Track skipped resources
	skippedAPIs := make(map[string]string)
	skippedPermissions := make(map[string]string)

	// Helper function to handle errors and track skipped resources
	handleEnumerationError := func(err error, service string) {
		if isAPIDisabledError(err) {
			skippedAPIs[service] = getUserFriendlyError(err, service)
		} else if isPermissionError(err) {
			skippedPermissions[service] = getUserFriendlyError(err, service)
		} else {
			cloud.ErrorLogger.Printf("Failed to enumerate %s: %v", service, err)
		}
	}

	// If no resource type specified, enumerate all
	if len(e.config.ResourceTypes) == 0 || e.config.ResourceTypes[0] == "" || e.config.ResourceTypes[0] == "all" {
		if err := e.enumerateStorageBuckets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Storage")
		}

		if err := e.enumerateComputeSnapshots(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Compute Engine")
		}

		if err := e.enumerateComputeImages(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Compute Engine Images")
		}

		if err := e.enumerateBigQueryDatasets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "BigQuery")
		}

		if err := e.enumerateIAMPolicies(results, sourceOpts); err != nil {
			handleEnumerationError(err, "IAM Policies")
		}

		if err := e.enumerateCloudRunServices(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Run Services")
		}

		if err := e.enumerateCloudFunctions(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Functions")
		}

		if err := e.enumerateArtifactRegistry(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Artifact Registry")
		}

		if err := e.enumerateSecrets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Secret Manager")
		}

		if err := e.enumerateCloudSQL(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud SQL")
		}

		if err := e.enumerateFirestore(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Firestore")
		}

		if err := e.enumeratePubSub(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Pub/Sub")
		}

		if err := e.enumerateVPCFirewallRules(results, sourceOpts); err != nil {
			handleEnumerationError(err, "VPC Firewall Rules")
		}

		if err := e.enumerateCloudBuild(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Build")
		}

		if err := e.enumerateGKEClusters(results, sourceOpts); err != nil {
			handleEnumerationError(err, "GKE Clusters")
		}

		// Log summary of skipped resources
		if len(skippedAPIs) > 0 {
			cloud.InfoLogger.Println("\nSkipped APIs (not enabled):")
			for _, msg := range skippedAPIs {
				cloud.InfoLogger.Printf("- %s", msg)
			}
		}
		if len(skippedPermissions) > 0 {
			cloud.InfoLogger.Println("\nSkipped Resources (insufficient permissions):")
			for _, msg := range skippedPermissions {
				cloud.InfoLogger.Printf("- %s", msg)
			}
		}

		return results, nil
	}

	// Enumerate only the specified resource type
	resourceType := e.config.ResourceTypes[0]
	switch resourceType {
	case "storage":
		if err := e.enumerateStorageBuckets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Storage")
		}
	case "compute":
		if err := e.enumerateComputeSnapshots(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Compute Engine Snapshots")
		}
		if err := e.enumerateComputeImages(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Compute Engine Images")
		}
	case "bigquery":
		if err := e.enumerateBigQueryDatasets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "BigQuery")
		}
	case "iam":
		if err := e.enumerateIAMPolicies(results, sourceOpts); err != nil {
			handleEnumerationError(err, "IAM Policies")
		}
	case "run":
		if err := e.enumerateCloudRunServices(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Run Services")
		}
	case "functions":
		if err := e.enumerateCloudFunctions(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Functions")
		}
	case "artifacts":
		if err := e.enumerateArtifactRegistry(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Artifact Registry")
		}
	case "secrets":
		if err := e.enumerateSecrets(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Secret Manager")
		}
	case "sql":
		if err := e.enumerateCloudSQL(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud SQL")
		}
	case "firestore":
		if err := e.enumerateFirestore(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Firestore")
		}
	case "pubsub":
		if err := e.enumeratePubSub(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Pub/Sub")
		}
	case "vpc":
		if err := e.enumerateVPCFirewallRules(results, sourceOpts); err != nil {
			handleEnumerationError(err, "VPC Firewall Rules")
		}
	case "build":
		if err := e.enumerateCloudBuild(results, sourceOpts); err != nil {
			handleEnumerationError(err, "Cloud Build")
		}
	case "gke":
		if err := e.enumerateGKEClusters(results, sourceOpts); err != nil {
			handleEnumerationError(err, "GKE Clusters")
		}
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	// Log summary of skipped resources for single resource type
	if len(skippedAPIs) > 0 || len(skippedPermissions) > 0 {
		cloud.InfoLogger.Printf("\nSkipped Resources for %s:", resourceType)
		for _, msg := range skippedAPIs {
			cloud.InfoLogger.Printf("- API not enabled: %s", msg)
		}
		for _, msg := range skippedPermissions {
			cloud.InfoLogger.Printf("- Insufficient permissions: %s", msg)
		}
	}

	return results, nil
}

// Helper function to check if a resource is accessible from the target project
func (e *Enumerator) isAccessibleFromTargetProject(resourceType, resourceName string, opts []option.ClientOption) bool {
	// Create a new service with target project credentials
	targetOpts := []option.ClientOption{
		option.WithScopes(requiredScopes...),
		option.WithQuotaProject(e.config.TargetProject),
	}

	switch resourceType {
	case "storage":
		service, err := storage.NewService(e.ctx, targetOpts...)
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to create storage service for target project: %v", err)
			return false
		}

		// Try to get bucket with target project
		_, err = service.Buckets.Get(resourceName).Do()
		return err == nil
	}

	return false
}

func (e *Enumerator) enumerateStorageBuckets(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Storage buckets...")

	// Create service with source project credentials
	service, err := storage.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create storage service: %v", err)
	}

	// List buckets in source project
	bucketList, err := service.Buckets.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list buckets: %v", err)
	}

	for _, bucket := range bucketList.Items {
		// Check for public or cross-project access
		policy, err := service.Buckets.GetIamPolicy(bucket.Name).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get IAM policy for bucket %s: %v", bucket.Name, err)
			continue
		}

		isPublic := false
		isCrossProject := false

		// Check for public access
		for _, binding := range policy.Bindings {
			for _, member := range binding.Members {
				if member == "allUsers" || member == "allAuthenticatedUsers" {
					isPublic = true
					break
				}
				// Check for target project access
				if strings.Contains(member, fmt.Sprintf("project/%s", e.config.TargetProject)) {
					isCrossProject = true
					break
				}
			}
			if isPublic || isCrossProject {
				break
			}
		}

		// If not public or directly granted, check for cross-project access
		if !isPublic && !isCrossProject {
			isCrossProject = e.isAccessibleFromTargetProject("storage", bucket.Name, opts)
		}

		if !isPublic && !isCrossProject {
			continue
		}

		tags := make(map[string]string)
		for key, value := range bucket.Labels {
			tags[key] = value
		}

		// Add access type and project info to tags
		if isPublic {
			tags["access_type"] = "public"
		} else {
			tags["access_type"] = "cross_project"
		}
		tags["source_project"] = e.config.SourceProject
		tags["target_project"] = e.config.TargetProject

		createdAt, _ := time.Parse(time.RFC3339, bucket.TimeCreated)

		resource := cloud.Resource{
			Type:      "storage-bucket",
			Name:      bucket.Name,
			ID:        bucket.Id,
			Location:  bucket.Location,
			CreatedAt: createdAt,
			Tags:      tags,
			Properties: map[string]interface{}{
				"storage_class": bucket.StorageClass,
				"project":      bucket.ProjectNumber,
				"public_access": isPublic,
				"public_members": policy.Bindings,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateComputeSnapshots(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating public Compute Engine snapshots...")

	service, err := compute.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create compute service: %v", err)
	}

	snapshots, err := service.Snapshots.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %v", err)
	}

	for _, snapshot := range snapshots.Items {
		// Check if snapshot is shared
		iam, err := service.Snapshots.GetIamPolicy(e.config.SourceProject, snapshot.Name).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get IAM policy for snapshot %s: %v", snapshot.Name, err)
			continue
		}

		isPublic := false
		for _, binding := range iam.Bindings {
			for _, member := range binding.Members {
				if isPublicOrCrossAccount(member, e.config.SourceProject) {
					isPublic = true
					break
				}
			}
		}

		if !isPublic {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, snapshot.CreationTimestamp)

		resource := cloud.Resource{
			Type:      "compute-snapshot",
			Name:      snapshot.Name,
			ID:        fmt.Sprintf("%d", snapshot.Id),
			Location:  firstOr(snapshot.StorageLocations, "global"),
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"disk_size_gb":    snapshot.DiskSizeGb,
				"storage_bytes":   snapshot.StorageBytes,
				"source_disk":     snapshot.SourceDisk,
				"shared_with":     iam.Bindings,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateComputeImages(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating public Compute Engine images...")

	service, err := compute.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create compute service: %v", err)
	}

	images, err := service.Images.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list images: %v", err)
	}

	for _, image := range images.Items {
		// Check if image is shared
		iam, err := service.Images.GetIamPolicy(e.config.SourceProject, image.Name).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get IAM policy for image %s: %v", image.Name, err)
			continue
		}

		isPublic := false
		for _, binding := range iam.Bindings {
			for _, member := range binding.Members {
				if isPublicOrCrossAccount(member, e.config.SourceProject) {
					isPublic = true
					break
				}
			}
		}

		if !isPublic {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, image.CreationTimestamp)

		resource := cloud.Resource{
			Type:      "compute-image",
			Name:      image.Name,
			ID:        fmt.Sprintf("%d", image.Id),
			Location:  "global",
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"disk_size_gb":  image.DiskSizeGb,
				"source_disk":   image.SourceDisk,
				"shared_with":   iam.Bindings,
				"family":        image.Family,
				"architecture": image.Architecture,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateBigQueryDatasets(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating public BigQuery datasets...")

	service, err := bigquery.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create BigQuery service: %v", err)
	}

	datasets, err := service.Datasets.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list datasets: %v", err)
	}

	for _, dataset := range datasets.Datasets {
		// Get detailed dataset info
		datasetDetail, err := service.Datasets.Get(e.config.SourceProject, dataset.DatasetReference.DatasetId).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get dataset details for %s: %v", dataset.DatasetReference.DatasetId, err)
			continue
		}

		// Check dataset access
		isPublic := false
		for _, access := range datasetDetail.Access {
			if access.SpecialGroup == "allUsers" || access.SpecialGroup == "allAuthenticatedUsers" {
				isPublic = true
				break
			}
		}

		if !isPublic {
			continue
		}

		createdAt := time.Unix(datasetDetail.CreationTime/1000, 0)

		resource := cloud.Resource{
			Type:      "bigquery-dataset",
			Name:      datasetDetail.DatasetReference.DatasetId,
			ID:        datasetDetail.Id,
			Location:  datasetDetail.Location,
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"access_controls": datasetDetail.Access,
				"default_partition_expiration_ms": datasetDetail.DefaultPartitionExpirationMs,
				"default_table_expiration_ms":    datasetDetail.DefaultTableExpirationMs,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateIAMPolicies(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating IAM policies...")

	service, err := cloudresourcemanager.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service: %v", err)
	}

	policy, err := service.Projects.GetIamPolicy(e.config.SourceProject, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get project IAM policy: %v", err)
	}

	for _, binding := range policy.Bindings {
		isPublic := false
		for _, member := range binding.Members {
			if isPublicOrCrossAccount(member, e.config.SourceProject) {
				isPublic = true
				break
			}
		}

		if !isPublic {
			continue
		}

		resource := cloud.Resource{
			Type:      "iam-policy",
			Name:      binding.Role,
			ID:        binding.Role,
			Location:  "global",
			CreatedAt: time.Now(),
			Properties: map[string]interface{}{
				"role":    binding.Role,
				"members": binding.Members,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateCloudRunServices(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Cloud Run services...")

	service, err := run.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Run service: %v", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", e.config.SourceProject)
	resp, err := service.Projects.Locations.Services.List(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}

	for _, svc := range resp.Items {
		// Check if service is public
		if svc.Status.Url != "" {
			createdAt, _ := time.Parse(time.RFC3339, svc.Metadata.CreationTimestamp)

			resource := cloud.Resource{
				Type:      "cloud-run-service",
				Name:      svc.Metadata.Name,
				ID:        svc.Metadata.Name,
				Location:  svc.Metadata.Namespace,
				CreatedAt: createdAt,
				Properties: map[string]interface{}{
					"url":    svc.Status.Url,
					"status": svc.Status.Conditions,
				},
			}

			results.Resources = append(results.Resources, resource)
		}
	}

	return nil
}

func (e *Enumerator) enumerateCloudFunctions(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Cloud Functions...")

	service, err := cloudfunctions.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Functions service: %v", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", e.config.SourceProject)
	resp, err := service.Projects.Locations.Functions.List(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to list functions: %v", err)
	}

	for _, function := range resp.Functions {
		if function.HttpsTrigger != nil && function.HttpsTrigger.Url != "" {
			// Function has a public HTTPS trigger
			createdAt, _ := time.Parse(time.RFC3339, function.UpdateTime)

			// Extract location from name (format: projects/*/locations/*/functions/*)
			parts := strings.Split(function.Name, "/")
			var location string
			if len(parts) >= 4 {
				location = parts[3]
			}

			resource := cloud.Resource{
				Type:      "cloud-function",
				Name:      function.Name,
				ID:        function.Name,
				Location:  location,
				CreatedAt: createdAt,
				Properties: map[string]interface{}{
					"url":         function.HttpsTrigger.Url,
					"status":      function.Status,
					"runtime":     function.Runtime,
					"entry_point": function.EntryPoint,
				},
			}

			results.Resources = append(results.Resources, resource)
		}
	}

	return nil
}

func (e *Enumerator) enumerateArtifactRegistry(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Artifact Registry repositories...")

	service, err := artifactregistry.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Artifact Registry service: %v", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", e.config.SourceProject)
	repoList, err := service.Projects.Locations.Repositories.List(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to list repositories: %v", err)
	}

	for _, repo := range repoList.Repositories {
		tags := make(map[string]string)
		for key, value := range repo.Labels {
			tags[key] = value
		}

		createdAt, _ := time.Parse(time.RFC3339, repo.CreateTime)

		// Extract location from the name (format: projects/*/locations/*/repositories/*)
		parts := strings.Split(repo.Name, "/")
		var location string
		if len(parts) >= 4 {
			location = parts[3]
		}

		resource := cloud.Resource{
			Type:      "artifact-repository",
			Name:      repo.Name,
			ID:        repo.Name,
			Location:  location,
			CreatedAt: createdAt,
			Tags:      tags,
			Properties: map[string]interface{}{
				"format":      repo.Format,
				"description": repo.Description,
				"update_time": repo.UpdateTime,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateSecrets(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Secret Manager secrets...")

	service, err := secretmanager.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Secret Manager service: %v", err)
	}

	parent := fmt.Sprintf("projects/%s", e.config.SourceProject)
	resp, err := service.Projects.Secrets.List(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to list secrets: %v", err)
	}

	for _, secret := range resp.Secrets {
		// Get IAM policy for the secret
		policy, err := service.Projects.Secrets.GetIamPolicy(secret.Name).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get IAM policy for secret %s: %v", secret.Name, err)
			continue
		}

		isPublic := false
		for _, binding := range policy.Bindings {
			for _, member := range binding.Members {
				if isPublicOrCrossAccount(member, e.config.SourceProject) {
					isPublic = true
					break
				}
			}
		}

		if !isPublic {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, secret.CreateTime)

		resource := cloud.Resource{
			Type:      "secret-manager-secret",
			Name:      secret.Name,
			ID:        secret.Name,
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"replication": secret.Replication,
				"policy":      policy.Bindings,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateCloudSQL(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Cloud SQL instances...")

	service, err := sqladmin.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud SQL service: %v", err)
	}

	instances, err := service.Instances.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list instances: %v", err)
	}

	for _, instance := range instances.Items {
		createdAt, _ := time.Parse(time.RFC3339, instance.CreateTime)

		resource := cloud.Resource{
			Type:      "cloud-sql-instance",
			Name:      instance.Name,
			ID:        instance.InstanceType,
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"state":        instance.State,
				"tier":        instance.Settings.Tier,
				"region":      instance.Region,
				"ip_addresses": instance.IpAddresses,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateFirestore(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Firestore collections...")

	service, err := firestore.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Firestore service: %v", err)
	}

	// Get project settings to determine database type
	parent := fmt.Sprintf("projects/%s", e.config.SourceProject)
	projectSettings, err := service.Projects.Databases.Get(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to get Firestore settings: %v", err)
	}

	// List collections based on database type
	var collections []*firestore.Document
	if projectSettings.Type == "firestore" {
		// Create ListCollectionIds request
		req := &firestore.ListCollectionIdsRequest{
			PageSize: 100,
		}

		// List collections
		resp, err := service.Projects.Databases.Documents.ListCollectionIds(
			fmt.Sprintf("%s/databases/(default)", parent),
			req,
		).Do()
		if err != nil {
			return fmt.Errorf("failed to list collections: %v", err)
		}

		// For each collection, check its documents
		for _, collectionId := range resp.CollectionIds {
			// Create List request for documents
			docs, err := service.Projects.Databases.Documents.List(
				fmt.Sprintf("%s/databases/(default)", parent),
				collectionId,
			).Do()
			if err != nil {
				cloud.ErrorLogger.Printf("Failed to list documents in collection %s: %v", collectionId, err)
				continue
			}
			collections = append(collections, docs.Documents...)
		}
	}

	// Check each document for public access using Cloud Resource Manager
	crmService, err := cloudresourcemanager.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service: %v", err)
	}

	projectPolicy, err := crmService.Projects.GetIamPolicy(e.config.SourceProject, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get project IAM policy: %v", err)
	}

	// Check for public access to Firestore
	hasPublicAccess := false
	for _, binding := range projectPolicy.Bindings {
		if strings.Contains(strings.ToLower(binding.Role), "firestore") {
			for _, member := range binding.Members {
				if isPublicOrCrossAccount(member, e.config.SourceProject) {
					hasPublicAccess = true
					break
				}
			}
		}
	}

	if !hasPublicAccess {
		return nil
	}

	// If there's public access to Firestore, add all documents as potentially exposed
	for _, doc := range collections {
		createdAt := time.Now() // Firestore API doesn't provide creation time

		resource := cloud.Resource{
			Type:      "firestore-document",
			Name:      doc.Name,
			ID:        doc.Name,
			Location:  "global",
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"fields":        doc.Fields,
				"project_policy": projectPolicy.Bindings,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumeratePubSub(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Pub/Sub topics...")

	service, err := pubsub.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Pub/Sub service: %v", err)
	}

	projectPath := fmt.Sprintf("projects/%s", e.config.SourceProject)
	resp, err := service.Projects.Topics.List(projectPath).Do()
	if err != nil {
		return fmt.Errorf("failed to list topics: %v", err)
	}

	for _, topic := range resp.Topics {
		// Get IAM policy for the topic
		policy, err := service.Projects.Topics.GetIamPolicy(topic.Name).Do()
		if err != nil {
			cloud.ErrorLogger.Printf("Failed to get IAM policy for topic %s: %v", topic.Name, err)
			continue
		}

		isPublic := false
		for _, binding := range policy.Bindings {
			for _, member := range binding.Members {
				if isPublicOrCrossAccount(member, e.config.SourceProject) {
					isPublic = true
					break
				}
			}
		}

		if !isPublic {
			continue
		}

		resource := cloud.Resource{
			Type:      "pubsub-topic",
			Name:      topic.Name,
			ID:        topic.Name,
			Location:  "global",
			CreatedAt: time.Now(), // PubSub API doesn't provide creation time
			Properties: map[string]interface{}{
				"policy": policy.Bindings,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateVPCFirewallRules(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating VPC firewall rules...")

	service, err := compute.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Compute service: %v", err)
	}

	resp, err := service.Firewalls.List(e.config.SourceProject).Do()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %v", err)
	}

	for _, rule := range resp.Items {
		// Check if rule allows 0.0.0.0/0
		isPublic := false
		for range rule.Allowed {
			for _, sourceRange := range rule.SourceRanges {
				if sourceRange == "0.0.0.0/0" {
					isPublic = true
					break
				}
			}
		}

		if !isPublic {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, rule.CreationTimestamp)

		resource := cloud.Resource{
			Type:      "vpc-firewall-rule",
			Name:      rule.Name,
			ID:        fmt.Sprintf("%d", rule.Id),
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"direction":     rule.Direction,
				"priority":     rule.Priority,
				"target_tags":  rule.TargetTags,
				"source_ranges": rule.SourceRanges,
				"allowed":      rule.Allowed,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateCloudBuild(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating Cloud Build artifacts...")

	service, err := cloudbuild.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Build service: %v", err)
	}

	builds, err := service.Projects.Locations.Builds.List(fmt.Sprintf("projects/%s/locations/-", e.config.SourceProject)).Do()
	if err != nil {
		return fmt.Errorf("failed to list builds: %v", err)
	}

	for _, build := range builds.Builds {
		createdAt, _ := time.Parse(time.RFC3339, build.CreateTime)

		resource := cloud.Resource{
			Type:      "cloud-build-build",
			Name:      build.Name,
			ID:        build.Id,
			CreatedAt: createdAt,
			Properties: map[string]interface{}{
				"status": build.Status,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

func (e *Enumerator) enumerateGKEClusters(results *cloud.Results, opts []option.ClientOption) error {
	cloud.InfoLogger.Println("Enumerating GKE clusters...")

	service, err := container.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create GKE service: %v", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", e.config.SourceProject)
	clusterList, err := service.Projects.Locations.Clusters.List(parent).Do()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %v", err)
	}

	for _, cluster := range clusterList.Clusters {
		tags := make(map[string]string)
		for key, value := range cluster.ResourceLabels {
			tags[key] = value
		}

		createdAt, _ := time.Parse(time.RFC3339, cluster.CreateTime)

		resource := cloud.Resource{
			Type:      "gke-cluster",
			Name:      cluster.Name,
			ID:        cluster.SelfLink,
			Location:  cluster.Location,
			CreatedAt: createdAt,
			Tags:      tags,
			Properties: map[string]interface{}{
				"status":           cluster.Status,
				"node_count":       cluster.CurrentNodeCount,
				"kubernetes_version": cluster.CurrentMasterVersion,
			},
		}

		results.Resources = append(results.Resources, resource)
	}

	return nil
}

// Helper function to check if a member is public or cross-account
func isPublicOrCrossAccount(member string, projectID string) bool {
	// Public access
	if member == "allUsers" || member == "allAuthenticatedUsers" {
		return true
	}

	// Cross-project service accounts. User-managed service accounts look like
	// NAME@PROJECT_ID.iam.gserviceaccount.com. Google-managed service agents
	// (gcp-sa-*, cloudservices, developer, cloudbuild, etc.) are expected
	// bindings, not cross-project exposure.
	if email, ok := strings.CutPrefix(member, "serviceAccount:"); ok {
		at := strings.LastIndex(email, "@")
		if at < 0 {
			return false
		}
		domain := email[at+1:]
		project, ok := strings.CutSuffix(domain, ".iam.gserviceaccount.com")
		if !ok || strings.HasPrefix(project, "gcp-sa-") {
			return false
		}
		return project != projectID
	}

	// user:, group:, and domain: members can't be classified as internal or
	// external from the project ID alone, so they are not flagged here.
	return false
}

func firstOr(values []string, fallback string) string {
	if len(values) > 0 {
		return values[0]
	}
	return fallback
}
