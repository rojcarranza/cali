// Package store keeps pristine copies of upstream skill folders, keyed by git
// tree SHA, under <data>/pristine/<tree>. They are the merge base.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/rojcarranza/cali/internal/skillfs"
)

var treeRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

func checkTree(tree string) error {
	if !treeRE.MatchString(tree) {
		return fmt.Errorf("invalid tree SHA %q, want 40 lowercase hex characters", tree)
	}
	return nil
}

// Put stores t as the pristine copy for tree. If one exists it must equal t.
// The copy is written to a temporary sibling and renamed into place.
func Put(dataDir, tree string, t skillfs.Tree) error {
	if err := checkTree(tree); err != nil {
		return err
	}
	if have, ok, err := Get(dataDir, tree); err != nil {
		return err
	} else if ok {
		if skillfs.Equal(have, t) {
			return nil
		}
		return fmt.Errorf("pristine %s already exists with different content", tree)
	}
	root := filepath.Join(dataDir, "pristine")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(root, ".tmp-")
	if err != nil {
		return err
	}
	if err := t.WriteTo(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(root, tree)); err != nil {
		_ = os.RemoveAll(tmp)
		// A concurrent Put may have won the race; accept an equal copy.
		if have, ok, gerr := Get(dataDir, tree); gerr == nil && ok {
			if skillfs.Equal(have, t) {
				return nil
			}
			return fmt.Errorf("pristine %s already exists with different content", tree)
		}
		return err
	}
	return nil
}

// Get reads the pristine copy for tree. It returns (nil, false, nil) when absent.
func Get(dataDir, tree string) (skillfs.Tree, bool, error) {
	if err := checkTree(tree); err != nil {
		return nil, false, err
	}
	dir := filepath.Join(dataDir, "pristine", tree)
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	t, err := skillfs.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}
