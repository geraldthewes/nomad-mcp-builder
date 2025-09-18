# Registry Cleanup Scripts

This directory contains scripts for managing and cleaning up the Docker registry.

## cleanup_registry.py

A comprehensive Python script for cleaning up Docker registry repositories and tags using the Docker Registry v2 API.

### Features

- 🔍 **Pattern Matching**: Clean up repositories using wildcards (e.g., `bdtemp*`)
- 🏷️ **Tag Management**: List and delete specific tags within repositories
- 🔒 **Authentication**: Support for username/password authentication
- 🛡️ **SSL Handling**: Works with self-signed certificates
- 🔍 **Dry Run Mode**: Preview deletions without actually performing them
- 📊 **Detailed Reporting**: Comprehensive logging and statistics
- ⚡ **Batch Operations**: Efficient bulk deletion of multiple repositories

### Installation

```bash
cd scripts
pip3 install -r requirements.txt
```

### Usage Examples

#### Show Registry Information
```bash
python3 cleanup_registry.py --registry https://registry.cluster:5000 --info
```

#### Clean Up All Old Temporary Images (Recommended)
```bash
# Dry run first to see what would be deleted
python3 cleanup_registry.py --registry https://registry.cluster:5000 --pattern "bdtemp*" --dry-run

# Actual cleanup
python3 cleanup_registry.py --registry https://registry.cluster:5000 --pattern "bdtemp*"
```

#### Clean Up Specific Repository
```bash
python3 cleanup_registry.py --registry https://registry.cluster:5000 --repository bdtemp
```

#### With Authentication
```bash
python3 cleanup_registry.py --registry https://registry.cluster:5000 \
  --username admin --password secret \
  --pattern "bdtemp*"
```

### Post-Cleanup: Garbage Collection

After running the cleanup script, you must run garbage collection to actually free disk space:

```bash
# Find your registry container
docker ps | grep registry

# Run garbage collection
docker exec <registry_container_name> /bin/registry garbage-collect /etc/docker/registry/config.yml

# For newer registries that need untagged cleanup
docker exec <registry_container_name> /bin/registry garbage-collect /etc/docker/registry/config.yml --delete-untagged=true
```

### Safety Features

- **Dry Run Mode**: Always test with `--dry-run` first
- **Confirmation Prompts**: Asks for confirmation before bulk deletions
- **Error Handling**: Continues processing even if individual deletions fail
- **SSL Verification**: Disabled by default for self-signed certificates

### Common Patterns

| Pattern | Description | Example |
|---------|-------------|---------|
| `bdtemp*` | All old temporary build images | `bdtemp`, `bdtemp-hello-world`, etc. |
| `*temp*` | Any repository with "temp" in the name | `buildtemp`, `tmpimages`, etc. |
| `test-*` | All test repositories | `test-app`, `test-service`, etc. |
| `build-*` | All build repositories | `build-123`, `build-prod`, etc. |

### Prerequisites

- Registry must have `REGISTRY_STORAGE_DELETE_ENABLED=true` in its configuration
- Network access to the registry's HTTP API
- Appropriate permissions if authentication is enabled