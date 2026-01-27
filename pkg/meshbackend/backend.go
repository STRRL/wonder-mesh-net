// Package meshbackend defines the interface for mesh network backends.
//
// Wonder Mesh Net supports multiple mesh network implementations (Tailscale/Headscale,
// Netbird, ZeroTier, etc.). This package provides a common interface that abstracts
// the underlying mesh technology, allowing the coordinator to work with any backend.
//
// Each backend returns its own metadata structure in CreateJoinCredentials, which
// clients interpret based on the MeshType. This avoids meaningless abstraction of
// backend-specific concepts.
package meshbackend

import (
	"context"
	"time"
)

// MeshType identifies the mesh network implementation.
// Clients use this to determine how to interpret the metadata returned by
// CreateJoinCredentials.
type MeshType string

const (
	MeshTypeTailscale MeshType = "tailscale"
	MeshTypeNetbird   MeshType = "netbird"
	MeshTypeZeroTier  MeshType = "zerotier"
)

// MeshBackend defines the interface for mesh network backends.
//
// Each implementation wraps a specific mesh technology (Headscale, Netbird, etc.)
// and provides a uniform way to manage realms (isolated network namespaces),
// generate join credentials, and list connected nodes.
type MeshBackend interface {
	// MeshType returns the mesh network type.
	// Clients use this to determine how to process the metadata from CreateJoinCredentials.
	MeshType() MeshType

	// CreateRealm creates an isolated network namespace.
	// The realm name should be unique within this backend instance.
	CreateRealm(ctx context.Context, name string) error

	// GetRealm checks if a realm exists.
	// Returns true if the realm exists, false otherwise.
	GetRealm(ctx context.Context, name string) (exists bool, err error)

	// CreateJoinCredentials generates credentials for a node to join the mesh.
	// Returns backend-specific metadata that will be serialized directly to the API response.
	//
	// For Tailscale/Headscale, this returns:
	//   - login_server: the Headscale control URL
	//   - authkey: the PreAuthKey
	//   - headscale_user: the Headscale user/namespace
	//
	// For Netbird (future), this might return:
	//   - setup_key: the setup key
	//   - management_url: the management server URL
	CreateJoinCredentials(ctx context.Context, realmName string, opts JoinOptions) (map[string]any, error)

	// ListNodes returns all nodes in a realm.
	ListNodes(ctx context.Context, realmName string) ([]*Node, error)

	// GetNode retrieves a single node by its ID.
	// Returns the node if found, or an error if not found or on failure.
	GetNode(ctx context.Context, nodeID string) (*Node, error)

	// DeleteNode removes a node from the mesh network.
	// nodeID is the backend-specific node identifier.
	DeleteNode(ctx context.Context, nodeID string) error

	// Healthy performs a health check on the backend.
	Healthy(ctx context.Context) error
}

// JoinOptions configures how join credentials are generated.
type JoinOptions struct {
	// TTL is how long the credential is valid.
	TTL time.Duration

	// Reusable indicates if the credential can be used multiple times.
	Reusable bool

	// Ephemeral indicates if nodes using this credential should be ephemeral
	// (automatically removed when they go offline).
	Ephemeral bool
}

// Node represents a device connected to the mesh network.
// This is a common structure that all backends can populate.
type Node struct {
	// ID is a unique identifier for the node within the backend.
	ID string

	// Name is the human-readable name of the node.
	Name string

	// Addresses are the mesh network IP addresses assigned to this node.
	Addresses []string

	// Online indicates if the node is currently connected.
	Online bool

	// LastSeen is when the node was last seen online.
	// May be nil if the node has never been seen or the backend doesn't track this.
	LastSeen *time.Time

	// Realm is the realm/namespace this node belongs to (e.g., Headscale user).
	// This is populated by GetNode and used for ownership verification.
	Realm string
}
