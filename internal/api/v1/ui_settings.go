package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
)

// Add-indexer page defaults: the filters the page starts with. Stored on the
// server so they apply in every browser.
const (
	uiNamespace           = "pulse.ui"
	addIndexerDefaultsKey = "add_indexer_defaults"
)

// AddIndexerDefaults are the add-indexer page's starting filters. Empty
// lists mean "any".
type AddIndexerDefaults struct {
	Protocols  []string `json:"protocols" required:"false" doc:"torrent, usenet"`
	Privacies  []string `json:"privacies" required:"false" doc:"public, semi-private, private"`
	Categories []string `json:"categories" required:"false" doc:"Movies, TV, Audio, Books, XXX, Other"`
	Languages  []string `json:"languages" required:"false" doc:"Catalog language codes, e.g. en-US"`
}

var (
	allowedProtocols  = []string{"torrent", "usenet"}
	allowedPrivacies  = []string{"public", "semi-private", "private"}
	allowedCategories = []string{"Movies", "TV", "Audio", "Books", "XXX", "Other"}
)

func (d *AddIndexerDefaults) normalize() {
	for _, l := range []*[]string{&d.Protocols, &d.Privacies, &d.Categories, &d.Languages} {
		if *l == nil {
			*l = []string{}
		}
	}
}

func (d AddIndexerDefaults) validate() error {
	check := func(field string, values, allowed []string) error {
		for _, v := range values {
			ok := false
			for _, a := range allowed {
				ok = ok || v == a
			}
			if !ok {
				return fmt.Errorf("%s: unknown value %q", field, v)
			}
		}
		return nil
	}
	if err := check("protocols", d.Protocols, allowedProtocols); err != nil {
		return err
	}
	if err := check("privacies", d.Privacies, allowedPrivacies); err != nil {
		return err
	}
	if err := check("categories", d.Categories, allowedCategories); err != nil {
		return err
	}
	if len(d.Languages) > 50 {
		return errors.New("languages: too many")
	}
	for _, l := range d.Languages {
		if l == "" || len(l) > 16 {
			return fmt.Errorf("languages: bad code %q", l)
		}
	}
	return nil
}

type addIndexerDefaultsBody struct {
	Body AddIndexerDefaults
}

// RegisterUISettingsRoutes exposes Pulse's own UI preferences.
func RegisterUISettingsRoutes(api huma.API, store *cfgstore.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "get-add-indexer-defaults",
		Method:      http.MethodGet,
		Path:        "/api/v1/settings/add-indexer-defaults",
		Summary:     "Filters the add-indexer page starts with",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, _ *struct{}) (*addIndexerDefaultsBody, error) {
		out := &addIndexerDefaultsBody{}
		entry, err := store.Get(ctx, uiNamespace, addIndexerDefaultsKey)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// never set: no defaults
		case err != nil:
			return nil, huma.Error500InternalServerError("loading defaults", err)
		default:
			if err := json.Unmarshal([]byte(entry.Value), &out.Body); err != nil {
				out.Body = AddIndexerDefaults{} // unreadable: fall back to none
			}
		}
		out.Body.normalize()
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-add-indexer-defaults",
		Method:      http.MethodPut,
		Path:        "/api/v1/settings/add-indexer-defaults",
		Summary:     "Set the filters the add-indexer page starts with",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, in *addIndexerDefaultsBody) (*addIndexerDefaultsBody, error) {
		d := in.Body
		d.normalize()
		if err := d.validate(); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		raw, _ := json.Marshal(d)
		if _, err := store.Set(ctx, uiNamespace, addIndexerDefaultsKey, string(raw)); err != nil {
			return nil, huma.Error500InternalServerError("saving defaults", err)
		}
		return &addIndexerDefaultsBody{Body: d}, nil
	})
}
