# Webhooks

Webhooks send Halo's audit and sign-in events to your own HTTPS endpoint as they happen, so a SIEM, a chat channel or an internal tool can react to them. Each request is signed with a secret only you and Halo know.

Security administrators manage webhook endpoints; every administrator can read them and their delivery logs.

## Events

Every event has the same envelope:

```json
{
  "id": "aud_01JA2X5Q6R7S8T9V0W1X2Y3Z4A",
  "type": "user.invite",
  "time": "2026-10-04T10:00:10.123456Z",
  "data": {
    "id": "aud_01JA2X5Q6R7S8T9V0W1X2Y3Z4A",
    "time": "2026-10-04T10:00:10.123456+00:00",
    "actorId": "usr_01J9Z8Q4M2N5P7R9T1V3X5Z7B9",
    "action": "user.invite",
    "summary": "Invited Sam Lee <sam@example.com>",
    "targetType": "user",
    "targetId": "usr_01JA2X5Q6R7S8T9V0W1X2Y3Z4B",
    "target": "Sam Lee",
    "ip": "203.0.113.7"
  }
}
```

There are two sources:

- **Audit events.** The type is the audit action, such as `user.invite`, `group.member.add`, `application.secret.rotate`, `policy.update`, `scim.user.suspend` or `access_request.approve`. `data` is the audit event: `id`, `time`, `actorId` (null for changes Halo made itself), `action`, `summary`, `targetType`, `targetId`, `target` and `ip`.
- **Sign-in events.** The type is `sign_in.success`, `sign_in.failure` or `sign_in.interrupted`. `data` is the sign-in event: `id`, `time`, `userId`, `email`, `appId`, `result`, `method`, `ip`, `location`, `device`, `risk` and `reason`.

The **Audit log** in the console shows every action name Halo writes. `GET /api/v1/webhooks/event-types` lists the families, such as `user.*`, `sign_in.*` and `policy.*`.

## Add an endpoint

1. In the console, open **Webhooks** and choose **Add endpoint**.
2. Enter the URL. It must use https; plain http works only for `localhost`, names under `.localhost` and loopback addresses while you test.
3. Choose the events, between 1 and 50 entries. An entry is an exact type such as `user.suspend`, a family such as `user.*`, which matches every type that starts with `user.`, or `*` for everything.
4. Save, and copy the **Signing secret**. It starts with `whsec_` and Halo shows it only now.
5. Send a test event from the endpoint's page. Halo sends a `webhook.test` event right away and shows the result.

An endpoint receives only events that happen after it was created. Pausing an endpoint stops deliveries: events that happen while it is paused are never sent, and deliveries that were already waiting resume when you turn it back on. Deleting an endpoint deletes its delivery log.

The secret cannot be changed. To replace it, add a new endpoint, move your receiver to the new secret, and delete the old endpoint.

## Delivery

Halo looks for new events every 10 seconds and sends events that are at least 5 seconds old. Each delivery is an HTTP `POST` with:

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `User-Agent` | `Halo-Webhooks` |
| `Halo-Signature` | `t=<unix time>,v1=<hex HMAC-SHA256>` |

- Any `2xx` answer within 10 seconds counts as delivered. Halo does not follow redirects, so a `3xx` answer is a failure.
- After a failure, Halo retries after 30 seconds, then waits twice as long before each further attempt: 1, 2, 4, 8, 16 and 32 minutes. After 8 attempts, about an hour after the first, the delivery is marked **failed**.
- Each event is queued once per endpoint. A retry sends the same body, with the same `id`, and a new signature.
- A retried delivery can arrive after newer events. Use `time` to order events and `id` to ignore ones you already processed.

The endpoint's page in the console lists its 50 most recent deliveries with their status, number of attempts, the HTTP status your endpoint answered, the error and the exact body sent.

## Verify the signature

The `Halo-Signature` header has two parts:

- `t`, the Unix time in seconds when Halo sent this attempt;
- `v1`, the hex-encoded HMAC-SHA256 of `t`, a dot, and the raw request body, keyed with the whole signing secret, including `whsec_`.

To verify a request:

1. Read the raw body before parsing it. Re-encoding the JSON changes the bytes and breaks the signature.
2. Split the header on `,` and each part on `=`.
3. Reject the request when `t` is more than 5 minutes away from your clock.
4. Compute the HMAC over `t + "." + body` and compare it with `v1` in constant time.
5. Answer `2xx` quickly, and do slow work afterwards.

A complete receiver in Go:

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type event struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Time time.Time       `json:"time"`
	Data json.RawMessage `json:"data"`
}

func verify(secret []byte, header string, body []byte) bool {
	var t, sig string
	for _, part := range strings.Split(header, ",") {
		key, value, _ := strings.Cut(part, "=")
		switch key {
		case "t":
			t = value
		case "v1":
			sig = value
		}
	}
	unix, err := strconv.ParseInt(t, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)).Abs() > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(t + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sig))
}

func main() {
	secret := []byte(os.Getenv("HALO_WEBHOOK_SECRET"))
	http.HandleFunc("POST /halo", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil || !verify(secret, r.Header.Get("Halo-Signature"), body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		var e event
		if err := json.Unmarshal(body, &e); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		log.Printf("%s %s %s", e.Time.Format(time.RFC3339), e.Type, e.ID)
		w.WriteHeader(http.StatusNoContent)
	})
	log.Fatal(http.ListenAndServe(":9100", nil))
}
```

Run it with the secret from step 4 of [Add an endpoint](#add-an-endpoint):

```bash
HALO_WEBHOOK_SECRET=whsec_… go run .
```

Halo stores the secret encrypted with `HALO_SECRET_KEY`. If that key changes, Halo can no longer sign deliveries for existing endpoints; add new endpoints.

## API

Endpoints are at `/api/v1/webhooks`, with `/api/v1/webhooks/{id}/test` to send a test event; see the [API](api.md).
