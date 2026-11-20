package auth_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"halo/internal/httpx"
	"halo/internal/store"
)

func TestLauncher(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	rule := `user.department == "Engineering"`
	eng, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	must(err)
	finance, err := e.st.CreateGroup(ctx, store.NewGroup{Name: "Finance", Kind: "assigned"})
	must(err)
	ada, err := e.st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Department: "Engineering", Status: "active"})
	must(err)
	app := func(name, protocol string, groupID string) store.Application {
		a, err := e.st.CreateApplication(ctx, store.NewApplication{Name: name, Protocol: protocol, Type: "web", Homepage: "https://" + name + ".example.com", RedirectURIs: []string{"https://" + name + ".example.com/cb"}, GroupIDs: []string{groupID}})
		must(err)
		return a
	}
	wiki, grafana, ledger, old := app("wiki", "bookmark", eng.ID), app("grafana", "oidc", eng.ID), app("ledger", "bookmark", finance.ID), app("old", "bookmark", eng.ID)
	must(e.st.SetApplicationStatus(ctx, old.ID, "disabled"))
	engineering, err := e.st.SaveCollection(ctx, "", "Engineering", []string{wiki.ID, grafana.ID, ledger.ID, old.ID}, []string{eng.ID})
	must(err)
	_, err = e.st.SaveCollection(ctx, "", "Finance", []string{wiki.ID, ledger.ID}, []string{finance.ID})
	must(err)
	_, err = e.st.SaveCollection(ctx, "", "Ledgers", []string{ledger.ID}, []string{eng.ID})
	must(err)

	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: ada.ID, Method: "passkey"})
	must(err)
	c := client()
	base, _ := url.Parse(e.srv.URL)
	c.Jar.SetCookies(base, []*http.Cookie{{Name: httpx.SessionCookie, Value: token}})

	type mine struct {
		ID        string `json:"id"`
		LaunchURL string `json:"launchUrl"`
		Favourite bool   `json:"favourite"`
	}
	var apps []mine
	if status := e.do(c, "GET", "/api/v1/me/applications", nil, &apps); status != http.StatusOK || len(apps) != 2 || apps[0].ID != grafana.ID || apps[1].ID != wiki.ID || apps[1].LaunchURL != "https://wiki.example.com" {
		t.Fatalf("applications: %d %+v", status, apps)
	}

	e.expect(c, "PUT", "/api/v1/me/applications", map[string]any{"order": []string{ledger.ID}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	e.expect(c, "PUT", "/api/v1/me/applications", map[string]any{"favourites": []string{old.ID}}, http.StatusUnprocessableEntity, "ERR_INVALID")
	if status := e.do(c, "PUT", "/api/v1/me/applications", map[string]any{"order": []string{wiki.ID, grafana.ID}, "favourites": []string{grafana.ID}}, &apps); status != http.StatusOK ||
		len(apps) != 2 || apps[0] != (mine{wiki.ID, "https://wiki.example.com", false}) || apps[1] != (mine{grafana.ID, "https://grafana.example.com", true}) {
		t.Fatalf("arranged: %d %+v", status, apps)
	}
	if status := e.do(c, "PUT", "/api/v1/me/applications", map[string]any{"favourites": []string{wiki.ID}}, &apps); status != http.StatusOK || len(apps) != 2 || apps[0].ID != grafana.ID || !apps[1].Favourite || apps[0].Favourite {
		t.Fatalf("favourites without an order: %d %+v", status, apps)
	}
	must(e.st.SetAppPreferences(ctx, ada.ID, []string{wiki.ID, grafana.ID}, nil))

	var collections []struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		ApplicationIDs []string `json:"applicationIds"`
	}
	if status := e.do(c, "GET", "/api/v1/me/collections", nil, &collections); status != http.StatusOK || len(collections) != 1 || collections[0].ID != engineering ||
		len(collections[0].ApplicationIDs) != 2 || collections[0].ApplicationIDs[0] != wiki.ID || collections[0].ApplicationIDs[1] != grafana.ID {
		t.Fatalf("collections: %d %+v", status, collections)
	}
	e.expect(client(), "GET", "/api/v1/me/collections", nil, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
}
