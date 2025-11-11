package github

// Client provides high-level GitHub operations.
// It serves as the main entry point for interacting with GitHub resources.
//
// Example usage:
//
//	// Create provider
//	provider, err := github.NewSDKProvider(github.SDKWithToken("ghp_..."))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create GitHub client
//	ghClient := github.NewClient(provider, "myorg")
//
//	// Access repositories
//	repo := client.Repository("myrepo")
type Client struct {
	provider Provider
	owner    string // default owner for operations
}

// NewClient creates a new GitHub client with the specified provider.
// The owner parameter sets a default owner for operations (can be overridden
// in individual method calls).
//
// The owner is typically an organization name or username that will be used
// as the default for repository and other resource operations.
func NewClient(provider Provider, owner string) *Client {
	return &Client{
		provider: provider,
		owner:    owner,
	}
}

// Repository returns a Repository instance for the given repository name.
// The repository name should not include the owner (e.g., "myrepo" not "owner/myrepo").
// The repository uses the client's default owner unless overridden.
//
// Note: This method does not validate that the repository exists. Call Get()
// on the returned Repository to fetch its data and ensure it exists.
func (c *Client) Repository(name string) *Repository {
	return &Repository{
		client: c,
		owner:  c.owner,
		name:   name,
		data:   nil,
	}
}

// Provider returns the underlying Provider.
// This is an escape hatch that allows direct access to the provider
// for operations not covered by the high-level Client API.
func (c *Client) Provider() Provider {
	return c.provider
}

// Owner returns the default owner for this client.
func (c *Client) Owner() string {
	return c.owner
}
