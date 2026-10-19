package message

import (
	"fmt"
	"text/template"
	"text/template/parse"

	"github.com/patchcaster-app/patchcaster/internal/markdown"
)

// functions are the functions that templates may call besides the
// comparisons and the logic of text/template (ADR-038). None of them reads
// files, the network or the database, and none returns forge text
// unescaped.
var functions = template.FuncMap{
	"mention": mention,
	"code":    code,
	"plural":  plural,
}

// allowedBuiltins are the functions of text/template that templates may
// call. The others go: call, index, slice, len, print, printf, println,
// html, js and urlquery could cut forge text apart or print it in another
// form.
var allowedBuiltins = map[string]bool{
	"and": true, "or": true, "not": true,
	"eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// mention writes a mention of target: "here" pings the people in the
// channel who are online, "channel" pings everyone in it.
func mention(target string) (string, error) {
	markup, ok := markdown.MentionMarkup(markdown.MentionTarget(target))
	if !ok {
		return "", fmt.Errorf("mention knows here and channel, not %q", target)
	}
	return markup, nil
}

// code shows forge text as a code span.
func code(text Text) string {
	return markdown.Code(string(text))
}

// plural words a count with the singular noun at one and the plural
// otherwise, as "1 comment" or "3 comments".
func plural(n int, singular, pluralNoun string) string {
	return countOf(n, singular, pluralNoun)
}

// checkFunctions refuses a template that calls a function outside the
// documented set, or that uses range, template or define. Without range,
// a message's size depends on its values only, which keeps the size check
// of a template honest.
func checkFunctions(tree *parse.Tree) error {
	return walk(tree, tree.Root)
}

// walk checks node and everything below it.
func walk(tree *parse.Tree, node parse.Node) error {
	switch n := node.(type) {
	case *parse.ListNode:
		return walkAll(tree, n.Nodes)
	case *parse.ActionNode:
		return walk(tree, n.Pipe)
	case *parse.IfNode:
		return walkBranch(tree, &n.BranchNode)
	case *parse.WithNode:
		return walkBranch(tree, &n.BranchNode)
	case *parse.PipeNode:
		for _, command := range n.Cmds {
			if err := walkAll(tree, command.Args); err != nil {
				return err
			}
		}
		return nil
	case *parse.ChainNode:
		return walk(tree, n.Node)
	case *parse.IdentifierNode:
		if allowedBuiltins[n.Ident] || functions[n.Ident] != nil {
			return nil
		}
		return refusal(tree, n, fmt.Sprintf("%s is not a template function", n.Ident))
	case *parse.RangeNode:
		return refusal(tree, n, "range is not available")
	case *parse.TemplateNode:
		return refusal(tree, n, "template is not available")
	default:
		return nil
	}
}

// walkAll checks each of nodes.
func walkAll(tree *parse.Tree, nodes []parse.Node) error {
	for _, node := range nodes {
		if err := walk(tree, node); err != nil {
			return err
		}
	}
	return nil
}

// walkBranch checks an if or a with: its condition and both of its lists.
func walkBranch(tree *parse.Tree, branch *parse.BranchNode) error {
	if err := walk(tree, branch.Pipe); err != nil {
		return err
	}
	if err := walk(tree, branch.List); err != nil {
		return err
	}
	if branch.ElseList == nil {
		return nil
	}
	return walk(tree, branch.ElseList)
}

// refusal returns an Error at node's line.
func refusal(tree *parse.Tree, node parse.Node, cause string) error {
	location, _ := tree.ErrorContext(node)
	return &Error{Line: lineOf(location), Cause: cause}
}
