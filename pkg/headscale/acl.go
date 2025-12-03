package headscale

import (
	"context"
	"encoding/json"
	"fmt"
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

// GenerateTenantIsolationPolicy generates an ACL policy that isolates tenants
// Each tenant can only access their own nodes
func GenerateTenantIsolationPolicy(usernames []string) *ACLPolicy {
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

// GenerateAutogroupSelfPolicy generates a policy using autogroup:self
// This is simpler but may have performance issues at scale
func GenerateAutogroupSelfPolicy() *ACLPolicy {
	return &ACLPolicy{
		ACLs: []ACLRule{
			{
				Action:       "accept",
				Sources:      []string{"autogroup:member"},
				Destinations: []string{"autogroup:self:*"},
			},
		},
	}
}

// ACLManager manages ACL policies in Headscale
type ACLManager struct {
	client *Client
}

// NewACLManager creates a new ACLManager
func NewACLManager(client *Client) *ACLManager {
	return &ACLManager{client: client}
}

// SetTenantIsolationPolicy sets the tenant isolation ACL policy
func (am *ACLManager) SetTenantIsolationPolicy(ctx context.Context) error {
	users, err := am.client.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	usernames := make([]string, len(users))
	for i, u := range users {
		usernames[i] = u.Name
	}

	policy := GenerateTenantIsolationPolicy(usernames)
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	return am.client.SetPolicy(ctx, string(policyJSON))
}

// SetAutogroupSelfPolicy sets the autogroup:self policy (simpler but less scalable)
func (am *ACLManager) SetAutogroupSelfPolicy(ctx context.Context) error {
	policy := GenerateAutogroupSelfPolicy()
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	return am.client.SetPolicy(ctx, string(policyJSON))
}

// AddTenantToPolicy adds a tenant to the isolation policy
func (am *ACLManager) AddTenantToPolicy(ctx context.Context, username string) error {
	policyStr, err := am.client.GetPolicy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}

	var policy ACLPolicy
	if policyStr != "" {
		if err := json.Unmarshal([]byte(policyStr), &policy); err != nil {
			return fmt.Errorf("failed to unmarshal policy: %w", err)
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
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	return am.client.SetPolicy(ctx, string(policyJSON))
}
