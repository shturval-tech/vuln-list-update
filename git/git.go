package git

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/aquasecurity/vuln-list-update/utils"
)

type Operations interface {
	CloneOrPull(string, string, string) (map[string]struct{}, error)
	RemoteBranch(string) ([]string, error)
	Checkout(string, string) error
}

type Config struct {
}

func (gc Config) CloneOrPull(url, repoPath, branch string, debug bool) (map[string]struct{}, error) {
	exists, err := utils.Exists(filepath.Join(repoPath, ".git"))
	if err != nil {
		return nil, err
	}

	updatedFiles := map[string]struct{}{}
	if exists {
		if debug {
			log.Println("Skip git pull")
			return nil, nil
		}

		log.Println("git pull")
		files, err := pull(url, repoPath, branch)
		if err != nil {
			return nil, xerrors.Errorf("git pull error: %w", err)
		}

		for _, filename := range files {
			updatedFiles[strings.TrimSpace(filename)] = struct{}{}
		}
	} else {
		if err = os.MkdirAll(repoPath, 0700); err != nil {
			return nil, err
		}
		if err := clone(url, repoPath, branch); err != nil {
			return nil, err
		}

		if err := fetchAll(repoPath); err != nil {
			return nil, err
		}
		err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			updatedFiles[path] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return updatedFiles, nil
}

func clone(url, repoPath, branch string) error {
	commandAndArgs := []string{
		"clone",
		"--depth",
		"1",
		url,
		repoPath,
	}
	if branch != "" {
		commandAndArgs = append(commandAndArgs, "-b", branch)
	}
	cmd := exec.Command("git", commandAndArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("failed to clone: %w", err)
	}
	return nil
}

func pull(url, repoPath, branch string) ([]string, error) {
	commandArgs := generateGitArgs(repoPath)

	remoteCmd := []string{
		"remote",
		"get-url",
		"--push",
		"origin",
	}
	output, err := utils.Exec("git", append(commandArgs, remoteCmd...))
	if err != nil {
		return nil, xerrors.Errorf("error in git rev-list: %w", err)
	}
	remoteURL := strings.TrimSpace(output)
	if remoteURL != url {
		return nil, xerrors.Errorf("remote url is %s, target is %s", remoteURL, url)
	}

	revParseCmd := []string{
		"rev-list",
		"-n",
		"1",
		"--all",
	}
	output, err = utils.Exec("git", append(commandArgs, revParseCmd...))
	if err != nil {
		return nil, xerrors.Errorf("error in git rev-list: %w", err)
	}
	commitHash := strings.TrimSpace(output)
	if len(commitHash) == 0 {
		log.Println("no commit yet")
		return nil, nil
	}

	pullCmd := []string{
		"pull",
		"origin",
	}
	if branch != "" {
		pullCmd = append(pullCmd, branch)
	}
	if _, err = utils.Exec("git", append(commandArgs, pullCmd...)); err != nil {
		return nil, xerrors.Errorf("error in git pull: %w", err)
	}

	fetchCmd := []string{
		"fetch",
		"--prune",
	}
	if _, err = utils.Exec("git", append(commandArgs, fetchCmd...)); err != nil {
		return nil, xerrors.Errorf("error in git fetch: %w", err)
	}

	diffCmd := []string{
		"diff",
		commitHash,
		"HEAD",
		"--name-only",
	}
	output, err = utils.Exec("git", append(commandArgs, diffCmd...))
	if err != nil {
		return nil, err
	}
	updatedFiles := strings.Split(strings.TrimSpace(output), "\n")
	return updatedFiles, nil
}

func fetchAll(repoPath string) error {
	commandArgs := generateGitArgs(repoPath)
	configCmd := []string{
		"config",
		"remote.origin.fetch",
		"+refs/heads/*:refs/remotes/origin/*",
	}
	if _, err := utils.Exec("git", append(commandArgs, configCmd...)); err != nil {
		return xerrors.Errorf("error in git config: %w", err)
	}

	fetchCmd := []string{
		"fetch",
		"--all",
	}
	if _, err := utils.Exec("git", append(commandArgs, fetchCmd...)); err != nil {
		return xerrors.Errorf("error in git fetch: %w", err)
	}
	return nil
}

func generateGitArgs(repoPath string) []string {
	gitDir := filepath.Join(repoPath, ".git")
	return []string{
		"--git-dir",
		gitDir,
		"--work-tree",
		repoPath,
	}
}


func (gc Config) Commit(repoPath, targetPath, message string) error {
	commandArgs := generateGitArgs(repoPath)
	addCmd := []string{"add", filepath.Join(repoPath, targetPath)}
	if _, err := utils.Exec("git", append(commandArgs, addCmd...)); err != nil {
		return xerrors.Errorf("error in git add: %w", err)
	}

	commitCmd := []string{"commit", "--message", message}
	if _, err := utils.Exec("git", append(commandArgs, commitCmd...)); err != nil {
		return xerrors.Errorf("error in git commit: %w", err)
	}

	return nil
}

func (gc Config) Push(repoPath, branch string) error {
	commandArgs := generateGitArgs(repoPath)
	pushCmd := []string{"push", "origin", branch}
	if _, err := utils.Exec("git", append(commandArgs, pushCmd...)); err != nil {
		return xerrors.Errorf("error in git push: %w", err)
	}
	return nil
}

func (gc Config) Clean(repoPath string) error {
	commandArgs := generateGitArgs(repoPath)
	resetCmd := []string{"reset", "--hard", "HEAD"}
	if _, err := utils.Exec("git", append(commandArgs, resetCmd...)); err != nil {
		return xerrors.Errorf("git reset error: %w", err)
	}

	cleanCmd := []string{"clean", "-df"}
	if _, err := utils.Exec("git", append(commandArgs, cleanCmd...)); err != nil {
		return xerrors.Errorf("git clean error: %w", err)
	}
	return nil
}

func (gc Config) RemoteBranch(repoPath string) ([]string, error) {
	commandArgs := generateGitArgs(repoPath)
	branchCmd := []string{"branch", "--remote"}
	output, err := utils.Exec("git", append(commandArgs, branchCmd...))
	if err != nil {
		return nil, xerrors.Errorf("error in git branch: %w", err)
	}
	return strings.Split(output, "\n"), nil
}

func (gc Config) Checkout(repoPath string, branch string) error {
	commandArgs := generateGitArgs(repoPath)
	checkoutCmd := []string{"checkout", branch}
	_, err := utils.Exec("git", append(commandArgs, checkoutCmd...))
	if err != nil {
		return xerrors.Errorf("error in git checkout: %w", err)
	}
	return nil
}

func (gc Config) Status(repoPath string) ([]string, error) {
	commandArgs := generateGitArgs(repoPath)

	statusCmd := []string{"status", "--porcelain"}
	output, err := utils.Exec("git", append(commandArgs, statusCmd...))
	if err != nil {
		return nil, xerrors.Errorf("error in git status: %w", err)
	}
	return strings.Split(strings.TrimSpace(output), "\n"), nil
}
