package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alesr/gh-self-reviewer/gh"
	"github.com/google/go-github/v52/github"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"golang.org/x/oauth2"
)

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Starting gh-self-reviewer...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		log.Println("Received shutdown signal, gracefully shutting down...")
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	token := os.Getenv("GITHUB_TOKEN_MCP_APP_REVIEW")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN_MCP_APP_REVIEW environment variable is not set")
	}

	githubClient, err := makeGitHubClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub client: %w", err)
	}

	server := mcp.NewServer(stdio.NewStdioServerTransport())
	log.Println("MCP server created")

	if err := registerTools(ctx, server, gh.NewGithubToolHandler(githubClient)); err != nil {
		return fmt.Errorf("could not register tools: %w", err)
	}

	log.Println("Tools registered successfully")

	go func() {
		log.Println("Server Serve() started")
		if err := server.Serve(); err != nil {
			log.Printf("MCP server error: %v", err)
		}
	}()

	<-ctx.Done()

	log.Println("Context canceled, shutting down server")
	return nil
}

func makeGitHubClient(ctx context.Context) (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN_MCP_APP_REVIEW")
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc), nil
}

func registerTools(ctx context.Context, server *mcp.Server, githubToolHandler *gh.GithubToolHandler) error {
	log.Println("Registering tool: list_my_pull_requests")

	if err := server.RegisterTool("list_my_pull_requests", "List my pull requests",
		func(arguments gh.PRListRequest) (*mcp.ToolResponse, error) {
			prs, err := githubToolHandler.ListMyOpenPullRequestsAcrossRepos(ctx)
			if err != nil {
				return nil, fmt.Errorf("could not list open PRs: %w", err)
			}

			prJSON, err := json.Marshal(prs)
			if err != nil {
				return nil, fmt.Errorf("could not marshal PRs: %w", err)
			}
			return mcp.NewToolResponse(mcp.NewTextContent(string(prJSON))), nil
		}); err != nil {
		return fmt.Errorf("could not register list_my_pull_requests tool: %w", err)
	}

	log.Println("Registering tool: comment_on_pr")

	if err := server.RegisterTool("comment_on_pr", "Comment on a pull request",
		func(arguments gh.PRCommentRequest) (*mcp.ToolResponse, error) {
			comment, err := githubToolHandler.CommentOnPullRequestByURL(ctx, arguments.PRURL, arguments.Body)
			if err != nil {
				return nil, fmt.Errorf("could not comment on PR: %w", err)
			}

			commentJSON, err := json.Marshal(comment)
			if err != nil {
				return nil, fmt.Errorf("could not marshal PR comment: %w", err)
			}
			return mcp.NewToolResponse(mcp.NewTextContent(string(commentJSON))), nil
		}); err != nil {
		return fmt.Errorf("could not register comment_on_pr tool: %w", err)
	}
	return nil
}
