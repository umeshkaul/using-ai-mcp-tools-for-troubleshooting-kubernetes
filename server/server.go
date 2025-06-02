package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// main is the entry point of the application that sets up and runs the MCP server
// with kubectl and k8sgpt tools for Kubernetes troubleshooting
func main() {
	// Parse command line flags
	sseMode := flag.Bool("sse", true, "Run in SSE mode instead of stdio mode")
	flag.Parse()

	// Create MCP server with basic capabilities
	mcpServer := server.NewMCPServer(
		"kubernetes-troubleshooter",
		"1.0.0",
	)

	// Create and add the k8sgpt tool for AI-powered Kubernetes analysis
	k8sGptTool := mcp.NewTool(
		"k8sgpt",
		mcp.WithDescription(
			"Execute 'k8sgpt' command to interact with a Kubernetes cluster. Use 'k8sgpt analyze' command to check if there are any problems with the cluster or pods running. ",
		),
		mcp.WithString(
			"arguments",
			mcp.Description("The arguments to pass to the k8sgpt command"),
			mcp.Required(),
		),
	)
	// Register k8sgpt handler with a 30-second timeout
	mcpServer.AddTool(k8sGptTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCommandExecution(ctx, req, "k8sgpt", 30*time.Second)
	})

	// Create and add the kubectl tool for direct Kubernetes cluster interaction
	kubectlTool := mcp.NewTool(
		"kubectl",
		mcp.WithDescription(
			"Use 'kubectl' command to check if there are any issues. Use 'kubectl logs' command to see if there are any issues. Do not use kubectl edit command, instead of that use kubectl patch command to make changes as and when required.",
		),
		mcp.WithString(
			"arguments",
			mcp.Description("The arguments to pass to the kubectl command"),
			mcp.Required(),
		),
	)
	// Register kubectl handler with a 30-second timeout
	mcpServer.AddTool(kubectlTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCommandExecution(ctx, req, "kubectl", 30*time.Second)
	})

	// Run server in appropriate mode based on the sseMode flag
	if *sseMode {
		// Create and start SSE server for real-time communication
		sseServer := server.NewSSEServer(mcpServer,
			server.WithBaseURL("http://localhost:8090"))
		log.Printf("Starting SSE server on localhost:8090")
		if err := sseServer.Start(":8090"); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		// Run as stdio server for direct command-line interaction
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

// handleCommandExecution is a generic handler for executing command line tools
// It provides unified error handling, timeout management, and output capture for both kubectl and k8sgpt commands
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - request: The MCP tool request containing command arguments
//   - commandName: Name of the command to execute (e.g., "kubectl" or "k8sgpt")
//   - timeout: Duration after which the command execution will be terminated
//
// Returns:
//   - *mcp.CallToolResult: The command output wrapped in an MCP result
//   - error: Any error that occurred during command execution
func handleCommandExecution(
	ctx context.Context,
	request mcp.CallToolRequest,
	commandName string,
	timeout time.Duration,
) (*mcp.CallToolResult, error) {
	// Extract and validate command arguments from the request
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return &mcp.CallToolResult{}, fmt.Errorf("invalid arguments format")
	}
	cmdArgs, ok := args["arguments"].(string)
	if !ok {
		return &mcp.CallToolResult{}, fmt.Errorf("missing or invalid %s argument", commandName)
	}

	// fmt.Printf("got args for %s: %s\n", commandName, cmdArgs)

	// Parse command arguments into individual components
	commandArgs := strings.Fields(cmdArgs)

	fmt.Printf("executing command: %s %s\n", commandName, strings.Join(commandArgs, " "))

	// Create a context with the specified timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare the command with the given context
	cmd := exec.CommandContext(ctx, commandName, commandArgs...)

	// Set up buffers to capture command output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command and handle various error cases
	err := cmd.Run()
	if err != nil {
		// Handle command exit errors (non-zero exit codes)
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("%s exited with code %d: %s\n", commandName, exitError.ExitCode(), stderr.String())
			return &mcp.CallToolResult{}, fmt.Errorf("%s exited with code %d: %s", commandName, exitError.ExitCode(), stderr.String())
		}
		// Handle timeout errors
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Printf("%s command timed out after %v\n", commandName, timeout)
			return &mcp.CallToolResult{}, fmt.Errorf("%s command timed out after %v", commandName, timeout)
		}
		// Handle other execution errors
		fmt.Printf("%s execution failed: %v\nstderr: %s", commandName, err, stderr.String())
		return &mcp.CallToolResult{}, fmt.Errorf("%s execution failed: %v\nstderr: %s", commandName, err, stderr.String())
	}

	// Process and return the command output
	output := stdout.String()
	if output == "" {
		output = "Command executed successfully with no output"
	}
	// fmt.Printf("%s output: %s\n", commandName, output)
	return mcp.NewToolResultText(output), nil
}
