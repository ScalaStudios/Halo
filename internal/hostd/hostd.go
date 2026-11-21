package hostd

import (
	"fmt"
	"regexp"
	"strconv"
)

const MinID = 65536

var (
	NamePattern    = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)
	PathPattern    = regexp.MustCompile(`^/[A-Za-z0-9_./-]{0,254}$`)
	CommandPattern = regexp.MustCompile(`^/[A-Za-z0-9_./+-]+( [A-Za-z0-9_./+-]+)*$`)
)

type Directory struct {
	Users  []User  `json:"users"`
	Groups []Group `json:"groups"`
}

type User struct {
	Name  string   `json:"name"`
	UID   int      `json:"uid"`
	GID   int      `json:"gid"`
	Gecos string   `json:"gecos"`
	Home  string   `json:"home"`
	Shell string   `json:"shell"`
	Sudo  []string `json:"sudo"`
}

type Group struct {
	Name    string   `json:"name"`
	GID     int      `json:"gid"`
	Members []string `json:"members"`
}

func DirectoryMessage(timestamp int64) []byte {
	return fmt.Appendf(nil, "halo host directory %d", timestamp)
}

func Principal(uid int) string {
	return "halo:" + strconv.Itoa(uid)
}

func ValidCommand(command string) bool {
	return command == "ALL" || len(command) <= 255 && CommandPattern.MatchString(command)
}
