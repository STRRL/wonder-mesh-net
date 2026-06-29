package headscale

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

// PrivilegedTag is the Headscale tag carried by nodes that belong to a
// privileged network. Nodes assigned this tag (via forced_tags) reach every
// node in the mesh.
const PrivilegedTag = "tag:privileged"

// privilegedPeerAnchorPort is a single, intentionally-unused destination port
// used only to anchor the member<->privileged peer relationship. Headscale's
// autogroup:self BuildPeerMap keys peer visibility off ACL access (CanAccess,
// which matches on IP only), so members must be a source toward tag:privileged
// for the privileged node to appear in their peer list. Granting that on one
// dead port instead of ":*" keeps peering working while preventing members from
// reaching any real service on privileged nodes — preserving the outbound-only
// intent of the legacy hub-spoke policy. Privileged nodes MUST NOT bind this
// port. Real privileged->member traffic flows over the tag:privileged -> *:*
// rule; members never need to initiate to privileged nodes.
const privilegedPeerAnchorPort = 47999

// ACLPolicy represents a Headscale ACL policy
type ACLPolicy struct {
	ACLs      []ACLRule           `json:"acls,omitempty"`
	Groups    map[string][]string `json:"groups,omitempty"`
	TagOwners map[string][]string `json:"tagOwners,omitempty"`
	Hosts     map[string]string   `json:"hosts,omitempty"`
}

// ACLRule represents a single ACL rule
type ACLRule struct {
	Action       string   `json:"action"`
	Sources      []string `json:"src"`
	Destinations []string `json:"dst"`
}

// GenerateWonderNetIsolationPolicy generates an ACL policy that isolates wonder nets
func GenerateWonderNetIsolationPolicy(usernames []string) *ACLPolicy {
	rules := make([]ACLRule, 0, len(usernames))

	for _, username := range usernames {
		rules = append(rules, ACLRule{
			Action:       "accept",
			Sources:      []string{username + "@"},
			Destinations: []string{username + "@:*"},
		})
	}

	return &ACLPolicy{
		ACLs: rules,
	}
}

// GenerateHubSpokePolicy generates an ACL policy where privileged namespaces
// can initiate connections to all nodes, while normal namespaces are isolated
// from each other. Tailscale ACLs are directional and control connection
// initiation only; reply traffic flows back over established connections
// without needing a separate rule.
func GenerateHubSpokePolicy(privilegedUsers []string, normalUsers []string) *ACLPolicy {
	rules := make([]ACLRule, 0, len(privilegedUsers)+len(normalUsers))

	for _, user := range privilegedUsers {
		rules = append(rules, ACLRule{
			Action:       "accept",
			Sources:      []string{user + "@"},
			Destinations: []string{"*:*"},
		})
	}

	for _, username := range normalUsers {
		rules = append(rules, ACLRule{
			Action:       "accept",
			Sources:      []string{username + "@"},
			Destinations: []string{username + "@:*"},
		})
	}

	return &ACLPolicy{ACLs: rules}
}

// GenerateTaggedHubSpokePolicy returns an ACL policy whose size is independent
// of the WonderNet count: at most two rules regardless of how many users exist.
//
//	tag:privileged    -> *:*               (privileged nodes reach everything)
//	autogroup:member  -> autogroup:self:*  (every normal node reaches only its own nodes)
//
// The self-isolation rule relies on Headscale's autogroup:self semantics:
// although the source is autogroup:member (all untagged nodes), the policy
// engine narrows both source and destination to the same user per node, so a
// member only ever sees its own untagged devices. autogroup:self is only valid
// in destinations, so the source must be autogroup:member.
//
// privilegedTagOwners is the list of Headscale usernames allowed to own
// tag:privileged. Actual per-node tag assignment happens out of band via
// SetTags (forced_tags) so existing nodes need not reconnect or re-register.
func GenerateTaggedHubSpokePolicy(privilegedTagOwners []string) *ACLPolicy {
	policy := &ACLPolicy{
		ACLs: []ACLRule{
			{Action: "accept", Sources: []string{"autogroup:member"}, Destinations: []string{"autogroup:self:*"}},
		},
	}

	if len(privilegedTagOwners) > 0 {
		owners := make([]string, len(privilegedTagOwners))
		for i, u := range privilegedTagOwners {
			owners[i] = u + "@"
		}
		policy.TagOwners = map[string][]string{PrivilegedTag: owners}
		// Prepend the privileged rule so it is evaluated first.
		policy.ACLs = append([]ACLRule{
			{Action: "accept", Sources: []string{PrivilegedTag}, Destinations: []string{"*:*"}},
		}, policy.ACLs...)
		// The privileged rule above is one-directional. Headscale's
		// BuildPeerMap (autogroup:self branch) only makes two nodes mutual
		// peers when each is a source toward the other, so tag:privileged ->
		// *:* alone lets the privileged node see members but leaves members
		// without the privileged node in their peer list ("no matching peer",
		// no handshake). This rule makes members a source toward tag:privileged
		// so the peer relationship resolves both ways. It is scoped to a single
		// dead anchor port (see privilegedPeerAnchorPort) so it restores peering
		// without granting members access to any real service on privileged
		// nodes. It does not widen member-to-member access (members stay
		// isolated via autogroup:self).
		policy.ACLs = append(policy.ACLs, ACLRule{
			Action:       "accept",
			Sources:      []string{"autogroup:member"},
			Destinations: []string{fmt.Sprintf("%s:%d", PrivilegedTag, privilegedPeerAnchorPort)},
		})
	}

	return policy
}

// ACLManager manages ACL policies in Headscale
type ACLManager struct {
	client v1.HeadscaleServiceClient
	mu     sync.Mutex
}

// NewACLManager creates a new ACLManager
func NewACLManager(client v1.HeadscaleServiceClient) *ACLManager {
	return &ACLManager{client: client}
}

// SetWonderNetIsolationPolicy sets the wonder net isolation ACL policy
func (am *ACLManager) SetWonderNetIsolationPolicy(ctx context.Context) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	resp, err := am.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	users := resp.GetUsers()
	usernames := make([]string, len(users))
	for i, u := range users {
		usernames[i] = u.GetName()
	}

	policy := GenerateWonderNetIsolationPolicy(usernames)
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	_, err = am.client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(policyJSON)})
	return err
}

// SetHubSpokePolicy sets an ACL policy where privileged namespaces can access
// all nodes while normal namespaces are isolated from each other.
func (am *ACLManager) SetHubSpokePolicy(ctx context.Context, privilegedUsers []string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	resp, err := am.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	privilegedSet := make(map[string]struct{}, len(privilegedUsers))
	for _, u := range privilegedUsers {
		privilegedSet[u] = struct{}{}
	}

	var normalUsers []string
	for _, u := range resp.GetUsers() {
		name := u.GetName()
		if _, ok := privilegedSet[name]; !ok {
			normalUsers = append(normalUsers, name)
		}
	}

	policy := GenerateHubSpokePolicy(privilegedUsers, normalUsers)
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	_, err = am.client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(policyJSON)})
	return err
}

// SetTaggedHubSpokePolicy writes the constant-size tag-based policy. It does
// not touch any node's tags; use EnsurePrivilegedTags for that.
func (am *ACLManager) SetTaggedHubSpokePolicy(ctx context.Context, privilegedUsers []string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	policy := GenerateTaggedHubSpokePolicy(privilegedUsers)
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	_, err = am.client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(policyJSON)})
	return err
}

// EnsurePrivilegedTags assigns PrivilegedTag to every node owned by a user in
// privilegedUsers via Headscale's forced_tags mechanism. It is idempotent and
// preserves any existing tags on each node. Tag changes propagate through the
// next mapper poll, so nodes do not need to reconnect or re-register.
func (am *ACLManager) EnsurePrivilegedTags(ctx context.Context, privilegedUsers []string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if len(privilegedUsers) == 0 {
		return nil
	}

	privileged := make(map[string]struct{}, len(privilegedUsers))
	for _, u := range privilegedUsers {
		privileged[u] = struct{}{}
	}

	resp, err := am.client.ListNodes(ctx, &v1.ListNodesRequest{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	var tagged, skipped, failed int
	for _, node := range resp.GetNodes() {
		if _, ok := privileged[node.GetUser().GetName()]; !ok {
			continue
		}
		if slices.Contains(node.GetForcedTags(), PrivilegedTag) {
			skipped++
			continue
		}

		newTags := append(slices.Clone(node.GetForcedTags()), PrivilegedTag)
		if _, err := am.client.SetTags(ctx, &v1.SetTagsRequest{
			NodeId: node.GetId(),
			Tags:   newTags,
		}); err != nil {
			slog.Warn("set privileged tag", "node_id", node.GetId(), "user", node.GetUser().GetName(), "error", err)
			failed++
			continue
		}
		tagged++
	}

	slog.Info("privileged tag sync", "tagged", tagged, "skipped", skipped, "failed", failed)
	if failed > 0 {
		return fmt.Errorf("privileged tag sync: %d node(s) failed", failed)
	}
	return nil
}

// AddWonderNetToPolicy adds a wonder net to the isolation policy.
//
// Only the legacy per-user policy path calls this. When UseTaggedACL is
// enabled the constant-size policy covers every WonderNet via autogroup:self,
// so this is intentionally not invoked. Kept for the non-tagged (default) path
// and for rollback; do not remove until the legacy path is retired.
func (am *ACLManager) AddWonderNetToPolicy(ctx context.Context, username string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	resp, err := am.client.GetPolicy(ctx, &v1.GetPolicyRequest{})
	if err != nil {
		return fmt.Errorf("get policy: %w", err)
	}

	policyStr := resp.GetPolicy()
	var policy ACLPolicy
	if policyStr != "" {
		if err := json.Unmarshal([]byte(policyStr), &policy); err != nil {
			return fmt.Errorf("unmarshal policy: %w", err)
		}
	}

	newRule := ACLRule{
		Action:       "accept",
		Sources:      []string{username + "@"},
		Destinations: []string{username + "@:*"},
	}

	for _, rule := range policy.ACLs {
		if len(rule.Sources) > 0 && rule.Sources[0] == newRule.Sources[0] {
			return nil
		}
	}

	policy.ACLs = append(policy.ACLs, newRule)

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	_, err = am.client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(policyJSON)})
	return err
}
