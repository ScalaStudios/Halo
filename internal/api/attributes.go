package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/store"
)

const (
	maxAttributes     = 50
	maxAttributeValue = 500
	maxListValues     = 50
	maxClaimMappings  = 50
	maxClaimName      = 256
)

var attributeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

var attributeKinds = map[string]string{
	"string":  "text",
	"number":  "a number",
	"boolean": "true or false",
	"date":    "a date in the form 2026-01-31",
	"list":    "a list of text values",
}

func (h *handlers) attribute(r *http.Request) (store.Attribute, error) {
	all, err := h.st.ListAttributes(r.Context())
	if err != nil {
		return store.Attribute{}, err
	}
	i := slices.IndexFunc(all, func(a store.Attribute) bool { return a.ID == r.PathValue("id") })
	if i < 0 {
		return store.Attribute{}, httpx.NotFound("This attribute")
	}
	return all[i], nil
}

func (h *handlers) listAttributes(w http.ResponseWriter, r *http.Request, _ store.User) error {
	all, err := h.st.ListAttributes(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(all))
}

func checkLabel(label string) error {
	if label == "" || utf8.RuneCountInString(label) > 100 {
		return httpx.Invalid("Enter a display name of up to 100 characters, such as Employee number.")
	}
	return nil
}

func (h *handlers) createAttribute(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Type        string `json:"type"`
		SelfService bool   `json:"selfService"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	a := store.Attribute{Name: strings.TrimSpace(in.Name), Label: strings.TrimSpace(in.Label), Type: in.Type, SelfService: in.SelfService}
	if !attributeName.MatchString(a.Name) {
		return httpx.Invalid("Start the name with a letter and use up to 64 letters, digits and underscores, such as employeeNumber. Applications and SCIM receive it under this name.")
	}
	if err := checkLabel(a.Label); err != nil {
		return err
	}
	if !slices.Contains(store.AttributeTypes, a.Type) {
		return httpx.Invalid("Choose a type: string, number, boolean, date or list.")
	}
	all, err := h.st.ListAttributes(r.Context())
	if err != nil {
		return err
	}
	if len(all) >= maxAttributes {
		return httpx.Invalid(fmt.Sprintf("Halo holds up to %d custom attributes. Delete one you no longer use first.", maxAttributes))
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		created, err := tx.CreateAttribute(r.Context(), a)
		if errors.Is(err, store.ErrConflict) {
			return conflict("An attribute called " + a.Name + " already exists. Pick another name.")
		}
		if err != nil {
			return err
		}
		a = created
		return record(r, tx, actor, store.AuditEvent{Action: "attribute.create", Summary: "Added the " + a.Type + " attribute " + a.Name, TargetType: "attribute", TargetID: a.ID, TargetLabel: a.Label})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, a)
}

func (h *handlers) updateAttribute(w http.ResponseWriter, r *http.Request, actor store.User) error {
	current, err := h.attribute(r)
	if err != nil {
		return err
	}
	var in struct {
		Label       *string `json:"label"`
		SelfService *bool   `json:"selfService"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	next := current
	if in.Label != nil {
		next.Label = strings.TrimSpace(*in.Label)
	}
	if in.SelfService != nil {
		next.SelfService = *in.SelfService
	}
	if err := checkLabel(next.Label); err != nil {
		return err
	}
	if next == current {
		return httpx.JSON(w, http.StatusOK, current)
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateAttribute(r.Context(), next); err != nil {
			return found(err, "This attribute")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "attribute.update", Summary: "Updated the attribute " + next.Name, TargetType: "attribute", TargetID: next.ID, TargetLabel: next.Label, Changes: store.Diff(current, next)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, next)
}

func (h *handlers) deleteAttribute(w http.ResponseWriter, r *http.Request, actor store.User) error {
	a, err := h.attribute(r)
	if err != nil {
		return err
	}
	apps, err := h.st.ApplicationsMapping(r.Context(), "attribute."+a.Name)
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		return conflict(fmt.Sprintf("%s is mapped to a claim in %s. Remove those claim mappings first, so the applications don't lose the value without notice.", a.Label, strings.Join(apps, ", ")))
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAttribute(r.Context(), a); err != nil {
			return found(err, "This attribute")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "attribute.delete", Summary: fmt.Sprintf("Deleted the attribute %s and its value for %s", a.Name, plural(a.People, "person")), TargetType: "attribute", TargetID: a.ID, TargetLabel: a.Label})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func attributeValue(a store.Attribute, raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	wrong := httpx.Invalid(fmt.Sprintf("%s takes %s.", a.Label, attributeKinds[a.Type]))
	long := httpx.Invalid(fmt.Sprintf("Keep each value of %s under %d characters.", a.Label, maxAttributeValue))
	switch v := v.(type) {
	case nil:
		return nil, nil
	case string:
		v = strings.TrimSpace(v)
		switch {
		case v == "":
			return nil, nil
		case a.Type == "date":
			if _, err := time.Parse(time.DateOnly, v); err != nil {
				return nil, wrong
			}
		case a.Type != "string":
			return nil, wrong
		case utf8.RuneCountInString(v) > maxAttributeValue:
			return nil, long
		}
		return v, nil
	case float64:
		if a.Type != "number" || math.Abs(v) > 1<<53 {
			return nil, wrong
		}
		return v, nil
	case bool:
		if a.Type != "boolean" {
			return nil, wrong
		}
		return v, nil
	case []any:
		if a.Type != "list" {
			return nil, wrong
		}
		items := []string{}
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, wrong
			}
			if s = strings.TrimSpace(s); s != "" && !slices.Contains(items, s) {
				if utf8.RuneCountInString(s) > maxAttributeValue {
					return nil, long
				}
				items = append(items, s)
			}
		}
		if len(items) > maxListValues {
			return nil, httpx.Invalid(fmt.Sprintf("%s holds up to %d values.", a.Label, maxListValues))
		}
		if len(items) == 0 {
			return nil, nil
		}
		return items, nil
	}
	return nil, wrong
}

func (h *handlers) setAttributes(ctx context.Context, current map[string]any, in map[string]json.RawMessage, self bool) (map[string]any, []string, error) {
	all, err := h.st.ListAttributes(ctx)
	if err != nil {
		return nil, nil, err
	}
	next, changed := maps.Clone(current), []string{}
	if next == nil {
		next = map[string]any{}
	}
	for _, name := range slices.Sorted(maps.Keys(in)) {
		i := slices.IndexFunc(all, func(a store.Attribute) bool { return a.Name == name })
		if i < 0 {
			return nil, nil, httpx.Invalid(fmt.Sprintf("There is no attribute called %q. Refresh the page, or ask a global administrator to add it under Settings, User attributes.", name))
		}
		a := all[i]
		if self && !a.SelfService {
			return nil, nil, httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", a.Label+" is managed by your administrators, so you can't change it here. Ask them to update it.")
		}
		value, err := attributeValue(a, in[name])
		if err != nil {
			return nil, nil, err
		}
		before, _ := json.Marshal(current[name])
		after, _ := json.Marshal(value)
		if string(before) == string(after) {
			continue
		}
		delete(next, name)
		if value != nil {
			next[name] = value
		}
		changed = append(changed, a.Label)
	}
	return next, changed, nil
}

func (h *handlers) updateMyAttributes(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in map[string]json.RawMessage
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	next := actor
	values, changed, err := h.setAttributes(r.Context(), actor.Attributes, in, true)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		return h.respondUser(w, r, actor.ID)
	}
	next.Attributes = values
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateUserProfile(r.Context(), next); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.update", Summary: "Updated their " + strings.Join(changed, ", "), TargetType: "user", TargetID: actor.ID, TargetLabel: actor.Name, Changes: store.Diff(actor, next)})
	})
	if err != nil {
		return err
	}
	return h.respondUser(w, r, actor.ID)
}

func (h *handlers) checkMappings(ctx context.Context, app store.Application, mappings []store.Claim) ([]store.Claim, error) {
	if len(mappings) > maxClaimMappings {
		return nil, httpx.Invalid(fmt.Sprintf("Map up to %d claims per application.", maxClaimMappings))
	}
	all, err := h.st.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	clean := []store.Claim{}
	for _, m := range mappings {
		m.Name = strings.TrimSpace(m.Name)
		attribute, custom := strings.CutPrefix(m.Source, "attribute.")
		switch {
		case m.Name == "" || len(m.Name) > maxClaimName || !utf8.ValidString(m.Name) || strings.ContainsFunc(m.Name, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }):
			return nil, httpx.Invalid(fmt.Sprintf("%q is not a valid claim name. Use up to %d characters without spaces, such as employee_id.", m.Name, maxClaimName))
		case app.Protocol != "saml" && slices.Contains(oidc.ReservedClaims, m.Name):
			return nil, httpx.Invalid(fmt.Sprintf("Halo sets %s itself in every token, so it can't be mapped. Pick another claim name.", m.Name))
		case slices.ContainsFunc(clean, func(c store.Claim) bool { return c.Name == m.Name }):
			return nil, httpx.Invalid(fmt.Sprintf("%s is mapped twice. Keep one mapping per claim name.", m.Name))
		case m.Source == store.OmitClaim || slices.Contains(store.ClaimSources, m.Source):
		case !custom || !slices.ContainsFunc(all, func(a store.Attribute) bool { return a.Name == attribute }):
			return nil, httpx.Invalid(fmt.Sprintf("%s has no source Halo knows. Refresh the page and pick a profile field or a custom attribute.", m.Name))
		}
		clean = append(clean, m)
	}
	return clean, nil
}
