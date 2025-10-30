module custom-auth

go 1.24.2

replace github.com/jmgilman/go/oci => ../..

require (
	github.com/jmgilman/go/oci v0.0.0-00010101000000-000000000000
	oras.land/oras-go/v2 v2.6.0
)

require (
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	golang.org/x/sync v0.14.0 // indirect
)
