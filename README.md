<div align="center">

# 🛡️ KubeVision

**The High-Fidelity SRE Cockpit & Kubernetes Terminal IDE.**

![License](https://img.shields.io/github/license/amidipayan/kubevision)
![Version](https://img.shields.io/badge/version-v1.0.0-blue)

</div>

---

🚀 **KubeVision** is a specialized, high-performance terminal IDE designed explicitly for **Incident Response**, **Security Auditing**, and **Blast Radius Analysis**.

🧠 Unlike standard resource viewers, KubeVision treats Kubernetes data as actionable intelligence. It provides real-time reliability scoring, structural topology graphing, and the most comprehensive deep-dive Helm forensics available in the terminal alongwith all the k8s IDE features.

---

## ⚡ Core Capabilities

* 🔭 **Live Observability:** Sub-second container status tracking and integrated heatmaps without leaving your keyboard.

* 🤖 **Automated Reliability (SRE):** Built-in heuristic SRE scoring that calculates weighted health metrics based on production best practices.

* ⚓ **Helm Forensics:** A complete command center for Helm. Track drift intelligence, evaluate security postures (A-F grading), and run predictive upgrade impact assessments.

* 🛡️ **Fail-Safe Operations:** A proactive "Blast Radius" deletion engine with a "Safe Mode" confirmation prevents catastrophic manual errors.

* 📡 **Deep Diagnostics & Streaming:** High-performance multi-pod log aggregation, timestamp-synchronized viewing, and a unified cluster event timeline via the core Event API.

* 💻 **Native Debugging Suite:** Instant, context-aware shell access leveraging native subresources. Includes advanced SRE debugging and an integrated Port Forward manager.

* 🧬 **Configuration Diff Detection:** High-fidelity, side-by-side manifest comparisons to immediately detect stealthy 'kubectl' overrides against your desired state.

* 🔍 **Advanced CRD & API Discovery:** Instantly switch contexts with fuzzy-search namespace picking. Features an advanced API discovery layer to seamlessly map, fetch, and interact with any Custom Resource Definition.

* 🧩 **Extensible Architecture:** Map custom shortcuts to any CLI command via a flexible YAML plugin system.

---

## 📸 Guided Tour

### 📦 Pod Screen
Real-time workload observability featuring integrated live-resource heatmaps and sub-second container status tracking.
<img src="./assets/podscreen.png" width="1200" alt="Pod Screen">

### 🗂️ Namespace Picker
A high-velocity context switcher with fuzzy-search to navigate complex, multi-tenant clusters instantly.
<img src="./assets/namespacepicker.png" width="1200" alt="Namespace Picker">

### 🚨 Event Viewer
A unified streaming timeline of cluster events with intelligent severity highlighting and object-reference mapping.
<img src="./assets/eventviewer.png" width="1200" alt="Event Viewer">

### 🧬 Diff
High-fidelity, side-by-side manifest comparisons to detect configuration drift.
<img src="./assets/DiffSource.png" width="1200" alt="Diff Source">
<img src="./assets/Diffscreen.png" width="1200" alt="Diff Screen">

### 🕵️ SRE Discovery Picker
An advanced API discovery layer that allows for rapid jumping between native resources and custom CRDs.
<img src="./assets/srediscoverypicker.png" width="1200" alt="SRE Discovery Picker">

### 🛠️ SRE Debug
Integrated diagnostic mode that provides shell with tools for actionable remediation paths.
<img src="./assets/sredebug.png" width="1200" alt="SRE Debug">

### 📜 Log Viewer
High-performance log aggregator with multi-pod streaming, integrated search, and timestamp-synchronized viewing.
<img src="./assets/logviewer.png" width="1200" alt="Log Viewer">
<img src="./assets/multipodlogviewer.png" width="1200" alt="Log Viewer">

### 🩺 Service Heuristic SRE
Automated reliability auditing that calculates a weighted Health Score using SRE best practices.
<img src="./assets/ServiceHeuristicSRE.png" width="1200" alt="Service Heuristic SRE">
<img src="./assets/ServiceHeuristic1SRE.png" width="1200" alt="Service Heuristic SRE">
<img src="./assets/ServiceHeuristic2SRE.png" width="1200" alt="Service Heuristic SRE">

### ⚓ Helm View
A centralized dashboard for managing the lifecycle of Helm releases with real-time health-status correlation.
<img src="./assets/helm.png" width="1200" alt="Helm View">

### 🎛️ Helm Dashboard
A comprehensive SRE command center for releases, integrating history, security, and drift intelligence into one view.
<img src="./assets/helmdashboard.png" width="1200" alt="Helm Dashboard">

### 🔄 Helm Upgrade
Predictive upgrade assessments that scan for breaking changes and resource-level impact before deployment.
<img src="./assets/helmupgrade.png" width="1200" alt="Helm Upgrade">
<img src="./assets/helmupgraded.png" width="1200" alt="Helm Upgraded">

### 🕰️ Helm History
Deep-dive forensic timeline of release revisions.
<img src="./assets/helmhistory.png" width="1200" alt="Helm History">

### ⚖️ Helm Diff
Visual manifest delta analysis between installed releases and local chart changes to prevent "blind" deployments.
<img src="./assets/helmdiff.png" width="1200" alt="Helm Diff">

### ⚠️ Helm Drift
A real-time drift intelligence engine that identifies manual 'kubectl' overrides against the desired Helm state.
<img src="./assets/helmdrift.png" width="1200" alt="Helm Drift">

### 🔒 Helm Security
Automated security posture grading (A-F) that scans for RBAC over-privilege and container hardening gaps.
<img src="./assets/helmsecurity.png" width="1200" alt="Helm Security">
<img src="./assets/helmsecuritydetails.png" width="1200" alt="Helm Security Details">
<img src="./assets/helmsecuritydetails1.png" width="1200" alt="Helm Security Details">

### ⚙️ Helm SRE
Specialized heuristic analysis that evaluates the reliability and blast radius of Helm-managed workloads.
<img src="./assets/helmSRE.png" width="1200" alt="Helm SRE">

### 🖥️ Node
A infrastructure centric view with real-time CPU/Memory saturation bars and other features.
<img src="./assets/node.png" width="1200" alt="Node View">

### 💻 Shell
Instant, context-aware terminal access to containers and node-level debug shells without leaving the IDE.
<img src="./assets/shell.png" width="1200" alt="Shell">

### 🗑️ Delete Resource
A fail-safe deletion engine with "Safe Mode" confirmation and proactive "Blast Radius" impact analysis.
<img src="./assets/delete.png" width="1200" alt="Delete">
<img src="./assets/delete1.png" width="1200" alt="Delete Resource">
<img src="./assets/deletepopup.png" width="1200" alt="Delete Popup">
<img src="./assets/deleteconfirm.png" width="1200" alt="Delete Confirm">
<img src="./assets/deleted.png" width="1200" alt="Deleted">
<img src="./assets/deleted1.png" width="1200" alt="Deleted">

### 📑 Tab to view K8s resources
A multi-tabbed interface for seamless navigation between different Kubernetes resource categories.
<img src="./assets/tab.png" width="1200" alt="K8s Resources Tab">

### 🕸️ X-Ray Topology
*(This requires improvements I think, I need to work on this to improve further)*
<img src="./assets/xray.png" width="1200" alt="X-Ray Topology">
<img src="./assets/xray1.png" width="1200" alt="X-Ray Topology">

### 🔌 Port Forward
Port forward capabilities with Integrated port forward manager.
<img src="./assets/portforward1.png" width="1200" alt="Port Forward">
<img src="./assets/portforward2.png" width="1200" alt="Port Forward">
<img src="./assets/portforwardmgr.png" width="1200" alt="Port Forward Manager">

### ✨ There are other features that this ide supports such as custom plugins , crd's , context switch to name a few..

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
git clone https://github.com/amidipayan/kubevision
cd kubevision

# Build the production-ready binary
go build -ldflags="-s -w" -o kubevision ./cmd/kubevision/main.go

---

 🔌 Plugins
Extend KubeVision with your own operational tools. Map custom shortcuts to any CLI command in ~/.kubevision/plugins.yaml.
```bash
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
      ```

⚖️ License
Distributed under the MIT License. See LICENSE for more information.

🤝 Contributing
Contributions are what make the open-source community an amazing place to learn, inspire, and create. Any contributions you make are greatly appreciated.