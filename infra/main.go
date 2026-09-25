package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/lukas-andrade/go-webservice/infra/echoservice"
	"github.com/lukas-andrade/go-webservice/infra/observability"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		var otlpEndpoint string
		if cfg.GetBool("observability") {
			obs, err := observability.New(ctx, "observability", observability.Args{ConfigDir: "../observability"})
			if err != nil {
				return err
			}
			otlpEndpoint = obs.OTLPEndpoint
		}

		svc, err := echoservice.New(ctx, "echo-service", echoservice.Args{
			Image:        cfg.Require("image"),
			Replicas:     cfg.GetInt("replicas"),
			OTLPEndpoint: otlpEndpoint,
		})
		if err != nil {
			return err
		}

		ctx.Export("deployment", svc.DeploymentName)
		ctx.Export("service", svc.ServiceName)
		return nil
	})
}
