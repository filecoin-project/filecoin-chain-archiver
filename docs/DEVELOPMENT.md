# DEVELOPMENT

## Summary

This document describes the development environment and how to set it up. This is a feature rich environment that
targets k8s.

Development can also be done using docker / docker-compose and tmux which is handy for doing quick work before
running on k8s somewhere, but is not covered here.

[Deploy MinIO on Docker Compose](https://docs.min.io/docs/deploy-minio-on-docker-compose.html)

This document is not step by step instructions, but provides the large parts. At the end you will have:

- kind k8s local cluster
- minio bucket
- lotus daemon
  - with shared jwt token that can be used by other k8s resources

MinIO API
```
http://minio.minio.svc.cluster.local
```

Lotus API Multiaddrs
```
/dns/lotus-a-lotus-daemon.ntwk-butterflynet-filsnap.svc.cluster.local/tcp/1234
/dns/lotus-b-lotus-daemon.ntwk-butterflynet-filsnap.svc.cluster.local/tcp/1234
/dns/lotus-c-lotus-daemon.ntwk-butterflynet-filsnap.svc.cluster.local/tcp/1234
```

## Requirements

The following tools are expected to be installed. This document does not cover their installation, but will cover their
usage as required to setup the development environment.

- [docker](https://docs.docker.com/get-docker/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- kubectl [krew](https://krew.sigs.k8s.io/)
- [helm](https://helm.sh/docs/intro/quickstart/)
- lotus-shed


Install the minio plugin:

```shell
kubectl krew update
kubectl krew install minio
```

Add required helm chart repositories:

```shell
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add filecoin https://filecoin-project.github.io/helm-charts
helm repo update
```

### Creating a Kind Cluster

[More Information: Cluster Configuration](https://kind.sigs.k8s.io/docs/user/quick-start/#configuring-your-kind-cluster)

Create a cluster with three worker nodes.

```yaml
# cluster.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
```
```shell
kind create cluster --config cluster.yaml
```

#### Moving Container Images

[More Information: Loading Images](https://kind.sigs.k8s.io/docs/user/quick-start/#loading-an-image-into-your-cluster)

When building the docker container, you will need to move it into the cluster. Kind provides an easy way to do this.

```shell
kind load docker-image filsnap:latest
```

### Install Monitoring Stack

[More Information: Helm Chart](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)

[More Information: Operator](https://github.com/prometheus-operator/prometheus-operator)


Install the prometheus monitoring stack with grafana enabled and persistence storage. This configuration enables cluster
wide monitoring with no selectors, and persistence of data.

```yaml
# values-prom-stack.yaml
prometheus:
  prometheusSpec:
    ruleSelector: {}
    ruleNamespaceSelector: {}
    ruleSelectorNilUsesHelmValues: false
    serviceMonitorSelector: {}
    serviceMonitorNamespaceSelector: {}
    serviceMonitorSelectorNilUsesHelmValues: false
    podMonitorSelector: {}
    podMonitorNamespaceSelector: {}
    podMonitorSelectorNilUsesHelmValues: false

    storageSpec:
      volumeClaimTemplate:
        spec:
          storageClassName: standard
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 10Gi

grafana:
  enabled: true
  persistence:
    enabled: true
    size: 10Gi
EOF
```
```shell
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --values values-prom-stack.yaml
```

### Installing MinIO Operator & Creating Tenant

[More Information: Operator](https://github.com/minio/operator)

```shell
kubectl minio init
```

#### Creating a Tenant

[More Information: Tenant](https://docs.min.io/minio/k8s/tenant-management/deploy-minio-tenant.html)

```shell
kubectl create namespace minio
kubectl minio proxy -n minio-operator
```

Follow the instruction provided by `minio proxy` and login to the operator console then click the `New Tenant`.

Fill out the form.

| Page     | Field              | Value    |
|----------|--------------------|----------|
| Setup    | Name               | minio    |
| Setup    | Namespace          | minio    |
| Setup    | Storage Class      | standard |
| Setup    | Number of Servers  | 3        |
| Setup    | Drivers per Server | 2        |
| Setup    | Total Size         | 300      |
| Setup    | CPU Request        | 2        |
| Setup    | Memory Request     | 4        |
| Security | Enabled TLS        | OFF      |

Copy down the Console Credentials, you will also use these for api access to the bucket.

[More Information: User Management](https://docs.min.io/minio/k8s/tutorials/user-management.html).

#### Accessining MinIO Console & Creating Bucket

```shell
kubectl port-forward service/minio-console 9090:9090 -n minio
```

Open the console http://localhost:9090 and login using the Console Credentials.

Create a bucket called `filsnap` with all options disabled.

### Creating Lotus Nodes

Create a namespace for the lotus daemons

```shell
kubectl create namespace ntwk-butterflynet-filsnap
```

Note: This is the same namespace you will develop in, not technically required, but it's easier
to share the same secret resource, than to copy and manage in two different places. This requirement
could be removed, but due to requiring the ability to shutdown daemons (due to a bug) the admin privilage
is required, otherwise all operations are `read-only` and wouldn't require a token at all.

#### Creating a shared jwt token

```shell
mkdir /tmp/secrets
lotus-shed jwt new node
lotus-shed base16 -decode < jwt-node.jwts > /tmp/secrets/auth-jwt-private
cp jwt-node.token /tmp/secrets/jwt-all-privs-token

lotus-shed jwt token --read         --output /tmp/secrets/jwt-ro-privs-token  jwt-node.jwts
lotus-shed jwt token --read --write --output /tmp/secrets/jwt-rw-privs-token  jwt-node.jwts
lotus-shed jwt token --sign         --output /tmp/secrets/jwt-so-privs-token  jwt-node.jwts

kubectl create secret generic lotus-jwt                           \
--from-file=auth-jwt-private=/tmp/secrets/auth-jwt-private        \
--from-file=jwt-all-privs-token=/tmp/secrets/jwt-all-privs-token  \
--from-file=jwt-ro-privs-token=/tmp/secrets/jwt-ro-privs-token    \
--from-file=jwt-rw-privs-token=/tmp/secrets/jwt-rw-privs-token    \
--from-file=jwt-so-privs-token=/tmp/secrets/jwt-so-privs-token    \
--output=name --namespace ntwk-butterflynet-filsnap

rm -rf /tmp/secrets
rm jwt-node.jwts jwt-node.token
```

#### Install Butterfly Lotus Daemons

[Docker Images](https://hub.docker.com/r/travisperson/lotus/tags?page=1&name=butterfly)

```yaml
# values-lotus.yaml
image:
  tag: butterflynet-<version>

prometheusOperatorServiceMonitor: true

secrets:
  jwt:
    enabled: true
    secretName: lotus-jwt

daemonEnvs:
  - name: GOLOG_LOG_FMT
    value: json

daemonConfig: |
  [API]
    ListenAddress = "/ip4/0.0.0.0/tcp/1234/http"
  [Libp2p]
    ListenAddresses = ["/ip4/0.0.0.0/tcp/1347"]

additionalLabels:
  network: butterflynet

persistence:
  datastore:
    enabled: true
    storageClassName: "standard"
    size: "15Gi"
  journal:
    enabled: true
    storageClassName: "standard"
    size: "1Gi"
  parameters:
    enabled: true
    storageClassName: "standard"
    size: "1Gi"
EOF
```
```shell
helm install lotus-a filecoin/lotus-fullnode --values values-lotus.yaml --namespace ntwk-butterflynet-filsnap
helm install lotus-b filecoin/lotus-fullnode --values values-lotus.yaml --namespace ntwk-butterflynet-filsnap
helm install lotus-c filecoin/lotus-fullnode --values values-lotus.yaml --namespace ntwk-butterflynet-filsnap
```
