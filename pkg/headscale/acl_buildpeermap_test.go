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

// newTaggedTwoNodePolicy builds the minimal two-node mesh used to exercise the
// real Headscale policy engine:
//
//   - gateway: a tag:privileged node owned by the privileged user
//   - member:  an untagged node owned by a different (normal) user
//
// It returns the live PolicyManager so tests can inspect both peer
// relationships (BuildPeerMap) and the compiled packet filter (FilterForNode).
func newTaggedTwoNodePolicy(t *testing.T, policy *ACLPolicy) (pm hspolicy.PolicyManager, gateway, member *types.Node, nodes types.Nodes) {
	t.Helper()

	privUser := types.User{Model: gorm.Model{ID: 1}, Name: privilegedUserName, Email: privilegedUserName + "@headscale.net"}
	normUser := types.User{Model: gorm.Model{ID: 2}, Name: "normuser", Email: "normuser@headscale.net"}
	users := []types.User{privUser, normUser}

	gateway = &types.Node{
		ID:         1,
		Hostname:   "gateway",
		IPv4:       mustAddrPtr("100.64.0.1"),
		IPv6:       mustAddrPtr("fd7a:115c:a1e0::1"),
		User:       privUser,
		UserID:     privUser.ID,
		ForcedTags: []string{PrivilegedTag},
		Hostinfo:   &tailcfg.Hostinfo{},
	}
	member = &types.Node{
		ID:       2,
		Hostname: "member",
		IPv4:     mustAddrPtr("100.64.0.2"),
		IPv6:     mustAddrPtr("fd7a:115c:a1e0::2"),
		User:     normUser,
		UserID:   normUser.ID,
		Hostinfo: &tailcfg.Hostinfo{},
	}
	nodes = types.Nodes{gateway, member}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}

	pm, err = hspolicy.NewPolicyManager(policyJSON, users, nodes.ViewSlice())
	if err != nil {
		t.Fatalf("new policy manager: %v", err)
	}
	return pm, gateway, member, nodes
}

func peersContain(peers []types.NodeView, id types.NodeID) bool {
	for _, p := range peers {
		if p.ID() == id {
			return true
		}
	}
	return false
}

// cidrOrIPContains reports whether a filter token (an IP, CIDR, or "*") covers ip.
func cidrOrIPContains(token string, ip netip.Addr) bool {
	if token == "*" {
		return true
	}
	if p, err := netip.ParsePrefix(token); err == nil {
		return p.Contains(ip)
	}
	if a, err := netip.ParseAddr(token); err == nil {
		return a == ip
	}
	return false
}

func srcMatches(rule tailcfg.FilterRule, src netip.Addr) bool {
	for _, s := range rule.SrcIPs {
		if cidrOrIPContains(s, src) {
			return true
		}
	}
	return false
}

// filterAllowsAllPorts reports whether rules permit src -> dst across the full
// 0-65535 port range.
func filterAllowsAllPorts(rules []tailcfg.FilterRule, src, dst netip.Addr) bool {
	for _, r := range rules {
		if !srcMatches(r, src) {
			continue
		}
		for _, d := range r.DstPorts {
			if cidrOrIPContains(d.IP, dst) && d.Ports.First == 0 && d.Ports.Last == 65535 {
				return true
			}
		}
	}
	return false
}

// filterAllowsPort reports whether rules permit src -> dst on a specific port.
func filterAllowsPort(rules []tailcfg.FilterRule, src, dst netip.Addr, port uint16) bool {
	for _, r := range rules {
		if !srcMatches(r, src) {
			continue
		}
		for _, d := range r.DstPorts {
			if cidrOrIPContains(d.IP, dst) && d.Ports.Contains(port) {
				return true
			}
		}
	}
	return false
}

// TestTaggedPolicy_PrivilegedAndMemberArePeers asserts the behavior the mesh
// actually needs: a privileged node and a normal node must be MUTUAL peers, so
// the privileged gateway can open a WireGuard session to any node.
//
// Headscale resolves peer relationships through BuildPeerMap. Under autogroup:self
// the one-directional rule tag:privileged -> *:* makes the gateway see the
// member but not vice-versa ("no matching peer"); the symmetric member ->
// tag:privileged rule fixes it. This reproduced the production regression
// (wonder-gateway -> 100.64.0.40).
func TestTaggedPolicy_PrivilegedAndMemberArePeers(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy([]string{privilegedUserName})
	pm, gateway, member, nodes := newTaggedTwoNodePolicy(t, policy)
	peerMap := pm.BuildPeerMap(nodes.ViewSlice())

	if !peersContain(peerMap[gateway.ID], member.ID) {
		t.Errorf("privileged gateway should have the member in its peer list, got %v", peerMap[gateway.ID])
	}
	if !peersContain(peerMap[member.ID], gateway.ID) {
		t.Errorf("member should have the privileged gateway in its peer list (else \"no matching peer\", handshake never completes), got %v", peerMap[member.ID])
	}
}

// TestTaggedPolicy_PrivilegedReachesMemberOnAllPorts proves the anchor-port
// scoping does NOT limit real connectivity: the privileged gateway must still
// reach the member on EVERY port. Gateway->member traffic flows over
// tag:privileged -> *:*, which is independent of the anchor-port rule. This is
// the direction the mesh actually uses.
func TestTaggedPolicy_PrivilegedReachesMemberOnAllPorts(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy([]string{privilegedUserName})
	pm, gateway, member, _ := newTaggedTwoNodePolicy(t, policy)

	rules, err := pm.FilterForNode(member.View())
	if err != nil {
		t.Fatalf("filter for member: %v", err)
	}

	if !filterAllowsAllPorts(rules, *gateway.IPv4, *member.IPv4) {
		t.Errorf("member's filter must allow the privileged gateway inbound on ALL ports (gateway->member connectivity), got %+v", rules)
	}
}

// TestTaggedPolicy_MemberToPrivilegedLimitedToAnchorPort proves the security
// property: the member -> privileged direction is restricted to the dead anchor
// port, so members cannot reach a real service (e.g. SSH) on privileged nodes.
func TestTaggedPolicy_MemberToPrivilegedLimitedToAnchorPort(t *testing.T) {
	policy := GenerateTaggedHubSpokePolicy([]string{privilegedUserName})
	pm, gateway, member, _ := newTaggedTwoNodePolicy(t, policy)

	rules, err := pm.FilterForNode(gateway.View())
	if err != nil {
		t.Fatalf("filter for gateway: %v", err)
	}

	if filterAllowsAllPorts(rules, *member.IPv4, *gateway.IPv4) {
		t.Errorf("member must NOT reach the privileged gateway on all ports (outbound-only intent), got %+v", rules)
	}
	if !filterAllowsPort(rules, *member.IPv4, *gateway.IPv4, privilegedPeerAnchorPort) {
		t.Errorf("member should reach the privileged gateway on the anchor port %d (peering anchor), got %+v", privilegedPeerAnchorPort, rules)
	}
	if filterAllowsPort(rules, *member.IPv4, *gateway.IPv4, 22) {
		t.Errorf("member must NOT reach the privileged gateway on a real service port (22/SSH), got %+v", rules)
	}
}
