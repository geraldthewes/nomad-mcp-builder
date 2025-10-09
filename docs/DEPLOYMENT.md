# Deployment Guide

This guide covers deploying the Nomad Build Service in production environments.

## Infrastructure Requirements

### Minimum Requirements

- **Nomad Cluster**: 3+ servers, 3+ clients
- **Consul Cluster**: 3+ servers 
- **Vault Cluster**: 3+ servers
- **Docker Registry**: Private registry for intermediate images
- **Load Balancer**: For high availability

### Recommended Specifications

#### Nomad Clients (Build Nodes)
- **CPU**: 8+ cores
- **Memory**: 16GB+ RAM
- **Disk**: 100GB+ SSD for build cache
- **Network**: 1Gbps+

#### Service Nodes
- **CPU**: 4+ cores
- **Memory**: 8GB+ RAM  
- **Disk**: 20GB+ SSD
- **Network**: 1Gbps+

## Pre-Deployment Setup

### 1. Nomad Client Configuration

Configure build-capable Nomad clients:

```hcl
# nomad-client.hcl
datacenter = "cluster"  # Update to match your datacenter
data_dir = "/opt/nomad/data"
log_level = "INFO"
server = false

client {
  enabled = true
  servers = ["nomad-server-1:4647", "nomad-server-2:4647", "nomad-server-3:4647"]
  
  # Build cache volume
  host_volume "buildah-cache" {
    path      = "/opt/nomad/data/buildah-cache"
    read_only = false
  }
  
  # Container runtime volume
  host_volume "containers-storage" {
    path      = "/var/lib/containers"
    read_only = false
  }
  
  node_class = "build-node"
  
  meta {
    "build-capable" = "true"
    "storage-type"  = "ssd"
  }
}

plugin "docker" {
  config {
    allow_privileged = false
    allow_caps = ["SYS_ADMIN", "SETUID", "SETGID"]
    volumes {
      enabled = true
    }
    
    # Garbage collection
    gc {
      image       = true
      image_delay = "3m"
      container   = true
    }
  }
}

# Resource limits
client {
  reserved {
    cpu            = 500
    memory         = 1024
    disk           = 5120
    reserved_ports = "22,80,443"
  }
}
```

### 2. User Namespace Configuration

On each Nomad client node:

```bash
#!/bin/bash
# setup-user-namespaces.sh

# Create build user
sudo useradd -r -s /bin/false -u 10000 build
sudo groupadd -g 10000 build

# Configure user namespaces
echo "build:10000:65536" | sudo tee -a /etc/subuid
echo "build:10000:65536" | sudo tee -a /etc/subgid

# Create directories
sudo mkdir -p /opt/nomad/data/buildah-cache
sudo mkdir -p /var/lib/containers
sudo chown -R build:build /opt/nomad/data/buildah-cache
sudo chown -R build:build /var/lib/containers

# Set up fuse device
sudo chmod 666 /dev/fuse
echo 'KERNEL=="fuse", MODE="0666"' | sudo tee /etc/udev/rules.d/99-fuse.rules
sudo udevadm control --reload-rules

# Install required packages
sudo apt-get update
sudo apt-get install -y fuse-overlayfs buildah podman

echo "User namespace setup completed"
```

### 3. Vault Configuration

#### Policies

Create Vault policies for the service:

```hcl
# jobforge-service-policy.hcl
path "secret/data/nomad/jobs/*" {
  capabilities = ["read"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/revoke-self" {
  capabilities = ["update"]
}
```

Apply the policy:
```bash
vault policy write jobforge-service jobforge-service-policy.hcl
```

#### Secrets Setup

Store service secrets:

```bash
# Git credentials (matches README.md and API examples)
vault kv put secret/nomad/jobs/git-credentials \
  username="your-git-user" \
  password="your-git-token" \
  ssh_key="$(cat ~/.ssh/id_rsa)"

# Registry credentials (matches README.md and API examples)
vault kv put secret/nomad/jobs/registry-credentials \
  username="your-registry-user" \
  password="your-registry-password"

# For public repositories and registries, you can use empty credentials:
# vault kv put secret/nomad/jobs/git-credentials username="" password="" ssh_key=""
# vault kv put secret/nomad/jobs/registry-credentials username="" password=""
```

### 4. Consul Configuration

Set up Consul KV structure:

```bash
#!/bin/bash
# setup-consul-config.sh

CONSUL_ADDR=${CONSUL_HTTP_ADDR:-localhost:8500}
PREFIX="jobforge-service"

# Default configuration
consul kv put ${PREFIX}/config/build_timeout "30m"
consul kv put ${PREFIX}/config/test_timeout "15m"
consul kv put ${PREFIX}/config/default_resource_limits/cpu "1000"
consul kv put ${PREFIX}/config/default_resource_limits/memory "2048"
consul kv put ${PREFIX}/config/default_resource_limits/disk "10240"

# Registry configuration
consul kv put ${PREFIX}/config/registry/url "your-registry.com"
consul kv put ${PREFIX}/config/registry/temp_prefix "temp"

echo "Consul configuration initialized"
```

## Deployment Options

### Option 1: Nomad Service Job

Deploy as a Nomad service job:

```hcl
# jobforge-service.nomad
job "jobforge-service" {
  datacenters = ["cluster"]
  type = "service"
  
  constraint {
    attribute = "${node.class}"
    operator = "!="
    value = "build-node"
  }

  group "service" {
    count = 3  # High availability
    
    network {
      port "http" {
        static = 8080
      }
      
      port "metrics" {
        static = 9090
      }
      
      port "health" {
        static = 8081
      }
    }

    service {
      name = "jobforge-service"
      port = "http"
      
      tags = [
        "build-service",
        "mcp",
        "api"
      ]
      
      check {
        type = "http"
        path = "/health"
        interval = "10s"
        timeout = "3s"
      }
    }
    
    service {
      name = "jobforge-service-metrics"
      port = "metrics"
      
      tags = [
        "metrics",
        "prometheus"
      ]
    }

    task "server" {
      driver = "docker"
      
      config {
        image = "jobforge-service:latest"
        ports = ["http", "metrics", "health"]
      }
      
      env {
        SERVER_HOST = "0.0.0.0"
        SERVER_PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        HEALTH_PORT = "${NOMAD_PORT_health}"
        
        NOMAD_ADDR = "http://nomad.service.consul:4646"
        NOMAD_DATACENTERS = "cluster"
        CONSUL_HTTP_ADDR = "consul.service.consul:8500"
        VAULT_ADDR = "http://vault.service.consul:8200"
        
        LOG_LEVEL = "info"
      }
      
      resources {
        cpu = 1000  # 1 GHz
        memory = 1024  # 1 GB
      }
      
      vault {
        policies = ["jobforge-service"]
        change_mode = "restart"
      }
      
      template {
        destination = "/secrets/vault-token"
        data = "{{ with secret \"auth/token/lookup-self\" }}{{ .auth.client_token }}{{ end }}"
        change_mode = "restart"
      }
      
      restart {
        attempts = 3
        interval = "5m"
        delay = "15s"
        mode = "delay"
      }
    }
  }
  
  update {
    max_parallel = 1
    min_healthy_time = "30s"
    healthy_deadline = "5m"
    progress_deadline = "10m"
    auto_revert = true
    canary = 1
  }
}
```

Deploy the job:
```bash
nomad job run jobforge-service.nomad
```

### Option 2: Docker Compose (Development)

```yaml
# docker-compose.yml
version: '3.8'

services:
  jobforge-service:
    image: jobforge-service:latest
    ports:
      - "8080:8080"
      - "9090:9090" 
      - "8081:8081"
    environment:
      - NOMAD_ADDR=http://nomad:4646
      - NOMAD_DATACENTERS=cluster
      - CONSUL_HTTP_ADDR=consul:8500
      - VAULT_ADDR=http://vault:8200
      - SERVER_PORT=8080
      - METRICS_PORT=9090
      - HEALTH_PORT=8081
    depends_on:
      - consul
      - vault
      - nomad
    restart: unless-stopped

  consul:
    image: hashicorp/consul:1.16
    ports:
      - "8500:8500"
    command: agent -server -bootstrap-expect=1 -ui -client=0.0.0.0

  vault:
    image: hashicorp/vault:1.15
    ports:
      - "8200:8200"
    environment:
      - VAULT_DEV_ROOT_TOKEN_ID=myroot
    command: vault server -dev

  nomad:
    image: multani/nomad:1.6
    ports:
      - "4646:4646"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    command: nomad agent -dev -bind=0.0.0.0 -network-interface=eth0
```

### Option 3: Kubernetes (Alternative)

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobforge-service
  labels:
    app: jobforge-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: jobforge-service
  template:
    metadata:
      labels:
        app: jobforge-service
    spec:
      containers:
      - name: jobforge-service
        image: jobforge-service:latest
        ports:
        - containerPort: 8080
        - containerPort: 9090
        - containerPort: 8081
        env:
        - name: NOMAD_ADDR
          value: "http://nomad:4646"
        - name: NOMAD_DATACENTERS
          value: "cluster"
        - name: CONSUL_HTTP_ADDR  
          value: "consul:8500"
        - name: VAULT_ADDR
          value: "http://vault:8200"
        livenessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: jobforge-service
spec:
  selector:
    app: jobforge-service
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: LoadBalancer
```

## Load Balancer Configuration

### HAProxy Example

```haproxy
# haproxy.cfg
global
    maxconn 4096
    log stdout local0

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

frontend build_service_frontend
    bind *:8080
    default_backend build_service_backend

backend build_service_backend
    balance roundrobin
    option httpchk GET /health
    server node1 jobforge-service-1:8080 check
    server node2 jobforge-service-2:8080 check
    server node3 jobforge-service-3:8080 check

frontend metrics_frontend
    bind *:9090
    default_backend metrics_backend

backend metrics_backend
    balance roundrobin
    server node1 jobforge-service-1:9090 check
    server node2 jobforge-service-2:9090 check
    server node3 jobforge-service-3:9090 check
```

### NGINX Example

```nginx
# nginx.conf
upstream build_service {
    least_conn;
    server jobforge-service-1:8080;
    server jobforge-service-2:8080;
    server jobforge-service-3:8080;
}

upstream metrics {
    server jobforge-service-1:9090;
    server jobforge-service-2:9090;
    server jobforge-service-3:9090;
}

server {
    listen 8080;
    location / {
        proxy_pass http://build_service;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
    
    location /health {
        access_log off;
        proxy_pass http://build_service;
    }
}

server {
    listen 9090;
    location /metrics {
        proxy_pass http://metrics;
    }
}
```

## Monitoring Setup

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'jobforge-service'
    consul_sd_configs:
      - server: 'consul:8500'
        services: ['jobforge-service-metrics']
    scrape_interval: 30s
    metrics_path: '/metrics'

  - job_name: 'nomad'
    static_configs:
      - targets: ['nomad-1:4646', 'nomad-2:4646', 'nomad-3:4646']
    metrics_path: '/v1/metrics'
    params:
      format: ['prometheus']

rule_files:
  - "build_service_rules.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

### Alerting Rules

```yaml
# build_service_rules.yml
groups:
  - name: build_service_alerts
    rules:
      - alert: BuildServiceDown
        expr: up{job="jobforge-service"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Build service instance down"
          
      - alert: HighBuildFailureRate
        expr: rate(total_jobs{status="failed"}[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High build failure rate detected"
          
      - alert: LongBuildTime
        expr: histogram_quantile(0.95, rate(build_duration_seconds_bucket[5m])) > 1800
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Build times are taking too long"
```

## Security Hardening

### 1. Network Security

- Use private networks for inter-service communication
- Implement firewall rules
- Enable TLS for all connections
- Use service mesh for advanced networking

### 2. Vault Security

```hcl
# vault-security.hcl
ui = false
api_addr = "https://vault.internal:8200"
cluster_addr = "https://vault.internal:8201"

storage "consul" {
  address = "consul.internal:8500"
  path = "vault/"
  tls_cert_file = "/etc/vault/tls/vault.crt"
  tls_key_file = "/etc/vault/tls/vault.key"
  tls_ca_file = "/etc/vault/tls/ca.crt"
}

listener "tcp" {
  address = "0.0.0.0:8200"
  tls_cert_file = "/etc/vault/tls/vault.crt"
  tls_key_file = "/etc/vault/tls/vault.key"
}

seal "awskms" {
  region = "us-west-2"
  kms_key_id = "alias/vault-key"
}
```

### 3. Container Security

```dockerfile
# Multi-stage build for minimal attack surface
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o jobforge-service ./cmd/server

FROM alpine:3.18
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/jobforge-service .

# Create non-root user
RUN adduser -D -s /bin/sh buildservice
USER buildservice

EXPOSE 8080 9090 8081
CMD ["./jobforge-service"]
```

## Backup and Recovery

### 1. Consul Backup

```bash
#!/bin/bash
# backup-consul.sh
DATE=$(date +%Y%m%d_%H%M%S)
consul snapshot save /backup/consul-${DATE}.snap
consul kv export jobforge-service/ > /backup/config-${DATE}.json
```

### 2. Vault Backup

```bash
#!/bin/bash  
# backup-vault.sh
DATE=$(date +%Y%m%d_%H%M%S)
vault operator raft snapshot save /backup/vault-${DATE}.snap
```

### 3. Automated Backup

```bash
#!/bin/bash
# scheduled-backup.sh (run via cron)
0 2 * * * /scripts/backup-consul.sh
0 3 * * * /scripts/backup-vault.sh

# Cleanup old backups (keep 30 days)
find /backup -name "consul-*.snap" -mtime +30 -delete
find /backup -name "vault-*.snap" -mtime +30 -delete  
find /backup -name "config-*.json" -mtime +30 -delete
```

## Troubleshooting

### Common Deployment Issues

1. **Service Discovery Issues**
   ```bash
   # Check Consul services
   consul catalog services
   consul health service jobforge-service
   ```

2. **Vault Authentication**
   ```bash
   # Test Vault connectivity
   vault status
   vault auth -method=userpass username=service
   ```

3. **Nomad Job Issues**
   ```bash
   # Check job status
   nomad job status jobforge-service
   nomad alloc logs <alloc-id>
   ```

### Performance Tuning

1. **Resource Allocation**
   - Monitor resource usage patterns
   - Adjust based on workload
   - Scale horizontally vs. vertically

2. **Build Cache Optimization**
   - Monitor cache hit rates
   - Increase cache size if needed
   - Consider distributed caching

3. **Network Optimization**
   - Use regional registries
   - Implement registry caching
   - Optimize network routes

This deployment guide provides a comprehensive foundation for running the Nomad Build Service in production environments.