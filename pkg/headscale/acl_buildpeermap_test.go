package headscale

import (
	"encoding/json"
	"net/netip"
	"testing"

	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"gorm.io/gorm"
	"tailscale.com/tailcfg"
)

// privilegedUserName is the Headscale username that owns tag:privileged in the
// scenarios below. It doubles as the entry passed to GenerateTaggedHubSpokePolicy.
const privilegedUserName = "privuser"

func mustAddrPtr(s string) *netip.Addr {
	addr := netip.MustParseAddr(s)
	return &addr
}

func peersContain(peers []types.NodeView, id types.NodeID) bool {
	for _, p := range peers {
		if p.ID() == id {
			return true
		}
	}
	return false
}

// buildPeerMapForPolicy runs a wonder ACLPolicy through the real Headscale
// policy engine and returns the per-node peer map for a two-node mesh:
//
//   - gateway: a tag:privileged node owned by the privileged user
//   - member:  an untagged node owned by a different (normal) user
//
// This is the minimal topology that exercises Headscale's BuildPeerMap, which
// is what actually decides whether two nodes can establish a WireGuard session.
func buildPeerMapForPolicy(t *testing.T, policy *ACLPolicy) (peerMap map[types.NodeID][]types.NodeView, gatewayID, memberID types.NodeID) {
	t.Helper()

	privUser := types.User{Model: gorm.Model{ID: 1}, Name: privilegedUserName, Email: privilegedUserName + "@headscale.net"}
	normUser := types.User{Model: gorm.Model{ID: 2}, Name: "normuser", Email: "normuser@headscale.net"}
	users := []types.User{privUser, normUser}

	gateway := &types.Node{
		ID:         1,
		Hostname:   "gateway",
		IPv4:       mustAddrPtr("100.64.0.1"),
		IPv6:       mustAddrPtr("fd7a:115c:a1e0::1"),
		User:       privUser,
		UserID:     privUser.ID,
		ForcedTags: []string{PrivilegedTag},
		Hostinfo:   &tailcfg.Hostinfo{},
	}
	member := &types.Node{
		ID:       2,
		Hostname: "member",
		IPv4:     mustAddrPtr("100.64.0.2"),
		IPv6:     mustAddrPtr("fd7a:115c:a1e0::2"),
		User:     normUser,
		UserID:   normUser.ID,
		Hostinfo: &tailcfg.Hostinfo{},
	}
	nodes := types.Nodes{gateway, member}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}

	pm, err := hspolicy.NewPolicyManager(policyJSON, users, nodes.ViewSlice())
	if err != nil {
		t.Fatalf("new policy manager: %v", err)
	}

	return pm.BuildPeerMap(nodes.ViewSlice()), gateway.ID, member.ID
}

// TestTaggedPolicy_PrivilegedAndMemberArePeers asserts the behavior the mesh
// actually needs: a privileged node and a normal node must be MUTUAL peers, so
// the privileged gateway can open a WireGuard session to any node.
//
// Headscale resolves peer relationships through BuildPeerMap. When the policy
// uses autogroup:self it takes a per-node code path that does NOT turn the
// one-directional rule `tag:privileged -> *:*` into a mutual peer: the gateway
// sees the member, but the member never gets the gateway in its peer list, so
// the member reports "no matching peer" and no handshake can occur.
//
// This test therefore reproduces the production regression: it is RED against
// the current two-rule GenerateTaggedHubSpokePolicy and is fixed by adding a
// symmetric `autogroup:member -> tag:privileged:*` rule.
func TestTaggedPolicy_PrivilegedAndMemberArePeers(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy([]string{privilegedUserName})
	peerMap, gatewayID, memberID := buildPeerMapForPolicy(t, policy)

	if !peersContain(peerMap[gatewayID], memberID) {
		t.Errorf("privileged gateway should have the member in its peer list, got %v", peerMap[gatewayID])
	}
	if !peersContain(peerMap[memberID], gatewayID) {
		t.Errorf("member should have the privileged gateway in its peer list (else: \"no matching peer\", handshake never completes), got %v", peerMap[memberID])
	}
}
