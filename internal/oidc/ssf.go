package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"halo/internal/httpx"
	"halo/internal/store"
	"halo/internal/webhooks"
)

const (
	verificationEvent = "https://schemas.openid.net/secevent/ssf/event-type/verification"
	verifyInterval    = 30 * time.Second
)

var (
	SignalClient = httpx.GuardedClient()
	pollWait     = 20 * time.Second
)

var (
	errReceiverToken = httpx.Fail(http.StatusUnauthorized, "ERR_UNAUTHENTICATED", "Send an access token from the client credentials grant of the receiver's service application as a Bearer token.")
	errNoStream      = httpx.NotFound("The stream")
)

type transmitter struct {
	st     *store.Store
	s      *storage
	issuer string
}

type delivery struct {
	Method              string `json:"method"`
	EndpointURL         string `json:"endpoint_url,omitempty"`
	AuthorizationHeader string `json:"authorization_header,omitempty"`
}

type streamConfig struct {
	StreamID                string   `json:"stream_id"`
	Iss                     string   `json:"iss"`
	Aud                     string   `json:"aud"`
	EventsSupported         []string `json:"events_supported"`
	EventsRequested         []string `json:"events_requested,omitempty"`
	EventsDelivered         []string `json:"events_delivered"`
	Delivery                delivery `json:"delivery"`
	MinVerificationInterval int      `json:"min_verification_interval"`
	Description             string   `json:"description,omitempty"`
}

type streamRequest struct {
	StreamID        string    `json:"stream_id"`
	EventsRequested *[]string `json:"events_requested"`
	Delivery        *delivery `json:"delivery"`
	Description     *string   `json:"description"`
}

type streamStatus struct {
	StreamID string `json:"stream_id"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

type receiverHandler func(http.ResponseWriter, *http.Request, store.Application, store.SignalReceiver) error

func bad(message string) *httpx.Error {
	return httpx.Fail(http.StatusBadRequest, "ERR_INVALID", message)
}

func registerTransmitter(mux *http.ServeMux, st *store.Store, issuer string) {
	t := &transmitter{st: st, s: &storage{st: st, issuer: issuer}, issuer: issuer}
	mux.Handle("GET /.well-known/ssf-configuration", httpx.Handle(t.metadata))
	mux.Handle("GET /oauth2/ssf/stream", t.receiver(t.readStream))
	mux.Handle("POST /oauth2/ssf/stream", t.receiver(t.createStream))
	mux.Handle("PATCH /oauth2/ssf/stream", t.receiver(t.updateStream))
	mux.Handle("PUT /oauth2/ssf/stream", t.receiver(t.updateStream))
	mux.Handle("DELETE /oauth2/ssf/stream", t.receiver(t.deleteStream))
	mux.Handle("GET /oauth2/ssf/status", t.receiver(t.readStatus))
	mux.Handle("POST /oauth2/ssf/status", t.receiver(t.updateStatus))
	mux.Handle("POST /oauth2/ssf/verify", t.receiver(t.verify))
	mux.Handle("POST /oauth2/ssf/poll/{id}", t.receiver(t.poll))
}

func (t *transmitter) metadata(w http.ResponseWriter, _ *http.Request) error {
	return httpx.JSON(w, http.StatusOK, map[string]any{
		"spec_version":               "1_0",
		"issuer":                     t.issuer,
		"jwks_uri":                   t.issuer + "/oauth2/keys",
		"delivery_methods_supported": []string{store.PushDelivery, store.PollDelivery},
		"configuration_endpoint":     t.issuer + "/oauth2/ssf/stream",
		"status_endpoint":            t.issuer + "/oauth2/ssf/status",
		"verification_endpoint":      t.issuer + "/oauth2/ssf/verify",
		"authorization_schemes":      []map[string]string{{"spec_urn": "urn:ietf:rfc:6749"}},
		"default_subjects":           "ALL",
	})
}

func (t *transmitter) receiver(fn receiverHandler) http.Handler {
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		token, subject, err := t.s.accessTokenRow(ctx, bearer)
		if !ok || errors.Is(err, errAccessToken) || err == nil && (token.UserID != nil || token.ClientID != subject || !slices.Equal(token.Audience, []string{token.ClientID})) {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			return errReceiverToken
		}
		if err != nil {
			return err
		}
		app, err := t.st.GetApplicationByClientID(ctx, token.ClientID)
		if errors.Is(err, store.ErrNotFound) {
			return errReceiverToken
		}
		if err != nil {
			return err
		}
		receiver, err := t.st.GetSignalReceiver(ctx, app.ID)
		if errors.Is(err, store.ErrNotFound) || err == nil && (app.Status != "active" || app.Type != "service") {
			return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Shared signals are not turned on for "+app.Name+". A security administrator turns them on in the application's settings in Halo.")
		}
		if err != nil {
			return err
		}
		return fn(w, r, app, receiver)
	})
}

func decodeSignal(r *http.Request, dst ...any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		return bad("The request body could not be read.")
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = []byte("{}")
	}
	for _, d := range dst {
		if json.Unmarshal(raw, d) != nil {
			return bad("The request body is not a valid JSON object.")
		}
	}
	return nil
}

func (t *transmitter) config(app store.Application, receiver store.SignalReceiver, st store.SignalStream) streamConfig {
	supported := slices.DeleteFunc(slices.Clone(store.SignalEvents), func(e string) bool { return e == store.IdentifierChanged && receiver.SubjectFormat != "email" })
	delivered := slices.DeleteFunc(slices.Clone(supported), func(e string) bool { return !slices.Contains(st.EventsRequested, e) })
	d := delivery{Method: st.DeliveryMethod, EndpointURL: st.EndpointURL}
	if st.DeliveryMethod == store.PollDelivery {
		d.EndpointURL = t.issuer + "/oauth2/ssf/poll/" + st.ID
	}
	return streamConfig{StreamID: st.ID, Iss: t.issuer, Aud: app.ClientID, EventsSupported: supported, EventsRequested: st.EventsRequested, EventsDelivered: delivered,
		Delivery: d, MinVerificationInterval: int(verifyInterval.Seconds()), Description: st.Description}
}

func (t *transmitter) apply(st *store.SignalStream, in streamRequest, replace bool) error {
	if in.Description != nil || replace {
		if st.Description = strings.TrimSpace(deref(in.Description)); len(st.Description) > 200 {
			return bad("Keep the description under 200 characters.")
		}
	}
	if in.EventsRequested != nil || replace {
		events := slices.Compact(slices.Sorted(slices.Values(deref(in.EventsRequested))))
		if len(events) > 50 || slices.ContainsFunc(events, func(e string) bool { return len(e) > 256 }) {
			return bad("Request at most 50 event types, each an event type URI under 256 characters.")
		}
		st.EventsRequested = events
	}
	if in.Delivery == nil && !replace {
		return nil
	}
	d := deref(in.Delivery)
	switch d.Method {
	case "", store.PollDelivery:
		st.DeliveryMethod, st.EndpointURL, st.AuthorizationSealed = store.PollDelivery, "", nil
	case store.PushDelivery:
		var invalid *httpx.Error
		if _, err := webhooks.PublicURL(d.EndpointURL, "push endpoint", "Halo only pushes events to"); errors.As(err, &invalid) {
			return bad(invalid.Message)
		}
		if len(d.AuthorizationHeader) > 1024 || strings.ContainsFunc(d.AuthorizationHeader, unicode.IsControl) {
			return bad("Keep authorization_header under 1,024 characters, on one line.")
		}
		st.DeliveryMethod, st.EndpointURL, st.AuthorizationSealed = store.PushDelivery, d.EndpointURL, nil
		if d.AuthorizationHeader != "" {
			st.AuthorizationSealed = t.st.Sealer.Seal([]byte(d.AuthorizationHeader))
		}
	default:
		return bad("Choose " + store.PushDelivery + " (push) or " + store.PollDelivery + " (poll) as the delivery method.")
	}
	return nil
}

func record(r *http.Request, tx *store.Store, app store.Application, e store.AuditEvent) error {
	e.TargetType, e.TargetID, e.TargetLabel, e.IP = "application", app.ID, app.Name, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func (t *transmitter) readStream(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	id := r.URL.Query().Get("stream_id")
	configs := []streamConfig{}
	if receiver.Stream != nil && (id == "" || id == receiver.Stream.ID) {
		configs = append(configs, t.config(app, receiver, *receiver.Stream))
	}
	switch {
	case id == "":
		return httpx.JSON(w, http.StatusOK, configs)
	case len(configs) == 0:
		return errNoStream
	}
	return httpx.JSON(w, http.StatusOK, configs[0])
}

func (t *transmitter) createStream(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	if receiver.Stream != nil {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", app.Name+" already has a stream, and Halo keeps one stream per receiver. Change it with PATCH or PUT, or delete it first.")
	}
	var in streamRequest
	if err := decodeSignal(r, &in); err != nil {
		return err
	}
	next := store.SignalStream{AppID: app.ID}
	if err := t.apply(&next, in, true); err != nil {
		return err
	}
	ctx := r.Context()
	err := t.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if next, err = tx.CreateSignalStream(ctx, next); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{Action: "ssf_stream.create", Summary: fmt.Sprintf("%s created a shared signals stream delivering %d event types by %s", app.Name, len(t.config(app, receiver, next).EventsDelivered), method(next))})
	})
	if errors.Is(err, store.ErrConflict) {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", app.Name+" already has a stream.")
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, t.config(app, receiver, next))
}

func method(st store.SignalStream) string {
	if st.DeliveryMethod == store.PushDelivery {
		return "push"
	}
	return "poll"
}

func (t *transmitter) updateStream(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	var in streamRequest
	var fields map[string]any
	if err := decodeSignal(r, &in, &fields); err != nil {
		return err
	}
	if receiver.Stream == nil || in.StreamID != receiver.Stream.ID {
		return errNoStream
	}
	var current map[string]any
	raw, _ := json.Marshal(t.config(app, receiver, *receiver.Stream))
	_ = json.Unmarshal(raw, &current)
	for _, key := range []string{"iss", "aud", "events_supported", "events_delivered", "min_verification_interval"} {
		if value, ok := fields[key]; ok && !reflect.DeepEqual(value, current[key]) {
			return bad(key + " is set by Halo and does not match the stream. Leave it out or send the value Halo returned.")
		}
	}
	next := *receiver.Stream
	if err := t.apply(&next, in, r.Method == http.MethodPut); err != nil {
		return err
	}
	ctx := r.Context()
	err := t.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.UpdateSignalStream(ctx, next); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{Action: "ssf_stream.update", Summary: app.Name + " changed its shared signals stream", Changes: store.Diff(*receiver.Stream, next)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, t.config(app, receiver, next))
}

func (t *transmitter) deleteStream(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	if receiver.Stream == nil || r.URL.Query().Get("stream_id") != receiver.Stream.ID {
		return errNoStream
	}
	ctx := r.Context()
	err := t.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteSignalStream(ctx, *receiver.Stream); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{Action: "ssf_stream.delete", Summary: app.Name + " deleted its shared signals stream and its undelivered events"})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (t *transmitter) readStatus(w http.ResponseWriter, r *http.Request, _ store.Application, receiver store.SignalReceiver) error {
	if receiver.Stream == nil || r.URL.Query().Get("stream_id") != receiver.Stream.ID {
		return errNoStream
	}
	return httpx.JSON(w, http.StatusOK, streamStatus{StreamID: receiver.Stream.ID, Status: receiver.Stream.Status, Reason: receiver.Stream.Reason})
}

func (t *transmitter) updateStatus(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	var in streamStatus
	if err := decodeSignal(r, &in); err != nil {
		return err
	}
	if receiver.Stream == nil || in.StreamID != receiver.Stream.ID {
		return errNoStream
	}
	if !slices.Contains([]string{"enabled", "paused", "disabled"}, in.Status) || len(in.Reason) > 200 {
		return bad("Set status to enabled, paused or disabled, with a reason under 200 characters.")
	}
	ctx := r.Context()
	err := t.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetSignalStreamStatus(ctx, *receiver.Stream, in.Status, in.Reason); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{Action: "ssf_stream.status", Summary: fmt.Sprintf("%s set its shared signals stream to %s", app.Name, in.Status)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, in)
}

func (t *transmitter) verify(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	var in struct {
		StreamID string  `json:"stream_id"`
		State    *string `json:"state"`
	}
	if err := decodeSignal(r, &in); err != nil {
		return err
	}
	if receiver.Stream == nil || in.StreamID != receiver.Stream.ID {
		return errNoStream
	}
	if receiver.Stream.Status == "disabled" {
		return bad("The stream is disabled. Enable it before asking for a verification event.")
	}
	if in.State != nil && len(*in.State) > 1024 {
		return bad("Keep state under 1,024 characters.")
	}
	ctx := r.Context()
	last, err := t.st.LastSignal(ctx, app.ID, verificationEvent)
	if err != nil {
		return err
	}
	if wait := verifyInterval - time.Since(last); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		return httpx.Fail(http.StatusTooManyRequests, "ERR_RATE_LIMITED", fmt.Sprintf("Ask for a verification event at most once every %d seconds.", int(verifyInterval.Seconds())))
	}
	event := map[string]any{}
	if in.State != nil {
		event["state"] = *in.State
	}
	if err := t.st.QueueSignal(ctx, app.ID, verificationEvent, store.Signal{Event: event}); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (t *transmitter) poll(w http.ResponseWriter, r *http.Request, app store.Application, receiver store.SignalReceiver) error {
	var in struct {
		MaxEvents         *int     `json:"maxEvents"`
		ReturnImmediately bool     `json:"returnImmediately"`
		Ack               []string `json:"ack"`
		SetErrs           map[string]struct {
			Err         string `json:"err"`
			Description string `json:"description"`
		} `json:"setErrs"`
	}
	if err := decodeSignal(r, &in); err != nil {
		return err
	}
	stream := receiver.Stream
	if stream == nil || stream.ID != r.PathValue("id") || stream.DeliveryMethod != store.PollDelivery {
		return errNoStream
	}
	if len(in.Ack)+len(in.SetErrs) > 1000 {
		return bad("Acknowledge at most 1,000 events in one request.")
	}
	errs := map[string]string{}
	for jti, e := range in.SetErrs {
		errs[jti] = strings.TrimSuffix(fmt.Sprintf("The receiver rejected it: %.64s: %.300s", e.Err, e.Description), ": ")
	}
	ctx := r.Context()
	if err := t.st.AckSignals(ctx, app.ID, in.Ack, errs); err != nil {
		return err
	}
	limit := 100
	if in.MaxEvents != nil {
		limit = min(max(*in.MaxEvents, 0), limit)
	}
	for deadline := time.Now().Add(pollWait); ; {
		pending := []store.WebhookDelivery{}
		if stream.Status == "enabled" {
			var err error
			if pending, err = t.st.PendingSignals(ctx, app.ID, limit+1); err != nil {
				return err
			}
		}
		if len(pending) > 0 || in.ReturnImmediately || time.Now().After(deadline) {
			sets := map[string]string{}
			for _, d := range pending[:min(limit, len(pending))] {
				set, err := t.set(ctx, app, receiver, d)
				if err != nil {
					return err
				}
				sets[d.EventID] = set
			}
			return httpx.JSON(w, http.StatusOK, map[string]any{"sets": sets, "moreAvailable": len(pending) > limit})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (t *transmitter) set(ctx context.Context, app store.Application, receiver store.SignalReceiver, d store.WebhookDelivery) (string, error) {
	var sig store.Signal
	if err := json.Unmarshal([]byte(d.Body), &sig); err != nil {
		return "", err
	}
	subject := map[string]any{"format": "opaque", "id": receiver.Stream.ID}
	if sig.User != "" {
		switch {
		case receiver.SubjectFormat == "email" || d.EventType == store.IdentifierChanged:
			subject = map[string]any{"format": "email", "email": sig.Email}
		case receiver.SubjectFormat == "opaque":
			subject = map[string]any{"format": "opaque", "id": sig.User}
		default:
			subject = map[string]any{"format": "iss_sub", "iss": t.issuer, "sub": sig.User}
		}
		if sig.Session != "" || sig.Device != "" {
			subject = map[string]any{"format": "complex", "user": subject}
		}
		if sig.Session != "" {
			subject["session"] = map[string]any{"format": "opaque", "id": sig.Session}
		}
		if sig.Device != "" {
			subject["device"] = map[string]any{"format": "opaque", "id": sig.Device}
		}
	}
	claims := map[string]any{"iss": t.issuer, "aud": app.ClientID, "jti": d.EventID, "iat": d.CreatedAt.Unix(), "sub_id": subject, "events": map[string]any{d.EventType: sig.Event}}
	if sig.Txn != "" {
		claims["txn"] = sig.Txn
	}
	return t.s.sign(ctx, "secevent+jwt", claims)
}

func DeliverSignals(ctx context.Context, st *store.Store, issuer string, now time.Time) error {
	due, err := st.ClaimSignalDeliveries(ctx, now, 2*time.Minute, 100)
	if err != nil {
		return err
	}
	t := &transmitter{st: st, s: &storage{st: st, issuer: issuer}, issuer: issuer}
	for _, d := range due {
		var receiver store.SignalReceiver
		app, err := st.GetApplication(ctx, d.AppID)
		if err == nil {
			receiver, err = st.GetSignalReceiver(ctx, d.AppID)
		}
		if errors.Is(err, store.ErrNotFound) || err == nil && receiver.Stream == nil {
			continue
		}
		if err != nil {
			return err
		}
		set, err := t.set(ctx, app, receiver, d)
		if err != nil {
			return err
		}
		code, message := 0, "The push endpoint is not a valid URL."
		if req, err := http.NewRequestWithContext(ctx, http.MethodPost, receiver.Stream.EndpointURL, strings.NewReader(set)); err == nil {
			req.Header.Set("Content-Type", "application/secevent+jwt")
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "Halo-Signals")
			if receiver.Stream.AuthorizationSealed != nil {
				authorization, err := st.Sealer.Open(receiver.Stream.AuthorizationSealed)
				if err != nil {
					return err
				}
				req.Header.Set("Authorization", string(authorization))
			}
			code, message = webhooks.Post(SignalClient, req)
		}
		if err := webhooks.Record(ctx, st, d, code, message, now); err != nil {
			return err
		}
	}
	return nil
}

type signalHandlers struct{ st *store.Store }

var subjectFormats = []string{"iss_sub", "email", "opaque"}

func (h *signalHandlers) application(r *http.Request) (store.Application, error) {
	app, err := h.st.GetApplication(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return app, httpx.NotFound("The application")
	}
	return app, err
}

func (h *signalHandlers) get(w http.ResponseWriter, r *http.Request, _ store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	receiver, err := h.st.GetSignalReceiver(ctx, app.ID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.JSON(w, http.StatusOK, map[string]any{"receiver": nil, "deliveries": []store.WebhookDelivery{}})
	}
	if err != nil {
		return err
	}
	deliveries, err := h.st.ListSignalDeliveries(ctx, app.ID, 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"receiver": receiver, "deliveries": deliveries})
}

func (h *signalHandlers) put(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	var in struct {
		SubjectFormat string `json:"subjectFormat"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if app.Type != "service" || app.Protocol != "oidc" && app.Protocol != "oauth" {
		return httpx.Invalid("Shared signals go to service applications, which call Halo with the client credentials grant. " + app.Name + " is not one.")
	}
	if !slices.Contains(subjectFormats, in.SubjectFormat) {
		return httpx.Invalid("Choose iss_sub, email or opaque as the subject format.")
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		summary := "Turned on shared signals with " + in.SubjectFormat + " subjects"
		previous, err := tx.GetSignalReceiver(ctx, app.ID)
		switch {
		case err == nil && previous.SubjectFormat == in.SubjectFormat:
			return nil
		case err == nil:
			summary = "Changed the shared signals subject format from " + previous.SubjectFormat + " to " + in.SubjectFormat
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		if err := tx.SaveSignalReceiver(ctx, app.ID, in.SubjectFormat); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{ActorID: &actor.ID, Action: "application.shared_signals.update", Summary: summary})
	})
	if err != nil {
		return err
	}
	return h.get(w, r, actor)
}

func (h *signalHandlers) remove(w http.ResponseWriter, r *http.Request, actor store.User) error {
	app, err := h.application(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteSignalReceiver(ctx, app.ID); err != nil {
			return err
		}
		return record(r, tx, app, store.AuditEvent{ActorID: &actor.ID, Action: "application.shared_signals.delete", Summary: "Turned off shared signals and deleted the stream with its undelivered events"})
	})
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("Shared signals for " + app.Name)
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
