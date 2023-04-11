# Catalogd

Catalogd runs in a Kubernetes cluster and servers content of [FBCs](https://olm.operatorframework.io/docs/reference/file-based-catalogs/) to clients.

## Quickstart

```bash
$ make kind-cluster; make install; kubectl apply -f config/samples/core_v1beta1_catalogsource.yaml
.
.
.

$ kubectl get catalogsource  
NAME                   AGE
catalogsource-sample   98s

$ kubectl get bundlemetadata 
NAME                                               AGE
3scale-community-operator.v0.7.0                   28s
3scale-community-operator.v0.8.2                   28s
3scale-community-operator.v0.9.0                   28s
falcon-operator.v0.5.1                             2s
falcon-operator.v0.5.2                             2s
falcon-operator.v0.5.3                             1s
falcon-operator.v0.5.4                             1s
falcon-operator.v0.5.5                             1s
flux.v0.13.4                                       1s
flux.v0.14.0                                       1s
flux.v0.14.1                                       1s
flux.v0.14.2                                       1s
flux.v0.15.2                                       1s
flux.v0.15.3                                       1s
.
.
.

$ kubectl get packages 
NAME                                        AGE
3scale-community-operator                   77m
ack-apigatewayv2-controller                 77m
ack-applicationautoscaling-controller       77m
ack-dynamodb-controller                     77m
ack-ec2-controller                          77m
ack-ecr-controller                          77m
ack-eks-controller                          77m
ack-elasticache-controller                  77m
ack-emrcontainers-controller                77m
ack-iam-controller                          77m
ack-kms-controller                          77m
ack-lambda-controller                       77m
ack-mq-controller                           77m
ack-opensearchservice-controller            77m
.
.
.
```

## Contributing

Thanks for your interest in contributing to `catalogd`!

`catalogd` is in the very early stages of development and a more in depth contributing guide will come in the near future.

In the mean time, it is assumed you know how to make contributions to open source projects in general and this guide will only focus on how to manually test your changes (no automated testing yet).

If you have any questions, feel free to reach out to us on the Kubernetes Slack channel [#olm-dev](https://kubernetes.slack.com/archives/C0181L6JYQ2) or [create an issue](https://github.com/operator-framework/catalogd/issues/new)

### Testing Local Changes

**Prerequisites**

- [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)

**Local (not on cluster)**

> **Note**: This will work *only* for the controller

- Create a cluster:

```sh
make kind-cluster
```

- Install CRDs and run the controller locally

```sh
kubectl apply -f config/crd/bases/ && make run
```

**On Cluster**

- Create a cluster:

```sh
make kind-cluster
```

- Install catalogd on cluster

```sh
make install
```
