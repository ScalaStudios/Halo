package ldapserver

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"

	"halo/internal/metrics"
	"halo/internal/store"
)

const maxResults = 1000

const (
	no = iota
	yes
	undefined
)

var schema = []string{"objectClass", "ou", "uid", "mail", "cn", "sn", "displayName", "memberOf", "member", "description", "namingContexts", "supportedLDAPVersion", "supportedControl", "supportedExtension"}

type attribute struct {
	name   string
	values []string
}

type entry struct {
	dn    *ldap.DN
	attrs []attribute
}

func (c *session) directory(ctx context.Context) ([]entry, error) {
	app, err := c.deps.Store.LDAPApplication(ctx, c.appID)
	if err == nil && app.Status != "active" {
		err = store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	users, err := c.deps.Store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := c.deps.Store.ListGroupsRaw(ctx)
	if err != nil {
		return nil, err
	}
	base := baseDN(c.deps.Config, app.ID)
	people, teams := child(base, "ou", "people"), child(base, "ou", "groups")
	unit := []string{"top", "organizationalUnit"}
	entries := []entry{
		{base, []attribute{{"objectClass", unit}, {"ou", []string{app.ID}}}},
		{people, []attribute{{"objectClass", unit}, {"ou", []string{"people"}}}},
		{teams, []attribute{{"objectClass", unit}, {"ou", []string{"groups"}}}},
	}
	groups = slices.DeleteFunc(groups, func(g store.Group) bool { return !slices.Contains(app.GroupIDs, g.ID) })
	members := map[string][]string{}
	for _, u := range users {
		if u.Status != "active" || !slices.Contains(u.AppIDs, app.ID) {
			continue
		}
		dn := child(people, "uid", u.Email).String()
		var memberOf []string
		for _, g := range groups {
			if slices.Contains(u.GroupIDs, g.ID) {
				memberOf = append(memberOf, child(teams, "cn", g.Name).String())
				members[g.ID] = append(members[g.ID], dn)
			}
		}
		if c.userID != "" && c.userID != u.ID {
			continue
		}
		name := cmp.Or(u.Name, u.Email)
		entries = append(entries, entry{child(people, "uid", u.Email), []attribute{
			{"objectClass", []string{"top", "person", "organizationalPerson", "inetOrgPerson"}},
			{"uid", []string{u.Email}},
			{"mail", []string{u.Email}},
			{"cn", []string{name}},
			{"sn", []string{name}},
			{"displayName", []string{name}},
			{"memberOf", memberOf},
		}})
	}
	for _, g := range groups {
		if c.userID != "" {
			break
		}
		var description []string
		if g.Description != "" {
			description = []string{g.Description}
		}
		entries = append(entries, entry{child(teams, "cn", g.Name), []attribute{
			{"objectClass", []string{"top", "groupOfNames"}},
			{"cn", []string{g.Name}},
			{"description", description},
			{"member", members[g.ID]},
		}})
	}
	return entries, nil
}

func (c *session) root() entry {
	var extensions []string
	if c.tls != nil {
		extensions = []string{startTLS}
	}
	return entry{&ldap.DN{}, []attribute{
		{"objectClass", []string{"top"}},
		{"namingContexts", []string{baseDN(c.deps.Config, c.appID).String()}},
		{"supportedLDAPVersion", []string{"3"}},
		{"supportedControl", []string{ldap.ControlTypePaging}},
		{"supportedExtension", extensions},
	}}
}

func (c *session) search(ctx context.Context, id int64, op *ber.Packet, controls []*ber.Packet) bool {
	done := func(code uint16, message string, extra ...*ber.Packet) bool {
		if code == ldap.LDAPResultSuccess || code == ldap.LDAPResultSizeLimitExceeded {
			metrics.LDAPSearches.Inc("success")
		} else {
			metrics.LDAPSearches.Inc("failure")
		}
		return c.reply(id, result(ldap.ApplicationSearchResultDone, code, message), extra...)
	}
	if len(op.Children) != 8 {
		return done(ldap.LDAPResultProtocolError, "The search request is malformed.")
	}
	if c.appID == "" {
		return done(ldap.LDAPResultInsufficientAccessRights, "Bind with the application's bind DN and password before you search.")
	}
	base, err := ldap.ParseDN(op.Children[0].Data.String())
	scope, _ := op.Children[1].Value.(int64)
	if err != nil || scope < 0 || scope > 3 {
		return done(ldap.LDAPResultInvalidDNSyntax, "The search base isn't a valid DN.")
	}
	size, _ := op.Children[3].Value.(int64)
	seconds, _ := op.Children[4].Value.(int64)
	typesOnly, _ := op.Children[5].Value.(bool)
	filter := op.Children[6]
	var attrs []string
	for _, a := range op.Children[7].Children {
		attrs = append(attrs, a.Data.String())
	}
	var paging *ldap.ControlPaging
	for _, p := range controls {
		if len(p.Children) > 0 && p.Children[0].Data.String() == ldap.ControlTypePaging {
			control, err := ldap.DecodeControl(p)
			if paging, _ = control.(*ldap.ControlPaging); err != nil || paging == nil {
				return done(ldap.LDAPResultProtocolError, "The paged results control is malformed.")
			}
		}
	}
	if seconds > 0 && seconds < int64(requestTimeout/time.Second) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
		defer cancel()
	}
	entries, err := c.directory(ctx)
	switch {
	case ctx.Err() != nil:
		return done(ldap.LDAPResultTimeLimitExceeded, "The search took longer than its time limit.")
	case errors.Is(err, store.ErrNotFound):
		return done(ldap.LDAPResultInsufficientAccessRights, "This application is turned off or was deleted.")
	case err != nil:
		code, message := failed(err)
		return done(code, message)
	}
	if len(base.RDNs) == 0 && scope == 0 {
		entries = []entry{c.root()}
	} else if !slices.ContainsFunc(entries, func(e entry) bool { return e.dn.EqualFold(base) }) {
		return done(ldap.LDAPResultNoSuchObject, "This application's directory has no entry with this DN. Search under "+entries[0].dn.String()+".")
	}
	var matches []entry
	for _, e := range entries {
		if ctx.Err() != nil {
			return done(ldap.LDAPResultTimeLimitExceeded, "The search took longer than its time limit.")
		}
		within := scope == 0 && e.dn.EqualFold(base) ||
			scope == 1 && len(e.dn.RDNs) > 0 && (&ldap.DN{RDNs: e.dn.RDNs[1:]}).EqualFold(base) ||
			scope == 2 && (e.dn.EqualFold(base) || base.AncestorOfFold(e.dn)) ||
			scope == 3 && base.AncestorOfFold(e.dn)
		if within && e.match(filter) == yes {
			matches = append(matches, e)
		}
	}
	truncated := size > 0 && int64(len(matches)) > size
	if truncated {
		matches = matches[:size]
	}
	var extra []*ber.Packet
	if paging == nil && len(matches) > maxResults {
		matches, truncated = matches[:maxResults], true
	}
	if paging != nil {
		offset, err := strconv.Atoi(cmp.Or(string(paging.Cookie), "0"))
		if err != nil || offset < 0 || offset > len(matches) {
			return done(ldap.LDAPResultUnwillingToPerform, "The paging cookie is out of date. Start the search again.")
		}
		end := min(offset+min(int(paging.PagingSize), maxResults), len(matches))
		cookie := ""
		if paging.PagingSize > 0 && end < len(matches) {
			cookie = strconv.Itoa(end)
		}
		matches, truncated = matches[offset:end], truncated && cookie == ""
		extra = append(extra, (&ldap.ControlPaging{Cookie: []byte(cookie)}).Encode())
	}
	for _, e := range matches {
		if !c.reply(id, e.packet(attrs, typesOnly)) {
			return false
		}
	}
	if truncated {
		return done(ldap.LDAPResultSizeLimitExceeded, "There are more entries than the size limit. Narrow the filter or use the paged results control.", extra...)
	}
	return done(ldap.LDAPResultSuccess, "", extra...)
}

func (e entry) values(name string) ([]string, bool) {
	for _, a := range e.attrs {
		if strings.EqualFold(a.name, name) {
			return a.values, true
		}
	}
	return nil, slices.ContainsFunc(schema, func(s string) bool { return strings.EqualFold(s, name) })
}

func (e entry) match(f *ber.Packet) int {
	if f.ClassType != ber.ClassContext {
		return undefined
	}
	switch f.Tag {
	case ldap.FilterAnd:
		out := yes
		for _, c := range f.Children {
			switch e.match(c) {
			case no:
				return no
			case undefined:
				out = undefined
			}
		}
		return out
	case ldap.FilterOr:
		out := no
		for _, c := range f.Children {
			switch e.match(c) {
			case yes:
				return yes
			case undefined:
				out = undefined
			}
		}
		return out
	case ldap.FilterNot:
		if len(f.Children) != 1 {
			return undefined
		}
		switch e.match(f.Children[0]) {
		case yes:
			return no
		case no:
			return yes
		}
		return undefined
	case ldap.FilterPresent:
		values, known := e.values(f.Data.String())
		if !known {
			return undefined
		}
		return truth(len(values) > 0)
	case ldap.FilterEqualityMatch, ldap.FilterApproxMatch, ldap.FilterSubstrings:
		if len(f.Children) != 2 {
			return undefined
		}
		name := f.Children[0].Data.String()
		values, known := e.values(name)
		if !known {
			return undefined
		}
		return truth(slices.ContainsFunc(values, func(v string) bool {
			if f.Tag == ldap.FilterSubstrings {
				return substring(v, f.Children[1].Children)
			}
			return equal(name, v, f.Children[1].Data.String())
		}))
	}
	return undefined
}

func truth(b bool) int {
	if b {
		return yes
	}
	return no
}

func equal(name, value, want string) bool {
	if strings.EqualFold(name, "member") || strings.EqualFold(name, "memberOf") {
		a, errA := ldap.ParseDN(value)
		b, errB := ldap.ParseDN(want)
		return errA == nil && errB == nil && a.EqualFold(b)
	}
	return strings.EqualFold(value, want)
}

func substring(value string, parts []*ber.Packet) bool {
	rest := strings.ToLower(value)
	for i, p := range parts {
		s := strings.ToLower(p.Data.String())
		switch {
		case p.Tag == ldap.FilterSubstringsInitial && i == 0 && strings.HasPrefix(rest, s):
			rest = rest[len(s):]
		case p.Tag == ldap.FilterSubstringsAny && strings.Contains(rest, s):
			rest = rest[strings.Index(rest, s)+len(s):]
		case p.Tag == ldap.FilterSubstringsFinal && i == len(parts)-1 && strings.HasSuffix(rest, s):
			rest = ""
		default:
			return false
		}
	}
	return len(parts) > 0
}

func selected(attrs []string, name string) bool {
	return len(attrs) == 0 || slices.ContainsFunc(attrs, func(a string) bool { return a == "*" || strings.EqualFold(a, name) })
}

func (e entry) packet(attrs []string, typesOnly bool) *ber.Packet {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
	p.AppendChild(octets(e.dn.String()))
	list := ber.NewSequence("")
	for _, a := range e.attrs {
		if len(a.values) == 0 || !selected(attrs, a.name) {
			continue
		}
		attr := ber.NewSequence("")
		attr.AppendChild(octets(a.name))
		set := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
		for _, v := range a.values {
			if !typesOnly {
				set.AppendChild(octets(v))
			}
		}
		attr.AppendChild(set)
		list.AppendChild(attr)
	}
	p.AppendChild(list)
	return p
}
