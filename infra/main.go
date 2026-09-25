package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/lukas-andrade/go-webservice/infra/echoservice"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		svc, err := echoservice.New(ctx, "echo-service", echoservice.Args{
			Image:    cfg.Require("image"),
			Replicas: cfg.GetInt("replicas"),
		})
		if err != nil {
			return err
		}

		ctx.Export("deployment", svc.DeploymentName)
		ctx.Export("service", svc.ServiceName)
		return nil
	})
}
