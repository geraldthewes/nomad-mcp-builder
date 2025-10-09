# Buildah cgroup v2 Fixes

## Problem
Buildah was failing with cgroup v2 permission errors:
```
open `/sys/fs/cgroup/cgroup.subtree_control` for writing: Read-only file system
error running container: enable controllers `+cpu +io +memory +pids +cpuset +hugetlb +misc`
```

## Root Cause
Modern container runtimes (like crun) need write access to cgroup controllers to manage container resources, but the container didn't have proper permissions.

## Solutions Applied

### 1. Privileged Mode
```go
"privileged": true,  // Enable privileged mode for cgroup access
```

### 2. cgroup Volume Mount
```go
"volumes": []string{
    "/sys/fs/cgroup:/sys/fs/cgroup:rw",  // Mount cgroup filesystem with write access
},
```

### 3. Required Linux Capabilities
```go
"cap_add": []string{
    "SYS_ADMIN",   // Required for mount operations
    "SETUID",      // Required for user namespace operations
    "SETGID",      // Required for user namespace operations
    "SYS_CHROOT",  // Required for chroot operations
},
```

### 4. Security Options
```go
"security_opt": []string{
    "seccomp=unconfined",   // Disable seccomp restrictions
    "apparmor=unconfined",  // Disable AppArmor restrictions
},
```

### 5. User Namespace Configuration
```go
"userns_mode": "host",  // Use host user namespace mapping
```

### 6. Buildah Environment Variables
```go
Env: map[string]string{
    "BUILDAH_ISOLATION": "oci",     // Use OCI runtime isolation
    "STORAGE_DRIVER":    "vfs",     // Use VFS storage driver (no FUSE)
    "BUILDAH_FORMAT":    "oci",     // Ensure OCI format
    "BUILDAH_LAYERS":    "true",    // Enable layer caching
    "CGROUP_MANAGER":    "systemd", // Use systemd cgroup manager
},
```

## Why These Fixes Work

1. **Privileged Mode**: Gives the container full access to host system resources
2. **cgroup Mount**: Provides write access to cgroup controllers
3. **Capabilities**: Grants specific permissions for namespace and mount operations
4. **Security Options**: Removes restrictions that might block container operations
5. **VFS Storage**: Avoids FUSE/overlay dependencies that require additional permissions
6. **systemd cgroup Manager**: Better compatibility with modern Linux systems

## Security Considerations

- **Privileged mode** reduces container isolation but is necessary for container-building containers
- **Host user namespaces** may reduce security isolation
- These settings are appropriate for build environments but should be used carefully in production

## Testing

After applying these fixes, Buildah should be able to:
- ✅ Create and manage containers
- ✅ Access cgroup controllers
- ✅ Build images successfully
- ✅ Push to registries

The cgroup v2 permission errors should be resolved.