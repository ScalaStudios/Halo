package store

import (
	"encoding/json"
	"slices"
	"time"
)

type Method struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	AddedAt    time.Time  `json:"addedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	Detail     string     `json:"detail,omitempty"`
}

type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	EmailVerified bool       `json:"emailVerified"`
	Name          string     `json:"name"`
	Title         string     `json:"title"`
	Department    string     `json:"department"`
	Location      string     `json:"location"`
	Status        string     `json:"status"`
	Strength      string     `json:"strength"`
	Methods       []Method   `json:"methods"`
	Roles         []string   `json:"roles"`
	RoleKeys      []string   `json:"-"`
	GroupIDs      []string   `json:"groupIds"`
	AppIDs        []string   `json:"appIds"`
	ManagerID     *string    `json:"managerId"`
	Source        string     `json:"source"`
	Kind          string     `json:"-"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSignInAt  *time.Time `json:"lastSignInAt"`
	AvatarURL     *string    `json:"avatarUrl"`
	Guest         bool       `json:"guest"`
	Sponsor       *NamedRef  `json:"sponsor"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	Directory     *NamedRef  `json:"directory"`

	EligibleRoles  []string            `json:"eligibleRoles"`
	Activations    []RoleActivation    `json:"activations"`
	RoleScopes     map[string][]string `json:"roleScopes"`
	EligibleScopes map[string][]string `json:"eligibleScopes"`

	Attributes map[string]any `json:"attributes"`

	POSIX *POSIXAccount `json:"posix"`
}

type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Kind        string     `json:"kind"`
	Rule        *string    `json:"rule,omitempty"`
	JoinPolicy  string     `json:"joinPolicy"`
	Owners      []NamedRef `json:"owners"`
	MemberCount int        `json:"memberCount"`
	Source      string     `json:"source"`
	CreatedAt   time.Time  `json:"createdAt"`

	POSIX *POSIXGroup `json:"posix"`
}

func (g Group) OwnedBy(userID string) bool {
	return slices.ContainsFunc(g.Owners, func(o NamedRef) bool { return o.ID == userID })
}

type Claim struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type TokenPolicy struct {
	AccessTokenTTL  int      `json:"accessTokenTtl"`
	RefreshTokenTTL int      `json:"refreshTokenTtl"`
	IDTokenTTL      int      `json:"idTokenTtl"`
	IDTokenAlg      string   `json:"idTokenAlg"`
	Rotation        bool     `json:"rotation"`
	Exchange        bool     `json:"exchange"`
	ExchangeFrom    []string `json:"exchangeFrom"`
}

type Credential struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	Hint       string     `json:"hint"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	SecretHash []byte     `json:"-"`
}

type Application struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Protocol       string       `json:"protocol"`
	Type           string       `json:"type"`
	Status         string       `json:"status"`
	ClientID       string       `json:"clientId"`
	Homepage       string       `json:"homepage"`
	RedirectURIs   []string     `json:"redirectUris"`
	PostLogoutURIs []string     `json:"postLogoutUris"`
	Scopes         []string     `json:"scopes"`
	Claims         []Claim      `json:"claims"`
	GroupIDs       []string     `json:"groupIds"`
	UserCount      int          `json:"userCount"`
	Credentials    []Credential `json:"credentials"`
	Owner          *string      `json:"owner"`
	CreatedAt      time.Time    `json:"createdAt"`
	SignIns7d      int          `json:"signIns7d"`
	FailureRate7d  float64      `json:"failureRate7d"`
	TokenPolicy    TokenPolicy  `json:"tokenPolicy"`
	MaxAuthAge     int          `json:"maxAuthAge"`
	SetupGuide     string       `json:"setupGuide"`
	LogoURL        *string      `json:"logoUrl"`
	ClaimMappings  []Claim      `json:"claimMappings"`
	Roles          []AppRole    `json:"roles"`

	BackchannelLogoutURI             string `json:"backchannelLogoutUri"`
	BackchannelLogoutSessionRequired bool   `json:"backchannelLogoutSessionRequired"`

	RequirePAR  bool `json:"requirePar"`
	RequirePKCE bool `json:"requirePkce"`
	RequireDPoP bool `json:"requireDpop"`

	KerberosSignIn bool `json:"kerberosSignIn"`

	TokenEndpointAuthMethod string          `json:"tokenEndpointAuthMethod"`
	JWKS                    json.RawMessage `json:"jwks,omitempty"`
	JWKSURI                 string          `json:"jwksUri"`
	FAPI2Profile            bool            `json:"fapi2Profile"`

	RegisteredDynamically bool   `json:"registeredDynamically"`
	AwaitingApproval      bool   `json:"awaitingApproval"`
	RequiresConsent       bool   `json:"requiresConsent"`
	RegistrationHash      []byte `json:"-"`
}

func (a Application) ReauthRequired(authTime time.Time) bool {
	return a.MaxAuthAge > 0 && time.Since(authTime) > time.Duration(a.MaxAuthAge)*time.Second
}

type Session struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	Device       string     `json:"device"`
	OS           string     `json:"os"`
	Browser      string     `json:"browser"`
	IP           string     `json:"ip"`
	Location     string     `json:"location"`
	Method       string     `json:"method"`
	SecondFactor bool       `json:"-"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastActiveAt time.Time  `json:"lastActiveAt"`
	ExpiresAt    time.Time  `json:"-"`
	RevokedAt    *time.Time `json:"-"`
	Current      bool       `json:"current,omitempty"`
	Client       string     `json:"client,omitempty"`

	ImpersonationID string `json:"impersonationId,omitempty"`

	ReauthenticatedAt *time.Time `json:"-"`
}

type SignInEvent struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	UserID    *string   `json:"userId"`
	Email     string    `json:"email"`
	AppID     *string   `json:"appId"`
	Result    string    `json:"result"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	Location  string    `json:"location"`
	Device    string    `json:"device"`
	Risk      string    `json:"risk"`
	Reason    string    `json:"reason,omitempty"`
	Challenge string    `json:"challenge,omitempty"`
	Hook      string    `json:"hook,omitempty"`

	ImpersonatorID *string `json:"impersonatorId,omitempty"`
}

type AuditEvent struct {
	ID          string    `json:"id"`
	Time        time.Time `json:"time"`
	ActorID     *string   `json:"actorId"`
	Action      string    `json:"action"`
	Summary     string    `json:"summary"`
	TargetType  string    `json:"targetType"`
	TargetID    string    `json:"targetId"`
	TargetLabel string    `json:"target"`
	IP          string    `json:"ip"`
	Changes     []Change  `json:"changes"`
}

type Change struct {
	Field    string          `json:"field"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
	Redacted bool            `json:"redacted,omitempty"`
}
