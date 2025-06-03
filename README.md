# Building an AI-Powered Kubernetes Troubleshooting Assistant with MCP

## 1. High-Level Overview

This project showcases the development of an AI-powered Kubernetes troubleshooting assistant using the Model-Controller-Presenter (MCP) protocol. Implemented in Go using [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK, the prototype integrates large language models (LLMs) with standard Kubernetes tools like `kubectl` and `k8sgpt` to deliver a natural language, conversational interface for managing and troubleshooting clusters. Key features include real-time cluster monitoring, automated issue detection and resolution, intelligent command execution, and verification of applied fixes.

## 2. System Architecture

### MCP Protocol Overview

The Model-Controller-Presenter (MCP) protocol is a standardized framework designed to facilitate robust and flexible communication between clients and servers. It enables the discovery and exposure of tools as callable functions, supports both synchronous and asynchronous operations, ensures type-safe communication, allows for bidirectional data exchange, and permits language-agnostic tool implementation, making it adaptable across diverse systems and environments.

In this project, MCP is used to:

- Expose `kubectl` and `k8sgpt` as callable tools
- Handle tool discovery and execution
- Manage communication between the AI client/agent and Kubernetes tools using SSE. SSE enables lightweight, real-time streaming from server to client—ideal for delivering tool outputs and logs in MCP-based AI workflows.
 
### System Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  User Input     │◄───►│  MCP Client     │◄───►│  OpenAI LLM     │
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  MCP Server     │
                        └────────┬────────┘
                                 │
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  MCP Tools      │
                        │  (kubectl,      │
                        │   k8sgpt)       │
                        └────────┬────────┘
                                 │
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  Kubernetes     │
                        │  Cluster        │
                        └─────────────────┘

Flow:
1. User Input → MCP Client (natural language query)
2. MCP Client → OpenAI LLM (query interpretation)
3. OpenAI LLM → MCP Client (tool selection and arguments)
4. MCP Client → MCP Server (tool execution request)
5. MCP Server → MCP Tools (kubectl/k8sgpt execution)
6. MCP Tools → Kubernetes Cluster (command execution)
7. Results flow back up through the same path
8. MCP Client → User (formatted response)

Example Flow:
1. User: "check to see if there are problems with any pods"
2. LLM interprets and selects k8sgpt analyze
3. MCP Server executes k8sgpt analyze
4. Results flow back to user
5. If issues found, LLM selects kubectl commands
6. MCP Server executes kubectl commands
7. Results flow back to user
```

## 3. Code Overview and Example Use

The project consists of two main components:

### Server Component
```go
// Create MCP server with basic capabilities
mcpServer := server.NewMCPServer(
    "kubernetes-troubleshooter",
    "1.0.0",
)

// Create and add the k8sgpt tool
k8sGptTool := mcp.NewTool(
    "k8sgpt",
    mcp.WithDescription(
        "Execute 'k8sgpt' command to interact with a Kubernetes cluster...",
    ),
)

// Create and add the kubectl tool
kubectlTool := mcp.NewTool(
    "kubectl",
    mcp.WithDescription(
        "Use 'kubectl' command to check if there are any issues...",
    ),
)
```

### Client Component
```go
// Create a new MCP client using HTTP transport
mcpClient, err := client.NewSSEMCPClient(MCPEndpoint)
if err != nil {
    log.Fatalf("failed to create MCP client: %v", err)
}

// Initialize OpenAI client
client := openai.NewClient()
```

### Example: Fixing an Image Pull Error

When a user asks to check for pod issues:
```
> check to see if there are problems with any pods running in my cluster
```
System detects: 
```
0: Pod default/nginx-5545cbc86d-wszvv(Deployment/nginx)
- Error: Back-off pulling image "nginx007": ErrImagePull: failed to pull and unpack image "docker.io/library/nginx007:latest"
```

The system automatically:
1. Deletes the problematic pod
2. Updates the deployment with the correct image
3. Monitors the rollout
4. Verifies the fix


## 4. Using the MCP Kubernetes Troubleshooter

### Prerequisites

1. Go 1.16 or later installed
2. A running Kubernetes cluster
3. `kubectl` configured with access to your cluster
4. `k8sgpt` CLI tool installed from [here](https://k8sgpt.ai). K8sGPT is an AI-powered tool designed to scan your Kubernetes clusters and identify issues, translating them into easy-to-understand explanations. It can use AI mode to summarize its findings but we are not using that mode here.
5. OpenAI API key

### Step 1: Clone and Setup

```bash
git clone <repository-url>
cd  using-ai-mcp-tools-for-troubleshooting-kubernetes
```

### Step 2: Configure Environment

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY="your-openai-api-key"
```

2. Verify your Kubernetes cluster access:
```bash
kubectl cluster-info
```
3. Verify that `k8sgpt` is working. *Note that `k8sgpt` can use AI mode to summarize its findings but we are not using that mode here.*

```bash
k8sgpt analyze

AI Provider: AI not used; --explain not set

No problems detected
```

### Step 3: Start the MCP Server

In a new terminal window:
```bash
cd server
go run server.go
```

You should see:
```
2025/06/02 07:58:51 Starting SSE server on localhost:8090
```

### Step 4: Start the MCP Client

In another terminal window:
```bash
cd client
go run client.go
```

You should see:
```
Initializing server...Initialized with server: kubernetes-troubleshooter 1.0.0
Available tools:
- k8sgpt: Execute 'k8sgpt' command to interact with a Kubernetes cluster...
- kubectl: Use 'kubectl' command to check if there are any issues...
```

### Step 5: Using the Troubleshooter

Let's walk through a real example of troubleshooting an image pull error:

1. First, let's check for any issues in the cluster:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
check to see if there are problems with any pods running in my cluster
```

The system will respond with any detected issues:
```
0: Pod default/nginx-5545cbc86d-wszvv(Deployment/nginx)
- Error: Back-off pulling image "nginx007": ErrImagePull: failed to pull and unpack image "docker.io/library/nginx007:latest"
```

2. To fix the issues, ask the system to take action:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
check to see if there are problems with any pods running in my cluster and fix them
```

The system will:

- Delete the problematic pod
- Update the deployment with the correct image
- Monitor the rollout
- Verify the fix

3. To verify the fix and wait for confirmation:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
check to see if there are problems with any pods running in my cluster and fix them, wait for 10 seconds and confirm that the problem is resolved before completing the task
```

### Step 6: Additional Troubleshooting Examples

1. Check kube-apiserver logs:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
get me the last 5 log lines of the pod which is running kube api server in kube-system namespace
```

2. Check kube-proxy logs:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
get me the last 5 log line of the pod running kube proxy in kube-system namespace
```

### Common Commands

Here are some useful queries you can try:
- "Get the status of all pods in the default namespace"
- "Check if there are any authentication issues in the cluster"

### Troubleshooting Tips

1. If the server fails to start:
   - Check if port 8090 is available
   - Verify your Go installation
   - Check server logs for errors

2. If the client fails to connect:
   - Verify the server is running
   - Check your OpenAI API key
   - Ensure your kubeconfig is properly configured

3. If commands fail:
   - Check your cluster permissions
   - Verify the tools (kubectl, k8sgpt) are installed
   - Look for error messages in the client output

### Exiting the Application

To exit the client:
```
Enter your question about the Kubernetes cluster (type 'quit' to exit):
quit
```
