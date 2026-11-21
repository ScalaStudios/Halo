package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"halo/internal/httpx"
	halomail "halo/internal/mail"
	"halo/internal/store"
)

const (
	maxImportRows  = 500
	maxImportBytes = 1 << 20
)

var importColumns = []string{"email", "name", "title", "department", "location", "manageremail", "groups"}

type importPerson struct {
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	Title        string   `json:"title"`
	Department   string   `json:"department"`
	Location     string   `json:"location"`
	ManagerEmail string   `json:"managerEmail"`
	Groups       []string `json:"groups"`
}

type importRow struct {
	Row int `json:"row"`
	importPerson
	Error     *httpx.Error `json:"error,omitempty"`
	UserID    string       `json:"userId,omitempty"`
	EnrollURL string       `json:"enrollUrl,omitempty"`
	ExpiresAt *time.Time   `json:"expiresAt,omitempty"`
	Emailed   bool         `json:"emailed"`
	groupIDs  []string
	managerID *string
	manager   int
	token     string
}

func (h *handlers) importUsers(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if e := h.auth.Throttle(w, r, "bulk"); e != nil {
		return e
	}
	dryRun, err := queryFlag(r, "dryRun")
	if err != nil {
		return err
	}
	send, err := queryFlag(r, "sendInvitations")
	if err != nil {
		return err
	}
	rows, err := readImport(w, r)
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows) > maxImportRows {
		return httpx.Invalid(fmt.Sprintf("Import between 1 and %d people at a time. This import has %d, so split it into smaller files.", maxImportRows, len(rows)))
	}
	order, err := h.checkImport(r, rows)
	if err != nil {
		return err
	}
	if !dryRun && len(order) > 0 {
		if err := h.applyImport(r, actor, rows, order, send); err != nil {
			return err
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"dryRun": dryRun, "ready": len(order), "skipped": len(rows) - len(order), "rows": rows})
}

func queryFlag(r *http.Request, name string) (bool, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(value)
	if err != nil {
		return false, httpx.Invalid(name + " must be true or false.")
	}
	return on, nil
}

func readImport(w http.ResponseWriter, r *http.Request) ([]importRow, error) {
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media == "text/csv" {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, httpx.Fail(http.StatusRequestEntityTooLarge, "ERR_TOO_LARGE", "The file is larger than 1 MB. Split it into smaller files and import each one.")
		}
		if err != nil {
			return nil, err
		}
		return parseCSV(raw)
	}
	var in struct {
		People []importPerson `json:"people"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return nil, err
	}
	rows := make([]importRow, len(in.People))
	for i, p := range in.People {
		rows[i] = importRow{Row: i + 1, importPerson: p}
	}
	return rows, nil
}

func parseCSV(raw []byte) ([]importRow, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		return nil, httpx.Invalid("The file is not UTF-8 text. Save it as CSV UTF-8 from your spreadsheet app, then upload it again.")
	}
	reader := csv.NewReader(bytes.NewReader(raw))
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, httpx.Invalid("The file is empty. Add a header row with email and name columns, then one row per person.")
	}
	if err != nil {
		return nil, csvError(err)
	}
	columns := make([]string, len(header))
	for i, title := range header {
		column := strings.NewReplacer(" ", "", "_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(title)))
		switch {
		case column == "":
			continue
		case column == "role" || column == "roles":
			return nil, httpx.Invalid("Imports never grant administrator roles. Remove the " + strconv.Quote(title) + " column and grant roles from each person's profile.")
		case !slices.Contains(importColumns, column):
			return nil, httpx.Invalid(fmt.Sprintf("Halo doesn't import a %q column. Use email, name, title, department, location, manager email and groups.", title))
		case slices.Contains(columns, column):
			return nil, httpx.Invalid(fmt.Sprintf("The %q column appears twice. Keep one of them.", title))
		}
		columns[i] = column
	}
	if !slices.Contains(columns, "email") || !slices.Contains(columns, "name") {
		return nil, httpx.Invalid("The header row needs an email column and a name column.")
	}
	rows := []importRow{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, csvError(err)
		}
		if !slices.ContainsFunc(record, func(v string) bool { return strings.TrimSpace(v) != "" }) {
			continue
		}
		line, _ := reader.FieldPos(0)
		row := importRow{Row: line}
		for i, value := range record {
			switch columns[i] {
			case "email":
				row.Email = value
			case "name":
				row.Name = value
			case "title":
				row.Title = value
			case "department":
				row.Department = value
			case "location":
				row.Location = value
			case "manageremail":
				row.ManagerEmail = value
			case "groups":
				row.Groups = strings.Split(value, ";")
			}
		}
		rows = append(rows, row)
	}
}

func csvError(err error) error {
	var parse *csv.ParseError
	if errors.As(err, &parse) {
		return httpx.Invalid(fmt.Sprintf("Line %d of the file can't be read: %v. Check the quotes and commas on that line, then upload the file again.", parse.Line, parse.Err))
	}
	return err
}

func (h *handlers) checkImport(r *http.Request, rows []importRow) ([]int, error) {
	emails := []string{}
	for _, row := range rows {
		emails = append(emails, row.Email, row.ManagerEmail)
	}
	known, err := h.st.UsersByEmail(r.Context(), emails)
	if err != nil {
		return nil, err
	}
	existing := map[string]store.User{}
	for _, u := range known {
		existing[strings.ToLower(u.Email)] = u
	}
	groups, err := h.st.ListGroupsRaw(r.Context())
	if err != nil {
		return nil, err
	}
	byEmail := map[string]int{}
	for i := range rows {
		row := &rows[i]
		row.manager = -1
		err := checkImportRow(row, existing, groups)
		key := strings.ToLower(row.Email)
		if j, dup := byEmail[key]; dup && err == nil {
			err = httpx.Invalid(fmt.Sprintf("Row %d already has the email %s. Each person needs their own address, so remove one of the rows.", rows[j].Row, row.Email))
		} else if !dup {
			byEmail[key] = i
		}
		if err != nil {
			row.Error = httpx.Problem(r, err)
		}
	}
	for i := range rows {
		row := &rows[i]
		if row.Error != nil || row.ManagerEmail == "" {
			continue
		}
		manager, inHalo := existing[strings.ToLower(row.ManagerEmail)]
		j, inFile := byEmail[strings.ToLower(row.ManagerEmail)]
		switch {
		case inHalo && manager.Kind != "person":
			row.Error = httpx.Invalid(manager.Name + " is a service account, so they can't be anyone's manager. Use the email of the person " + row.Name + " reports to.")
		case inHalo:
			row.managerID = &manager.ID
		case !inFile:
			row.Error = httpx.Invalid("Nobody with the email " + row.ManagerEmail + " is in Halo or in this import. Correct the manager email, or leave it empty.")
		case j == i:
			row.Error = httpx.Invalid(row.Name + " can't be their own manager. Use the email of the person they report to.")
		default:
			row.manager = j
		}
	}
	order, state := []int{}, make([]int, len(rows))
	var visit func(int)
	visit = func(i int) {
		if state[i] != 0 {
			return
		}
		state[i] = 1
		if j := rows[i].manager; j >= 0 && rows[i].Error == nil {
			if state[j] != 1 {
				visit(j)
			}
			switch {
			case state[j] == 1:
				rows[i].Error = httpx.Invalid(fmt.Sprintf("This row reports to row %d, whose manager emails lead back to this row. Change one of them so the reporting line ends.", rows[j].Row))
			case rows[j].Error != nil:
				rows[i].Error = httpx.Invalid(fmt.Sprintf("Their manager on row %d can't be imported. Fix that row first.", rows[j].Row))
			}
		}
		state[i] = 2
		if rows[i].Error == nil {
			order = append(order, i)
		}
	}
	for i := range rows {
		visit(i)
	}
	return order, nil
}

func checkImportRow(row *importRow, existing map[string]store.User, groups []store.Group) error {
	for _, field := range []*string{&row.Email, &row.Name, &row.Title, &row.Department, &row.Location, &row.ManagerEmail} {
		*field = strings.TrimSpace(*field)
	}
	names := []string{}
	for _, name := range row.Groups {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	row.Groups = names
	if _, err := checkEmail(row.Email); err != nil {
		return err
	}
	if u, ok := existing[strings.ToLower(row.Email)]; ok {
		return conflict("Someone with the email " + u.Email + " is already in Halo. Remove this row, or change their profile instead.")
	}
	if row.Name == "" {
		return httpx.Invalid(nameRequired)
	}
	if tooLong(200, row.Name, row.Title, row.Department, row.Location) {
		return httpx.Invalid(profileTooLong)
	}
	for _, name := range row.Groups {
		i := slices.IndexFunc(groups, func(g store.Group) bool { return g.ID == name || strings.EqualFold(g.Name, name) })
		if i < 0 {
			return httpx.Invalid(fmt.Sprintf("There is no group named %q. Create it first or correct the name. Separate several groups with semicolons.", name))
		}
		if groups[i].Kind == "dynamic" {
			return httpx.Invalid(groups[i].Name + " is a dynamic group, so people join it through its rule. Remove it from this row.")
		}
		if !slices.Contains(row.groupIDs, groups[i].ID) {
			row.groupIDs = append(row.groupIDs, groups[i].ID)
		}
	}
	return nil
}

func (h *handlers) applyImport(r *http.Request, actor store.User, rows []importRow, order []int, send bool) error {
	invitations := "holding the setup links"
	if send {
		invitations = "emailing the setup links"
	}
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		for _, i := range order {
			row := &rows[i]
			if row.manager >= 0 {
				row.managerID = &rows[row.manager].UserID
			}
			user, token, expires, err := h.invite(r, tx, actor, store.NewUser{Email: row.Email, Name: row.Name, Title: row.Title, Department: row.Department, Location: row.Location, ManagerID: row.managerID, GroupIDs: row.groupIDs}, "")
			if err != nil {
				return err
			}
			row.UserID, row.token, row.EnrollURL, row.ExpiresAt = user.ID, token, h.cfg.Issuer()+store.EnrollPath(token), &expires
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.import", Summary: fmt.Sprintf("Imported %d of %s, %s", len(order), plural(len(rows), "row"), invitations), TargetType: "user", TargetLabel: "People import"})
	})
	if err != nil || !send {
		return err
	}
	detached := r.WithContext(context.WithoutCancel(r.Context()))
	for _, i := range order {
		row := &rows[i]
		if row.Emailed, err = h.emailLink(detached, row.token, halomail.Invite(row.Email, row.Name, actor.Name, row.EnrollURL, *row.ExpiresAt)); err != nil {
			return err
		}
	}
	return nil
}
