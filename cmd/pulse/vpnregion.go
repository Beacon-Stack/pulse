package main

import (
	"context"

	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
)

// regionPreference keeps the VPN region the user picked in the shared
// config store (namespace "vpn"), so Pulse can put it back after the VPN
// container restarts with its own SERVER_REGIONS.
type regionPreference struct {
	store *cfgstore.Store
}

const (
	vpnConfigNamespace = "vpn"
	vpnRegionKey       = "preferred_region"
)

func (p regionPreference) Get(ctx context.Context) (string, error) {
	e, err := p.store.Get(ctx, vpnConfigNamespace, vpnRegionKey)
	if err != nil {
		return "", nil // none chosen yet
	}
	return e.Value, nil
}

func (p regionPreference) Set(ctx context.Context, region string) error {
	_, err := p.store.Set(ctx, vpnConfigNamespace, vpnRegionKey, region)
	return err
}
