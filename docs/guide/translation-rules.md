---
sidebar_position: 4
---

# Translation Rules

This page documents how Kubernetes resources from Helm charts are translated into Docker Compose equivalents. This is the core of what `composed build` does when processing `x-helm` services.

## Workloads to Compose Services

**Deployment**, **StatefulSet**, and **DaemonSet** each produce one compose service per container in `spec.template.spec.containers[]`:

```
K8s Deployment "redis-master"                Compose service "redis-master"
├── .spec.replicas                      →    deploy.replicas
├── .spec.template.spec.containers[0]
│   ├── .image                          →    image
│   ├── .command                        →    entrypoint
│   ├── .args                           →    command
│   ├── .env[]                          →    environment (inline values)
│   ├── .env[].valueFrom.configMapKeyRef →   environment (resolved from ConfigMap)
│   ├── .env[].valueFrom.secretKeyRef   →    environment (resolved from Secret)
│   ├── .envFrom[].configMapRef         →    environment (bulk merge from ConfigMap.data)
│   ├── .envFrom[].secretRef            →    environment (bulk merge from Secret.data)
│   ├── .ports[].containerPort          →    (used by Service translation for mapping)
│   ├── .volumeMounts[]                 →    volumes (cross-ref with PVC or ConfigMap)
│   ├── .resources.limits               →    deploy.resources.limits
│   ├── .resources.requests             →    deploy.resources.reservations
│   ├── .livenessProbe                  →    healthcheck
│   └── .readinessProbe                 →    healthcheck (fallback if no liveness)
└── .spec.template.spec.initContainers[]→    separate service + depends_on chain
```

**Multi-container pods:** each sidecar becomes `<deployment>-<container-name>`.

## K8s Service to Port Mappings

The translator matches a K8s Service's `.spec.selector` to a Deployment's `.spec.template.metadata.labels`.

| K8s Service Type | Compose Mapping |
|------------------|-----------------|
| ClusterIP | Port mappings for Compose inter-service communication |
| NodePort | `nodePort:targetPort` on the matched compose service |
| LoadBalancer | `port:targetPort` on the matched compose service |

In Kubernetes, ClusterIP provides implicit routing via cluster DNS. Docker Compose requires explicit port declarations for inter-service communication, so ports are mapped even for ClusterIP services.

## ConfigMap to Environment or Config

| Usage Pattern | Compose Mapping |
|---------------|-----------------|
| `envFrom: configMapRef` | Merge all `.data` keys into `service.environment` |
| `env[].valueFrom.configMapKeyRef` | Single key into `service.environment` |
| `volumeMount` referencing a ConfigMap | Compose `configs:` top-level + service config mount |
| Unreferenced | Skipped with warning |

## Secret to Environment

Same as ConfigMap, but `.data` values are already decoded (the Kubernetes API types handle base64 encoding). A warning is emitted that secrets will appear as plaintext in the generated compose file.

## PersistentVolumeClaim to Named Volume

```
K8s PVC "redis-data"          Compose volume "redis-data"
├── .metadata.name        →   volume name
└── .spec.resources        →   (informational, compose doesn't enforce)
```

Volume mounts cross-reference: if a container's `volumeMount` references a volume with `persistentVolumeClaim.claimName`, the compose service gets `redis-data:/data/mountPath`.

## Init Containers to depends\_on Chain

Each init container becomes a separate compose service with `restart_policy` set to `on-failure` and a maximum of 3 attempts. The main container gets a `depends_on` entry with `condition: service_completed_successfully`.

```yaml
services:
  redis-master-init-sysctl:
    image: bitnami/os-shell:12
    entrypoint: ["/bin/sh", "-c", "sysctl -w net.core.somaxconn=65535"]
    deploy:
      restart_policy:
        condition: on-failure
        max_attempts: 3
  redis-master:
    depends_on:
      redis-master-init-sysctl:
        condition: service_completed_successfully
```

## Job to One-Shot Service

Jobs become services with `deploy.restart_policy` set to `condition: on-failure` and `max_attempts: 3`. No ports are mapped since jobs are not long-running.

## Probe to Healthcheck

| K8s Probe | Compose Healthcheck |
|-----------|---------------------|
| `exec.command` | `test: ["CMD", ...command]` |
| `httpGet` | `test: ["CMD", "wget", "-q", "--spider", "http://localhost:port/path"]` |
| `tcpSocket` | `test: ["CMD", "sh", "-c", "cat < /dev/tcp/localhost/port"]` |
| `periodSeconds` | `interval` |
| `timeoutSeconds` | `timeout` |
| `failureThreshold` | `retries` |
| `initialDelaySeconds` | `start_period` |

The translator prefers `livenessProbe`. If no liveness probe is defined, `readinessProbe` is used as a fallback.

## Resource Limits

| K8s | Compose |
|-----|---------|
| `limits.memory: 256Mi` | `limits.memory: 256M` |
| `limits.cpu: "500m"` | `limits.cpus: "0.5"` |
| `requests.memory: 128Mi` | `reservations.memory: 128M` |
| `requests.cpu: "250m"` | `reservations.cpus: "0.25"` |

Kubernetes memory units use binary suffixes (`Mi`, `Gi`) while Compose uses SI-style shorthand (`M`, `G`). CPU millicores are converted to fractional CPU counts (e.g., `500m` becomes `0.5`).

## Skipped Resources

The following Kubernetes resource types have no Docker Compose equivalent and are skipped during translation. A warning is emitted for each skipped resource.

- ServiceAccount
- ClusterRole
- ClusterRoleBinding
- Role
- RoleBinding
- NetworkPolicy
- Ingress
- HorizontalPodAutoscaler
- PodDisruptionBudget
- CustomResourceDefinition
- Any unknown CRD instance
