package git

import (
	"fmt"
	"strings"

	"github.com/cszatma/publisher/util"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

type Repository = git.Repository

func RootDir() (string, error) {
	path, err := util.ExecOutput("git", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(path), errors.Wrapf(err, "Failed to get root dir of git repo")
}

func SHA(ref string) (string, error) {
	sha, err := util.ExecOutput("git", "rev-parse", ref)
	return strings.TrimSpace(sha), errors.Wrapf(err, "Failed to get SHA of ref %s", ref)
}

func Clone(name, branch, path string) (*git.Repository, error) {
	util.VerbosePrintf("Cloning repo %s to %s", name, path)
	repo, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:           fmt.Sprintf("git@github.com:%s.git", name),
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone %s to %s", name, path)
	}

	return repo, nil
}

func Open(name, branch, path string) (*git.Repository, error) {
	util.VerbosePrintf("Opening repo %s at path %s", name, path)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open repo at path %s", path)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get worktree for repo %s", name)
	}

	util.VerbosePrintf("Cleaning %s", name)
	err = wt.Clean(&git.CleanOptions{
		Dir: true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clean repo %s", name)
	}

	util.VerbosePrintf("Checkout out branch %s in %s", branch, name)
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
		Force:  true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to checkout branch %s in repo %s", branch, name)
	}

	util.VerbosePrintf("Pulling changes from remote for %s", name)
	err = wt.Pull(&git.PullOptions{
		SingleBranch: true,
		Force:        true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, errors.Wrapf(err, "failed to pull changes from remote for repo %s", name)
	}

	return repo, nil
}