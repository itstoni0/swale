// Package managed marks each inventory resource from a Terragrunt repository.
// Mark reads state as JSON.
// Mark does not write state.
package managed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/itstoni0/swale/internal/inventory"
)

// These errors tell why Mark refused the call or could not read state.
var (
	ErrNilRunner    = errors.New("runner is nil")
	ErrNilContext   = errors.New("context is nil")
	ErrEmptyRoot    = errors.New("repository path is empty")
	ErrNotDirectory = errors.New("repository path is not a directory")
	ErrStateUnread  = errors.New("state could not be read")
)

// Mark sets Managed on each resource in inv.
// Mark reads Terragrunt units under root.
// Mark returns a new resource list.
// Mark leaves inv unchanged.
// A resource is managed when its ID or its ARN is in state.
// A resource is unmanaged when every unit state was read and the ID and the ARN are absent from state.
// A resource is unknown when it has no ID and no ARN.
// A resource is unknown when a unit state could not be read and the resource did not match other state.
func Mark(ctx context.Context, run Runner, root string, inv inventory.Inventory) (inventory.Inventory, error) {
	if run == nil {
		return inv, fmt.Errorf("mark resources: %w", ErrNilRunner)
	}
	if ctx == nil {
		return inv, fmt.Errorf("mark resources: %w", ErrNilContext)
	}
	if err := ctx.Err(); err != nil {
		return inv, fmt.Errorf("mark resources: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return inv, fmt.Errorf("mark resources: %w", ErrEmptyRoot)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return inv, fmt.Errorf("mark resources: %w", err)
	}
	if !info.IsDir() {
		return inv, fmt.Errorf("mark resources: %w: %s", ErrNotDirectory, root)
	}

	out := inv
	out.Resources = slices.Clone(inv.Resources)

	findOut, err := run.Run(ctx, root, "terragrunt", "find", "--json")
	if err != nil {
		assign(out.Resources, nil, true)
		return out, fmt.Errorf("mark resources: %w: find units: %w", ErrStateUnread, err)
	}
	dirs, err := parseFind(root, findOut)
	if err != nil {
		assign(out.Resources, nil, true)
		return out, fmt.Errorf("mark resources: %w: find units: %w", ErrStateUnread, err)
	}

	known := make(map[string]struct{})
	var unread error
	for _, dir := range dirs {
		showOut, runErr := run.Run(ctx, dir, "terragrunt", "show", "-json")
		if runErr == nil {
			ids, parseErr := parseShow(showOut)
			if parseErr == nil {
				for id := range ids {
					known[id] = struct{}{}
				}
				continue
			}
			runErr = parseErr
		}
		unread = errors.Join(unread, fmt.Errorf("show %s: %w", dir, runErr))
	}
	assign(out.Resources, known, unread != nil)
	if unread != nil {
		return out, fmt.Errorf("mark resources: %w: %w", ErrStateUnread, unread)
	}
	return out, nil
}

// foundComponent is one item from `terragrunt find --json`.
type foundComponent struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// parseFind returns the unit directories under root.
// parseFind skips a stack.
// parseFind returns each unit directory once.
func parseFind(root string, data []byte) ([]string, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("find JSON is empty")
	}
	var items []foundComponent
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse find JSON: %w", err)
	}
	dirs := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Type != "unit" {
			continue
		}
		dir, err := unitDir(root, item.Path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// unitDir joins rel to root.
// unitDir rejects a path that leaves root.
func unitDir(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("unit path is empty")
	}
	// filepath.Join places an absolute rel under root.
	// Reject an absolute rel before Join.
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("unit path %q escapes the repository", rel)
	}
	root = filepath.Clean(root)
	full := filepath.Clean(filepath.Join(root, rel))
	sep := string(filepath.Separator)
	if full != root && !strings.HasPrefix(full, root+sep) {
		return "", fmt.Errorf("unit path %q escapes the repository", rel)
	}
	return full, nil
}

// showDocument is the JSON document from `terraform show -json`.
type showDocument struct {
	Values *showValues `json:"values"`
}

// showValues holds the root module from show JSON.
type showValues struct {
	RootModule *moduleState `json:"root_module"`
}

// moduleState is one Terraform module in show JSON.
type moduleState struct {
	Resources    []resourceState `json:"resources"`
	ChildModules []moduleState   `json:"child_modules"`
}

// resourceState is one resource in show JSON.
type resourceState struct {
	Mode   string         `json:"mode"`
	Values map[string]any `json:"values"`
}

// parseShow returns the ID and ARN values for managed resources.
func parseShow(data []byte) (map[string]struct{}, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("state JSON is empty")
	}
	var doc showDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse state JSON: %w", err)
	}
	if doc.Values == nil {
		return nil, errors.New("state JSON has no values")
	}
	ids := make(map[string]struct{})
	if doc.Values.RootModule != nil {
		collect(doc.Values.RootModule, ids)
	}
	return ids, nil
}

// collect adds each managed ID and each managed ARN.
// collect walks each child module.
func collect(mod *moduleState, ids map[string]struct{}) {
	for _, res := range mod.Resources {
		if res.Mode != "managed" {
			continue
		}
		addIdentity(ids, res.Values["id"])
		addIdentity(ids, res.Values["arn"])
	}
	for i := range mod.ChildModules {
		collect(&mod.ChildModules[i], ids)
	}
}

// addIdentity stores a text ID or ARN.
// addIdentity ignores a value that is not text.
// addIdentity ignores blank text.
func addIdentity(ids map[string]struct{}, value any) {
	text, ok := value.(string)
	if !ok {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	ids[text] = struct{}{}
}

// assign sets Managed on each resource.
func assign(resources []inventory.Resource, known map[string]struct{}, unread bool) {
	for i := range resources {
		resources[i].Managed = resourceStatus(resources[i], known, unread)
	}
}

// resourceStatus returns the managed status for one resource.
func resourceStatus(res inventory.Resource, known map[string]struct{}, unread bool) inventory.ManagedStatus {
	id := strings.TrimSpace(res.ID)
	arn := strings.TrimSpace(res.ARN)
	if id == "" && arn == "" {
		return inventory.StatusUnknown
	}
	if id != "" {
		if _, ok := known[id]; ok {
			return inventory.StatusManaged
		}
	}
	if arn != "" {
		if _, ok := known[arn]; ok {
			return inventory.StatusManaged
		}
	}
	if unread {
		return inventory.StatusUnknown
	}
	return inventory.StatusUnmanaged
}
