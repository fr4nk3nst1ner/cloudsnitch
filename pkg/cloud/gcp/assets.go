package gcp

import (
	"fmt"
	"strings"

	"enumcloud/pkg/cloud"

	cloudasset "google.golang.org/api/cloudasset/v1"
	"google.golang.org/api/option"
)

// publicMembersQuery matches IAM bindings that grant a role to everyone or to
// any Google account.
const publicMembersQuery = "policy:(allUsers OR allAuthenticatedUsers)"

// enumerateAssetInventory uses Cloud Asset Inventory to find every IAM policy
// in the scope that grants access to allUsers or allAuthenticatedUsers. One
// search covers every resource type that supports IAM policies, and the scope
// can be a project, folder, or organization, so a single call can audit a
// whole org.
//
// Requires the Cloud Asset API (cloudasset.googleapis.com) to be enabled on
// the quota project and the caller to hold cloudasset.assets.searchAllIamPolicies
// (e.g. roles/cloudasset.viewer) on the scope.
func (e *Enumerator) enumerateAssetInventory(results *cloud.Results, opts []option.ClientOption) error {
	scope := e.config.GCPScope
	if scope == "" {
		scope = "projects/" + e.config.SourceProject
	}
	cloud.InfoLogger.Printf("Searching Cloud Asset Inventory for public IAM bindings in %s...", scope)

	service, err := cloudasset.NewService(e.ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Asset service: %v", err)
	}

	return service.V1.SearchAllIamPolicies(scope).
		Query(publicMembersQuery).
		PageSize(500).
		Pages(e.ctx, func(resp *cloudasset.SearchAllIamPoliciesResponse) error {
			for _, r := range resp.Results {
				if res, ok := assetPolicyToResource(r); ok {
					results.Resources = append(results.Resources, res)
				}
			}
			return nil
		})
}

// publicBinding is one role granted to allUsers or allAuthenticatedUsers.
type publicBinding struct {
	Role      string   `json:"role"`
	Members   []string `json:"members"`
	Condition string   `json:"condition,omitempty"`
}

// publicBindings returns only the bindings in the policy that grant a role to
// allUsers or allAuthenticatedUsers, listing only those members.
func publicBindings(policy *cloudasset.Policy) []publicBinding {
	if policy == nil {
		return nil
	}
	var out []publicBinding
	for _, b := range policy.Bindings {
		var members []string
		for _, m := range b.Members {
			if m == "allUsers" || m == "allAuthenticatedUsers" {
				members = append(members, m)
			}
		}
		if len(members) == 0 {
			continue
		}
		pb := publicBinding{Role: b.Role, Members: members}
		if b.Condition != nil {
			pb.Condition = b.Condition.Expression
		}
		out = append(out, pb)
	}
	return out
}

// assetPolicyToResource converts a search result into a Resource, or returns
// false if none of its bindings are actually public.
func assetPolicyToResource(r *cloudasset.IamPolicySearchResult) (cloud.Resource, bool) {
	if r == nil {
		return cloud.Resource{}, false
	}
	bindings := publicBindings(r.Policy)
	if len(bindings) == 0 {
		return cloud.Resource{}, false
	}

	exposure := "authenticated-users"
	for _, b := range bindings {
		for _, m := range b.Members {
			if m == "allUsers" {
				exposure = "public"
			}
		}
	}

	return cloud.Resource{
		Type:     "asset-inventory/" + r.AssetType,
		Name:     assetName(r.Resource),
		ID:       r.Resource,
		Location: r.Project,
		Properties: map[string]interface{}{
			"exposure":        exposure,
			"public_bindings": bindings,
			"project":         r.Project,
			"folders":         r.Folders,
			"organization":    r.Organization,
		},
	}, true
}

// assetName returns the last path segment of a full resource name such as
// //storage.googleapis.com/my-bucket.
func assetName(fullName string) string {
	trimmed := strings.TrimRight(fullName, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i < len(trimmed)-1 {
		return trimmed[i+1:]
	}
	return fullName
}
