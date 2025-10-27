# HLB Examples

This folder contains manifests to test the implementation.

- hostedloadbalancer.yaml: deploys a hosted load balancer to the PROD AWS partition
- hostedloadbalancer-tp.yaml: deploys a hosted load balancer to the DEV AWS partition
- listener.yaml: deploys a listener
- listener-v2.yaml: deploys a listener and tests whether it is possible to deploy a listener on a port that is served by another listener.
