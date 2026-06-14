package service

import (
	"context"
	"strings"
)

// buildSnapshot recursively reads the directory tree starting at rootPath
// and returns a tree of FSNode objects.
func (imp *Importer) buildSnapshot(ctx context.Context, rootPath string) (*FSNode, error) {
	return imp.buildSnapshotRecursive(ctx, rootPath)
}

func (imp *Importer) buildSnapshotRecursive(ctx context.Context, dirPath string) (*FSNode, error) {
	entries, err := imp.fs.ReadDir(ctx, dirPath)
	if err != nil {
		return nil, err
	}

	node := &FSNode{
		Path:     dirPath,
		Name:     baseName(dirPath),
		Kind:     ImportNodeDir,
		IsDir:    true,
		Children: make([]*FSNode, 0, len(entries)),
	}

	for _, entry := range entries {
		fullPath := imp.fs.Join(dirPath, entry.Name)

		if entry.IsDir {
			childNode, err := imp.buildSnapshotRecursive(ctx, fullPath)
			if err != nil {
				// Skip directories we can't read
				continue
			}
			node.Children = append(node.Children, childNode)
		} else {
			nodeKind := imp.classifyNodeFile(entry.Name)
			fileNode := &FSNode{
				Path:  fullPath,
				Name:  entry.Name,
				Kind:  nodeKind,
				IsDir: false,
			}
			node.Children = append(node.Children, fileNode)
		}
	}

	return node, nil
}

// classifyNodeFile determines the ImportNodeKind for a file based on its extension.
func (imp *Importer) classifyNodeFile(name string) ImportNodeKind {
	ext := ""
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		ext = strings.ToLower(name[idx:])
	}

	switch ext {
	case ".nfo":
		return ImportNodeNFO
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return ImportNodeImage
	case ".srt", ".sub", ".ass", ".ssa", ".vtt":
		return ImportNodeSubtitle
	default:
		if videoExtensions[ext] {
			return ImportNodeVideo
		}
		return ImportNodeOther
	}
}
