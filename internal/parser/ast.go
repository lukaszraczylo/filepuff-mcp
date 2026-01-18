package parser

import (
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// FindNodeAtPosition finds the node at the given line and column.
func FindNodeAtPosition(tree *sitter.Tree, line, col int) *sitter.Node {
	if tree == nil {
		return nil
	}

	root := tree.RootNode()
	if root == nil {
		return nil
	}

	// Convert to 0-indexed
	point := sitter.Point{
		Row:    uint32(line - 1), // #nosec G115 - line numbers are bounded by file size
		Column: uint32(col - 1),  // #nosec G115 - column numbers are bounded by line length
	}

	return findNodeAtPoint(root, point)
}

// findNodeAtPoint recursively finds the smallest node containing the point.
func findNodeAtPoint(node *sitter.Node, point sitter.Point) *sitter.Node {
	if node == nil {
		return nil
	}

	startPoint := node.StartPoint()
	endPoint := node.EndPoint()

	// Check if point is within this node
	if !pointInRange(point, startPoint, endPoint) {
		return nil
	}

	// Try to find a more specific child node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if result := findNodeAtPoint(child, point); result != nil {
			return result
		}
	}

	// No child contains the point, return this node
	return node
}

// pointInRange checks if a point is within a range.
func pointInRange(point, start, end sitter.Point) bool {
	// Before start?
	if point.Row < start.Row || (point.Row == start.Row && point.Column < start.Column) {
		return false
	}
	// After end?
	if point.Row > end.Row || (point.Row == end.Row && point.Column >= end.Column) {
		return false
	}
	return true
}

// FindParentOfKind finds the nearest ancestor of the given node type.
func FindParentOfKind(node *sitter.Node, kind string) *sitter.Node {
	if node == nil {
		return nil
	}

	current := node.Parent()
	for current != nil {
		if current.Type() == kind {
			return current
		}
		current = current.Parent()
	}
	return nil
}

// GetNodeText returns the text content of a node.
func GetNodeText(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	start := node.StartByte()
	end := node.EndByte()

	if int(start) >= len(content) || int(end) > len(content) {
		return ""
	}

	return string(content[start:end])
}

// WalkTree walks the tree calling fn for each node.
// If fn returns false, the walk stops.
func WalkTree(node *sitter.Node, fn func(*sitter.Node) bool) {
	if node == nil {
		return
	}

	if !fn(node) {
		return
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		WalkTree(node.Child(i), fn)
	}
}

// FindNodesByKind finds all nodes of a given kind.
func FindNodesByKind(root *sitter.Node, kind string) []*sitter.Node {
	var nodes []*sitter.Node

	WalkTree(root, func(n *sitter.Node) bool {
		if n.Type() == kind {
			nodes = append(nodes, n)
		}
		return true
	})

	return nodes
}

// FindNamedChildren returns all named (non-anonymous) children of a node.
func FindNamedChildren(node *sitter.Node) []*sitter.Node {
	if node == nil {
		return nil
	}

	var children []*sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if child := node.NamedChild(i); child != nil {
			children = append(children, child)
		}
	}
	return children
}

// GetChildByFieldName returns the child node with the given field name.
func GetChildByFieldName(node *sitter.Node, fieldName string) *sitter.Node {
	if node == nil {
		return nil
	}
	return node.ChildByFieldName(fieldName)
}

// NodeLocation returns the location of a node.
func NodeLocation(node *sitter.Node, filename string) protocol.Location {
	if node == nil {
		return protocol.Location{}
	}

	startPoint := node.StartPoint()
	return protocol.Location{
		File:   filename,
		Line:   int(startPoint.Row) + 1,
		Column: int(startPoint.Column) + 1,
	}
}

// NodeRange returns the range of a node.
func NodeRange(node *sitter.Node, filename string) protocol.Range {
	if node == nil {
		return protocol.Range{}
	}

	startPoint := node.StartPoint()
	endPoint := node.EndPoint()

	return protocol.Range{
		Start: protocol.Location{
			File:   filename,
			Line:   int(startPoint.Row) + 1,
			Column: int(startPoint.Column) + 1,
		},
		End: protocol.Location{
			File:   filename,
			Line:   int(endPoint.Row) + 1,
			Column: int(endPoint.Column) + 1,
		},
	}
}
