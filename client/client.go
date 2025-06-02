package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go"
)

const (
	MCPEndpoint = "http://localhost:8090/sse" // MCP server endpoint
)

func main() {
	// Create a new MCP client using HTTP transport
	mcpClient, err := client.NewSSEMCPClient(MCPEndpoint)
	if err != nil {
		log.Fatalf("failed to create MCP client: %v", err)
	}
	defer mcpClient.Close()

	if err == nil {
		err = mcpClient.Start(context.Background())
		if err != nil {
			log.Fatalf("failed to start MCP client: %v", err)
		}
	}

	fmt.Printf("Initializing server...")
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mcphost",
		Version: "0.1.0",
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	// Connect to the MCP server
	ctx := context.Background()

	initResult, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		log.Fatalf("failed to initialize MCP client: %v", err)
	}
	fmt.Printf("Initialized with server: %s %s\n", initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// List available tools on the server
	listToolsRequest := mcp.ListToolsRequest{}
	listToolsResult, err := mcpClient.ListTools(ctx, listToolsRequest)
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	fmt.Println("Available tools:")
	for _, tool := range listToolsResult.Tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
	}

	OpenAIToken := os.Getenv("OPENAI_API_KEY")
	if OpenAIToken == "" {
		log.Fatal("OPENAI_API_KEY environment variable is not set")
	}

	// Initialize OpenAI client
	client := openai.NewClient()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\nEnter your question about the Kubernetes cluster (type 'quit' to exit):\n")
		question, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Error reading input: %v", err)
		}
		question = strings.TrimSpace(question)
		if strings.EqualFold(question, "quit") {
			fmt.Println("Exiting.")
			break
		}

		fmt.Printf("> %s\n", question)

		err = runPrompt(question, listToolsResult, client, mcpClient, ctx)
		if err != nil {
			fmt.Printf("Error running prompt: %v", err)
		}
	}
}

func runPrompt(question string, listToolsResult *mcp.ListToolsResult, client openai.Client, mcpClient *client.Client, ctx context.Context) error {
	// Create tool definitions dynamically from available tools
	toolDefinitions := make([]openai.ChatCompletionToolParam, len(listToolsResult.Tools))
	for i, tool := range listToolsResult.Tools {
		toolDefinitions[i] = openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"arguments": map[string]string{
							"type": "string",
						},
					},
					"required": []string{"arguments"},
				},
			},
		}
	}

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(`You are an expert Kubernetes engineer with deep knowledge of cluster operations, troubleshooting, and best practices. Your primary goal is to understand the user's intent and respond appropriately.

IMPORTANT: Only provide analysis or interpretation when explicitly asked. For all other requests, just return the raw output.

When executing commands:
- Plan your tool calls efficiently to minimize the number of calls needed
- Once you have obtained the requested information, return it immediately
- Do not make additional tool calls if you already have the information
- Do not repeat the same tool call unless absolutely necessary
- Consider combining commands where possible to reduce the number of calls

When the user wants you to fix issues or take action:
- Use k8sgpt tool to get an initial analysis which should include a list of issues and a summary of the issues.
- Use kubectl tool to implement the necessary fixes. Do not use "kubectl edit  " command rather use "kubectl path" command to fix the issues.
- Verify the fixes worked
- Report back on what was fixed
- Do not provide analysis or steps unless specifically requested

When the user wants just to know if there is an issue with the cluster:
- Do not make any changes to the cluster unless the user explicitly asks you to do so.
- Use k8sgpt tool to get an initial analysis
- Use kubectl tool to get more detailed information
- Provide a clear, human-friendly explanation of the issues
- If asked provide a step-by-step plan to fix each issues
- If asked provide specific kubectl commands that would be needed to fix the issues

When the user asks for specific information (like logs, status, etc.):
- Execute the requested command and return ONLY the raw output
- DO NOT analyze or interpret the output
- DO NOT add any explanations or summaries
- DO NOT format or modify the output
- If the user wants analysis, they will explicitly ask for it

Pay close attention to the user's exact request and whether they want you to make changes or just provide suggestions. Always provide clear explanations and consider security, performance, and reliability in your recommendations.`),
			openai.UserMessage(question),
		},
		Tools: toolDefinitions,
		Seed:  openai.Int(0),
		Model: openai.ChatModelGPT4oMini,
	}

	maxIterations := 5 // Prevent infinite loops
	iteration := 1

	for iteration < maxIterations {
		completion, err := client.Chat.Completions.New(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to create chat completion: %v", err)
		}

		// Print the LLM's response if there is one
		if completion.Choices[0].Message.Content != "" {
			// fmt.Printf("\nLLM Response: %s\n", completion.Choices[0].Message.Content)
			// If we got a response, we're done
			fmt.Printf("\nGot LLM response: %s\nTask completed after %d iterations\n", completion.Choices[0].Message.Content, iteration-1)
			return nil
		}

		toolCalls := completion.Choices[0].Message.ToolCalls
		if len(toolCalls) == 0 {
			// If there are no tool calls, we're done
			fmt.Printf("\nNo more tool calls, task completed after %d iterations\n", iteration-1)
			return nil
		}

		fmt.Printf("checking for tool calls\n")
		for _, toolCall := range toolCalls {
			fmt.Printf("Iteration %d - Tool call: %s\n", iteration, toolCall.Function.Name)
			fmt.Printf("Tool call arguments: %s\n", toolCall.Function.Arguments)
			var args map[string]interface{}
			err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
			if err != nil {
				params.Messages = append(params.Messages, openai.ChatCompletionMessage{
					Role:    "system",
					Content: fmt.Sprintf("Error parsing tool arguments: %v. Please try again with valid JSON.", err),
				}.ToParam())
				continue
			}

			var toolResultPtr *mcp.CallToolResult
			req := mcp.CallToolRequest{}
			req.Params.Name = toolCall.Function.Name
			req.Params.Arguments = args

			// Create a new context with timeout for each retry
			retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			toolResultPtr, err = mcpClient.CallTool(retryCtx, req)
			cancel()

			if err != nil {
				fmt.Printf("tool execution failed or timeout: %v\n", err)
				fmt.Printf("sleeping for 2 seconds, before retrying\n")
				time.Sleep(2 * time.Second)
				params.Messages = append(params.Messages, openai.ChatCompletionMessage{
					Role:    "system",
					Content: fmt.Sprintf("Tool execution failed with error: %v\nPlease try a different approach.", err),
				}.ToParam())
			} else if toolResultPtr != nil {
				toolResult := *toolResultPtr
				if len(toolResult.Content) > 0 {
					fmt.Printf("Tool result content: %v\n", toolResult.Content)

					// Add the tool result to the conversation
					toolResultMsg := fmt.Sprintf("%v", toolResult.Content)
					params.Messages = append(params.Messages, openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: toolResultMsg,
					}.ToParam())

					// Add a system message to guide the LLM to return results
					params.Messages = append(params.Messages, openai.ChatCompletionMessage{
						Role:    "system",
						Content: "If you have obtained the requested information. Please return it directly to the user without making additional tool calls.",
					}.ToParam())
				}
			}
		}

		iteration++
	}

	fmt.Printf("\nReached maximum iterations (%d) without completing the task\n", maxIterations)
	return nil
}
