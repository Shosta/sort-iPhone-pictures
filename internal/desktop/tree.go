package desktop

import (
	"path/filepath"
	"sort"
	"strings"
)

type fileNode struct {
	name     string
	path     string
	preview  string
	folder   bool
	children []string
}

type fileTree struct {
	nodes map[string]*fileNode
	roots []string
}

func newFileTree() *fileTree {
	return &fileTree{nodes: make(map[string]*fileNode)}
}

func (tree *fileTree) add(root, path, preview string) {
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return
	}
	if tree.nodes[root] == nil {
		tree.nodes[root] = &fileNode{name: filepath.Base(root), path: root, folder: true}
		tree.roots = append(tree.roots, root)
		sort.Strings(tree.roots)
	}
	parent := root
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current := filepath.Join(parent, part)
		if tree.nodes[current] == nil {
			node := &fileNode{name: part, path: current, folder: index < len(parts)-1}
			if !node.folder {
				node.preview = preview
			}
			tree.nodes[current] = node
			tree.nodes[parent].children = append(tree.nodes[parent].children, current)
			sort.Slice(tree.nodes[parent].children, func(left, right int) bool {
				first := tree.nodes[tree.nodes[parent].children[left]]
				second := tree.nodes[tree.nodes[parent].children[right]]
				if first.folder != second.folder {
					return first.folder
				}
				return first.name < second.name
			})
		}
		parent = current
	}
}

func (tree *fileTree) children(id string) []string {
	if id == "" {
		return tree.roots
	}
	if node := tree.nodes[id]; node != nil {
		return node.children
	}
	return nil
}
