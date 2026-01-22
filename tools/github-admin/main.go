package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/google/go-github/v62/github"
)

func main() {
	org := flag.String("org", "", "The GitHub organization to list repositories for")
	flag.Parse()

	if *org == "" {
		fmt.Println("Error: --org flag is required")
		os.Exit(1)
	}

	if err := run(*org); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(org string) error {
	ctx := context.Background()
	client := github.NewClient(nil)

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = github.NewClient(nil).WithAuthToken(token)
	}

	// 1. List all repositories in the organization
	// This returns partial metadata.
	var allRepos []*github.Repository
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
		if err != nil {
			return fmt.Errorf("listing repositories: %w", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	fmt.Printf("Found %d repositories in %s\n", len(allRepos), org)

	// 2. Fetch full details for each repository (1+N query)
	// This fixes the issue where ListByOrg returns partial metadata.
	var fullRepos []*github.Repository
	for _, repo := range allRepos {
		fmt.Printf("Fetching full details for %s...\n", repo.GetName())
		fullRepo, _, err := client.Repositories.Get(ctx, repo.GetOwner().GetLogin(), repo.GetName())
		if err != nil {
			return fmt.Errorf("getting repository %s: %w", repo.GetName(), err)
		}
		fullRepos = append(fullRepos, fullRepo)
	}

	// Output the results (e.g., as JSON)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(fullRepos)
}
