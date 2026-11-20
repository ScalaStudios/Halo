package filter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Kind int

const (
	String Kind = iota
	Boolean
	DateTime
)

type Attr struct {
	Expr  string
	Kind  Kind
	Exact bool
}

type Schema struct {
	Attrs    map[string]Attr
	Multi    map[string]string
	Prefixes map[string]string
}

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	maxLength = 4096
	maxDepth  = 16
)

type token struct {
	kind byte
	text string
}

type parser struct {
	toks   []token
	pos    int
	schema Schema
	args   []any
	depth  int
}

func Compile(raw string, schema Schema) (string, []any, error) {
	if strings.TrimSpace(raw) == "" {
		return "true", nil, nil
	}
	if len(raw) > maxLength {
		return "", nil, Error(fmt.Sprintf("The filter is longer than %d characters.", maxLength))
	}
	toks, err := tokenize(raw)
	if err != nil {
		return "", nil, err
	}
	p := &parser{toks: toks, schema: schema}
	sql, err := p.or("")
	if err != nil {
		return "", nil, err
	}
	if t := p.peek(); t.kind != 0 {
		return "", nil, Error("Unexpected " + describe(t) + " in the filter.")
	}
	return sql, p.args, nil
}

func tokenize(raw string) ([]token, error) {
	var toks []token
	for i := 0; i < len(raw); {
		c := raw[i]
		switch {
		case strings.IndexByte(" \t\r\n", c) >= 0:
			i++
		case strings.IndexByte("()[]", c) >= 0:
			toks = append(toks, token{kind: c})
			i++
		case c == '"':
			j := i + 1
			for ; j < len(raw) && raw[j] != '"'; j++ {
				if raw[j] == '\\' {
					j++
				}
			}
			var s string
			if j >= len(raw) || json.Unmarshal([]byte(raw[i:j+1]), &s) != nil || strings.ContainsRune(s, 0) {
				return nil, Error("A string value in the filter is not a valid JSON string. Quote values with \" and escape quotes inside them as \\\".")
			}
			toks = append(toks, token{kind: 's', text: s})
			i = j + 1
		default:
			j := i
			for j < len(raw) && strings.IndexByte(" \t\r\n()[]\"", raw[j]) < 0 {
				j++
			}
			toks = append(toks, token{kind: 'w', text: raw[i:j]})
			i = j
		}
	}
	return toks, nil
}

func describe(t token) string {
	switch t.kind {
	case 0:
		return "end of filter"
	case 's':
		return strconv.Quote(t.text)
	case 'w':
		return t.text
	}
	return string(t.kind)
}

func (p *parser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{}
}

func (p *parser) take() token {
	t := p.peek()
	if t.kind != 0 {
		p.pos++
	}
	return t
}

func (p *parser) accept(kind byte) bool {
	if p.peek().kind == kind {
		p.pos++
		return true
	}
	return false
}

func (p *parser) keyword(word string) bool {
	if t := p.peek(); t.kind == 'w' && strings.EqualFold(t.text, word) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expect(kind byte) error {
	if t := p.take(); t.kind != kind {
		return Error(fmt.Sprintf("Expected %c but found %s in the filter.", kind, describe(t)))
	}
	return nil
}

func (p *parser) or(parent string) (string, error) {
	if p.depth++; p.depth > maxDepth {
		return "", Error(fmt.Sprintf("The filter nests more than %d levels deep.", maxDepth))
	}
	defer func() { p.depth-- }()
	left, err := p.and(parent)
	for err == nil && p.keyword("or") {
		var right string
		right, err = p.and(parent)
		left = "(" + left + " or " + right + ")"
	}
	return left, err
}

func (p *parser) and(parent string) (string, error) {
	left, err := p.unary(parent)
	for err == nil && p.keyword("and") {
		var right string
		right, err = p.unary(parent)
		left = "(" + left + " and " + right + ")"
	}
	return left, err
}

func (p *parser) unary(parent string) (string, error) {
	negate := p.keyword("not")
	if !p.accept('(') {
		if negate {
			return "", Error("not must be followed by a condition in parentheses, such as not (active eq true).")
		}
		return p.attribute(parent)
	}
	inner, err := p.or(parent)
	if err == nil {
		err = p.expect(')')
	}
	if negate {
		return "(not " + inner + ")", err
	}
	return "(" + inner + ")", err
}

func (p *parser) attribute(parent string) (string, error) {
	t := p.take()
	if t.kind != 'w' {
		return "", Error("Expected an attribute name but found " + describe(t) + " in the filter.")
	}
	path := strings.ToLower(t.text)
	for prefix, replacement := range p.schema.Prefixes {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			path = replacement + rest
		}
	}
	if parent != "" {
		return p.compare(parent + "." + path)
	}
	if !p.accept('[') {
		cond, err := p.compare(path)
		if multi, ok := p.schema.Multi[strings.SplitN(path, ".", 2)[0]]; ok {
			cond = fmt.Sprintf(multi, cond)
		}
		return cond, err
	}
	inner, err := p.or(path)
	if err == nil {
		err = p.expect(']')
	}
	if sub := p.peek(); err == nil && sub.kind == 'w' && strings.HasPrefix(sub.text, ".") {
		p.pos++
		var cond string
		cond, err = p.compare(path + strings.ToLower(sub.text))
		inner = "(" + inner + " and " + cond + ")"
	}
	if multi, ok := p.schema.Multi[path]; ok {
		return fmt.Sprintf(multi, inner), err
	}
	return inner, err
}

func (p *parser) arg(v any, cast string) string {
	p.args = append(p.args, v)
	return "$" + strconv.Itoa(len(p.args)) + "::" + cast
}

func (p *parser) compare(path string) (string, error) {
	cond, err := p.condition(path)
	return "(" + cond + ")", err
}

func (p *parser) condition(path string) (string, error) {
	a, ok := p.schema.Attrs[path]
	if !ok {
		return "", Error(path + " is not an attribute Halo can filter on.")
	}
	expr := "(" + a.Expr + ")"
	present := expr + " is not null"
	if a.Kind == String {
		present = "coalesce(" + expr + ", '') <> ''"
	}
	t := p.take()
	op := strings.ToLower(t.text)
	if t.kind == 'w' && op == "pr" {
		return present, nil
	}
	if t.kind != 'w' || !strings.Contains(" eq ne co sw ew gt ge lt le ", " "+op+" ") {
		return "", Error(fmt.Sprintf("Expected an operator such as eq, ne, co, sw, ew, pr, gt, ge, lt or le after %s.", path))
	}
	v := p.take()
	var value any
	switch {
	case v.kind == 's':
		value = v.text
	case v.kind == 'w' && strings.EqualFold(v.text, "true"):
		value = true
	case v.kind == 'w' && strings.EqualFold(v.text, "false"):
		value = false
	case v.kind == 'w' && strings.EqualFold(v.text, "null"):
		switch op {
		case "eq":
			return "not (" + present + ")", nil
		case "ne":
			return present, nil
		}
		return "", Error("Only eq and ne can compare with null.")
	default:
		return "", Error(fmt.Sprintf("Expected a value such as \"text\", true, false or null after %s %s.", path, op))
	}
	switch a.Kind {
	case Boolean:
		b, ok := value.(bool)
		if !ok || (op != "eq" && op != "ne") {
			return "", Error(path + " is true or false, so it supports eq, ne and pr with true or false.")
		}
		return relation(expr, op, p.arg(b, "boolean")), nil
	case DateTime:
		s, _ := value.(string)
		at, err := time.Parse(time.RFC3339Nano, s)
		if err != nil || op == "co" || op == "sw" || op == "ew" {
			return "", Error(path + ` is a date and time, so it supports eq, ne, gt, ge, lt, le and pr with a value such as "2026-01-31T09:00:00Z".`)
		}
		return relation(expr, op, p.arg(at, "timestamptz")), nil
	}
	s, ok := value.(string)
	if !ok {
		return "", Error(path + ` is text, so compare it with a quoted value such as "sam".`)
	}
	right := p.arg(s, "text")
	if !a.Exact {
		expr, right = "lower("+expr+")", "lower("+right+")"
	}
	switch op {
	case "co":
		return "coalesce(strpos(" + expr + ", " + right + ") > 0, false)", nil
	case "sw":
		return "coalesce(starts_with(" + expr + ", " + right + "), false)", nil
	case "ew":
		return "coalesce(right(" + expr + ", char_length(" + right + ")) = " + right + ", false)", nil
	}
	return relation(expr, op, right), nil
}

func relation(left, op, right string) string {
	switch op {
	case "eq":
		return left + " = " + right + " and " + left + " is not null"
	case "ne":
		return left + " is distinct from " + right
	}
	symbol := map[string]string{"gt": ">", "ge": ">=", "lt": "<", "le": "<="}[op]
	return "coalesce(" + left + " " + symbol + " " + right + ", false)"
}
