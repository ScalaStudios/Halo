package ldapsync

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-ldap/ldap/v3"

	"halo/internal/id"
	"halo/internal/metrics"
	"halo/internal/store"
)

var (
	pageSize  uint32 = 500
	errDryRun        = errors.New("dry run")
)

type change struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

type report struct {
	store.LDAPSync
	DryRun  bool     `json:"dryRun"`
	Changes []change `json:"changes"`
}

func fetch(d store.LDAPDirectory, password string) ([]*ldap.Entry, []*ldap.Entry, error) {
	u, _ := url.Parse(d.URL)
	cfg := &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}
	if d.CACertificate != "" {
		cfg.RootCAs = x509.NewCertPool()
		cfg.RootCAs.AppendCertsFromPEM([]byte(d.CACertificate))
	}
	conn, err := ldap.DialURL(d.URL, ldap.DialWithTLSConfig(cfg), ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}))
	if err != nil {
		return nil, nil, fmt.Errorf("Halo could not connect to %s: %w", u.Host, err)
	}
	defer conn.Close()
	conn.SetTimeout(time.Minute)
	if d.Security == "starttls" {
		if err := conn.StartTLS(cfg); err != nil {
			return nil, nil, fmt.Errorf("StartTLS with %s failed, so Halo did not send the bind password: %w", u.Host, err)
		}
	}
	if err := conn.Bind(d.BindDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return nil, nil, errors.New("The directory refused the bind DN and password. Check them, and that the account is not disabled or locked")
		}
		return nil, nil, fmt.Errorf("Binding as %s failed: %w", d.BindDN, err)
	}
	m := d.Mapping
	people, err := search(conn, d.UserBaseDN, d.UserFilter, append([]string{m.ID, m.Email, m.Name, m.Title, m.Department, m.Location, m.Manager, "userAccountControl"}, slices.Collect(maps.Values(m.Attributes))...))
	if err != nil || d.GroupBaseDN == "" {
		return people, nil, err
	}
	groups, err := search(conn, d.GroupBaseDN, d.GroupFilter, []string{m.ID, m.GroupName, m.Member})
	return people, groups, err
}

func search(conn *ldap.Conn, base, filter string, attributes []string) ([]*ldap.Entry, error) {
	attributes = slices.DeleteFunc(attributes, func(a string) bool { return a == "" })
	res, err := conn.SearchWithPaging(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, filter, attributes, nil), pageSize)
	if err != nil {
		return nil, fmt.Errorf("Searching %s failed: %w", base, err)
	}
	return res.Entries, nil
}

func (h *handlers) sync(ctx context.Context, d store.LDAPDirectory, actorID *string, dryRun bool) (report, error) {
	rep := report{LDAPSync: store.LDAPSync{StartedAt: time.Now(), Result: "succeeded", Skipped: []string{}}, DryRun: dryRun, Changes: []change{}}
	if !dryRun {
		rep.ID = id.New("lds")
	}
	password, err := h.st.Sealer.Open(d.Sealed)
	if err != nil {
		return rep, err
	}
	people, groups, failure := fetch(d, string(password))
	if failure == nil && (len(people) == 0 && d.People > 0 || d.GroupBaseDN != "" && len(groups) == 0 && d.Groups > 0) {
		failure = errors.New("The people or group search returned nothing although this directory synced some before, so Halo changed nothing. Check the base DNs and filters")
	}
	if failure == nil {
		failure = h.st.Tx(ctx, func(tx *store.Store) error {
			s := &syncer{ctx: ctx, tx: tx, dir: d, actorID: actorID, rep: &rep, users: map[string]store.User{}, emails: map[string]string{}, byDN: map[string]string{}, mine: map[string]bool{}}
			if err := s.run(people, groups); err != nil {
				return err
			}
			if dryRun {
				return errDryRun
			}
			rep.FinishedAt = time.Now()
			return tx.RecordLDAPSync(ctx, d.ID, rep.LDAPSync)
		})
		if errors.Is(failure, errDryRun) {
			failure = nil
		}
	}
	rep.FinishedAt = cmp.Or(rep.FinishedAt, time.Now())
	if failure != nil {
		message := failure.Error()
		if len(message) > 500 {
			message = strings.ToValidUTF8(message[:500], "")
		}
		rep.LDAPSync = store.LDAPSync{ID: rep.ID, StartedAt: rep.StartedAt, FinishedAt: rep.FinishedAt, Result: "failed", Error: message, Skipped: []string{}}
		rep.Changes = []change{}
		if dryRun {
			return rep, nil
		}
		metrics.LDAPRuns.Inc("failure")
		return rep, h.st.RecordLDAPSync(ctx, d.ID, rep.LDAPSync)
	}
	if !dryRun {
		c := rep.Counts
		metrics.LDAPRuns.Inc("success")
		for action, n := range map[string]int{"created": c.Created, "updated": c.Updated, "suspended": c.Suspended, "restored": c.Restored,
			"group_created": c.GroupsCreated, "group_renamed": c.GroupsRenamed, "member_added": c.MembersAdded, "member_removed": c.MembersRemoved} {
			metrics.LDAPChanges.Add(float64(n), action)
		}
	}
	return rep, nil
}

type syncer struct {
	ctx       context.Context
	tx        *store.Store
	dir       store.LDAPDirectory
	actorID   *string
	rep       *report
	emergency []string
	attrs     []store.Attribute
	users     map[string]store.User
	emails    map[string]string
	byDN      map[string]string
	mine      map[string]bool
}

type match struct {
	entry   *ldap.Entry
	link    store.LDAPPerson
	created bool
}

func (s *syncer) skip(message string) {
	if len(s.rep.Skipped) < 100 {
		s.rep.Skipped = append(s.rep.Skipped, message)
	}
}

func (s *syncer) note(action, target, targetType, targetID, summary string) error {
	if len(s.rep.Changes) < 1000 {
		s.rep.Changes = append(s.rep.Changes, change{strings.TrimPrefix(action, "ldap."), target, summary})
	}
	return s.tx.RecordAudit(s.ctx, store.AuditEvent{ActorID: s.actorID, Action: action, Summary: summary, TargetType: targetType, TargetID: targetID, TargetLabel: target})
}

func entryID(raw []byte) string {
	if slices.ContainsFunc(raw, func(b byte) bool { return b < 0x21 || b > 0x7e }) {
		return hex.EncodeToString(raw)
	}
	return string(raw)
}

func value(e *ldap.Entry, attr string) string {
	v := strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(e.GetEqualFoldAttributeValue(attr), "\x00", ""), ""))
	if utf8.RuneCountInString(v) > 200 {
		v = string([]rune(v)[:200])
	}
	return v
}

func normalize(dn string) string {
	if parsed, err := ldap.ParseDN(dn); err == nil {
		return strings.ToLower(parsed.String())
	}
	return strings.ToLower(dn)
}

func disabled(e *ldap.Entry) bool {
	flags, err := strconv.Atoi(e.GetEqualFoldAttributeValue("userAccountControl"))
	return err == nil && flags&2 != 0
}

func validEmail(email string) bool {
	a, err := mail.ParseAddress(email)
	return err == nil && a.Address == email && len(email) <= 320
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func (s *syncer) run(people, groups []*ldap.Entry) error {
	ctx, tx, m := s.ctx, s.tx, s.dir.Mapping
	if err := tx.LockLDAPDirectory(ctx, s.dir.ID); err != nil {
		return err
	}
	settings, err := tx.CurrentSettings(ctx)
	if err != nil {
		return err
	}
	s.emergency = settings.EmergencyAccessIDs
	if s.attrs, err = tx.ListAttributes(ctx); err != nil {
		return err
	}
	links, err := tx.LDAPPeople(ctx, s.dir.ID)
	if err != nil {
		return err
	}
	users, err := tx.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		s.users[u.ID] = u
	}
	emails := make([]string, len(people))
	for i, e := range people {
		emails[i] = value(e, m.Email)
	}
	owners, err := tx.UsersByEmail(ctx, emails)
	if err != nil {
		return err
	}
	for _, u := range append(users, owners...) {
		s.emails[strings.ToLower(u.Email)] = u.ID
	}
	linked := map[string]store.LDAPPerson{}
	for _, p := range links {
		linked[p.EntryID], s.mine[p.UserID] = p, true
	}
	var matches []match
	seen, taken := map[string]bool{}, map[string]bool{}
	for _, e := range people {
		id, email := entryID(e.GetEqualFoldRawAttributeValue(m.ID)), value(e, m.Email)
		if id == "" {
			s.skip(e.DN + " has no " + m.ID + ", so Halo can't follow it from one sync to the next")
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		link, ok := linked[id]
		if ok {
			s.byDN[normalize(e.DN)] = link.UserID
		}
		key := strings.ToLower(email)
		switch {
		case !validEmail(email):
			s.skip(e.DN + " has no valid email address in " + m.Email)
			continue
		case taken[key]:
			s.skip(e.DN + " has the email address " + email + ", which another entry in the directory also has")
			continue
		}
		taken[key] = true
		if ok {
			if link.DN != e.DN {
				link.DN = e.DN
				if err := tx.SaveLDAPPerson(ctx, s.dir.ID, link); err != nil {
					return err
				}
			}
			matches = append(matches, match{e, link, false})
			continue
		}
		if owner, exists := s.emails[key]; exists {
			s.skip(email + map[bool]string{true: " is already synced from another entry in this directory, so Halo left them alone", false: " is already in Halo and isn't synced from this directory, so Halo left them alone"}[s.mine[owner]])
			continue
		}
		if disabled(e) {
			continue
		}
		u, err := s.create(e, email)
		if err != nil {
			return err
		}
		link = store.LDAPPerson{EntryID: id, UserID: u.ID, DN: e.DN}
		if err := tx.SaveLDAPPerson(ctx, s.dir.ID, link); err != nil {
			return err
		}
		s.byDN[normalize(e.DN)] = u.ID
		matches = append(matches, match{e, link, true})
	}
	for _, mt := range matches {
		if err := s.update(mt); err != nil {
			return err
		}
	}
	for _, p := range links {
		if u, ok := s.users[p.UserID]; ok && !seen[p.EntryID] {
			if err := s.suspend(u, p, "no longer in "+s.dir.Name+" or outside its people filter"); err != nil {
				return err
			}
		}
	}
	if s.dir.GroupBaseDN == "" {
		return nil
	}
	return s.groups(groups)
}

func (s *syncer) create(e *ldap.Entry, email string) (store.User, error) {
	m := s.dir.Mapping
	u, err := s.tx.CreateUser(s.ctx, store.NewUser{Email: email, Name: cmp.Or(value(e, m.Name), email), Title: value(e, m.Title), Department: value(e, m.Department),
		Location: value(e, m.Location), Status: "invited", Source: "LDAP · " + s.dir.Name})
	if err != nil {
		return u, err
	}
	s.users[u.ID], s.emails[strings.ToLower(email)], s.mine[u.ID] = u, u.ID, true
	s.rep.Counts.Created++
	return u, s.note("ldap.user.create", u.Name, "user", u.ID, "Created from LDAP directory "+s.dir.Name+" as "+email)
}

func (s *syncer) update(mt match) error {
	e, m := mt.entry, s.dir.Mapping
	u := s.users[mt.link.UserID]
	next := u
	next.Email = value(e, m.Email)
	next.Name = cmp.Or(value(e, m.Name), next.Email)
	for _, f := range []struct {
		attr  string
		field *string
	}{{m.Title, &next.Title}, {m.Department, &next.Department}, {m.Location, &next.Location}} {
		if f.attr != "" {
			*f.field = value(e, f.attr)
		}
	}
	if m.Manager != "" {
		next.ManagerID = nil
		if id, ok := s.byDN[normalize(e.GetEqualFoldAttributeValue(m.Manager))]; ok && id != u.ID {
			next.ManagerID = &id
		}
	}
	changed := []string{}
	for _, f := range []struct {
		label   string
		differs bool
	}{
		{"email", u.Email != next.Email}, {"name", u.Name != next.Name}, {"title", u.Title != next.Title},
		{"department", u.Department != next.Department}, {"location", u.Location != next.Location}, {"manager", deref(u.ManagerID) != deref(next.ManagerID)},
	} {
		if f.differs {
			changed = append(changed, f.label)
		}
	}
	if next.Email != u.Email && len(u.RoleKeys)+len(u.EligibleRoles) > 0 {
		s.skip("Didn't change the email address of " + u.Name + " to " + next.Email + " because they hold or are eligible for an administrator role in Halo")
		next.Email = u.Email
		changed = slices.DeleteFunc(changed, func(f string) bool { return f == "email" })
	}
	attributes, labels := s.attributes(e, u)
	if owner, ok := s.emails[strings.ToLower(next.Email)]; ok && owner != u.ID {
		s.skip("Didn't update " + u.Name + " because their new email address " + next.Email + " belongs to someone else in Halo")
		return s.status(e, mt.link, u)
	}
	if len(changed) > 0 {
		if err := s.tx.UpdateUserProfile(s.ctx, next); err != nil {
			return err
		}
		delete(s.emails, strings.ToLower(u.Email))
		s.emails[strings.ToLower(next.Email)] = u.ID
	}
	if len(labels) > 0 {
		if err := s.tx.SetUserAttributes(s.ctx, u.ID, attributes); err != nil {
			return err
		}
		changed = append(changed, labels...)
	}
	next.Attributes = attributes
	s.users[u.ID] = next
	if len(changed) > 0 && !mt.created {
		s.rep.Counts.Updated++
		if err := s.note("ldap.user.update", next.Name, "user", u.ID, "Updated "+strings.Join(changed, ", ")+" from LDAP directory "+s.dir.Name); err != nil {
			return err
		}
	}
	return s.status(e, mt.link, next)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *syncer) attributes(e *ldap.Entry, u store.User) (map[string]any, []string) {
	next, labels := maps.Clone(u.Attributes), []string{}
	if next == nil {
		next = map[string]any{}
	}
	for _, a := range s.attrs {
		source, ok := s.dir.Mapping.Attributes[a.Name]
		if !ok || a.SelfService {
			continue
		}
		v, ok := convert(a, e.GetEqualFoldAttributeValues(source))
		if !ok {
			s.skip(fmt.Sprintf("%s's %s isn't a valid %s value for %s, so Halo cleared it", u.Email, source, a.Type, a.Label))
		}
		before, _ := json.Marshal(u.Attributes[a.Name])
		after, _ := json.Marshal(v)
		if string(before) == string(after) {
			continue
		}
		delete(next, a.Name)
		if v != nil {
			next[a.Name] = v
		}
		labels = append(labels, a.Label)
	}
	return next, labels
}

func convert(a store.Attribute, values []string) (any, bool) {
	clean := []string{}
	for _, v := range values {
		if v = strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(v, "\x00", ""), "")); v != "" && !slices.Contains(clean, v) {
			if utf8.RuneCountInString(v) > 500 {
				v = string([]rune(v)[:500])
			}
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return nil, true
	}
	v := clean[0]
	switch a.Type {
	case "list":
		return clean[:min(len(clean), 50)], true
	case "number":
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.Abs(f) > 1<<53 {
			return nil, false
		}
		return f, true
	case "boolean":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, false
		}
		return b, true
	case "date":
		for _, layout := range []string{time.DateOnly, "20060102"} {
			if t, err := time.Parse(layout, v[:min(len(v), len(layout))]); err == nil {
				return t.Format(time.DateOnly), true
			}
		}
		return nil, false
	}
	return v, true
}

func (s *syncer) status(e *ldap.Entry, link store.LDAPPerson, u store.User) error {
	if disabled(e) {
		return s.suspend(u, link, "disabled in "+s.dir.Name)
	}
	if !link.Suspended {
		return nil
	}
	link.Suspended = false
	if err := s.tx.SaveLDAPPerson(s.ctx, s.dir.ID, link); err != nil {
		return err
	}
	if u.Status != "suspended" {
		return nil
	}
	status := "active"
	if u.LastSignInAt == nil && len(u.Methods) == 0 {
		status = "invited"
	}
	if err := s.tx.SetUserStatus(s.ctx, u.ID, status); err != nil {
		return err
	}
	s.rep.Counts.Restored++
	return s.note("ldap.user.restore", u.Name, "user", u.ID, "Restored because they are back and enabled in LDAP directory "+s.dir.Name)
}

func (s *syncer) suspend(u store.User, link store.LDAPPerson, reason string) error {
	if u.Status != "active" && u.Status != "invited" {
		return nil
	}
	if slices.Contains(s.emergency, u.ID) {
		s.skip("Didn't suspend " + u.Name + " (" + reason + ") because they are an emergency access account")
		return nil
	}
	last, err := s.tx.IsLastActiveGlobalAdmin(s.ctx, u.ID)
	if err != nil {
		return err
	}
	if last {
		s.skip("Didn't suspend " + u.Name + " (" + reason + ") because they are the last active global administrator")
		return nil
	}
	if err := s.tx.SetUserStatus(s.ctx, u.ID, "suspended"); err != nil {
		return err
	}
	n, err := s.tx.RevokeUserSessions(s.ctx, u.ID, "")
	if err != nil {
		return err
	}
	link.Suspended = true
	if err := s.tx.SaveLDAPPerson(s.ctx, s.dir.ID, link); err != nil {
		return err
	}
	s.rep.Counts.Suspended++
	return s.note("ldap.user.suspend", u.Name, "user", u.ID, "Suspended because they are "+reason+", and ended "+plural(n, "session"))
}

func (s *syncer) groups(entries []*ldap.Entry) error {
	ctx, tx, m := s.ctx, s.tx, s.dir.Mapping
	links, err := tx.LDAPGroups(ctx, s.dir.ID)
	if err != nil {
		return err
	}
	all, err := tx.ListGroupsRaw(ctx)
	if err != nil {
		return err
	}
	byID, names := map[string]store.Group{}, map[string]string{}
	for _, g := range all {
		byID[g.ID], names[strings.ToLower(g.Name)] = g, g.ID
	}
	have := map[string]map[string]bool{}
	for userID := range s.mine {
		for _, groupID := range s.users[userID].GroupIDs {
			if have[groupID] == nil {
				have[groupID] = map[string]bool{}
			}
			have[groupID][userID] = true
		}
	}
	seen := map[string]bool{}
	for _, e := range entries {
		id, name := entryID(e.GetEqualFoldRawAttributeValue(m.ID)), value(e, m.GroupName)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if name == "" {
			s.skip(e.DN + " has no " + m.GroupName + ", so Halo skipped the group")
			continue
		}
		groupID, linked := links[id]
		g, other := byID[groupID], names[strings.ToLower(name)]
		switch {
		case !linked && other != "":
			s.skip("A group named " + name + " is already in Halo and isn't synced from this directory, so Halo left it alone")
			continue
		case !linked:
			if g, err = tx.CreateGroup(ctx, store.NewGroup{Name: name, Kind: "assigned", Source: "LDAP · " + s.dir.Name}); err != nil {
				return err
			}
			if err := tx.LinkLDAPGroup(ctx, s.dir.ID, id, g.ID); err != nil {
				return err
			}
			names[strings.ToLower(name)] = g.ID
			s.rep.Counts.GroupsCreated++
			if err := s.note("ldap.group.create", g.Name, "group", g.ID, "Created from LDAP directory "+s.dir.Name); err != nil {
				return err
			}
		case g.Name != name && other != "" && other != g.ID:
			s.skip("Didn't rename " + g.Name + " to " + name + " because another group in Halo has that name")
		case g.Name != name:
			old := g.Name
			g.Name = name
			if err := tx.UpdateGroup(ctx, g); err != nil {
				return err
			}
			delete(names, strings.ToLower(old))
			names[strings.ToLower(name)] = g.ID
			s.rep.Counts.GroupsRenamed++
			if err := s.note("ldap.group.update", g.Name, "group", g.ID, "Renamed from "+old+" to match LDAP directory "+s.dir.Name); err != nil {
				return err
			}
		}
		want := map[string]bool{}
		for _, dn := range e.GetEqualFoldAttributeValues(m.Member) {
			if userID, ok := s.byDN[normalize(dn)]; ok {
				want[userID] = true
			}
		}
		if err := s.members(g, want, have[g.ID]); err != nil {
			return err
		}
	}
	for id, groupID := range links {
		if g, ok := byID[groupID]; ok && !seen[id] {
			if err := s.members(g, nil, have[groupID]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *syncer) members(g store.Group, want, have map[string]bool) error {
	added, removed := 0, 0
	for _, userID := range slices.Sorted(maps.Keys(want)) {
		if have[userID] {
			continue
		}
		err := s.tx.AddGroupMember(s.ctx, g.ID, userID)
		if errors.Is(err, store.ErrConflict) {
			s.skip("Didn't change the members of " + g.Name + " because it is no longer an assigned group")
			return nil
		}
		if err != nil {
			return err
		}
		added++
	}
	for _, userID := range slices.Sorted(maps.Keys(have)) {
		if want[userID] {
			continue
		}
		if err := s.tx.RemoveGroupMember(s.ctx, g.ID, userID); err != nil {
			return err
		}
		removed++
	}
	if added+removed == 0 {
		return nil
	}
	s.rep.Counts.MembersAdded += added
	s.rep.Counts.MembersRemoved += removed
	return s.note("ldap.group.update", g.Name, "group", g.ID, fmt.Sprintf("Added %s and removed %s to match LDAP directory %s", plural(added, "member"), plural(removed, "member"), s.dir.Name))
}
