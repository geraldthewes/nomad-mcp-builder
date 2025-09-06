# Buildah Best Practices for Container Builds

## Overview

This document summarizes the best practices for running Buildah in containerized environments, specifically within Nomad jobs. These practices are based on official Buildah documentation and successful implementation experience.

## Key Configuration Requirements

### 1. Container Image
- **Use**: `quay.io/buildah/stable:latest` (official Buildah container image)
- **Avoid**: Custom or unofficial images that may lack proper Buildah setup

### 2. Security Configuration

#### Essential Security Options
```hcl
security_opt = [
  "seccomp=unconfined",  # Required for Buildah syscalls
  "apparmor=unconfined"  # Prevents AppArmor interference
]
```

#### Device Access
```hcl
devices = [
  {
    host_path      = "/dev/fuse"
    container_path = "/dev/fuse"
  }
]
```

### 3. Privilege Settings
- **privileged = false** - Buildah works without privileged containers
- Rootless operation is supported and preferred for security

### 4. Storage Configuration

#### Volume Mounts
```hcl
volumes = [
  "/opt/nomad/data/buildah-cache:/var/lib/containers:rw",    # Build cache
  "/opt/nomad/data/buildah-shared:/var/lib/shared:ro"        # Additional image stores
]
```

#### Storage Driver
- **fuse-overlayfs** is the recommended storage driver for containers
- Automatically configured with proper device mounting

### 5. Build Commands

#### Isolation Mode
```bash
buildah bud --isolation=chroot --file Dockerfile --tag image:tag .
```

#### Key Options:
- `--isolation=chroot`: Preferred isolation method for container environments
- `--file`: Explicitly specify Dockerfile path
- `--tag`: Tag the built image appropriately

## Implementation Example

### Nomad Job Configuration
```hcl
task "build" {
  driver = "docker"
  
  config {
    image   = "quay.io/buildah/stable:latest"
    command = "/bin/bash"
    args    = ["-c", "buildah bud --isolation=chroot --file Dockerfile --tag ${image_name} ."]
    
    privileged = false
    
    devices = [
      {
        host_path      = "/dev/fuse"
        container_path = "/dev/fuse"
      }
    ]
    
    security_opt = [
      "seccomp=unconfined",
      "apparmor=unconfined"
    ]
    
    volumes = [
      "/opt/nomad/data/buildah-cache:/var/lib/containers:rw",
      "/opt/nomad/data/buildah-shared:/var/lib/shared:ro"
    ]
  }
}
```

### Build Script Template
```bash
#!/bin/bash
set -euo pipefail

# Clone repository
git clone ${repo_url} /tmp/repo
cd /tmp/repo
git checkout ${git_ref}

# Build image with Buildah
buildah bud --isolation=chroot --file ${dockerfile_path} --tag ${temp_image} .

# Push to registry
buildah push ${temp_image} docker://${registry_url}

echo 'Build completed successfully'
```

## Common Issues Avoided

### 1. Docker-in-Docker Problems
- **Issue**: Complex cgroup v2 setup, BuildKit path creation failures
- **Solution**: Use Buildah directly instead of Docker-in-Docker

### 2. User Namespace Errors
- **Issue**: "Error during unshare(CLONE_NEWUSER): Operation not permitted"
- **Solution**: Proper security configuration with `seccomp=unconfined`

### 3. Storage Driver Issues
- **Issue**: Storage backend failures without proper device access
- **Solution**: Mount `/dev/fuse` and use appropriate volumes

## Performance Optimizations

### 1. Build Caching
- Mount persistent volume at `/var/lib/containers` for layer caching
- Significantly reduces rebuild times for similar images

### 2. Additional Image Stores
- Mount shared read-only volume at `/var/lib/shared`
- Allows sharing of base layers between builds

### 3. Resource Allocation
- **CPU**: 1000 MHz minimum for reasonable build performance
- **Memory**: 2 GiB minimum for complex builds with large dependencies
- **Disk**: 10 GiB minimum for build artifacts and caching

## Security Considerations

### 1. Rootless Operation
- Buildah supports rootless container builds
- No need for privileged containers when properly configured

### 2. Secrets Handling
- Use Nomad Vault integration for registry credentials
- Never embed credentials in build scripts or job definitions

### 3. Network Security
- Builds require network access for package downloads
- Consider network policies for production environments

## References

- [Official Buildah Documentation](https://buildah.io/)
- [Best Practices for Running Buildah in a Container](https://buildah.io/blogs/2021/05/27/running-buildah-in-a-container.html) by Dan Walsh
- [Buildah Installation and Configuration](https://github.com/containers/buildah/blob/main/install.md)

## Troubleshooting

### Log Analysis
```bash
# Monitor build progress
nomad alloc logs -f <allocation-id>

# Check for specific build steps
nomad alloc logs <allocation-id> | grep -E "STEP|Build completed|Error"

# Verify allocation status
nomad alloc status <allocation-id>
```

### Common Error Patterns
1. **cgroup errors**: Usually indicates Docker-in-Docker issues - switch to Buildah
2. **Permission denied**: Check device mounting and security options
3. **Storage driver failures**: Verify `/dev/fuse` access and volume mounts

---

*Last updated: 2025-09-06*  
*Based on successful implementation following official Buildah best practices*