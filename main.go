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
		log.Fatalf("Error: %v", err)
	}
}

func run(ctx context.Context) error {
	githubClient, err := makeGitHubClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub client: %w", err)
	}

	server := mcp.NewServer(stdio.NewStdioServerTransport())

	githubToolHandler := gh.NewGithubToolHandler(githubClient)
	if err := registerTools(ctx, server, githubToolHandler); err != nil {
		return fmt.Errorf("could not register tools: %w", err)
	}

	log.Println("Starting MCP server...")

	if err := server.Serve(); err != nil {
		return fmt.Errorf("could not serve MCP server: %w", err)
	}
	return nil
}

func makeGitHubClient(ctx context.Context) (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN_MCP_APP_REVIEW")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN_MCP_APP_REVIEW environment variable is required")
	}
	return github.NewClient(
		oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token},
			),
		),
	), nil
}

func registerTools(ctx context.Context, server *mcp.Server, githubToolHandler *gh.GithubToolHandler) error {
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
