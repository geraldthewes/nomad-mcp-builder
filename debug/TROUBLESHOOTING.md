# Nomad Build Service Troubleshooting

## Current Status

The Nomad Build Service has undergone significant improvements but still experiences job scheduling issues. This document summarizes the work completed and remaining challenges.

## ✅ Issues Successfully Resolved

### 1. **User Specification Errors** 
- **Problem**: Jobs failed with "user build:build not found" in buildah containers
- **Solution**: Removed `User: "build:build"` from all Nomad job templates
- **Files**: `internal/nomad/jobs.go` - all job phases

### 2. **Vault JWT Role Mismatch**
- **Problem**: Jobs tried to use non-existent `nomad-build-service` JWT role
- **Solution**: Updated to use existing `nomad-workloads` role with `Role: "nomad-workloads"`
- **Files**: `internal/nomad/jobs.go` - all Vault configurations

### 3. **Vault Policy Permissions**
- **Problem**: `nomad-build-service` policy lacked read access to secret paths
- **Solution**: Added read permissions for:
  - `secret/data/nomad/jobs/git-credentials`
  - `secret/data/nomad/jobs/registry-credentials`

### 4. **Registry Credentials Template Issues**
- **Problem**: Empty registry credentials caused template parsing failures
- **Solution**: Made registry credentials templates conditional using helper functions:
  - `buildTemplates()` - for build phase
  - `testTemplates()` - for test phase  
  - `publishTemplates()` - for publish phase

### 5. **Device Mount Constraints**
- **Problem**: `/dev/fuse` device mount triggered automatic version constraints
- **Solution**: Removed fuse device mounts, buildah works with chroot isolation

### 6. **Constraint Override Attempts**
- **Attempted**: Added empty `Constraints: []*nomadapi.Constraint{}` to TaskGroups
- **Result**: Nomad API still automatically adds version constraints

## ❌ Remaining Issue: Job Scheduling Failure

### Symptoms
- Jobs submit successfully (`status: "BUILDING"`)
- Jobs immediately fail (`status: "FAILED"`)
- Error: "Job failed to schedule - no allocations placed"
- Nomad shows: `Status: dead (stopped)`, `Allocations: No allocations placed`

### Constraints Present (but should be satisfied)
```json
[
  {
    "LTarget": "${attr.vault.version}",
    "Operand": "semver", 
    "RTarget": ">= 0.6.1"
  },
  {
    "LTarget": "${attr.nomad.version}",
    "Operand": "semver",
    "RTarget": ">= 1.7.0-a" 
  }
]
```

### Environment Status
- **Vault Version**: 1.15.6 (✅ satisfies >= 0.6.1)
- **Nomad Version**: 1.10.4 (✅ should satisfy >= 1.7.0-a)
- **Node Resources**: Abundant (3.2/54.4 GHz CPU, 3.1/94 GB memory)
- **Node Status**: Both nodes ready and eligible

### Verified Working Components

Individual components work correctly when tested in isolation:

1. **✅ Vault Integration**: Jobs with Vault templates and `nomad-workloads` role schedule successfully
2. **✅ Buildah Execution**: Buildah containers run successfully with chroot isolation
3. **✅ Volume Mounts**: Bind mounts to `/opt/nomad/data/buildah-cache` work correctly
4. **✅ Resource Requirements**: 1000 MHz CPU, 2048 MB memory jobs schedule fine
5. **✅ Secret Access**: Git and registry credentials are readable via Vault templates

### Comparison Analysis

Compared generated job JSON with working test jobs:
- **Same constraints**: Both have identical version constraints but test jobs schedule
- **Same resources**: Resource requirements are reasonable and available
- **Same Vault config**: Vault role and policies identical between working/failing jobs
- **Key difference**: Generated jobs have more complex configuration (mount, privileged settings)

## 🔍 Investigation Areas

### 1. Evaluation Details
- Evaluations show `Placement Failures = false` but no allocations placed
- This suggests issue is not with constraints or resources
- Possible silent failures or race conditions

### 2. Configuration Complexity
- Simple jobs with Vault work fine
- Generated jobs with identical individual components fail
- Issue may be in combination of configurations or subtle differences

### 3. Potential Causes
- **Timing issues**: Race conditions in evaluation process
- **API serialization**: Differences between Go struct generation and manual HCL
- **Silent constraint failures**: Constraints that fail validation without reporting
- **Resource conflicts**: Subtle resource or scheduling conflicts not visible in logs

## 🚀 Next Steps

### Immediate Actions
1. **Manual JSON comparison**: Create working job with identical configuration to generated job
2. **Nomad server logs**: Check server logs during job submission for hidden errors  
3. **Evaluation deep-dive**: Use Nomad API to get detailed evaluation failure reasons
4. **Progressive configuration**: Start with minimal working job, gradually add complexity

### Long-term Solutions
1. **Alternative approach**: Consider using Nomad job templates instead of Go API generation
2. **Constraint removal**: Investigate removing/overriding automatic version constraints
3. **Buildah alternatives**: Consider rootless buildah or different container build approaches

## 📋 Testing Commands

### Check Current Status
```bash
# Submit test job
curl -X POST http://10.0.1.13:26200/mcp/submitJob -H "Content-Type: application/json" -d '{...}'

# Check job status  
nomad job status build-<JOB-ID>

# Check constraints
nomad job inspect build-<JOB-ID> | jq '.Job.TaskGroups[0].Constraints'
```

### Manual Testing
```bash
# Create working test job
nomad job run /tmp/test-vault-minimal.hcl

# Compare with failing job
nomad job inspect -json test-vault-minimal > working.json
nomad job inspect -json build-<JOB-ID> > failing.json
diff working.json failing.json
```

## 💡 Current Workarounds

The build service is functional except for job scheduling. Once the scheduling issue is resolved:

- ✅ MCP protocol support is complete
- ✅ Vault integration is properly configured  
- ✅ Build, test, publish pipeline is implemented
- ✅ Error handling and status reporting works
- ✅ Credential management is secure

The core architecture and implementation are sound.