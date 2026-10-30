package rule

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Position %d: %s", e.Pos, e.Msg)
}

type Expr struct {
	Op          string
	Attr        string
	Values      []string
	Left, Right *Expr
	Pos         int
}

var columns = map[string]string{
	"status":      "u.status",
	"department":  "u.department",
	"location":    "u.location",
	"title":       "u.title",
	"source":      "u.source",
	"email":       "u.email",
	"emailDomain": "split_part(u.email, '@', 2)",
	"name":        "u.name",
	"kind":        "u.kind",
	"manager":     "coalesce((select mgr.email from users mgr where mgr.id = u.manager_id), '')",
}

const attributeList = "status, department, location, title, source, email, emailDomain, name, kind, manager or memberOf"

type token struct {
	kind byte
	text string
	pos  int
}

func lex(src string) ([]token, error) {
	rs := []rune(src)
	var out []token
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '"':
			end := slices.Index(rs[i+1:], '"')
			if end < 0 {
				return nil, &Error{i + 1, `this quote is never closed. Add a closing ".`}
			}
			out = append(out, token{'s', string(rs[i+1 : i+1+end]), i + 1})
			i += end + 2
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, token{'w', string(rs[i:j]), i + 1})
			i = j
		case (r == '=' || r == '!') && i+1 < len(rs) && rs[i+1] == '=':
			out = append(out, token{'p', string(rs[i : i+2]), i + 1})
			i += 2
		case strings.ContainsRune(".()[],", r):
			out = append(out, token{'p', string(r), i + 1})
			i++
		default:
			return nil, &Error{i + 1, fmt.Sprintf("unexpected %q. Compare with ==, and use true to match everyone.", r)}
		}
	}
	return append(out, token{0, "", len(rs) + 1}), nil
}

type parser struct {
	toks []token
	i    int
}

func Parse(src string) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if t := p.toks[p.i]; t.kind != 0 {
		return nil, p.fail(t, "and, or, or the end of the rule")
	}
	return e, nil
}

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != 0 {
		p.i++
	}
	return t
}

func (p *parser) accept(text string) bool {
	if t := p.toks[p.i]; t.kind != 's' && t.kind != 0 && t.text == text {
		p.i++
		return true
	}
	return false
}

func (p *parser) expect(text string) error {
	if !p.accept(text) {
		return p.fail(p.toks[p.i], text)
	}
	return nil
}

func (p *parser) fail(t token, want string) error {
	found := fmt.Sprintf("%q", t.text)
	switch t.kind {
	case 0:
		found = "the end of the rule"
	case 's':
		found = "the value " + found
	}
	return &Error{t.pos, "expected " + want + ", found " + found + "."}
}

func (p *parser) or() (*Expr, error) {
	return p.binary("or", p.and)
}

func (p *parser) and() (*Expr, error) {
	return p.binary("and", p.unary)
}

func (p *parser) binary(op string, operand func() (*Expr, error)) (*Expr, error) {
	left, err := operand()
	for err == nil && p.accept(op) {
		var right *Expr
		right, err = operand()
		left = &Expr{Op: op, Left: left, Right: right}
	}
	return left, err
}

func (p *parser) unary() (*Expr, error) {
	if !p.accept("not") {
		return p.primary()
	}
	x, err := p.unary()
	return &Expr{Op: "not", Left: x}, err
}

func (p *parser) primary() (*Expr, error) {
	t := p.next()
	switch {
	case t.kind == 'p' && t.text == "(":
		e, err := p.or()
		if err == nil && !p.accept(")") {
			err = p.fail(p.toks[p.i], fmt.Sprintf(") to close the parenthesis at position %d", t.pos))
		}
		return e, err
	case t.kind == 'w' && t.text == "true":
		return &Expr{Op: "true", Pos: t.pos}, nil
	case t.kind != 'w' || t.text != "user":
		return nil, p.fail(t, `user.<attribute>, not, ( or true`)
	}
	if err := p.expect("."); err != nil {
		return nil, err
	}
	attr := p.next()
	if attr.text == "memberOf" && attr.kind == 'w' {
		if err := p.expect("("); err != nil {
			return nil, err
		}
		name, err := p.str()
		if err == nil {
			err = p.expect(")")
		}
		return &Expr{Op: "memberOf", Values: []string{name}, Pos: t.pos}, err
	}
	if _, ok := columns[attr.text]; !ok || attr.kind != 'w' {
		return nil, p.fail(attr, "an attribute: "+attributeList)
	}
	e := &Expr{Attr: attr.text, Pos: t.pos}
	op := p.next()
	e.Op = op.text
	switch {
	case op.kind == 's' || op.kind == 0:
		return nil, p.fail(op, "==, !=, contains, startsWith, endsWith or in")
	case slices.Contains([]string{"==", "!=", "contains", "startsWith", "endsWith"}, op.text):
		v, err := p.str()
		e.Values = []string{v}
		return e, err
	case op.text == "in":
		if err := p.expect("["); err != nil {
			return nil, err
		}
		for {
			v, err := p.str()
			if err != nil {
				return nil, err
			}
			e.Values = append(e.Values, v)
			if !p.accept(",") {
				return e, p.expect("]")
			}
		}
	}
	return nil, p.fail(op, "==, !=, contains, startsWith, endsWith or in")
}

func (p *parser) str() (string, error) {
	t := p.next()
	if t.kind != 's' {
		return "", p.fail(t, `a value in double quotes, like "Engineering"`)
	}
	return t.text, nil
}

type Group struct {
	ID   string
	Rule *string
}

type Compiler struct {
	Groups map[string]Group
	Args   []any
	stack  []string
}

var errLoop = errors.New("loop")

func (c *Compiler) Param(v string) string {
	c.Args = append(c.Args, v)
	return fmt.Sprintf("$%d::text", len(c.Args))
}

func (c *Compiler) Rule(groupID, src string) (string, error) {
	e, err := Parse(src)
	if err != nil {
		return "", err
	}
	n := len(c.Args)
	c.stack = []string{groupID}
	s, err := c.sql(e)
	if err != nil {
		c.Args = c.Args[:n]
	}
	return s, err
}

func (c *Compiler) sql(e *Expr) (string, error) {
	switch e.Op {
	case "true":
		return "true", nil
	case "and", "or":
		l, err := c.sql(e.Left)
		if err != nil {
			return "", err
		}
		r, err := c.sql(e.Right)
		return "(" + l + " " + e.Op + " " + r + ")", err
	case "not":
		x, err := c.sql(e.Left)
		return "not (" + x + ")", err
	case "memberOf":
		return c.memberOf(e)
	}
	col := "lower(" + columns[e.Attr] + ")"
	v := "lower(" + c.Param(e.Values[0]) + ")"
	switch e.Op {
	case "==":
		return col + " = " + v, nil
	case "!=":
		return col + " <> " + v, nil
	case "contains":
		return "strpos(" + col + ", " + v + ") > 0", nil
	case "startsWith":
		return "starts_with(" + col + ", " + v + ")", nil
	case "endsWith":
		return "right(" + col + ", char_length(" + v + ")) = " + v, nil
	}
	for _, value := range e.Values[1:] {
		v += ", lower(" + c.Param(value) + ")"
	}
	return col + " in (" + v + ")", nil
}

func (c *Compiler) memberOf(e *Expr) (string, error) {
	name := e.Values[0]
	g, ok := c.Groups[strings.ToLower(name)]
	if !ok {
		return "", &Error{e.Pos, fmt.Sprintf("there is no group named %q.", name)}
	}
	if g.Rule == nil {
		return "exists (select 1 from group_members gm where gm.group_id = " + c.Param(g.ID) + " and gm.user_id = u.id)", nil
	}
	err := errLoop
	switch {
	case g.ID == c.stack[0]:
	case slices.Contains(c.stack, g.ID):
		err = errors.New("loop between other groups")
	default:
		var nested *Expr
		if nested, err = Parse(*g.Rule); err == nil {
			c.stack = append(c.stack, g.ID)
			var s string
			s, err = c.sql(nested)
			c.stack = c.stack[:len(c.stack)-1]
			if err == nil {
				return "(" + s + ")", nil
			}
		}
	}
	if len(c.stack) > 1 {
		return "", err
	}
	if errors.Is(err, errLoop) {
		return "", &Error{e.Pos, fmt.Sprintf("user.memberOf(%q) makes a loop, because that group's membership depends on this group.", name)}
	}
	return "", &Error{e.Pos, fmt.Sprintf("the rule of %q cannot be used here because it is not valid.", name)}
}
