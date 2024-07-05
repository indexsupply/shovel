# shovel

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.6](https://img.shields.io/badge/AppVersion-1.6-informational?style=flat-square)

Helm chart for Shovel

## Maintainers
| Name | Email | Url |
| ---- | ------ | --- |
| indexsupply | <info@indexsupply.com> | <https://indexsupply.com> |

## Source Code

* <https://github.com/indexsupply/code>
## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.bitnami.com/bitnami | postgresql | 15.4.0 |

## Quick Start

```bash
helm install shovel shovel
```

## PostgreSQL

By default the chart will install a PostgreSQL database using the bitnami PostgreSQL chart listening on port `5432` with service type `ClusterIP`
and username and password `shovel` (see below Values section for more detail).

If you want to use your own existing database instead (e.g. managed postgresql or [cloudnative-pg](https://cloudnative-pg.io)),
specify the url to the database in the values.yaml:

```yaml
shovel:
  pgUrl: "postgres:///my-postgresql"
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| fullnameOverride | string | `""` | Overrides the chart's computed fullname |
| global.affinity | object | `{}` | Set affinity |
| global.imagePullSecrets | list | `[]` | A list of pull secrets is used when credentials are needed to access a container registry with username and password. |
| global.nodeSelector | object | `{}` |  |
| global.podMonitor.create | bool | `false` | Create a [PodMonitor](https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/user-guides/getting-started.md) |
| global.postgresql.auth.database | string | `"shovel"` |  |
| global.postgresql.auth.password | string | `"shovel"` |  |
| global.postgresql.auth.username | string | `"shovel"` |  |
| global.postgresql.enabled | bool | `true` |  |
| global.securityContext | object | `{"fsGroup":1000,"runAsGroup":1000,"runAsNonRoot":true,"runAsUser":1000}` | Pod security context |
| global.service.annotations | object | `{}` | Service annotations |
| global.service.enabled | bool | `true` | Enable Service |
| global.service.type | string | `"ClusterIP"` | Service type, ClusterIP, LoadBalancer or ClusterIP. |
| global.serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| global.serviceAccount.create | bool | `true` | Enable service account (Note: Service Account will only be automatically created if `global.serviceAccount.name` is not set) |
| global.serviceAccount.name | string | `""` | Name of an already existing service account. Setting this value disables the automatic service account creation |
| global.tolerations | list | `[]` |  |
| nameOverride | string | `""` | Overrides the chart's name |
| postgresql | object | `{"primary":{"persistence":{"size":"8Gi","storageClass":""},"service":{"loadBalancerClass":"","ports":{"postgresql":5432},"type":"ClusterIP"}}}` | PostgreSQL parameters, only used if global.postgresql.enabled is set to true. See https://github.com/bitnami/charts/blob/main/bitnami/postgresql/values.yaml for full list of available option. |
| postgresql.primary.persistence | object | `{"size":"8Gi","storageClass":""}` | PostgreSQL Primary persistence configuration |
| postgresql.primary.persistence.size | string | `"8Gi"` | PostgreSQL Primary PVC Storage Request for data volume. |
| postgresql.primary.persistence.storageClass | string | `""` | PostgreSQL Primary PVC Storage Class for data volume. If undefined (the default) or set to null, no storageClassName spec is set, choosing the default provisioner. |
| postgresql.primary.service | object | `{"loadBalancerClass":"","ports":{"postgresql":5432},"type":"ClusterIP"}` | PostgreSQL Primary service configuration |
| postgresql.primary.service.loadBalancerClass | string | `""` | PostgreSQL Primary Load balancer class if service type is `LoadBalancer` |
| postgresql.primary.service.ports | object | `{"postgresql":5432}` | PostgreSQL Primary service port |
| postgresql.primary.service.type | string | `"ClusterIP"` | PostgreSQL Primary service type |
| shovel.config | object | `{"eth_sources":[{"chain_id":1,"name":"mainnet","url":"https://eth.merkle.io"}],"integrations":[{"block":[{"column":"log_addr","filter_arg":["a0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"],"filter_op":"contains","name":"log_addr"}],"dashboard":{"disable_authn":true},"enabled":true,"event":{"anonymous":false,"inputs":[{"column":"from","indexed":true,"name":"from","type":"address"},{"column":"to","indexed":true,"name":"to","type":"address"},{"column":"value","name":"value","type":"uint256"}],"name":"Transfer","type":"event"},"name":"usdc-transfer","sources":[{"batch_size":100,"name":"mainnet"}],"table":{"columns":[{"name":"log_addr","type":"bytea"},{"name":"block_time","type":"numeric"},{"name":"from","type":"bytea"},{"name":"to","type":"bytea"},{"name":"value","type":"numeric"}],"name":"usdc"}}],"pg_url":"$PG_URL"}` | Shovel config example (see https://indexsupply.com/shovel/docs/#config for more information) |
| shovel.containerSecurityContext.runAsGroup | int | `1000` |  |
| shovel.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| shovel.containerSecurityContext.runAsUser | int | `1000` |  |
| shovel.image.pullPolicy | string | `"IfNotPresent"` | Container pull policy |
| shovel.image.repository | string | `"nbtim/shovel"` | Container image |
| shovel.image.tag | string | `"v1.6"` | Container image tag |
| shovel.name | string | `"shovel"` | Name of the container |
| shovel.pgUrl | string | `"postgres:///shovel"` | Shovel pg_url, use for setting your own PostgreSQL endpoint, by default it uses the integrated PostgreSQL from the bitnami/postgresql dependency |
| shovel.podAnnotations | object | `{}` | Pod annotation to be added |
| shovel.podLabels | object | `{}` | Pod labels to be added |
| shovel.podMonitor.interval | string | `"30s"` |  |
| shovel.podMonitor.path | string | `"/metrics"` |  |
| shovel.podMonitor.scrapeTimeout | string | `"10s"` |  |
| shovel.ports.http | int | `8546` | Dashboard port |
| shovel.ports.metrics | int | `9090` | Metrics port |
| shovel.resources | object | `{"limits":{"cpu":"1000m","memory":"4Gi"},"requests":{"cpu":"500m","memory":"2Gi"}}` | Resource requests and limits |