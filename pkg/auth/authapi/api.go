// Package authapi is the login settings API, the same in every Beacon
// app. Mount it on the app's protected routes; the login, logout and
// status handlers on auth.Auth go on the open ones.
package authapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/pkg/auth"
)

// API types have their own names: huma names schemas after Go types, and
// plain names would clash with types in the apps that mount this.

type authSettingsBody struct {
	Required      string   `json:"required"       enum:"none,disabled_for_local,enabled" doc:"Who must log in: none (local network only, no login), disabled_for_local (login from outside the local network), enabled (login for everyone)"`
	Username      string   `json:"username"       doc:"Login username; empty when no login is set up"`
	PasswordSet   bool     `json:"password_set"   doc:"Whether a password is stored"`
	LocalNetworks []string `json:"local_networks" doc:"Extra address ranges (CIDR) that count as the local network"`
	ClientAddress string   `json:"client_address" doc:"The address this request appears to come from"`
	ClientIsLocal bool     `json:"client_is_local" doc:"Whether that address counts as the local network"`
}

type authSettingsOutput struct {
	Body *authSettingsBody
}

type authSettingsInput struct {
	Body struct {
		Required        string   `json:"required"                 enum:"none,disabled_for_local,enabled" doc:"Who must log in"`
		Username        string   `json:"username,omitempty"       doc:"Login username. Empty removes the login (only with required=none)."`
		Password        string   `json:"password,omitempty"       doc:"New password. Omit to keep the current one."`
		CurrentPassword string   `json:"current_password,omitempty" doc:"The current password. Needed to change these settings once a login is set, unless the request uses the API key."`
		LocalNetworks   []string `json:"local_networks,omitempty" doc:"Extra address ranges (CIDR) that count as the local network"`
	}
}

func body(ctx context.Context, a *auth.Auth) *authSettingsBody {
	s := a.Settings()
	addr, local := a.CallerAddress(ctx)
	nets := s.LocalNetworks
	if nets == nil {
		nets = []string{}
	}
	return &authSettingsBody{
		Required:      s.Required,
		Username:      s.Username,
		PasswordSet:   s.PasswordHash != "",
		LocalNetworks: nets,
		ClientAddress: addr,
		ClientIsLocal: local,
	}
}

// Register adds GET and PUT /api/v1/system/auth to api.
func Register(api huma.API, a *auth.Auth) {
	huma.Register(api, huma.Operation{
		OperationID: "get-auth-settings",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/auth",
		Summary:     "Get login settings",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*authSettingsOutput, error) {
		return &authSettingsOutput{Body: body(ctx, a)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-auth-settings",
		Method:      http.MethodPut,
		Path:        "/api/v1/system/auth",
		Summary:     "Update login settings",
		Tags:        []string{"System"},
	}, func(ctx context.Context, in *authSettingsInput) (*authSettingsOutput, error) {
		if err := a.ConfirmPassword(ctx, in.Body.CurrentPassword); err != nil {
			var tooMany auth.ErrTooManyAttempts
			switch {
			case errors.As(err, &tooMany):
				return nil, huma.NewError(http.StatusTooManyRequests, err.Error())
			case in.Body.CurrentPassword == "":
				return nil, huma.NewError(http.StatusForbidden, "Enter the current password to change these settings.")
			default:
				return nil, huma.NewError(http.StatusForbidden, "The current password is wrong.")
			}
		}

		cur := a.Settings()
		next := auth.Settings{
			Required:      in.Body.Required,
			Username:      strings.TrimSpace(in.Body.Username),
			PasswordHash:  cur.PasswordHash,
			LocalNetworks: in.Body.LocalNetworks,
		}
		if next.Required == "" {
			next.Required = auth.RequiredNone
		}
		if _, err := auth.ParseNetworks(next.LocalNetworks); err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "Local networks: "+err.Error()+".")
		}

		switch {
		case next.Username == "":
			if in.Body.Password != "" {
				return nil, huma.NewError(http.StatusBadRequest, "Enter a username to go with the password.")
			}
			next.PasswordHash = ""
		case in.Body.Password != "":
			if len(in.Body.Password) < 8 {
				return nil, huma.NewError(http.StatusBadRequest, "Password must be at least 8 characters.")
			}
			hash, err := auth.HashPassword(in.Body.Password)
			if err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "Could not hash password.", err)
			}
			next.PasswordHash = hash
		case next.PasswordHash == "":
			return nil, huma.NewError(http.StatusBadRequest, "Enter a password.")
		}
		if next.Required != auth.RequiredNone && !next.CredentialsConfigured() {
			return nil, huma.NewError(http.StatusBadRequest, "Set a username and password before requiring a login.")
		}
		if a.WouldLockOut(ctx, next) {
			return nil, huma.NewError(http.StatusBadRequest,
				"That would lock you out: your own address would stop counting as the local network. Keep its range in the list, or set a login first.")
		}

		if err := a.SetSettings(next); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "Could not save the login settings.", err)
		}
		return &authSettingsOutput{Body: body(ctx, a)}, nil
	})
}
