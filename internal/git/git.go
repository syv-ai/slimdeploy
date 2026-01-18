package git

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Manager handles Git operations
type Manager struct {
	deploymentsDir string
	sshKeyPath     string
}

// NewManager creates a new Git manager
func NewManager(deploymentsDir, sshKeyPath string) *Manager {
	return &Manager{
		deploymentsDir: deploymentsDir,
		sshKeyPath:     sshKeyPath,
	}
}

// GetRepoDir returns the directory for a project's repository
func (m *Manager) GetRepoDir(projectName string) string {
	return filepath.Join(m.deploymentsDir, projectName)
}

// getAuth returns the appropriate authentication method
func (m *Manager) getAuth(gitURL string) (transport.AuthMethod, error) {
	// Check if this is an SSH URL
	if isSSHURL(gitURL) && m.sshKeyPath != "" {
		// Check if key file exists
		if _, err := os.Stat(m.sshKeyPath); err == nil {
			auth, err := ssh.NewPublicKeysFromFile("git", m.sshKeyPath, "")
			if err != nil {
				return nil, fmt.Errorf("failed to create SSH auth: %w", err)
			}
			return auth, nil
		}
	}
	return nil, nil // No auth needed for public repos
}

// Clone clones a repository
func (m *Manager) Clone(gitURL, branch, projectName string) error {
	repoDir := m.GetRepoDir(projectName)

	// Remove existing directory if it exists
	if err := os.RemoveAll(repoDir); err != nil {
		return fmt.Errorf("failed to remove existing directory: %w", err)
	}

	// Get auth
	auth, err := m.getAuth(gitURL)
	if err != nil {
		return err
	}

	// Clone options
	cloneOpts := &git.CloneOptions{
		URL:           gitURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
		Progress:      nil,
	}
	if auth != nil {
		cloneOpts.Auth = auth
	}

	// Clone the repository
	_, err = git.PlainClone(repoDir, false, cloneOpts)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// Pull pulls the latest changes from a repository
func (m *Manager) Pull(gitURL, branch, projectName string) error {
	repoDir := m.GetRepoDir(projectName)

	// Open the repository
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		// If repo doesn't exist, clone it
		if err == git.ErrRepositoryNotExists {
			return m.Clone(gitURL, branch, projectName)
		}
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get auth
	auth, err := m.getAuth(gitURL)
	if err != nil {
		return err
	}

	// Pull options
	pullOpts := &git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Force:         true,
	}
	if auth != nil {
		pullOpts.Auth = auth
	}

	// Pull changes
	err = worktree.Pull(pullOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}

// GetLatestCommit gets the latest commit hash for a branch
func (m *Manager) GetLatestCommit(projectName string) (string, error) {
	repoDir := m.GetRepoDir(projectName)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

// GetRemoteLatestCommit fetches and returns the latest commit hash from remote
func (m *Manager) GetRemoteLatestCommit(gitURL, branch, projectName string) (string, error) {
	repoDir := m.GetRepoDir(projectName)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	// Get auth
	auth, err := m.getAuth(gitURL)
	if err != nil {
		return "", err
	}

	// Fetch from remote
	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)),
		},
		Force: true,
	}
	if auth != nil {
		fetchOpts.Auth = auth
	}

	err = repo.Fetch(fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("failed to fetch: %w", err)
	}

	// Get the remote branch reference
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return "", fmt.Errorf("failed to get remote reference: %w", err)
	}

	return ref.Hash().String(), nil
}

// CheckForUpdates checks if there are new commits on the remote
func (m *Manager) CheckForUpdates(gitURL, branch, projectName string) (bool, string, error) {
	// Get current commit
	currentCommit, err := m.GetLatestCommit(projectName)
	if err != nil {
		return false, "", err
	}

	// Get remote commit
	remoteCommit, err := m.GetRemoteLatestCommit(gitURL, branch, projectName)
	if err != nil {
		return false, "", err
	}

	return currentCommit != remoteCommit, remoteCommit, nil
}

// SwitchBranch switches to a different branch
func (m *Manager) SwitchBranch(gitURL, branch, projectName string) error {
	repoDir := m.GetRepoDir(projectName)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get auth
	auth, err := m.getAuth(gitURL)
	if err != nil {
		return err
	}

	// Fetch the branch from remote
	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)),
		},
		Force: true,
	}
	if auth != nil {
		fetchOpts.Auth = auth
	}

	err = repo.Fetch(fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch branch: %w", err)
	}

	// Get worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Checkout the branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewRemoteReferenceName("origin", branch),
		Force:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch: %w", err)
	}

	return nil
}

// Exists checks if a repository exists locally
func (m *Manager) Exists(projectName string) bool {
	repoDir := m.GetRepoDir(projectName)
	_, err := git.PlainOpen(repoDir)
	return err == nil
}

// Remove removes a repository from disk
func (m *Manager) Remove(projectName string) error {
	repoDir := m.GetRepoDir(projectName)
	return os.RemoveAll(repoDir)
}

// isSSHURL checks if a URL is an SSH URL
func isSSHURL(url string) bool {
	return len(url) > 4 && (url[:4] == "git@" || url[:6] == "ssh://")
}

// GetDefaultBranch detects the default branch of a remote repository
func (m *Manager) GetDefaultBranch(gitURL string) (string, error) {
	// Get auth
	auth, err := m.getAuth(gitURL)
	if err != nil {
		return "", err
	}

	// Create remote
	remote := git.NewRemote(nil, &config.RemoteConfig{
		Name: "origin",
		URLs: []string{gitURL},
	})

	// List references
	listOpts := &git.ListOptions{}
	if auth != nil {
		listOpts.Auth = auth
	}

	refs, err := remote.List(listOpts)
	if err != nil {
		return "", fmt.Errorf("failed to list remote refs: %w", err)
	}

	// Find HEAD reference to determine default branch
	var headTarget string
	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD {
			// HEAD is a symbolic reference pointing to the default branch
			headTarget = ref.Target().Short()
			break
		}
	}

	if headTarget != "" {
		return headTarget, nil
	}

	// Fallback: check for common branch names
	branchPriority := []string{"main", "master", "develop", "trunk"}
	branchSet := make(map[string]bool)
	for _, ref := range refs {
		if ref.Name().IsBranch() {
			branchSet[ref.Name().Short()] = true
		}
	}

	for _, branch := range branchPriority {
		if branchSet[branch] {
			return branch, nil
		}
	}

	// Return first branch found
	for _, ref := range refs {
		if ref.Name().IsBranch() {
			return ref.Name().Short(), nil
		}
	}

	return "", fmt.Errorf("no branches found in repository")
}
