// Package steamach detects Steam achievement unlocks from the local Steam
// client's cache files, without the Web API.
package steamach

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Binary KeyValues type bytes, as written by the Steam client.
const (
	kvNone    byte = 0x00 // subtree
	kvString  byte = 0x01
	kvInt32   byte = 0x02
	kvFloat32 byte = 0x03
	kvPointer byte = 0x04
	kvWString byte = 0x05
	kvColor   byte = 0x06
	kvUint64  byte = 0x07
	kvEnd     byte = 0x08
	kvInt64   byte = 0x0a
	kvEndAlt  byte = 0x0b
)

var ErrTruncated = errors.New("steamach: truncated keyvalues")

// Node is one KeyValues entry: either a subtree (Children) or a scalar.
type Node struct {
	Key      string
	Type     byte
	Str      string
	Int      int64
	Float    float32
	Children []*Node
}

// Child returns the first direct child whose key matches, ignoring case.
func (n *Node) Child(key string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// Get walks a path of keys.
func (n *Node) Get(path ...string) *Node {
	cur := n
	for _, key := range path {
		cur = cur.Child(key)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// String returns the scalar as text (numbers are formatted).
func (n *Node) String() string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case kvString, kvWString:
		return n.Str
	case kvInt32, kvUint64, kvInt64, kvColor, kvPointer:
		return strconv.FormatInt(n.Int, 10)
	case kvFloat32:
		return strconv.FormatFloat(float64(n.Float), 'g', -1, 32)
	}
	return ""
}

// Int64 returns the scalar as an integer; strings are parsed, others yield 0.
func (n *Node) Int64() int64 {
	if n == nil {
		return 0
	}
	switch n.Type {
	case kvString, kvWString:
		v, _ := strconv.ParseInt(strings.TrimSpace(n.Str), 10, 64)
		return v
	case kvFloat32:
		return int64(n.Float)
	}
	return n.Int
}

// ParseBinary decodes a binary KeyValues blob (UserGameStats*.bin). The result
// is a synthetic root whose Children are the top-level entries.
func ParseBinary(data []byte) (*Node, error) {
	p := &binParser{data: data}
	children, err := p.list(0)
	if err != nil {
		return nil, err
	}
	return &Node{Type: kvNone, Children: children}, nil
}

type binParser struct {
	data []byte
	pos  int
}

const maxKVDepth = 64

func (p *binParser) list(depth int) ([]*Node, error) {
	if depth > maxKVDepth {
		return nil, errors.New("steamach: keyvalues nested too deep")
	}
	var out []*Node
	for {
		if p.pos >= len(p.data) {
			if depth == 0 {
				return out, nil
			}
			return nil, ErrTruncated
		}
		t := p.data[p.pos]
		p.pos++
		if t == kvEnd || t == kvEndAlt {
			return out, nil
		}
		key, err := p.cstring()
		if err != nil {
			return nil, err
		}
		node := &Node{Key: key, Type: t}
		switch t {
		case kvNone:
			node.Children, err = p.list(depth + 1)
			if err != nil {
				return nil, err
			}
		case kvString:
			node.Str, err = p.cstring()
			if err != nil {
				return nil, err
			}
		case kvWString:
			node.Str, err = p.wstring()
			if err != nil {
				return nil, err
			}
		case kvInt32, kvColor, kvPointer:
			b, err := p.take(4)
			if err != nil {
				return nil, err
			}
			node.Int = int64(int32(binary.LittleEndian.Uint32(b)))
		case kvFloat32:
			b, err := p.take(4)
			if err != nil {
				return nil, err
			}
			node.Float = math.Float32frombits(binary.LittleEndian.Uint32(b))
		case kvUint64, kvInt64:
			b, err := p.take(8)
			if err != nil {
				return nil, err
			}
			node.Int = int64(binary.LittleEndian.Uint64(b))
		default:
			return nil, fmt.Errorf("steamach: unknown keyvalues type 0x%02x at offset %d", t, p.pos-1)
		}
		out = append(out, node)
	}
}

func (p *binParser) take(n int) ([]byte, error) {
	if p.pos+n > len(p.data) {
		return nil, ErrTruncated
	}
	b := p.data[p.pos : p.pos+n]
	p.pos += n
	return b, nil
}

func (p *binParser) cstring() (string, error) {
	start := p.pos
	for p.pos < len(p.data) {
		if p.data[p.pos] == 0 {
			s := string(p.data[start:p.pos])
			p.pos++
			return s, nil
		}
		p.pos++
	}
	return "", ErrTruncated
}

func (p *binParser) wstring() (string, error) {
	var units []uint16
	for {
		b, err := p.take(2)
		if err != nil {
			return "", err
		}
		u := binary.LittleEndian.Uint16(b)
		if u == 0 {
			return string(utf16.Decode(units)), nil
		}
		units = append(units, u)
	}
}

// ParseText decodes a text KeyValues/VDF document (loginusers.vdf,
// libraryfolders.vdf, appmanifest_*.acf). Values are stored as strings.
func ParseText(data string) (*Node, error) {
	p := &textParser{src: data}
	children, err := p.list(0)
	if err != nil {
		return nil, err
	}
	return &Node{Type: kvNone, Children: children}, nil
}

type textParser struct {
	src string
	pos int
}

func (p *textParser) list(depth int) ([]*Node, error) {
	if depth > maxKVDepth {
		return nil, errors.New("steamach: vdf nested too deep")
	}
	var out []*Node
	for {
		tok, kind := p.token()
		switch kind {
		case tokEOF:
			if depth == 0 {
				return out, nil
			}
			return nil, ErrTruncated
		case tokClose:
			return out, nil
		case tokOpen:
			return nil, fmt.Errorf("steamach: unexpected '{' at %d", p.pos)
		}
		node := &Node{Key: tok}
		val, vkind := p.token()
		switch vkind {
		case tokOpen:
			children, err := p.list(depth + 1)
			if err != nil {
				return nil, err
			}
			node.Type = kvNone
			node.Children = children
		case tokString:
			node.Type = kvString
			node.Str = val
		default:
			return nil, fmt.Errorf("steamach: key %q without value", tok)
		}
		out = append(out, node)
	}
}

const (
	tokEOF = iota
	tokString
	tokOpen
	tokClose
)

func (p *textParser) token() (string, int) {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		case c == '{':
			p.pos++
			return "", tokOpen
		case c == '}':
			p.pos++
			return "", tokClose
		case c == '"':
			p.pos++
			var sb strings.Builder
			for p.pos < len(p.src) {
				ch := p.src[p.pos]
				if ch == '\\' && p.pos+1 < len(p.src) {
					next := p.src[p.pos+1]
					switch next {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						sb.WriteByte(next)
					}
					p.pos += 2
					continue
				}
				if ch == '"' {
					p.pos++
					return sb.String(), tokString
				}
				sb.WriteByte(ch)
				p.pos++
			}
			return sb.String(), tokString
		default:
			start := p.pos
			for p.pos < len(p.src) {
				ch := p.src[p.pos]
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '{' || ch == '}' || ch == '"' {
					break
				}
				p.pos++
			}
			return p.src[start:p.pos], tokString
		}
	}
	return "", tokEOF
}
