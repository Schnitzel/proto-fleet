// Package main implements the Mujina plugin for the Fleet mining system.
//
// Mujina (https://github.com/256foundation/mujina) is open-source Bitcoin
// mining firmware from the 256 Foundation.  It exposes an unauthenticated
// REST API on port 7785 (ASCII "MU") for monitoring and control.
//
// The plugin is loaded by the Fleet server at runtime and communicates with
// it via the Fleet plugin SDK over a local gRPC connection.
package main

import (
	"log"

	"github.com/block/proto-fleet/plugin/mujina/internal/driver"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
	"github.com/hashicorp/go-plugin"
)

func main() {
	d := driver.New()

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: sdk.HandshakeConfig,
		Plugins: map[string]plugin.Plugin{
			"driver": &sdk.DriverPlugin{Impl: d},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})

	log.Println("Mujina plugin exiting")
}
