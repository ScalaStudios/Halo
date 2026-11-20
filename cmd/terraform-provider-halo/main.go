package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"halo/internal/terraform"
)

var version = "dev"

func main() {
	if err := providerserver.Serve(context.Background(), terraform.New(version), providerserver.ServeOpts{Address: "registry.terraform.io/scalastudios/halo"}); err != nil {
		log.Fatal(err)
	}
}
