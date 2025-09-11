/*
Copyright 2021 Upbound Inc.
*/

package config

import (
	// Note(turkenh): we are importing this to embed provider schema document
	_ "embed"

	ujconfig "github.com/crossplane/upjet/pkg/config"

	"github.com/footprint-it-solutions/provider-zonehero/config/hlb_listener_attachment"
	"github.com/footprint-it-solutions/provider-zonehero/config/hlb_load_balancer"
)

const (
	resourcePrefix = "zonehero"
	modulePath     = "github.com/footprint-it-solutions/provider-zonehero"
)

//go:embed schema.json
var providerSchema string

//go:embed provider-metadata.yaml
var providerMetadata string

// GetProvider returns provider configuration
func GetProvider() *ujconfig.Provider {
	pc := ujconfig.NewProvider([]byte(providerSchema), resourcePrefix, modulePath, []byte(providerMetadata),
		ujconfig.WithRootGroup("footprintit.net"),
		ujconfig.WithIncludeList(ExternalNameConfigured()),
		ujconfig.WithFeaturesPackage("internal/features"),
		ujconfig.WithDefaultResourceOptions(
			ExternalNameConfigurations(),
		))

	for _, configure := range []func(provider *ujconfig.Provider){
		// add custom config functions
		hlb_listener_attachment.Configure,
		hlb_load_balancer.Configure,
	} {
		configure(pc)
	}

	pc.ConfigureResources()
	return pc
}
