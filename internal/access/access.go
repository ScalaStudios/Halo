package access

import (
	"context"

	"halo/internal/store"
)

const (
	Allow                    = "allow"
	Block                    = "block"
	RequirePhishingResistant = "require-phishing-resistant"
	RequireTerms             = "require-terms"
	RequireContext           = "require-context"
)

type Input struct {
	User        store.User
	App         *store.Application
	Method      string
	IP          string
	UserAgent   string
	DeviceToken string
	Session     *store.Session
}

type Decision struct {
	Effect   string
	Policy   string
	Reason   string
	Risk     string
	EventID  string
	Terms    []store.Terms
	Contexts []store.AuthContext
	Signals  []string
}

func (d Decision) StepUp(requested []store.AuthContext, sess store.Session) (string, *store.AuthContext) {
	for i, c := range d.Contexts {
		if !c.SatisfiedBy(sess) {
			return "", &d.Contexts[i]
		}
	}
	for _, c := range requested {
		if c.SatisfiedBy(sess) {
			return c.ID, nil
		}
	}
	if len(requested) > 0 {
		return "", &requested[0]
	}
	if len(d.Contexts) > 0 {
		return d.Contexts[0].ID, nil
	}
	return "", nil
}

type Engine interface {
	Evaluate(ctx context.Context, in Input) (Decision, error)
}

type AllowAll struct{}

func (AllowAll) Evaluate(context.Context, Input) (Decision, error) {
	return Decision{Effect: Allow}, nil
}
