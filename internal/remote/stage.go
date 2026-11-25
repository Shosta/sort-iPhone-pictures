package remote

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// StageMarker identifies a staging folder; it is never uploaded.
const StageMarker = ".photosort-stage"

// Stage lays the items out under root as links to the original photos, so no
// photo data is copied. Symbolic links are used; where they are not allowed
// (some Windows setups) hard links are tried. rsync follows the links (-L).
func Stage(root string, items []Item) ([]Item, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("staging directory must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	// Only a folder that is empty or already a staging folder may be used (and
	// later removed): never somebody's library.
	if existing, err := os.ReadDir(root); err != nil {
		return nil, err
	} else if _, marked := os.Lstat(filepath.Join(root, StageMarker)); marked != nil && len(existing) > 0 {
		return nil, fmt.Errorf("%q is not an empty staging folder", root)
	}
	if err := os.WriteFile(filepath.Join(root, StageMarker), nil, 0o600); err != nil {
		return nil, err
	}
	staged := make([]Item, 0, len(items))
	for _, item := range items {
		if !filepath.IsLocal(item.Target) || item.Target == "." {
			return nil, fmt.Errorf("unsafe target %q", item.Target)
		}
		info, err := os.Stat(item.Source)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%q is not a regular file", item.Source)
		}
		source, err := filepath.Abs(item.Source)
		if err != nil {
			return nil, err
		}
		link := filepath.Join(root, item.Target)
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			return nil, err
		}
		if err := os.Symlink(source, link); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return nil, fmt.Errorf("duplicate target %q", item.Target)
			}
			if linkErr := os.Link(source, link); linkErr != nil {
				return nil, fmt.Errorf("link %q: %w", item.Target, err)
			}
		}
		item.Size = info.Size()
		item.ModTime = info.ModTime()
		staged = append(staged, item)
	}
	return staged, nil
}

// Unstage removes the staged tree. It only deletes links and directories
// (os.RemoveAll never follows symbolic links), never the photos themselves.
func Unstage(root string) error {
	if root == "" || root == string(filepath.Separator) || !filepath.IsAbs(root) {
		return errors.New("refusing to remove " + root)
	}
	if _, err := os.Lstat(filepath.Join(root, StageMarker)); err != nil {
		return errors.New("refusing to remove " + root + ": not a staging folder")
	}
	return os.RemoveAll(root)
}

// Options tunes Send.
type Options struct {
	DryRun     bool
	OnSent     func(Item)
	OnProgress func(Progress)
	// OnPhase reports "prepare", "send" and "verify" as they begin.
	OnPhase func(string)
}

// Send stages the items under stageRoot, uploads them with the transport and
// removes the staged tree, whatever the outcome. Interrupted uploads are
// resumed by calling Send again with the same items.
func Send(ctx context.Context, t Transport, cfg Config, stageRoot string, items []Item, opts Options) (Result, error) {
	if opts.OnPhase != nil {
		opts.OnPhase("prepare")
	}
	staged, err := Stage(stageRoot, items)
	defer Unstage(stageRoot)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return t.Upload(ctx, cfg, Job{Dir: stageRoot, Items: staged, DryRun: opts.DryRun, OnSent: opts.OnSent, OnProgress: opts.OnProgress, OnPhase: opts.OnPhase})
}
