# Configuration Strategy & KillTimeout

## Configuration Storage Approach

The service uses a **hybrid configuration strategy** that balances flexibility with simplicity:

### **Environment Variables (Bootstrap/Infrastructure)**
Use for settings that:
- Configure infrastructure connections
- Set initial/default values
- Are required at startup
- Don't change frequently

**Examples:**
```bash
NOMAD_ADDR=http://localhost:4646        # Connection endpoints
CONSUL_HTTP_ADDR=localhost:8500
VAULT_ADDR=http://localhost:8200

BUILD_TIMEOUT=30m                       # Default timeouts
TEST_TIMEOUT=15m  
KILL_TIMEOUT=30s                        # NEW: Graceful shutdown timeout

DEFAULT_CPU_LIMIT=1000                  # Default resource limits
DEFAULT_MEMORY_LIMIT=2048
DEFAULT_DISK_LIMIT=10240

LOG_LEVEL=debug                         # Logging configuration
LOG_JOB_SPECS=true
LOG_HCL_FORMAT=true
```

### **Consul KV Store (Runtime/Dynamic)**
Use for settings that:
- Change during operation
- Need cluster-wide consistency
- Require hot-reload capability
- Are tuning/performance related

**Examples:**
```bash
# Set via Consul KV
consul kv put nomad-build-service/config/build_timeout "45m"
consul kv put nomad-build-service/config/test_timeout "20m" 
consul kv put nomad-build-service/config/kill_timeout "60s"

consul kv put nomad-build-service/config/default_resource_limits/cpu "1500"
consul kv put nomad-build-service/config/default_resource_limits/memory "4096"
```

## KillTimeout Configuration

### **What is KillTimeout?**
The time Nomad waits between sending SIGTERM (graceful shutdown) and SIGKILL (force terminate):

```
Container needs to stop → SIGTERM → Wait KillTimeout → SIGKILL (if needed)
```

### **Why 30 seconds default?**
- **Docker builds** need time to clean up build contexts, temp files
- **Registry operations** may need to complete current uploads
- **5 seconds** (from manual job) was too aggressive for build containers
- **30 seconds** balances cleanup time vs responsiveness

### **Configuration Options:**

**Via Environment (Default):**
```bash
export KILL_TIMEOUT=30s   # 30 second default
```

**Via Consul (Runtime Override):**
```bash
consul kv put nomad-build-service/config/kill_timeout "60s"
```

### **Recommended Values:**
- **Development**: `10s` - Fast iteration
- **Production**: `30s-60s` - Allow proper cleanup
- **Large builds**: `120s` - Complex multi-stage builds

## Benefits of This Approach

### **Environment Variables:**
- ✅ Simple deployment configuration
- ✅ Container/k8s friendly
- ✅ Version controlled in deployment manifests
- ✅ Required for service startup

### **Consul Dynamic Config:**
- ✅ Hot-reload without restart
- ✅ Cluster-wide consistency
- ✅ Operational tuning capability
- ✅ Audit trail of changes

### **Priority Order:**
1. **Consul KV** (if available) - Runtime override
2. **Environment Variables** - Bootstrap default
3. **Code defaults** - Final fallback

This ensures the service always starts (env vars) but can be tuned dynamically (Consul) without downtime.