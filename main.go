package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/joho/godotenv"
)

//go:embed instructions/*
var instructions embed.FS

const DeepSeekApiUrl = "https://api.deepseek.com/v1/chat/completions"
const ClaudeApiUrl = "https://api.anthropic.com/v1/messages"

// DeepSeek API types
type DeepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type DeepSeekRequest struct {
	Model       string            `json:"model"`
	Messages    []DeepSeekMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

type DeepSeekChoice struct {
	Message      DeepSeekMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type DeepSeekResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Choices []DeepSeekChoice `json:"choices"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Claude API types
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeRequest struct {
	Model     string          `json:"model"`
	Messages  []ClaudeMessage `json:"messages,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type ClaudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func loadServiceTemplates(dockerComposeContent string) (string, error) {
	entries, err := instructions.ReadDir("instructions")
	if err != nil {
		return "", fmt.Errorf("error reading instructions directory: %v", err)
	}

	var services []string
	for _, entry := range entries {
		serviceName := strings.TrimSuffix(entry.Name(), ".md")
		if strings.Contains(strings.ToLower(dockerComposeContent), strings.ToLower(serviceName)) {
			content, err := instructions.ReadFile(path.Join("instructions", entry.Name()))
			if err != nil {
				return "", fmt.Errorf("error reading template for %s: %v", serviceName, err)
			}
			services = append(services, fmt.Sprintf(`<service>
<name>%s</name>
<template>%s</template>
</service>`, serviceName, string(content)))
		}
	}

	if len(services) > 0 {
		return fmt.Sprintf("<services>%s</services>", strings.Join(services, "\n")), nil
	}
	return "", nil
}

func callDeepSeek(apiKey string, dockerCompose string, schema string) (string, error) {
	serviceDocs, err := loadServiceTemplates(dockerCompose)
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`<input>
<docker-compose>%s</docker-compose>
<schema>%s</schema>
%s
<instructions>
1. Convert the docker-compose.yaml to zeabur-template.yaml based on the provided schema.
2. Use provided service templates directly when available.
3. Place config content directly in YAML instead of using volume mounts.
  Exception: configs that auto-generate at startup and reset on restart.
</instructions>
</input>
<output-format>
Provide only zeabur-template.yaml content without explanations or code blocks.
</output-format>
`, dockerCompose, schema, serviceDocs)

	requestBody := DeepSeekRequest{
		Model: "deepseek-coder",
		Messages: []DeepSeekMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", DeepSeekApiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	var response DeepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if len(response.Choices) == 0 {
		if response.Error.Message != "" {
			return "", fmt.Errorf("API error: %s", response.Error.Message)
		}
		return "", fmt.Errorf("no response choices returned")
	}

	return response.Choices[0].Message.Content, nil
}

func callClaude(apiKey string, dockerCompose string, schema string) (string, error) {
	serviceDocs, err := loadServiceTemplates(dockerCompose)
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`<input>
<docker-compose>%s</docker-compose>
<schema>%s</schema>
%s
<instructions>
1. Convert the docker-compose.yaml to zeabur-template.yaml based on the provided schema.
2. Use provided service templates directly when available.
3. Place config content directly in YAML instead of using volume mounts.
  Exception: configs that auto-generate at startup and reset on restart.
</instructions>
</input>
<output-format>
Provide only zeabur-template.yaml content without explanations or code blocks.
</output-format>
`, dockerCompose, schema, serviceDocs)

	requestBody := ClaudeRequest{
		Model:     "claude-3-sonnet-20240229",
		Messages:  []ClaudeMessage{{Role: "user", Content: prompt}},
		MaxTokens: 4096,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", ClaudeApiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	var response ClaudeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if response.Error.Message != "" {
		return "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Content) > 0 {
		return response.Content[0].Text, nil
	}

	return "", fmt.Errorf("no content in response")
}

func main() {
	// Parse command line argument for AI model
	aiModel := flag.String("ai-model", "claude", "AI model to use (deepseek or claude)")
	flag.Parse()

	// Load .env file
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Error loading .env file")
		return
	}

	// Check for docker-compose.yaml or docker-compose.yml
	var dockerCompose []byte
	if _, err := os.Stat("docker-compose.yaml"); err == nil {
		dockerCompose, err = os.ReadFile("docker-compose.yaml")
	} else if _, err := os.Stat("docker-compose.yml"); err == nil {
		dockerCompose, err = os.ReadFile("docker-compose.yml")
	} else {
		fmt.Println("Error: Neither docker-compose.yaml nor docker-compose.yml found")
		return
	}
	if err != nil {
		fmt.Printf("Error reading docker-compose file: %v\n", err)
		return
	}

	schema, err := os.ReadFile("schema.json")
	if err != nil {
		fmt.Printf("Error reading schema.json: %v\n", err)
		return
	}

	var zeaburTemplate string
	// Select AI model based on command line argument
	switch strings.ToLower(*aiModel) {
	case "deepseek":
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			fmt.Println("Please set DEEPSEEK_API_KEY in the .env file")
			return
		}
		fmt.Println("Using DeepSeek API")
		zeaburTemplate, err = callDeepSeek(apiKey, string(dockerCompose), string(schema))
	case "claude":
		apiKey := os.Getenv("CLAUDE_API_KEY")
		if apiKey == "" {
			fmt.Println("Please set CLAUDE_API_KEY in the .env file")
			return
		}
		fmt.Println("Using Claude API")
		zeaburTemplate, err = callClaude(apiKey, string(dockerCompose), string(schema))
	default:
		fmt.Println("Invalid AI model. Please choose 'deepseek' or 'claude'")
		return
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	err = os.WriteFile("zeabur-template.yaml", []byte(zeaburTemplate), 0644)
	if err != nil {
		fmt.Printf("Error writing zeabur-template.yaml: %v\n", err)
		return
	}

	fmt.Println("Successfully converted to zeabur-template.yaml")
}
