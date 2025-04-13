package gh

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/go-github/v52/github"
)

// GitHubPR represents the structure of a GitHub Pull Request.
type GitHubPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Base      string `json:"base"`
	Head      string `json:"head"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

// GitHubComment represents the structure of a GitHub Comment.
type GitHubComment struct {
	URL  string `json:"url"`
	Body string `json:"body"`
	User string `json:"user"`
}

// PRListRequest represents the parameters for listing pull requests
type PRListRequest struct {
	// empty for now since we're listing all PRs for the authenticated user
}

// PRCommentRequest represents the parameters for commenting on a pull request
type PRCommentRequest struct {
	PRURL string `json:"pr_url" jsonschema:"required,description=URL of the pull request to comment on"`
	Body  string `json:"body" jsonschema:"required,description=Content of the comment to post"`
}

// GithubToolHandler handles requests related to GitHub actions.
type GithubToolHandler struct {
	client *github.Client
}

// NewGithubToolHandler creates a new GithubToolHandler.
func NewGithubToolHandler(client *github.Client) *GithubToolHandler {
	return &GithubToolHandler{
		client: client,
	}
}

// ListMyOpenPullRequestsAcrossRepos lists open PRs authored by the authenticated user.
func (h *GithubToolHandler) ListMyOpenPullRequestsAcrossRepos(ctx context.Context) ([]GitHubPR, error) {
	user, _, err := h.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}

	searchQuery := fmt.Sprintf("is:pr is:open author:%s", user.GetLogin())
	searchOpts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allMyOpenPRs []GitHubPR
	searchResults, _, err := h.client.Search.Issues(ctx, searchQuery, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to search for pull requests: %w", err)
	}

	for _, issue := range searchResults.Issues {
		// extract owner and repo from the repository URL
		prURLParts := strings.Split(issue.GetHTMLURL(), "/")
		if len(prURLParts) < 5 {
			continue
		}
		owner := prURLParts[len(prURLParts)-4]
		repoName := prURLParts[len(prURLParts)-3]

		// get PR details to get base and head refs
		pr, _, err := h.client.PullRequests.Get(ctx, owner, repoName, issue.GetNumber())
		if err != nil {
			log.Printf("failed to get PR details for %s/%s#%d: %v", owner, repoName, issue.GetNumber(), err)
			continue
		}

		allMyOpenPRs = append(allMyOpenPRs, GitHubPR{
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			URL:       issue.GetHTMLURL(),
			Base:      pr.GetBase().GetRef(),
			Head:      pr.GetHead().GetRef(),
			RepoOwner: owner,
			RepoName:  repoName,
		})
	}
	return allMyOpenPRs, nil
}

// CommentOnPullRequestByURL adds a comment to a specific pull request.
func (h *GithubToolHandler) CommentOnPullRequestByURL(ctx context.Context, prURLStr string, body string) (*GitHubComment, error) {
	parts := strings.Split(prURLStr, "/")

	// find the pull index
	pullIndex := -1
	for i, part := range parts {
		if part == "pull" {
			pullIndex = i
			break
		}
	}

	if pullIndex == -1 || pullIndex+1 >= len(parts) {
		return nil, fmt.Errorf("invalid pull request URL: %s", prURLStr)
	}

	owner := parts[pullIndex-2]
	repo := parts[pullIndex-1]
	numberStr := parts[pullIndex+1]

	prNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pull request number in URL: %s", prURLStr)
	}

	comment := github.IssueComment{
		Body: &body,
	}

	ic, _, err := h.client.Issues.CreateComment(ctx, owner, repo, prNumber, &comment)
	if err != nil {
		return nil, fmt.Errorf("could not create comment on PR %s: %w", prURLStr, err)
	}

	return &GitHubComment{
		URL:  ic.GetHTMLURL(),
		Body: ic.GetBody(),
		User: ic.GetUser().GetLogin(),
	}, nil
}
