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
	Effect  string
	Policy  string
	Reason  string
	Risk    string
	EventID string
	Terms   []store.Terms
}

type Engine interface {
	Evaluate(ctx context.Context, in Input) (Decision, error)
}

type AllowAll struct{}

func (AllowAll) Evaluate(context.Context, Input) (Decision, error) {
	return Decision{Effect: Allow}, nil
}
