package scim

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"halo/internal/store"
)

const (
	bulkResponseSchema = "urn:ietf:params:scim:api:messages:2.0:BulkResponse"
	maxOperations      = 100
	maxPayloadSize     = 1 << 20
)

var (
	bulkPath    = regexp.MustCompile(`^/(?:Users|Groups)(?:/([^/?#\s]+))?$`)
	selectorRef = regexp.MustCompile(`"bulkId:([^"]*)"`)
)

type bulkOp struct {
	Method string          `json:"method"`
	BulkID string          `json:"bulkId"`
	Path   string          `json:"path"`
	Data   json.RawMessage `json:"data"`
}

type bulkResult struct {
	Location string          `json:"location,omitempty"`
	Method   string          `json:"method"`
	BulkID   string          `json:"bulkId,omitempty"`
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response,omitempty"`
	code     int
}

func (res *bulkResult) fail(e *scimError) *bulkResult {
	res.code, res.Status = e.status, strconv.Itoa(e.status)
	res.Response, _ = json.Marshal(e.body())
	return res
}

func (s *server) bulk(w http.ResponseWriter, r *http.Request, _ store.User) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadSize+1))
	if err != nil {
		return err
	}
	if len(raw) > maxPayloadSize {
		return fail(http.StatusRequestEntityTooLarge, "", fmt.Sprintf("The size of the bulk request exceeds the maxPayloadSize (%d bytes). Split it into smaller requests.", maxPayloadSize))
	}
	var req struct {
		FailOnErrors int      `json:"failOnErrors"`
		Operations   []bulkOp `json:"Operations"`
	}
	if json.Unmarshal(raw, &req) != nil || len(req.Operations) == 0 {
		return fail(http.StatusBadRequest, "invalidSyntax", "Send a BulkRequest document with at least one entry in Operations.")
	}
	if len(req.Operations) > maxOperations {
		return fail(http.StatusRequestEntityTooLarge, "", fmt.Sprintf("The bulk request has %d operations, more than the maxOperations (%d). Split it into smaller requests.", len(req.Operations), maxOperations))
	}
	ids := map[string]string{}
	results := make([]*bulkResult, len(req.Operations))
	failed := 0
	stopped := func() bool { return req.FailOnErrors > 0 && failed >= req.FailOnErrors }
	pending := make([]int, len(req.Operations))
	for i := range pending {
		pending[i] = i
	}
	for progress := true; progress && !stopped(); {
		progress = false
		var waiting []int
		blocked := map[string]bool{}
		for _, i := range pending {
			if stopped() {
				break
			}
			op := req.Operations[i]
			resource := op.Path
			if strings.ToUpper(op.Method) == "POST" {
				resource = ""
			}
			target, data, ok := resolve(op, ids)
			if !ok || blocked[resource] {
				waiting = append(waiting, i)
				blocked[resource] = resource != ""
				continue
			}
			results[i], progress = s.runBulk(r, op, target, data, ids), true
			if results[i].code >= 400 {
				failed++
			}
		}
		pending = waiting
	}
	for _, i := range pending {
		if stopped() {
			break
		}
		op := req.Operations[i]
		res := &bulkResult{Method: strings.ToUpper(op.Method), BulkID: op.BulkID}
		if res.Method != "POST" {
			target, _, _ := resolve(op, ids)
			res.Location = s.base + cmp.Or(target, op.Path)
		}
		results[i] = res.fail(fail(http.StatusConflict, "invalidValue",
			"This operation references a bulkId that no successful POST in this request created, bulkIds that reference each other in a cycle, or follows such an operation on the same resource."))
		failed++
	}
	ops := []*bulkResult{}
	for _, res := range results {
		if res != nil {
			ops = append(ops, res)
		}
	}
	return write(w, http.StatusOK, map[string]any{"schemas": []string{bulkResponseSchema}, "Operations": ops})
}

func resolve(op bulkOp, ids map[string]string) (string, []byte, bool) {
	target := op.Path
	if m := bulkPath.FindStringSubmatch(target); m != nil {
		if ref, isRef := strings.CutPrefix(m[1], "bulkId:"); isRef {
			id, ok := ids[ref]
			if !ok {
				return "", nil, false
			}
			target = strings.TrimSuffix(target, m[1]) + id
		}
	}
	var v any
	if len(op.Data) == 0 || json.Unmarshal(op.Data, &v) != nil {
		return target, op.Data, true
	}
	v, ok := substitute(v, ids)
	data, _ := json.Marshal(v)
	return target, data, ok
}

func substitute(v any, ids map[string]string) (any, bool) {
	ok := true
	switch t := v.(type) {
	case string:
		if ref, isRef := strings.CutPrefix(t, "bulkId:"); isRef {
			id, found := ids[ref]
			return id, found
		}
	case []any:
		for i := range t {
			var resolved bool
			t[i], resolved = substitute(t[i], ids)
			ok = ok && resolved
		}
	case map[string]any:
		for k := range t {
			var resolved bool
			if selector, isPath := t[k].(string); isPath && strings.EqualFold(k, "path") {
				resolved = true
				t[k] = selectorRef.ReplaceAllStringFunc(selector, func(ref string) string {
					id, found := ids[selectorRef.FindStringSubmatch(ref)[1]]
					resolved = resolved && found
					return strconv.Quote(id)
				})
			} else {
				t[k], resolved = substitute(t[k], ids)
			}
			ok = ok && resolved
		}
	}
	return v, ok
}

func (s *server) runBulk(r *http.Request, op bulkOp, target string, data []byte, ids map[string]string) *bulkResult {
	method := strings.ToUpper(op.Method)
	res := &bulkResult{Method: method, BulkID: op.BulkID}
	switch {
	case !slices.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, method) || !bulkPath.MatchString(target) || path.Clean(target) != target:
		return res.fail(fail(http.StatusBadRequest, "invalidSyntax", "Each operation needs the method POST, PUT, PATCH or DELETE and a path such as /Users or /Groups/{id}."))
	case method == "POST" && op.BulkID == "":
		return res.fail(fail(http.StatusBadRequest, "invalidSyntax", "A POST operation needs a bulkId."))
	}
	sub := r.Clone(r.Context())
	sub.Method, sub.URL, sub.RequestURI = method, &url.URL{Path: "/scim/v2" + target}, ""
	sub.Body, sub.ContentLength = io.NopCloser(bytes.NewReader(data)), int64(len(data))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, sub)
	res.code, res.Status = rec.Code, strconv.Itoa(rec.Code)
	if rec.Code >= 400 {
		res.Response = rec.Body.Bytes()
	}
	if method != "POST" || rec.Code < 400 {
		res.Location = cmp.Or(rec.Header().Get("Location"), s.base+target)
	}
	if method == "POST" && rec.Code < 400 {
		var created struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &created)
		ids[op.BulkID] = created.ID
	}
	return res
}
