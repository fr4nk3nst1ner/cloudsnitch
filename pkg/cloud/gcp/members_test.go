package gcp
import "testing"
func TestMembers(t *testing.T) {
	cases := map[string]bool{
		"allUsers": true,
		"allAuthenticatedUsers": true,
		"serviceAccount:deployer@other-proj.iam.gserviceaccount.com": true,
		"serviceAccount:deployer@my-proj.iam.gserviceaccount.com": false,
		"serviceAccount:service-123@gcp-sa-artifactregistry.iam.gserviceaccount.com": false,
		"serviceAccount:123@cloudservices.gserviceaccount.com": false,
		"serviceAccount:123-compute@developer.gserviceaccount.com": false,
		"user:alice@example.com": false,
	}
	for m, want := range cases {
		if got := isPublicOrCrossAccount(m, "my-proj"); got != want {
			t.Errorf("%s: got %v want %v", m, got, want)
		}
	}
}
