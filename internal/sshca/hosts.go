package sshca

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/hostd"
	"halo/internal/httpx"
	"halo/internal/store"
)

var errNoHostGroup = httpx.NotFound("This host group")

func (h *handlers) posixSettings(w http.ResponseWriter, r *http.Request, _ store.User) error {
	p, err := h.st.GetPOSIXSettings(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, p)
}

func (h *handlers) savePOSIXSettings(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in store.POSIXSettings
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	switch {
	case in.UIDMin < hostd.MinID || in.GIDMin < hostd.MinID || in.UIDMax < in.UIDMin || in.GIDMax < in.GIDMin || in.UIDMax > math.MaxInt32 || in.GIDMax > math.MaxInt32:
		return httpx.Invalid(fmt.Sprintf("Start both ranges at %d or above, above the local accounts and nobody, end each range at or after its start, and stay at or below %d.", hostd.MinID, math.MaxInt32))
	case in.UIDMin <= in.GIDMax && in.GIDMin <= in.UIDMax:
		return httpx.Invalid("The person and group ranges overlap. Give each its own numbers, because a person's private group uses their user number.")
	case !hostd.PathPattern.MatchString(in.LoginShell) || in.LoginShell == "/":
		return httpx.Invalid("Set the default login shell to an absolute path, such as /bin/bash.")
	}
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SavePOSIXSettings(r.Context(), in); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.posix", Summary: fmt.Sprintf("Set the POSIX ranges to %d–%d for people and %d–%d for groups, with the login shell %s", in.UIDMin, in.UIDMax, in.GIDMin, in.GIDMax, in.LoginShell),
			TargetType: "posix_settings", TargetID: "posix", TargetLabel: "POSIX settings"})
	})
	if err != nil {
		return err
	}
	return h.posixSettings(w, r, actor)
}

func (h *handlers) listHostGroups(w http.ResponseWriter, r *http.Request, _ store.User) error {
	groups, err := h.st.ListHostGroups(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, groups)
}

func (h *handlers) readHostGroup(r *http.Request) (store.HostGroup, map[string]string, error) {
	var in struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Rules       []store.HostRule `json:"rules"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.HostGroup{}, nil, err
	}
	g := store.HostGroup{Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Rules: []store.HostRule{}}
	if g.Name == "" || len(g.Name) > 100 || len(g.Description) > 500 {
		return g, nil, httpx.Invalid("Name the host group in up to 100 characters, such as Production, with a description of up to 500.")
	}
	groups, err := h.st.ListGroupsRaw(r.Context())
	if err != nil {
		return g, nil, err
	}
	names := map[string]string{}
	for _, rule := range in.Rules {
		i := slices.IndexFunc(groups, func(x store.Group) bool { return x.ID == rule.GroupID })
		if i < 0 {
			return g, nil, httpx.Invalid("Choose the groups whose members may log in from your groups.")
		}
		if _, dup := names[rule.GroupID]; dup {
			return g, nil, httpx.Invalid(groups[i].Name + " has more than one rule. Give each group one rule.")
		}
		names[rule.GroupID] = groups[i].Name
		sudo := []string{}
		for _, c := range rule.Sudo {
			c = strings.Join(strings.Fields(c), " ")
			if c == "" || slices.Contains(sudo, c) {
				continue
			}
			if !hostd.ValidCommand(c) {
				return g, nil, httpx.Invalid(fmt.Sprintf("“%s” is not a sudo command Halo accepts. Use ALL, or an absolute path with plain arguments, such as /usr/bin/systemctl restart nginx, without wildcards, quotes or the characters , : = \\.", c))
			}
			sudo = append(sudo, c)
		}
		if len(sudo) > 32 {
			return g, nil, httpx.Invalid("Give a group at most 32 sudo commands.")
		}
		if slices.Contains(sudo, "ALL") {
			sudo = []string{"ALL"}
		}
		g.Rules = append(g.Rules, store.HostRule{GroupID: rule.GroupID, Sudo: sudo})
	}
	return g, names, nil
}

func ruleSummary(g store.HostGroup, names map[string]string) string {
	if len(g.Rules) == 0 {
		return "nobody may log in"
	}
	var parts []string
	for _, rule := range g.Rules {
		switch {
		case len(rule.Sudo) == 0:
			parts = append(parts, names[rule.GroupID])
		case rule.Sudo[0] == "ALL":
			parts = append(parts, names[rule.GroupID]+" with full sudo")
		default:
			parts = append(parts, names[rule.GroupID]+" with sudo for "+strings.Join(rule.Sudo, ", "))
		}
	}
	return "may log in: " + strings.Join(parts, "; ")
}

func (h *handlers) saveHostGroup(w http.ResponseWriter, r *http.Request, actor store.User, g store.HostGroup, names map[string]string, action, verb string, status int) error {
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if g.ID, err = tx.SaveHostGroup(r.Context(), g); errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "Another host group is called "+g.Name+". Choose a different name.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: verb + " the host group " + g.Name + ", " + ruleSummary(g, names), TargetType: "host_group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	saved, err := h.st.GetHostGroup(r.Context(), g.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
}

func (h *handlers) createHostGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, names, err := h.readHostGroup(r)
	if err != nil {
		return err
	}
	return h.saveHostGroup(w, r, actor, g, names, "ssh.host_group.create", "Created", http.StatusCreated)
}

func (h *handlers) updateHostGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.st.GetHostGroup(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return errNoHostGroup
	} else if err != nil {
		return err
	}
	g, names, err := h.readHostGroup(r)
	if err != nil {
		return err
	}
	g.ID = old.ID
	return h.saveHostGroup(w, r, actor, g, names, "ssh.host_group.update", "Updated", http.StatusOK)
}

func (h *handlers) deleteHostGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	g, err := h.st.GetHostGroup(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return errNoHostGroup
	} else if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteHostGroup(r.Context(), g.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.host_group.delete", Summary: fmt.Sprintf("Deleted the host group %s, so nobody may log in to its %d hosts through halo-hostd", g.Name, g.HostCount), TargetType: "host_group", TargetID: g.ID, TargetLabel: g.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) setHostGroup(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		HostGroupID *string `json:"hostGroupId"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	label := "no host group"
	if in.HostGroupID != nil && *in.HostGroupID == "" {
		in.HostGroupID = nil
	}
	if in.HostGroupID != nil {
		g, err := h.st.GetHostGroup(r.Context(), *in.HostGroupID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Invalid("Choose one of your host groups.")
		} else if err != nil {
			return err
		}
		label = "the host group " + g.Name
	}
	var host store.SSHHost
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if host, err = tx.SetSSHHostGroup(r.Context(), r.PathValue("id"), in.HostGroupID); errors.Is(err, store.ErrNotFound) {
			return errNoHost
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.host.group", Summary: "Moved the SSH host " + strings.Join(host.Hostnames, ", ") + " to " + label, TargetType: "ssh_host", TargetID: host.ID, TargetLabel: host.Hostnames[0]})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, host)
}

func (h *handlers) directory(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		PublicKey string `json:"publicKey"`
		Timestamp int64  `json:"timestamp"`
		Signature string `json:"signature"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key, err := parseKey(in.PublicKey)
	if err != nil {
		return err
	}
	var sig ssh.Signature
	blob, err := base64.StdEncoding.DecodeString(in.Signature)
	if err != nil || ssh.Unmarshal(blob, &sig) != nil || time.Since(time.Unix(in.Timestamp, 0)).Abs() > clockSkew || key.Verify(hostd.DirectoryMessage(in.Timestamp), &sig) != nil {
		return errBadSignature
	}
	host, err := h.st.SSHHostByKey(r.Context(), authorizedKey(key))
	if errors.Is(err, store.ErrNotFound) {
		return errUnknownHost
	} else if err != nil {
		return err
	}
	d := hostd.Directory{Users: []hostd.User{}, Groups: []hostd.Group{}}
	if host.HostGroupID == nil {
		return httpx.JSON(w, http.StatusOK, d)
	}
	ctx := r.Context()
	rules, err := h.st.HostRules(ctx)
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(ctx)
	if err != nil {
		return err
	}
	groups, err := h.st.ListGroupsRaw(ctx)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, hostDirectory(rules[*host.HostGroupID], users, groups))
}

func hostDirectory(rules []store.HostRule, users []store.User, groups []store.Group) hostd.Directory {
	d := hostd.Directory{Users: []hostd.User{}, Groups: []hostd.Group{}}
	members := map[string][]string{}
	for _, u := range users {
		if u.Status != "active" || u.Kind != "person" || u.POSIX == nil {
			continue
		}
		allowed, sudo := false, []string{}
		for _, rule := range rules {
			if !slices.Contains(u.GroupIDs, rule.GroupID) {
				continue
			}
			allowed = true
			for _, c := range rule.Sudo {
				if !slices.Contains(sudo, c) {
					sudo = append(sudo, c)
				}
			}
		}
		if !allowed {
			continue
		}
		if slices.Contains(sudo, "ALL") {
			sudo = []string{"ALL"}
		}
		p := u.POSIX
		d.Users = append(d.Users, hostd.User{Name: p.Username, UID: p.UIDNumber, GID: p.GIDNumber, Gecos: cmp.Or(u.Name, u.Email), Home: p.HomeDirectory, Shell: p.LoginShell, Sudo: sudo})
		for _, id := range u.GroupIDs {
			members[id] = append(members[id], p.Username)
		}
	}
	for _, g := range groups {
		if g.POSIX != nil && len(members[g.ID]) > 0 {
			d.Groups = append(d.Groups, hostd.Group{Name: g.POSIX.Name, GID: g.POSIX.GIDNumber, Members: members[g.ID]})
		}
	}
	return d
}

func (h *handlers) hostPrincipal(r *http.Request, u store.User) (string, error) {
	if u.POSIX == nil {
		return "", nil
	}
	rules, err := h.st.HostRules(r.Context())
	if err != nil {
		return "", err
	}
	for _, list := range rules {
		for _, rule := range list {
			if slices.Contains(u.GroupIDs, rule.GroupID) {
				return hostd.Principal(u.POSIX.UIDNumber), nil
			}
		}
	}
	return "", nil
}
