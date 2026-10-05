package store

import "time"

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
}

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Rule        *string   `json:"rule,omitempty"`
	MemberCount int       `json:"memberCount"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Claim struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type TokenPolicy struct {
	AccessTokenTTL  int  `json:"accessTokenTtl"`
	RefreshTokenTTL int  `json:"refreshTokenTtl"`
	IDTokenTTL      int  `json:"idTokenTtl"`
	Rotation        bool `json:"rotation"`
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
	SetupGuide     string       `json:"setupGuide"`
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
	CreatedAt    time.Time  `json:"createdAt"`
	LastActiveAt time.Time  `json:"lastActiveAt"`
	ExpiresAt    time.Time  `json:"-"`
	RevokedAt    *time.Time `json:"-"`
	Current      bool       `json:"current,omitempty"`
	Client       string     `json:"client,omitempty"`
}

type SignInEvent struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	UserID   *string   `json:"userId"`
	Email    string    `json:"email"`
	AppID    *string   `json:"appId"`
	Result   string    `json:"result"`
	Method   string    `json:"method"`
	IP       string    `json:"ip"`
	Location string    `json:"location"`
	Device   string    `json:"device"`
	Risk     string    `json:"risk"`
	Reason   string    `json:"reason,omitempty"`
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
}
