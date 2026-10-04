package httpx

import (
	"halo/internal/access"
	"halo/internal/config"
	"halo/internal/jobs"
	"halo/internal/mail"
	"halo/internal/store"
)

type Deps struct {
	Config config.Config
	Store  *store.Store
	Auth   *Auth
	Policy access.Engine
	Mailer mail.Mailer
	Jobs   *jobs.Scheduler
}
