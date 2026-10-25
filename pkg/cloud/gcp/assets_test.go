package gcp

import (
	"testing"

	cloudasset "google.golang.org/api/cloudasset/v1"
)

func TestPublicBindings(t *testing.T) {
	policy := &cloudasset.Policy{Bindings: []*cloudasset.Binding{
		{Role: "roles/storage.objectViewer", Members: []string{"allUsers", "user:alice@example.com"}},
		{Role: "roles/storage.admin", Members: []string{"group:admins@example.com"}},
		{
			Role:      "roles/run.invoker",
			Members:   []string{"allAuthenticatedUsers"},
			Condition: &cloudasset.Expr{Expression: "request.time < timestamp('2030-01-01T00:00:00Z')"},
		},
	}}

	got := publicBindings(policy)
	if len(got) != 2 {
		t.Fatalf("expected 2 public bindings, got %d: %+v", len(got), got)
	}
	if got[0].Role != "roles/storage.objectViewer" || len(got[0].Members) != 1 || got[0].Members[0] != "allUsers" {
		t.Errorf("first binding should keep only allUsers: %+v", got[0])
	}
	if got[1].Condition == "" {
		t.Errorf("condition should be carried through: %+v", got[1])
	}
	if publicBindings(nil) != nil {
		t.Errorf("nil policy should yield nil")
	}
}

func TestAssetPolicyToResource(t *testing.T) {
	r := &cloudasset.IamPolicySearchResult{
		AssetType: "storage.googleapis.com/Bucket",
		Resource:  "//storage.googleapis.com/open-bucket",
		Project:   "projects/123",
		Policy: &cloudasset.Policy{Bindings: []*cloudasset.Binding{
			{Role: "roles/storage.objectViewer", Members: []string{"allUsers"}},
		}},
	}
	res, ok := assetPolicyToResource(r)
	if !ok {
		t.Fatal("expected a resource")
	}
	if res.Type != "asset-inventory/storage.googleapis.com/Bucket" || res.Name != "open-bucket" {
		t.Errorf("unexpected type/name: %q %q", res.Type, res.Name)
	}
	if res.Properties["exposure"] != "public" {
		t.Errorf("expected public exposure, got %v", res.Properties["exposure"])
	}

	r.Policy.Bindings[0].Members = []string{"allAuthenticatedUsers"}
	if res, _ := assetPolicyToResource(r); res.Properties["exposure"] != "authenticated-users" {
		t.Errorf("expected authenticated-users exposure, got %v", res.Properties["exposure"])
	}

	r.Policy.Bindings[0].Members = []string{"user:alice@example.com"}
	if _, ok := assetPolicyToResource(r); ok {
		t.Errorf("non-public policy should be dropped")
	}
}
