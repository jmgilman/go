package testutil

// Test user information used across all test helpers.
const (
	// TestAuthor is the default author name for test commits.
	TestAuthor = "Test User"
	
	// TestEmail is the default email for test commits.
	TestEmail = "test@example.com"
	
	// TestAuthor2 is an alternate author name for testing multi-user scenarios.
	TestAuthor2 = "Another User"
	
	// TestEmail2 is an alternate email for testing multi-user scenarios.
	TestEmail2 = "another@example.com"
)

// Test repository URLs.
const (
	// TestRepoURL is a sample HTTPS repository URL for testing.
	TestRepoURL = "https://github.com/test/repo.git"
	
	// TestRepoSSHURL is a sample SSH repository URL for testing.
	TestRepoSSHURL = "git@github.com:test/repo.git"
)

// Test file content.
const (
	// TestFileContent is sample content for README files.
	TestFileContent = "# Test Repository\n\nThis is a test repository.\n"
	
	// TestGoFileContent is sample Go source code.
	TestGoFileContent = `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	
	// TestMarkdownContent is sample markdown content.
	TestMarkdownContent = `# Documentation

## Overview

This is test documentation.

## Usage

` + "```" + `go
package main

func main() {
    // Example code
}
` + "```" + `
`

	// TestJSONContent is sample JSON configuration.
	TestJSONContent = `{
  "name": "test-project",
  "version": "1.0.0",
  "description": "A test project"
}
`

	// TestYAMLContent is sample YAML configuration.
	TestYAMLContent = `name: test-project
version: 1.0.0
description: A test project
settings:
  enabled: true
  timeout: 30
`

	// TestConfigContent is sample configuration content.
	TestConfigContent = `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false

[remote "origin"]
	url = https://github.com/test/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
)

// Test commit messages.
const (
	// TestCommitMessage is a standard test commit message.
	TestCommitMessage = "Test commit"
	
	// TestInitialCommit is a message for initial commits.
	TestInitialCommit = "Initial commit"
	
	// TestFeatureCommit is a message for feature commits.
	TestFeatureCommit = "Add new feature"
	
	// TestBugfixCommit is a message for bugfix commits.
	TestBugfixCommit = "Fix critical bug"
	
	// TestMultilineCommit is a multi-line commit message.
	TestMultilineCommit = `Add complex feature

This commit adds a complex feature that required
multiple changes across several files.

- Added new functionality
- Updated documentation
- Fixed related bugs
`
)

// Test branch names.
const (
	// TestBranchName is a standard test branch name.
	TestBranchName = "feature/test-branch"
	
	// TestBranchMain is the main branch name.
	TestBranchMain = "main"
	
	// TestBranchDevelop is the develop branch name.
	TestBranchDevelop = "develop"
	
	// TestBranchRelease is a release branch name.
	TestBranchRelease = "release/v1.0.0"
)

// Test tag names.
const (
	// TestTagName is a standard test tag name.
	TestTagName = "v1.0.0"
	
	// TestTagName2 is a second test tag name.
	TestTagName2 = "v1.1.0"
	
	// TestTagName3 is a third test tag name.
	TestTagName3 = "v2.0.0"
	
	// TestTagMessage is a standard tag message.
	TestTagMessage = "Release version 1.0.0"
)

// Test remote names.
const (
	// TestRemoteName is a standard remote name.
	TestRemoteName = "origin"
	
	// TestRemoteNameUpstream is an upstream remote name.
	TestRemoteNameUpstream = "upstream"
)

// Test file paths.
const (
	// TestFilePath is a standard test file path.
	TestFilePath = "README.md"
	
	// TestFilePath2 is a second test file path.
	TestFilePath2 = "docs/guide.md"
	
	// TestGoFilePath is a Go source file path.
	TestGoFilePath = "main.go"
	
	// TestConfigFilePath is a configuration file path.
	TestConfigFilePath = "config.yaml"
)

// TestSSHPrivateKey is a dummy SSH private key for testing (NOT A REAL KEY).
const TestSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBTEST_KEY_NOT_FOR_REAL_USE_TESTING_ONLY_ZXJhbA==
-----END OPENSSH PRIVATE KEY-----`

// TestSSHPublicKey is a dummy SSH public key for testing (NOT A REAL KEY).
const TestSSHPublicKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITEST_KEY_NOT_FOR_REAL_USE_TESTING_ONLY test@example.com`
