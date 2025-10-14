package git

import (
	"fmt"
	"iter"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogit "github.com/go-git/go-git/v5"
)

// CreateCommit creates a new commit with the specified options.
//
// By default, CreateCommit will fail if there are no changes to commit (clean
// working tree). Use the AllowEmpty option to create commits without changes,
// which is useful for triggers, markers, or testing.
//
// The commit is created on the current HEAD. To commit to a different branch,
// first checkout that branch using CheckoutBranch.
//
// Returns the commit hash as a string, or an error if the commit fails.
// Common errors include ErrConflict for a clean working tree without AllowEmpty,
// or ErrInvalidInput for missing author/email/message.
//
// Examples:
//
//	// Create a commit with changes
//	hash, err := repo.CreateCommit(git.CommitOptions{
//	    Author:  "John Doe",
//	    Email:   "john@example.com",
//	    Message: "Add new feature",
//	})
//
//	// Create an empty commit (e.g., for triggering CI)
//	hash, err := repo.CreateCommit(git.CommitOptions{
//	    Author:     "Bot",
//	    Email:      "bot@example.com",
//	    Message:    "Trigger rebuild",
//	    AllowEmpty: true,
//	})
func (r *Repository) CreateCommit(opts CommitOptions) (string, error) {
	// Validate required fields
	if opts.Author == "" {
		return "", wrapError(fmt.Errorf("author is required"), "failed to create commit")
	}
	if opts.Email == "" {
		return "", wrapError(fmt.Errorf("email is required"), "failed to create commit")
	}
	if opts.Message == "" {
		return "", wrapError(fmt.Errorf("message is required"), "failed to create commit")
	}

	// Get the worktree
	wt, err := r.repo.Worktree()
	if err != nil {
		return "", wrapError(err, "failed to get worktree")
	}

	// Create the commit using go-git's Worktree.Commit
	hash, err := wt.Commit(opts.Message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  opts.Author,
			Email: opts.Email,
		},
		AllowEmptyCommits: opts.AllowEmpty,
	})

	if err != nil {
		return "", wrapError(err, "failed to create commit")
	}

	return hash.String(), nil
}

// WalkCommits walks the commit history starting from a reference and returns
// commits in reverse chronological order (newest to oldest).
//
// The iterator yields commits one at a time, enabling true streaming without
// loading all commits into memory. This is especially efficient for limiting
// results with early termination using 'break'.
//
// The from parameter specifies where to stop (exclusive). When empty, walks
// all ancestors from 'to'.
//
// The to parameter is required and can be:
//   - A commit hash (e.g., "abc123...")
//   - A branch name (e.g., "main", "develop")
//   - A tag name (e.g., "v1.0.0")
//   - "HEAD" for the current commit
//
// The iterator yields (Commit, error) pairs. Check for errors on each iteration.
// The walk includes the commit pointed to by 'to' but excludes the commit
// pointed to by 'from'. This matches the behavior of "git log from..to".
//
// Examples:
//
//	// Get last 10 commits (newest first)
//	count := 0
//	for commit, err := range repo.WalkCommits("", "HEAD") {
//	    if err != nil { return err }
//	    if count >= 10 { break }
//	    fmt.Println(commit.Message)
//	    count++
//	}
//
//	// Walk all commits from HEAD
//	for commit, err := range repo.WalkCommits("", "HEAD") {
//	    if err != nil { return err }
//	    // process commit (newest→oldest order)
//	}
//
//	// Walk between two refs
//	for commit, err := range repo.WalkCommits("v1.0.0", "v2.0.0") {
//	    if err != nil { return err }
//	    // commits from v2.0.0 back to (but not including) v1.0.0
//	}
func (r *Repository) WalkCommits(from, to string) iter.Seq2[Commit, error] {
	return func(yield func(Commit, error) bool) {
		// Validate required 'to' parameter
		if to == "" {
			yield(Commit{}, wrapError(fmt.Errorf("to reference is required"), "failed to walk commits"))
			return
		}

		// Resolve 'to' reference to a commit hash
		toHash, err := r.repo.ResolveRevision(plumbing.Revision(to))
		if err != nil {
			yield(Commit{}, wrapError(err, fmt.Sprintf("failed to resolve to reference %q", to)))
			return
		}

		// Resolve 'from' reference if provided
		var fromHash *plumbing.Hash
		if from != "" {
			fromHash, err = r.repo.ResolveRevision(plumbing.Revision(from))
			if err != nil {
				yield(Commit{}, wrapError(err, fmt.Sprintf("failed to resolve from reference %q", from)))
				return
			}

			// If from and to are the same, return immediately (no commits)
			if *fromHash == *toHash {
				return
			}
		}

		// Get the commit object for 'to'
		toCommit, err := r.repo.CommitObject(*toHash)
		if err != nil {
			yield(Commit{}, wrapError(err, fmt.Sprintf("failed to get commit for %q", to)))
			return
		}

		// Create commit iterator (yields newest→oldest)
		commitsSeen := make(map[plumbing.Hash]bool)
		iter := object.NewCommitPreorderIter(toCommit, commitsSeen, nil)
		defer iter.Close()

		// Stream commits one at a time
		err = iter.ForEach(func(c *object.Commit) error {
			// Stop when we reach the from commit (exclusive)
			if fromHash != nil && c.Hash == *fromHash {
				return nil
			}

			// Yield this commit
			commit := Commit{
				Hash:      c.Hash.String(),
				Author:    c.Author.Name,
				Email:     c.Author.Email,
				Message:   c.Message,
				Timestamp: c.Author.When,
				raw:       c,
			}

			if !yield(commit, nil) {
				// User called break, stop iteration
				return fmt.Errorf("iteration stopped")
			}

			return nil
		})

		// If iteration failed (not due to user break), yield the error
		if err != nil && err.Error() != "iteration stopped" {
			yield(Commit{}, wrapError(err, "failed to iterate commits"))
		}
	}
}

// GetCommit retrieves a single commit by reference.
//
// The ref parameter can be:
//   - A commit hash (e.g., "abc123...")
//   - A branch name (e.g., "main")
//   - A tag name (e.g., "v1.0.0")
//   - "HEAD" for the current commit
//
// Returns ErrNotFound if the reference doesn't exist or doesn't point to a commit.
//
// Examples:
//
//	// Get commit by hash
//	commit, err := repo.GetCommit("abc123")
//
//	// Get current HEAD commit
//	commit, err := repo.GetCommit("HEAD")
//
//	// Get commit for a tag
//	commit, err := repo.GetCommit("v1.0.0")
func (r *Repository) GetCommit(ref string) (*Commit, error) {
	if ref == "" {
		return nil, wrapError(fmt.Errorf("reference is required"), "failed to get commit")
	}

	// Resolve the reference to a commit hash
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, wrapError(err, fmt.Sprintf("failed to resolve reference %q", ref))
	}

	// Get the commit object
	commitObj, err := r.repo.CommitObject(*hash)
	if err != nil {
		return nil, wrapError(err, fmt.Sprintf("failed to get commit for %q", ref))
	}

	// Convert to our Commit type
	commit := &Commit{
		Hash:      commitObj.Hash.String(),
		Author:    commitObj.Author.Name,
		Email:     commitObj.Author.Email,
		Message:   commitObj.Message,
		Timestamp: commitObj.Author.When,
		raw:       commitObj,
	}

	return commit, nil
}

// Underlying returns the underlying go-git commit object for advanced operations
// not covered by this wrapper. This escape hatch allows direct access to
// go-git's full commit API when needed.
//
// The returned *object.Commit can be used for any go-git commit operation,
// such as accessing the tree, parent commits, or other low-level details.
//
// Example:
//
//	commit, _ := repo.GetCommit("HEAD")
//	gogitCommit := commit.Underlying()
//	tree, _ := gogitCommit.Tree()
//	// Use go-git tree operations...
func (c *Commit) Underlying() *object.Commit {
	return c.raw
}

