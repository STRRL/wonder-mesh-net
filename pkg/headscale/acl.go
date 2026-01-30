package headscale

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

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

// GenerateEmptyPolicy generates an empty policy with no rules (deny all by default)
func GenerateEmptyPolicy() *ACLPolicy {
	return &ACLPolicy{
		ACLs: []ACLRule{},
	}
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

// SetEmptyPolicy sets an empty ACL policy (deny all by default, isolation enforced)
func (am *ACLManager) SetEmptyPolicy(ctx context.Context) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	policy := GenerateEmptyPolicy()
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	_, err = am.client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(policyJSON)})
	return err
}

// AddWonderNetToPolicy adds a wonder net to the isolation policy
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
