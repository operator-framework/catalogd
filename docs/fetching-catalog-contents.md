# Fetching `Catalog` contents from the Catalogd HTTP Server
This document covers how to fetch the contents for a `Catalog` from the
Catalogd HTTP Server that runs when the `HTTPServer` feature-gate is enabled
(enabled by default).

For example purposes we make the following assumption:
- A `Catalog` named `operatorhubio` has been created and successfully unpacked
(denoted in the `Catalog.Status`)

`Catalog` CRs have a status.contentURL field whose value is the location where the content 
of a catalog can be read from:

```yaml
  status:
    conditions:
    - lastTransitionTime: "2023-09-14T15:21:18Z"
      message: successfully unpacked the catalog image "quay.io/operatorhubio/catalog@sha256:e53267559addc85227c2a7901ca54b980bc900276fc24d3f4db0549cb38ecf76"
      reason: UnpackSuccessful
      status: "True"
      type: Unpacked
    contentURL: http://catalogd-catalogserver.catalogd-system.svc/catalogs/operatorhubio/all.json
    phase: Unpacked
    resolvedSource:
      image:
        ref: quay.io/operatorhubio/catalog@sha256:e53267559addc85227c2a7901ca54b980bc900276fc24d3f4db0549cb38ecf76
      type: image
```

All responses will be a JSON stream where each JSON object is a File-Based Catalog (FBC)
object.


## On cluster

When making a request for the contents of the `operatorhubio` `Catalog` from within
the cluster issue a HTTP `GET` request to 
`http://catalogd-catalogserver.catalogd-system.svc/catalogs/operatorhubio/all.json`

An example command to run a `Pod` to `curl` the catalog contents:
```sh
kubectl run fetcher --image=curlimages/curl:latest -- curl http://catalogd-catalogserver.catalogd-system.svc/catalogs/operatorhubio/all.json
```

## Off cluster

When making a request for the contents of the `operatorhubio` `Catalog` from outside
the cluster, we have to perform an extra step:
1. Port forward the `catalogd-catalogserver` service in the `catalogd-system` namespace:
```sh
kubectl -n catalogd-system port-forward svc/catalogd-catalogserver 8080:80
```

Once the service has been successfully forwarded to a localhost port, issue a HTTP `GET`
request to `http://localhost:8080/catalogs/operatorhubio/all.json`

An example `curl` request that assumes the port-forwarding is mapped to port 8080 on the local machine:
```sh
curl http://localhost:8080/catalogs/operatorhubio/all.json
```

# Fetching `Catalog` contents from the `Catalogd` Service outside of the cluster

This section outlines a way of exposing the `Catalogd` Service's endpoints outside the cluster and then accessing the catalog contents using `Ingress`. 

**Prerequisites**
- [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- Install latest version of `Catalogd` by navigating to the [releases page](https://github.com/operator-framework/catalogd/releases) and following the install instructions included in the release you want to install.

1. Create a `kind` cluster with `extraPortMappings` and `node-labels` by running the below command:

    ```sh
      cat <<EOF | kind create cluster --config=-
      kind: Cluster
      apiVersion: kind.x-k8s.io/v1alpha4
      nodes:
      - role: control-plane
        kubeadmConfigPatches:
        - |
          kind: InitConfiguration
          nodeRegistration:
            kubeletExtraArgs:
              node-labels: "ingress-ready=true"
      extraPortMappings:
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      - containerPort: 443
        hostPort: 443
        protocol: TCP
      EOF
    ```

    - `extraPortMappings` allows the localhost to make requests to the Ingress controller over ports `80/443`. Setting this config option when creating a cluster allows to forward ports from the host to an ingress controller running on a node.
    - `node-labels` only allows the ingress controller to run on a specific node(s) matching the label selector.

1. Create a `Catalog` object that points to the OperatorHub Community catalog by running the following command:

    ```sh
      $ kubectl apply -f - << EOF
      apiVersion: catalogd.operatorframework.io/v1alpha1
      kind: Catalog
        metadata:
          name: operatorhubio
        spec:
          source:
            type: image
            image:
              ref: quay.io/operatorhubio/catalog:latest
        EOF
    ```

1. Before proceeding further let's verify that the `Catalog` object was created successfully by running the below command: 

    ```sh
      $ kubectl describe catalog/operatorhubio
    ```

1. Now we can proceed to install the `Ingress` controller. We will be using `Ingress NGINX` for the controller. Run the following command to install the `Ingress` controller:

    ```sh
      $ kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
    ```

1. Wait until `Ingress` is ready by running the below `kubectl wait` command: 
  
    ```sh
      $ kubectl wait --namespace ingress-nginx \
      --for=condition=ready pod \
      --selector=app.kubernetes.io/component=controller \
      --timeout=90s
    ```

1. At this point the `Ingress` controller is ready to process requests. Let's create an `Ingress` object by running the below command:

    ```sh
      $ kubectl apply -f https://github.com/operator-framework/catalogd/tree/main/manifests/overlays/nginx-ingress/resources/nginx_ingress.yaml

        Sample `Ingress` Resource:
        ```yaml
        apiVersion: networking.k8s.io/v1
        kind: Ingress
        metadata:
          name: catalogd-nginx-ingress
          namespace: catalogd-system
          annotations:
            nginx.org/proxy-connect-timeout: "30s"
            nginx.org/proxy-read-timeout: "20s"
            nginx.org/client-max-body-size: "4m"
        spec:
          ingressClassName: nginx
          rules:
          - http:
              paths:
              - path: /
                pathType: Prefix
                backend:
                  service:
                    name: catalogd-catalogserver
                    port:
                      number: 80
    ```


1. Once the `Ingress` object has been successfully created, issue a `curl` request. Below is an example `curl` request to retrieve all of the catalog contents:

    ```sh
      $ curl http://localhost/catalogs/operatorhubio/all.json
    ```
    
    You can further use the `curl` commands outlined in the [Catalogd README](https://github.com/operator-framework/catalogd/blob/main/README.md) to filter out the JSON content by list of bundles, channels & packages.
