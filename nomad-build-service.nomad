job "nomad-build-service" {
  # Nomad datacenter and region
  datacenters = ["dc1"]
  region      = "global"
  
  # Job type - service runs continuously
  type = "service"
  
  group "app" {
    count = 1
    
    # Restart policy
    restart {
      attempts = 3
      delay    = "30s"
      interval = "5m"
      mode     = "fail"
    }
    
    # Update strategy
    update {
      max_parallel      = 1
      min_healthy_time  = "30s"
      healthy_deadline  = "2m"
      progress_deadline = "5m"
      auto_revert       = true
    }
    
    # Network configuration
    network {
      port "http" {
        static = 8080
        to     = 8080
      }
      
      port "metrics" {
        static = 9090
        to     = 9090
      }
      
      port "health" {
        static = 8081
        to     = 8081
      }
    }
    
    # Main API service registration
    service {
      name = "nomad-build-service"
      port = "http"
      
      tags = [
        "http",
        "api",
        "mcp",
        "nomad-build-service"
      ]
      
      meta {
        version     = "v2.0"
        environment = "production"
      }
      
      check {
        type     = "http"
        path     = "/health"
        interval = "30s"
        timeout  = "10s"
        port     = "health"
        
        check_restart {
          limit           = 3
          grace           = "10s"
          ignore_warnings = false
        }
      }
    }
    
    # Metrics service registration for Prometheus
    service {
      name = "nomad-build-service-metrics"
      port = "metrics"
      
      tags = [
        "metrics",
        "prometheus",
        "monitoring"
      ]
      
      meta {
        metrics_path = "/metrics"
        scrape       = "true"
      }
      
      check {
        type     = "http"
        path     = "/metrics"
        interval = "60s"
        timeout  = "10s"
        port     = "metrics"
      }
    }
    
    # Main application task
    task "server" {
      driver = "docker"
      
      config {
        # Build and push your image to your registry
        image = "${REGISTRY_URL}/nomad-build-service:latest"
        
        ports = ["http", "metrics", "health"]
        
        # Optional: mount build cache volume
        volumes = [
          "/opt/nomad/data/buildah-cache:/opt/nomad/data/buildah-cache"
        ]
        
        # Logging configuration
        logging {
          type = "journald"
          config {
            tag = "nomad-build-service"
          }
        }
      }
      
      # Environment variables
      env {
        # Server configuration
        SERVER_HOST = "0.0.0.0"
        SERVER_PORT = "8080"
        
        # Nomad configuration
        NOMAD_ADDR   = "http://{{ env "attr.unique.network.ip-address" }}:4646"
        NOMAD_REGION = "global"  # Update to match your region
        
        # Consul configuration  
        CONSUL_HTTP_ADDR   = "{{ env "attr.unique.network.ip-address" }}:8500"
        CONSUL_DATACENTER  = "dc1"  # Update to match your datacenter
        
        # Vault configuration
        VAULT_ADDR = "http://{{ env "attr.unique.network.ip-address" }}:8200"
        
        # Registry configuration
        REGISTRY_URL         = "${REGISTRY_URL}"        # Set via variable
        REGISTRY_TEMP_PREFIX = "temp"
        
        # Monitoring
        MONITORING_ENABLED = "true"
        METRICS_PORT      = "9090"
        HEALTH_PORT       = "8081"
        
        # Timeouts
        BUILD_TIMEOUT = "30m"
        TEST_TIMEOUT  = "15m"
        
        # Logging
        LOG_LEVEL = "info"
      }
      
      # Resource requirements
      resources {
        cpu    = 500   # MHz
        memory = 1024  # MB
      }
      
      # Vault integration for secrets
      vault {
        policies = ["nomad-build-service"]
      }
      
      # Template for dynamic configuration from Consul
      template {
        destination = "local/dynamic.env"
        env         = true
        change_mode = "restart"
        
        data = <<EOH
# Dynamic configuration from Consul
{{- with key "nomad-build-service/config/build_timeout" }}
BUILD_TIMEOUT={{ . }}
{{- end }}
{{- with key "nomad-build-service/config/test_timeout" }}
TEST_TIMEOUT={{ . }}
{{- end }}
{{- with key "nomad-build-service/config/log_level" }}
LOG_LEVEL={{ . }}
{{- end }}
EOH
      }
    }
    
    # Constraint: run only on nodes with Docker
    constraint {
      attribute = "${driver.docker}"
      value     = "1"
    }
    
    # Constraint: prefer nodes with build capabilities
    constraint {
      attribute = "${node.class}"
      value     = "build"
      operator  = "regexp"
    }
  }
}