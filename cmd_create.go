package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"
	"github.com/x-motemen/ghq/cmdutil"
)

func doCreate(c *cli.Context) error {
	var (
		name    = c.Args().First()
		vcs     = c.String("vcs")
		w       = c.App.Writer
		andLook = c.Bool("look")
	)
	u, err := newURL(name, false, true)
	if err != nil {
		return err
	}

	localRepo, err := LocalRepositoryFromURL(u)
	if err != nil {
		return err
	}

	p := localRepo.FullPath
	ok, err := isNotExistOrEmpty(p)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("directory %q already exists and not empty", p)
	}

	remoteRepo, err := NewRemoteRepository(u)
	if err != nil {
		return err
	}

	vcsBackend, ok := vcsRegistry[vcs]
	if !ok {
		vcsBackend, _, err = remoteRepo.VCS()
		if err != nil {
			return err
		}
	}
	if vcsBackend == nil {
		return fmt.Errorf("failed to init: unsupported VCS")
	}

	initFunc := vcsBackend.Init
	if initFunc == nil {
		return fmt.Errorf("failed to init: unsupported VCS")
	}

	if err := os.MkdirAll(p, 0755); err != nil {
		return err
	}

	if err := initFunc(p); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, p)

	if andLook {
		err = look(name)
		if err != nil {
			return err
		}
	}
	return err
}

func isNotExistOrEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func look(name string) error {
	var (
		reposFound []*LocalRepository
		mu         sync.Mutex
	)
	if err := walkAllLocalRepositories(func(repo *LocalRepository) {
		if repo.Matches(name) {
			mu.Lock()
			reposFound = append(reposFound, repo)
			mu.Unlock()
		}
	}); err != nil {
		return err
	}

	if len(reposFound) == 0 {
		if url, err := newURL(name, false, false); err == nil {
			repo, err := LocalRepositoryFromURL(url)
			if err != nil {
				return err
			}
			_, err = os.Stat(repo.FullPath)

			// if the directory exists
			if err == nil {
				reposFound = append(reposFound, repo)
			}
		}
	}

	switch len(reposFound) {
	case 0:
		return fmt.Errorf("No repository found")
	case 1:
		repo := reposFound[0]
		cmd := exec.Command(detectShell())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = repo.FullPath
		cmd.Env = append(os.Environ(), "GHQ_LOOK="+filepath.ToSlash(repo.RelPath))
		return cmdutil.RunCommand(cmd, true)
	default:
		b := &strings.Builder{}
		b.WriteString("More than one repositories are found; Try more precise name\n")
		for _, repo := range reposFound {
			b.WriteString(fmt.Sprintf("       - %s\n", strings.Join(repo.PathParts, "/")))
		}
		return errors.New(b.String())
	}
}
