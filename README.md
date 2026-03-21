# 🛡️ KubeVision

**The High-Fidelity SRE Cockpit & Kubernetes IDE.**

[![Go Report Card](https://goreportcard.com/badge/github.com/amidipayan/kubevision)](https://goreportcard.com/report/github.com/amidipayan/kubevision)
![License](https://img.shields.io/github/license/amidipayan/kubevision)
![Version](https://img.shields.io/badge/version-v1.0.0-blue)

KubeVision is a specialized IDE designed for **Incident Response**, **Security Auditing**, and **Blast Radius Analysis**. Unlike standard viewers, KubeVision treats Kubernetes data as actionable intelligence, providing real-time reliability scoring, topology graphing, and deep-dive Helm forensics.

---

## ✨ Features at a Glance :- 

## 📸 Guided Tour

### 1. Pod Screen :- Real-time workload observability featuring integrated live-resource heatmaps and sub-second container status tracking.
![pod screen](./assets/podscreen.png)

### 2. namespace picker :- A high-velocity context switcher with fuzzy-search to navigate complex, multi-tenant clusters instantly.
![namespace picker](./assets/namespacepicker.png)

### 3. Event viewer :- A unified streaming timeline of cluster events with intelligent severity highlighting and object-reference mapping.
![event viewer](./assets/eventviewer.png)

### 4. Diff :- High-fidelity, side-by-side manifest comparisons to detect configuration drift.
![Diff Source](./assets/DiffSource.png)
![Diff Screen](./assets/Diffscreen.png)

### 5. Sre Discovery Picker :- An advanced API discovery layer that allows for rapid jumping between native resources and custom CRDs.
![sre discovery picker](./assets/srediscoverypicker.png)

### 6. Sre Debug :- Integrated diagnostic mode that provides shell with tools for actionable remediation paths.
![sre debug](./assets/sredebug.png)

### 7. Log Viewer :- High-performance log aggregator with multi-pod streaming, integrated search, and timestamp-synchronized viewing.
![log viewer](./assets/logviewer.png)

### 8. Service Heuristic SRE :- Automated reliability auditing that calculates a weighted Health Score using SRE best practices.
![Service Heuristic SRE](./assets/ServiceHeuristicSRE.png)
![Service Heuristic SRE](./assets/ServiceHeuristic1SRE.png)
![Service Heuristic SRE](./assets/ServiceHeuristic2SRE.png)

### 9. Helm View :- A centralized dashboard for managing the lifecycle of Helm releases with real-time health-status correlation.
![Helm](./assets/helm.png)

### 10. Helm Dashboard :- A comprehensive SRE command center for releases, integrating history, security, and drift intelligence into one view.
![Helm Dashboard](./assets/helmdashboard.png)

### 11. Helm Upgrade :- Predictive upgrade assessments that scan for breaking changes and resource-level impact before deployment.
![Helm Upgrade](./assets/helmupgrade.png)
![Helm Upgraded](./assets/helmupgraded.png)

### 12. Helm History :- Deep-dive forensic timeline of release revisions.
![Helm History](./assets/helmhistory.png)

### 13. Helm Diff :- Visual manifest delta analysis between installed releases and local chart changes to prevent "blind" deployments.
![Helm Diff](./assets/helmdiff.png)

### 14. Helm Drift :- A real-time drift intelligence engine that identifies manual 'kubectl' overrides against the desired Helm state.
![Helm Drift](./assets/helmdrift.png)

### 15. Helm Security :- Automated security posture grading (A-F) that scans for RBAC over-privilege and container hardening gaps.
![Helm Security](./assets/helmsecurity.png)
![Helm Security Details](./assets/helmsecuritydetails.png)
![Helm Security Details](./assets/helmsecuritydetails1.png)

### 16. Helm SRE :- Specialized heuristic analysis that evaluates the reliability and blast radius of Helm-managed workloads.
![Helm SRE](./assets/helmSRE.png)

### 17. node :- A infrastructure centric view with real-time CPU/Memory saturation bars and other features.
![node](./assets/node.png)

### 18. Shell :- Instant, context-aware terminal access to containers and node-level debug shells without leaving the IDE.
![Shell](./assets/shell.png)

### 19. Delete Resource :- A fail-safe deletion engine with "Safe Mode" confirmation and proactive "Blast Radius" impact analysis.
![Delete](./assets/delete.png)
![Delete Resource](./assets/delete1.png)
![Delete Popup](./assets/deletepopup.png)
![Delete Confirm](./assets/deleteconfirm.png)
![Deleted](./assets/deleted.png)

### 20. Tab to view K8s resources :- A multi-tabbed interface for seamless navigation between different Kubernetes resource categories.
![K8s Resources](./assets/tab.png)

### 21. X-Ray Topology :- (This requires improvements I think , I need to work on this to improve further)
![X-Ray Topology](./assets/xray.png)
![X-Ray Topology](./assets/xray1.png)

### 22. Port Forward :- Port forward capabilities with Integrated port forward manager.
![Port Forward](./assets/portforward1.png)
![Port Forward](./assets/portforward2.png)
![Port Forward Mgr](./assets/portforwardmgr.png)

### There are other features that this ide supports such as custom plugins , crd's , context switch to name a few..

---

## 📦 Installation

### ⚡ Option 1: Pre-built Binaries (Recommended)
KubeVision is distributed as a single, hardened static binary. 

1. Navigate to the [Releases](https://github.com/amidipayan/kubevision/releases) page.
2. Download the archive for your operating system:
    * **macOS (Intel):** `kubevision_darwin_amd64.tar.gz`
    * **macOS (Apple Silicon):** `kubevision_darwin_arm64.tar.gz`
    * **Linux:** `kubevision_linux_amd64.tar.gz`
    * **Windows:** `kubevision_windows_amd64.zip`
3. Extract and install:
    ```bash
    # Example for Linux
    tar -xvf kubevision_linux_amd64.tar.gz
    sudo mv kubevision /usr/local/bin/
    ```

### 🔨 Option 2: Build From Source
Requires **Go 1.21+**.

```bash
# Clone the repository
git clone [https://github.com/amidipayan/kubevision](https://github.com/amidipayan/kubevision)
cd kubevision

# Build the production-ready binary
go build -ldflags="-s -w" -o kubevision ./cmd/kubevision/main.go

🔌 Plugins
Extend KubeVision with your own operational tools. Map custom shortcuts to any CLI command in ~/.kubevision/plugins.yaml.

Example

plugins:
  pod:
    - shortCut: "shift+l"
      description: "Tail logs with Stern"
      command: "stern {{.Name}} -n {{.Namespace}}"
  pods:
    - shortCut: "L"
      description: "Test Plugin: Echo Pod Name"
      command: "echo 'Selected Pod: {{name}} in Namespace: {{namespace}}' && echo 'Plugin System is Working!' && read -p 'Press Enter to continue...' var"
  pod:
    - shortCut: "ctrl+t"
      description: "Tail logs with Stern"
      command: "stern {{name}} -n {{namespace}}"
      background: false
    - shortCut: "ctrl+v"
      description: "Vulnerability Scan (Trivy)"
      command: "trivy k8s pod {{name}} -n {{namespace}}"
      background: false

  deployment:
    - shortCut: "ctrl+m"
      description: "Trigger Rollout Restart"
      command: "kubectl rollout restart deployment/{{name}} -n {{namespace}}"
      background: true 

  node:
    - shortCut: "ctrl+h"
      description: "Root Debug Shell (nsenter)"
      command: "kubectl debug node/{{name}} -it --image=nicolaka/netshoot -- chroot /host"
      background: false


⚖️ License
Distributed under the MIT License. See LICENSE for more information.

🤝 Contributing
Contributions are what make the open-source community an amazing place to learn, inspire, and create. Any contributions you make are greatly appreciated.