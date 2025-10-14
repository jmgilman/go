package git

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CreateTag creates an annotated tag with a message at the specified reference.
//
// An annotated tag is a full object in the Git database with its own hash,
// containing a message, tagger information, and a timestamp. This is the
// recommended tag type for releases and other significant milestones.
//
// The ref parameter can be:
//   - A commit hash (e.g., "abc123...")
//   - A branch name (e.g., "main")
//   - Another tag name
//   - "HEAD" for the current commit
//
// Returns ErrAlreadyExists if a tag with the given name already exists,
// ErrNotFound if the specified reference doesn't exist, or ErrInvalidInput
// for invalid parameters.
//
// Examples:
//
//	// Tag the current HEAD
//	err := repo.CreateTag("v1.0.0", "HEAD", "Release version 1.0.0")
//
//	// Tag a specific commit
//	err := repo.CreateTag("v1.0.1", "abc123", "Hotfix release")
//
//	// Tag a branch
//	err := repo.CreateTag("release-candidate", "develop", "RC for testing")
func (r *Repository) CreateTag(name string, ref string, message string) error {
	if name == "" {
		return wrapError(fmt.Errorf("tag name is required"), "failed to create tag")
	}
	if ref == "" {
		return wrapError(fmt.Errorf("reference is required"), "failed to create tag")
	}
	if message == "" {
		return wrapError(fmt.Errorf("message is required for annotated tag"), "failed to create tag")
	}

	// Resolve the reference to a hash
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return wrapError(err, fmt.Sprintf("failed to resolve reference %q", ref))
	}

	// Check if tag already exists
	tagRef := plumbing.NewTagReferenceName(name)
	if _, err := r.repo.Reference(tagRef, false); err == nil {
		return wrapError(fmt.Errorf("tag %q already exists", name), "failed to create tag")
	}

	// Get repository config for tagger information
	cfg, err := r.repo.Config()
	if err != nil {
		return wrapError(err, "failed to get repository config")
	}

	// Create the annotated tag object
	tag := &object.Tag{
		Name:   name,
		Tagger: object.Signature{
			Name:  cfg.User.Name,
			Email: cfg.User.Email,
			When:  time.Now(),
		},
		Message:    message,
		TargetType: plumbing.CommitObject,
		Target:     *hash,
	}

	// Encode and store the tag object
	obj := r.repo.Storer.NewEncodedObject()
	if err := tag.Encode(obj); err != nil {
		return wrapError(err, "failed to encode tag object")
	}

	tagHash, err := r.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return wrapError(err, "failed to store tag object")
	}

	// Create the reference pointing to the tag object
	newRef := plumbing.NewHashReference(tagRef, tagHash)
	if err := r.repo.Storer.SetReference(newRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to create tag reference %q", name))
	}

	return nil
}

// CreateLightweightTag creates a lightweight tag at the specified reference.
//
// A lightweight tag is simply a reference to a commit, similar to a branch but
// immutable. Unlike annotated tags, lightweight tags don't have a message or
// tagger information. They are useful for temporary or local markers.
//
// The ref parameter can be:
//   - A commit hash (e.g., "abc123...")
//   - A branch name (e.g., "main")
//   - Another tag name
//   - "HEAD" for the current commit
//
// Returns ErrAlreadyExists if a tag with the given name already exists,
// ErrNotFound if the specified reference doesn't exist, or ErrInvalidInput
// for invalid parameters.
//
// Examples:
//
//	// Tag the current HEAD with a lightweight tag
//	err := repo.CreateLightweightTag("build-123", "HEAD")
//
//	// Tag a specific commit
//	err := repo.CreateLightweightTag("tested", "abc123")
func (r *Repository) CreateLightweightTag(name string, ref string) error {
	if name == "" {
		return wrapError(fmt.Errorf("tag name is required"), "failed to create lightweight tag")
	}
	if ref == "" {
		return wrapError(fmt.Errorf("reference is required"), "failed to create lightweight tag")
	}

	// Resolve the reference to a hash
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return wrapError(err, fmt.Sprintf("failed to resolve reference %q", ref))
	}

	// Check if tag already exists
	tagRef := plumbing.NewTagReferenceName(name)
	if _, err := r.repo.Reference(tagRef, false); err == nil {
		return wrapError(fmt.Errorf("tag %q already exists", name), "failed to create lightweight tag")
	}

	// Create a reference directly to the commit (lightweight tag)
	newRef := plumbing.NewHashReference(tagRef, *hash)
	if err := r.repo.Storer.SetReference(newRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to create lightweight tag %q", name))
	}

	return nil
}

// ListTags returns all tags in the repository, including both annotated and lightweight tags.
//
// Annotated tags will have the Message field populated with the tag message.
// Lightweight tags will have an empty Message field.
//
// The Hash field contains the hash of the tagged commit for lightweight tags,
// or the hash of the tag object itself for annotated tags.
//
// Examples:
//
//	tags, err := repo.ListTags()
//	for _, tag := range tags {
//	    if tag.Message != "" {
//	        fmt.Printf("Annotated: %s (%s): %s\n", tag.Name, tag.Hash, tag.Message)
//	    } else {
//	        fmt.Printf("Lightweight: %s (%s)\n", tag.Name, tag.Hash)
//	    }
//	}
func (r *Repository) ListTags() ([]Tag, error) {
	var tags []Tag

	// Get tag references iterator
	tagRefs, err := r.repo.Tags()
	if err != nil {
		return nil, wrapError(err, "failed to get tags")
	}

	// Iterate through all tag references
	err = tagRefs.ForEach(func(ref *plumbing.Reference) error {
		tagName := ref.Name().Short()
		tagHash := ref.Hash()

		// Try to get the tag object to determine if it's annotated
		tagObj, err := r.repo.TagObject(tagHash)
		if err == nil {
			// This is an annotated tag
			tags = append(tags, Tag{
				Name:    tagName,
				Hash:    tagHash,
				Message: tagObj.Message,
			})
			return nil
		}

		// Not an annotated tag, must be lightweight
		// For lightweight tags, the reference points directly to a commit
		tags = append(tags, Tag{
			Name:    tagName,
			Hash:    tagHash,
			Message: "", // Empty for lightweight tags
		})
		return nil
	})

	if err != nil {
		return nil, wrapError(err, "failed to iterate tags")
	}

	return tags, nil
}

// DeleteTag removes a tag from the repository.
//
// This deletes both annotated and lightweight tags. For annotated tags, only
// the reference is removed; the tag object remains in the Git object database
// but becomes unreachable (will be garbage collected eventually).
//
// Returns ErrNotFound if the tag doesn't exist.
//
// Examples:
//
//	// Delete a tag
//	err := repo.DeleteTag("v1.0.0")
//
//	// Delete a lightweight tag
//	err := repo.DeleteTag("build-123")
func (r *Repository) DeleteTag(name string) error {
	if name == "" {
		return wrapError(fmt.Errorf("tag name is required"), "failed to delete tag")
	}

	// Build the tag reference name
	tagRef := plumbing.NewTagReferenceName(name)

	// Check if tag exists
	if _, err := r.repo.Reference(tagRef, false); err != nil {
		return wrapError(err, fmt.Sprintf("failed to find tag %q", name))
	}

	// Delete the reference
	if err := r.repo.Storer.RemoveReference(tagRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to delete tag %q", name))
	}

	return nil
}

