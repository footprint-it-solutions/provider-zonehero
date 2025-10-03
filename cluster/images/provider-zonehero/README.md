# Provider ZoneHero Dockerfile

The default `Dockerfile` targets resources that have to be present in both https://releases.hashicorp.com and the Terraform Registry. If using a custom (not official) provider from Terraform Registry it is very likely that the custom provider will not be in https://releases.hashicorp.com and `docker build` will fail. For this reason this foler has `Dockerfile.local` that serves as an example on how to overcome issues with `docker build` because of the custom provider.
