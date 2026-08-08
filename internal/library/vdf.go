// SPDX-License-Identifier: GPL-3.0-only

// Package library scans a user's local Steam installation — library
// folders and installed-app manifests — to power `rigfit library` (§11,
// §13 M9). Every read is confined to files Steam itself created
// (libraryfolders.vdf, appmanifest_*.acf) under directories Steam itself
// reported; this package never walks the wider filesystem.
package library

import (
	"fmt"
	"strings"
)

// node is one parsed KeyValues (VDF) node: either a leaf (Value set,
// Children nil) or an object (Children set, Value ignored). Valve's
// format — used for libraryfolders.vdf and appmanifest_*.acf alike — is a
// simple nested "key" "value" / "key" { ... } text format; there is no
// approved dependency for it (§2), and the grammar is small enough that a
// hand-rolled parser is the right call over adding one.
type node struct {
	Value    string
	Children map[string]*node
}

func (n *node) str(key string) (string, bool) {
	if n == nil {
		return "", false
	}
	child, ok := n.Children[key]
	if !ok || child.Children != nil {
		return "", false
	}
	return child.Value, true
}

func (n *node) obj(key string) (*node, bool) {
	if n == nil {
		return nil, false
	}
	child, ok := n.Children[key]
	if !ok || child.Children == nil {
		return nil, false
	}
	return child, true
}

// parseVDF parses a full KeyValues document: a single root "key" { ... }
// pair. It returns the root object and its key (e.g. "libraryfolders",
// "AppState").
func parseVDF(data []byte) (root *node, rootKey string, err error) {
	p := &vdfParser{data: data}
	p.skipTrivia()
	rootKey, ok := p.readToken()
	if !ok {
		return nil, "", fmt.Errorf("library: empty or malformed VDF document")
	}
	root, err = p.parseValue()
	if err != nil {
		return nil, "", err
	}
	return root, rootKey, nil
}

type vdfParser struct {
	data []byte
	pos  int
}

// skipTrivia skips whitespace and "// line comment" runs — Valve's own
// VDF files occasionally carry comments even though the format doesn't
// formally specify them.
func (p *vdfParser) skipTrivia() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.pos++
			continue
		}
		if c == '/' && p.pos+1 < len(p.data) && p.data[p.pos+1] == '/' {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

// readToken reads either a brace ("{"/"}") or a quoted string, with
// backslash-escape handling (VDF escapes backslashes and quotes, e.g.
// Windows paths appear as "C:\\Program Files (x86)\\Steam").
func (p *vdfParser) readToken() (string, bool) {
	p.skipTrivia()
	if p.pos >= len(p.data) {
		return "", false
	}
	c := p.data[p.pos]
	if c == '{' || c == '}' {
		p.pos++
		return string(c), true
	}
	if c != '"' {
		return "", false
	}
	p.pos++
	var sb strings.Builder
	for p.pos < len(p.data) && p.data[p.pos] != '"' {
		if p.data[p.pos] == '\\' && p.pos+1 < len(p.data) {
			sb.WriteByte(p.data[p.pos+1])
			p.pos += 2
			continue
		}
		sb.WriteByte(p.data[p.pos])
		p.pos++
	}
	if p.pos >= len(p.data) {
		return "", false // unterminated string
	}
	p.pos++ // closing quote
	return sb.String(), true
}

func (p *vdfParser) parseValue() (*node, error) {
	tok, ok := p.readToken()
	if !ok {
		return nil, fmt.Errorf("library: unexpected end of VDF input")
	}
	if tok == "{" {
		return p.parseObjectBody()
	}
	if tok == "}" {
		return nil, fmt.Errorf("library: unexpected '}' in VDF input")
	}
	return &node{Value: tok}, nil
}

func (p *vdfParser) parseObjectBody() (*node, error) {
	obj := &node{Children: map[string]*node{}}
	for {
		p.skipTrivia()
		if p.pos >= len(p.data) {
			return nil, fmt.Errorf("library: unterminated object in VDF input")
		}
		if p.data[p.pos] == '}' {
			p.pos++
			return obj, nil
		}
		key, ok := p.readToken()
		if !ok || key == "{" || key == "}" {
			return nil, fmt.Errorf("library: expected a key, got %q", key)
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.Children[key] = val
	}
}
