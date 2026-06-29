package headscale

import (
	"fmt"
	"testing"
)

func TestGenerateWonderNetIsolationPolicy(t *testing.T) {
	policy := GenerateWonderNetIsolationPolicy([]string{"user1", "user2"})

	if len(policy.ACLs) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(policy.ACLs))
	}

	assertRule(t, policy.ACLs[0], "accept", []string{"user1@"}, []string{"user1@:*"})
	assertRule(t, policy.ACLs[1], "accept", []string{"user2@"}, []string{"user2@:*"})
}

func TestGenerateHubSpokePolicy(t *testing.T) {
	policy := GenerateHubSpokePolicy([]string{"zeabur"}, []string{"uuid1", "uuid2"})

	// 1 rule for privileged (outbound only) + 2 rules for normal users
	if len(policy.ACLs) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(policy.ACLs))
	}

	assertRule(t, policy.ACLs[0], "accept", []string{"zeabur@"}, []string{"*:*"})
	assertRule(t, policy.ACLs[1], "accept", []string{"uuid1@"}, []string{"uuid1@:*"})
	assertRule(t, policy.ACLs[2], "accept", []string{"uuid2@"}, []string{"uuid2@:*"})
}

func TestGenerateHubSpokePolicy_MultiplePrivileged(t *testing.T) {
	policy := GenerateHubSpokePolicy([]string{"zeabur", "admin"}, []string{"uuid1"})

	// 1 rule per privileged user (2) + 1 normal user
	if len(policy.ACLs) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(policy.ACLs))
	}

	assertRule(t, policy.ACLs[0], "accept", []string{"zeabur@"}, []string{"*:*"})
	assertRule(t, policy.ACLs[1], "accept", []string{"admin@"}, []string{"*:*"})
	assertRule(t, policy.ACLs[2], "accept", []string{"uuid1@"}, []string{"uuid1@:*"})
}

func TestGenerateHubSpokePolicy_NoNormalUsers(t *testing.T) {
	policy := GenerateHubSpokePolicy([]string{"zeabur"}, nil)

	if len(policy.ACLs) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(policy.ACLs))
	}

	assertRule(t, policy.ACLs[0], "accept", []string{"zeabur@"}, []string{"*:*"})
}

func TestGenerateTaggedHubSpokePolicy_ConstantSize(t *testing.T) {
	for _, n := range []int{0, 1, 10, 2105} {
		owners := make([]string, n)
		for i := range owners {
			owners[i] = fmt.Sprintf("u%d", i)
		}

		policy := GenerateTaggedHubSpokePolicy(owners)

		// Constant size regardless of owner count: 3 rules when privileged
		// owners exist (privileged->*, member->self, member->privileged),
		// 1 rule (member->self) when there are none.
		want := 3
		if n == 0 {
			want = 1
		}
		if len(policy.ACLs) != want {
			t.Errorf("n=%d: expected %d rules, got %d", n, want, len(policy.ACLs))
		}
	}
}

func TestGenerateTaggedHubSpokePolicy_RuleShape(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy([]string{"zeabur"})

	owners := policy.TagOwners[PrivilegedTag]
	if len(owners) != 1 || owners[0] != "zeabur@" {
		t.Errorf("tag owners: expected [zeabur@], got %v", owners)
	}

	assertRule(t, policy.ACLs[0], "accept", []string{PrivilegedTag}, []string{"*:*"})
	// autogroup:self is only valid in destinations; the source must be
	// autogroup:member, and the engine narrows it to the same user per node.
	assertRule(t, policy.ACLs[1], "accept", []string{"autogroup:member"}, []string{"autogroup:self:*"})
	// Symmetric rule so privileged and members resolve as mutual peers in
	// Headscale's autogroup:self BuildPeerMap path (see acl_buildpeermap_test),
	// scoped to the dead anchor port so no real service is exposed to members.
	assertRule(t, policy.ACLs[2], "accept", []string{"autogroup:member"}, []string{fmt.Sprintf("%s:%d", PrivilegedTag, privilegedPeerAnchorPort)})
}

func TestGenerateTaggedHubSpokePolicy_NoPrivileged(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy(nil)

	if len(policy.ACLs) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(policy.ACLs))
	}
	if policy.TagOwners != nil {
		t.Errorf("expected no tag owners, got %v", policy.TagOwners)
	}
	assertRule(t, policy.ACLs[0], "accept", []string{"autogroup:member"}, []string{"autogroup:self:*"})
}

func assertRule(t *testing.T, rule ACLRule, action string, src, dst []string) {
	t.Helper()
	if rule.Action != action {
		t.Errorf("expected action %q, got %q", action, rule.Action)
	}
	if len(rule.Sources) != len(src) {
		t.Errorf("expected %d sources, got %d", len(src), len(rule.Sources))
		return
	}
	for i := range src {
		if rule.Sources[i] != src[i] {
			t.Errorf("source[%d]: expected %q, got %q", i, src[i], rule.Sources[i])
		}
	}
	if len(rule.Destinations) != len(dst) {
		t.Errorf("expected %d destinations, got %d", len(dst), len(rule.Destinations))
		return
	}
	for i := range dst {
		if rule.Destinations[i] != dst[i] {
			t.Errorf("destination[%d]: expected %q, got %q", i, dst[i], rule.Destinations[i])
		}
	}
}
