package hostd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

type accounts struct {
	names map[string]bool
	ids   map[int]bool
}

func readAccounts(path string) (accounts, error) {
	a := accounts{names: map[string]bool{}, ids: map[int]bool{}}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 3 {
			continue
		}
		a.names[strings.TrimLeft(fields[0], "+-")] = true
		if n, err := strconv.Atoi(fields[2]); err == nil {
			a.ids[n] = true
		}
	}
	return a, scanner.Err()
}

type files struct {
	passwd, group, sudoers []byte
	skipped                []string
}

func render(d Directory, localUsers, localGroups accounts, homeBase string) files {
	var f files
	var passwd, group, sudoers strings.Builder
	names, ids, accepted, homes := map[string]bool{}, map[int]bool{}, map[string]bool{}, map[string]bool{}
	for _, u := range d.Users {
		if !NamePattern.MatchString(u.Name) || u.UID < MinID || u.GID != u.UID || !ValidHome(u.Home, homeBase) || !PathPattern.MatchString(u.Shell) ||
			localUsers.names[u.Name] || localUsers.ids[u.UID] || localGroups.names[u.Name] || localGroups.ids[u.GID] || names[u.Name] || ids[u.UID] || homes[u.Home] {
			f.skipped = append(f.skipped, u.Name)
			continue
		}
		names[u.Name], ids[u.UID], accepted[u.Name], homes[u.Home] = true, true, true, true
		fmt.Fprintf(&passwd, "%s:*:%d:%d:%s:%s:%s\n", u.Name, u.UID, u.GID, gecos(u.Gecos), u.Home, u.Shell)
		fmt.Fprintf(&group, "%s:x:%d:\n", u.Name, u.GID)
		sudoers.WriteString(sudoLine(u))
	}
	for _, g := range d.Groups {
		if !NamePattern.MatchString(g.Name) || g.GID < MinID || localGroups.names[g.Name] || localGroups.ids[g.GID] || names[g.Name] || ids[g.GID] {
			f.skipped = append(f.skipped, g.Name)
			continue
		}
		names[g.Name], ids[g.GID] = true, true
		members := slices.DeleteFunc(slices.Clone(g.Members), func(m string) bool { return !accepted[m] })
		fmt.Fprintf(&group, "%s:x:%d:%s\n", g.Name, g.GID, strings.Join(members, ","))
	}
	f.passwd, f.group, f.sudoers = []byte(passwd.String()), []byte(group.String()), []byte(sudoers.String())
	return f
}

func gecos(name string) string {
	return strings.Map(func(r rune) rune {
		if r == ':' || r == ',' || r == '=' || r == '\\' || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, name)
}

func sudoLine(u User) string {
	var commands []string
	for _, c := range u.Sudo {
		if c == "ALL" {
			return fmt.Sprintf("#%d ALL=(root) NOPASSWD: ALL\n", u.UID)
		}
		if ValidCommand(c) && !slices.Contains(commands, c) {
			commands = append(commands, c)
		}
	}
	if len(commands) == 0 {
		return ""
	}
	return fmt.Sprintf("#%d ALL=(root) NOPASSWD: %s\n", u.UID, strings.Join(commands, ", "))
}
