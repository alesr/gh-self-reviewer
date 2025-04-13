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

// PRListRequest represents the parameters for listing pull requests
type PRListRequest struct {
	Dummy string `json:"dummy,omitempty" jsonschema:"description=This field is not used but is required for schema generation"`
}

// PRReviewSubmitRequest represents the parameters for submitting a PR review
type PRReviewSubmitRequest struct {
	PRURL      string `json:"pr_url" jsonschema:"required,description=URL of the pull request to review"`
	ReviewBody string `json:"review_body" jsonschema:"required,description=Content of the review to submit"`
}

// GitHubPRFile represents a file in a pull request with its changes
type GitHubPRFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	Patch       string `json:"patch,omitempty"`
	BlobURL     string `json:"blob_url"`
	ContentsURL string `json:"contents_url"`
}

// GitHubPRContent represents the complete content of a pull request
type GitHubPRContent struct {
	PR          GitHubPR       `json:"pr"`
	Files       []GitHubPRFile `json:"files"`
	Description string         `json:"description"`
}

// PRReviewRequest represents the parameters for reviewing a pull request
type PRReviewRequest struct {
	PRURL string `json:"pr_url" jsonschema:"required,description=URL of the pull request to review"`
}

// PRReview represents a review of a pull request
type PRReview struct {
	Body    string `json:"body"`
	HTMLURL string `json:"html_url,omitempty"`
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

// GetPullRequestContents retrieves the content of a specific pull request
func (h *GithubToolHandler) GetPullRequestContents(ctx context.Context, prURLStr string) (*GitHubPRContent, error) {
	owner, repo, prNumber, err := parsePullRequestURL(prURLStr)
	if err != nil {
		return nil, err
	}

	// get PR details

	pr, _, err := h.client.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR details: %w", err)
	}

	files, _, err := h.client.PullRequests.ListFiles(ctx, owner, repo, prNumber, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to list files in PR: %w", err)
	}

	prFiles := []GitHubPRFile{}
	for _, file := range files {
		prFile := GitHubPRFile{
			Filename:    file.GetFilename(),
			Status:      file.GetStatus(),
			Additions:   file.GetAdditions(),
			Deletions:   file.GetDeletions(),
			Changes:     file.GetChanges(),
			Patch:       file.GetPatch(),
			BlobURL:     file.GetBlobURL(),
			ContentsURL: file.GetContentsURL(),
		}
		prFiles = append(prFiles, prFile)
	}

	prContent := &GitHubPRContent{
		PR: GitHubPR{
			Number:    pr.GetNumber(),
			Title:     pr.GetTitle(),
			URL:       pr.GetHTMLURL(),
			Base:      pr.GetBase().GetRef(),
			Head:      pr.GetHead().GetRef(),
			RepoOwner: owner,
			RepoName:  repo,
		},
		Files:       prFiles,
		Description: pr.GetBody(),
	}
	return prContent, nil
}

// SubmitPullRequestReview submits a review on a pull request
func (h *GithubToolHandler) SubmitPullRequestReview(ctx context.Context, prURLStr string, reviewBody string) (*PRReview, error) {
	owner, repo, prNumber, err := parsePullRequestURL(prURLStr)
	if err != nil {
		return nil, err
	}

	reviewRequest := &github.PullRequestReviewRequest{
		Body:  &reviewBody,
		Event: github.String("COMMENT"),
	}

	review, _, err := h.client.PullRequests.CreateReview(ctx, owner, repo, prNumber, reviewRequest)
	if err != nil {
		return nil, fmt.Errorf("could not submit PR review: %w", err)
	}
	return &PRReview{
		Body:    review.GetBody(),
		HTMLURL: review.GetHTMLURL(),
	}, nil
}

func parsePullRequestURL(prURLStr string) (owner, repo string, prNumber int, err error) {
	parts := strings.Split(prURLStr, "/")
	pullIndex := -1
	for i, part := range parts {
		if part == "pull" {
			pullIndex = i
			break
		}
	}

	if pullIndex == -1 || pullIndex+1 >= len(parts) {
		return "", "", 0, fmt.Errorf("invalid pull request URL: %s", prURLStr)
	}

	owner = parts[pullIndex-2]
	repo = parts[pullIndex-1]
	numberStr := parts[pullIndex+1]

	prNumber, err = strconv.Atoi(numberStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid pull request number in URL: %s", prURLStr)
	}
	return owner, repo, prNumber, nil
}
