package scim

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	UserSchema       = "urn:ietf:params:scim:schemas:core:2.0:User"
	GroupSchema      = "urn:ietf:params:scim:schemas:core:2.0:Group"
	EnterpriseSchema = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	PatchSchema      = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	listSchema       = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	errorSchema      = "urn:ietf:params:scim:api:messages:2.0:Error"
	maxResults       = 200
)

type Name struct {
	Formatted  string `json:"formatted"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type Email struct {
	Value   string `json:"value"`
	Type    string `json:"type"`
	Primary bool   `json:"primary"`
}

type Photo struct {
	Value   string `json:"value"`
	Type    string `json:"type"`
	Primary bool   `json:"primary"`
}

type Manager struct {
	Value       string `json:"value"`
	Ref         string `json:"$ref,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type Enterprise struct {
	Department string   `json:"department,omitempty"`
	Manager    *Manager `json:"manager,omitempty"`
}

type Meta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
}

type User struct {
	Schemas     []string    `json:"schemas"`
	ID          string      `json:"id,omitempty"`
	ExternalID  string      `json:"externalId,omitempty"`
	UserName    string      `json:"userName"`
	Name        Name        `json:"name"`
	DisplayName string      `json:"displayName"`
	Title       string      `json:"title,omitempty"`
	Emails      []Email     `json:"emails"`
	Photos      []Photo     `json:"photos,omitempty"`
	Active      bool        `json:"active"`
	Enterprise  *Enterprise `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User,omitempty"`
	Meta        *Meta       `json:"meta,omitempty"`
}

func NewUser(email, name, title, department string, active bool) User {
	given, family, _ := strings.Cut(name, " ")
	u := User{
		Schemas:     []string{UserSchema},
		UserName:    email,
		Name:        Name{Formatted: name, GivenName: given, FamilyName: family},
		DisplayName: name,
		Title:       title,
		Emails:      []Email{{Value: email, Type: "work", Primary: true}},
		Active:      active,
	}
	if department != "" {
		u.Schemas = append(u.Schemas, EnterpriseSchema)
		u.Enterprise = &Enterprise{Department: department}
	}
	return u
}

type scimError struct {
	status   int
	scimType string
	detail   string
}

func (e *scimError) Error() string {
	return e.detail
}

func fail(status int, scimType, detail string) *scimError {
	return &scimError{status: status, scimType: scimType, detail: detail}
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, actor store.User) error

type server struct {
	st   *store.Store
	base string
}

func New(d httpx.Deps) (http.Handler, error) {
	s := &server{st: d.Store, base: d.Config.Issuer() + "/scim/v2"}
	mux := http.NewServeMux()
	for pattern, fn := range map[string]handlerFunc{
		"GET /scim/v2/ServiceProviderConfig": s.serviceProviderConfig,
		"GET /scim/v2/ResourceTypes":         discover(s.resourceTypes),
		"GET /scim/v2/ResourceTypes/{id}":    discover(s.resourceTypes),
		"GET /scim/v2/Schemas":               discover(s.schemas),
		"GET /scim/v2/Schemas/{id}":          discover(s.schemas),
		"GET /scim/v2/Users":                 s.listUsers,
		"POST /scim/v2/Users":                s.createUser,
		"GET /scim/v2/Users/{id}":            s.getUser,
		"PUT /scim/v2/Users/{id}":            s.replaceUser,
		"PATCH /scim/v2/Users/{id}":          s.patchUser,
		"DELETE /scim/v2/Users/{id}":         s.deleteUser,
		"GET /scim/v2/Groups":                s.listGroups,
		"POST /scim/v2/Groups":               s.createGroup,
		"GET /scim/v2/Groups/{id}":           s.getGroup,
		"PUT /scim/v2/Groups/{id}":           s.replaceGroup,
		"PATCH /scim/v2/Groups/{id}":         s.patchGroup,
		"DELETE /scim/v2/Groups/{id}":        s.deleteGroup,
		"/scim/": func(http.ResponseWriter, *http.Request, store.User) error {
			return fail(http.StatusNotFound, "", "This SCIM endpoint does not exist. Halo serves /scim/v2/Users, /scim/v2/Groups and the discovery endpoints.")
		},
	} {
		mux.Handle(pattern, s.handle(fn))
	}
	return mux, nil
}

func (s *server) handle(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, err := authorize(r)
		if err == nil {
			err = fn(w, r, actor)
		}
		if err == nil {
			return
		}
		var e *scimError
		switch {
		case errors.As(err, &e):
		case errors.Is(err, store.ErrNotFound):
			e = fail(http.StatusNotFound, "", "This resource was not found. It may have been deleted.")
		case errors.Is(err, store.ErrConflict):
			e = fail(http.StatusConflict, "uniqueness", "Another resource already uses this userName, displayName or externalId.")
		default:
			slog.ErrorContext(r.Context(), "scim request failed", "method", r.Method, "path", r.URL.Path, "error", err)
			e = fail(http.StatusInternalServerError, "", "Halo could not complete the request. Try again; if it keeps failing, check the Halo server log.")
		}
		if e.status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Halo SCIM"`)
		}
		body := map[string]any{"schemas": []string{errorSchema}, "status": strconv.Itoa(e.status), "detail": e.detail}
		if e.scimType != "" {
			body["scimType"] = e.scimType
		}
		_ = write(w, e.status, body)
	})
}

func authorize(r *http.Request) (store.User, error) {
	scopes, viaKey := httpx.KeyScopes(r)
	actor, ok := httpx.CurrentUser(r)
	if !viaKey || !ok || !slices.Contains(scopes, "scim") {
		return store.User{}, fail(http.StatusUnauthorized, "", "Authenticate with a Halo API key that has the scim scope, sent as Authorization: Bearer hlk_….")
	}
	if !actor.HasRole("global_admin", "user_admin") {
		return store.User{}, fail(http.StatusForbidden, "", actor.Name+" does not have the User administrator role, so it cannot provision people. Ask a global administrator to assign the role to the service account.")
	}
	return actor, nil
}

func write(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/scim+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}

func decode(r *http.Request, dst any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(dst); err != nil {
		return fail(http.StatusBadRequest, "invalidSyntax", "The request body is not valid JSON. Send a SCIM resource or a PatchOp document.")
	}
	return nil
}

func list[T any](w http.ResponseWriter, total, start int, resources []T) error {
	return write(w, http.StatusOK, map[string]any{"schemas": []string{listSchema}, "totalResults": total, "startIndex": start, "itemsPerPage": len(resources), "Resources": resources})
}

func discover(items func() []map[string]any) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, _ store.User) error {
		all := items()
		if r.PathValue("id") == "" {
			return list(w, len(all), 1, all)
		}
		for _, item := range all {
			if item["id"] == r.PathValue("id") {
				return write(w, http.StatusOK, item)
			}
		}
		return fail(http.StatusNotFound, "", r.PathValue("id")+" is not a resource type or schema Halo supports.")
	}
}

func paging(r *http.Request) (int, int) {
	start, err := strconv.Atoi(r.URL.Query().Get("startIndex"))
	if err != nil || start < 1 {
		start = 1
	}
	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		count = maxResults
	}
	return start, min(max(count, 0), maxResults)
}

var filterPattern = regexp.MustCompile(`(?i)^\s*([a-z.]+)\s+eq\s+"((?:[^"\\]|\\.)*)"\s*$`)

func parseFilter(raw, example string, allowed ...string) (string, string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", nil
	}
	m := filterPattern.FindStringSubmatch(raw)
	var value string
	if m == nil || !slices.Contains(allowed, strings.ToLower(m[1])) || json.Unmarshal([]byte(`"`+m[2]+`"`), &value) != nil {
		return "", "", fail(http.StatusBadRequest, "invalidFilter", "Halo supports a single eq condition here, such as "+example+".")
	}
	return strings.ToLower(m[1]), value, nil
}

func (s *server) serviceProviderConfig(w http.ResponseWriter, _ *http.Request, _ store.User) error {
	return write(w, http.StatusOK, map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]bool{"supported": true},
		"bulk":           map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         map[string]any{"supported": true, "maxResults": maxResults},
		"changePassword": map[string]bool{"supported": false},
		"sort":           map[string]bool{"supported": false},
		"etag":           map[string]bool{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type": "oauthbearertoken", "name": "Halo API key", "primary": true,
			"description": "Send a Halo API key with the scim scope as a bearer token. The key's service account needs the User administrator role.",
		}},
		"meta": map[string]string{"resourceType": "ServiceProviderConfig", "location": s.base + "/ServiceProviderConfig"},
	})
}

func (s *server) resourceTypes() []map[string]any {
	kind := func(id, endpoint, description, schema string) map[string]any {
		return map[string]any{
			"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": id, "name": id, "endpoint": endpoint, "description": description, "schema": schema,
			"meta": map[string]string{"resourceType": "ResourceType", "location": s.base + "/ResourceTypes/" + id},
		}
	}
	user := kind("User", "/Users", "People in Halo.", UserSchema)
	user["schemaExtensions"] = []map[string]any{{"schema": EnterpriseSchema, "required": false}}
	return []map[string]any{user, kind("Group", "/Groups", "Assigned groups in Halo. Rule-based groups are not exposed.", GroupSchema)}
}

type attribute struct {
	Name          string      `json:"name"`
	Type          string      `json:"type"`
	MultiValued   bool        `json:"multiValued"`
	Description   string      `json:"description"`
	Required      bool        `json:"required"`
	CaseExact     bool        `json:"caseExact"`
	Mutability    string      `json:"mutability"`
	Returned      string      `json:"returned"`
	Uniqueness    string      `json:"uniqueness"`
	SubAttributes []attribute `json:"subAttributes,omitempty"`
}

func attr(name, kind, description string, sub ...attribute) attribute {
	return attribute{Name: name, Type: kind, Description: description, Mutability: "readWrite", Returned: "default", Uniqueness: "none", SubAttributes: sub}
}

func required(a attribute) attribute {
	a.Required, a.Uniqueness = true, "server"
	return a
}

func multiValued(a attribute) attribute {
	a.MultiValued = true
	return a
}

func (s *server) schemas() []map[string]any {
	schema := func(id, name, description string, attributes ...attribute) map[string]any {
		return map[string]any{
			"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Schema"}, "id": id, "name": name, "description": description, "attributes": attributes,
			"meta": map[string]string{"resourceType": "Schema", "location": s.base + "/Schemas/" + id},
		}
	}
	reference := func(what string) []attribute {
		return []attribute{attr("value", "string", "Id of the "+what+"."), attr("$ref", "reference", "Location of the "+what+"."), attr("display", "string", "Name of the "+what+".")}
	}
	return []map[string]any{
		schema(UserSchema, "User", "A person in Halo.",
			required(attr("userName", "string", "Email address. Halo identifies people by email.")),
			attr("name", "complex", "The person's name.", attr("formatted", "string", "Full name."), attr("givenName", "string", "Given name."), attr("familyName", "string", "Family name.")),
			attr("displayName", "string", "Name shown in Halo."),
			attr("title", "string", "Job title."),
			attr("active", "boolean", "False suspends the person in Halo; true restores them."),
			multiValued(attr("emails", "complex", "Email addresses. Halo keeps the primary one.", attr("value", "string", "Email address."), attr("type", "string", "Address type, usually work."), attr("primary", "boolean", "Whether this is the primary address."))),
		),
		schema(GroupSchema, "Group", "An assigned group in Halo.",
			required(attr("displayName", "string", "Group name, unique in Halo.")),
			multiValued(attr("members", "complex", "People in the group.", reference("user")...)),
		),
		schema(EnterpriseSchema, "EnterpriseUser", "Enterprise attributes of a person.",
			attr("department", "string", "Department."),
			attr("manager", "complex", "The person's manager.", reference("manager")...),
		),
	}
}
