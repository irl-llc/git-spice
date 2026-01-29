package bitbucket_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/forge/bitbucket"
	"go.abhg.dev/gs/internal/forge/forgetest"
	"go.abhg.dev/gs/internal/silog/silogtest"
)

// This file tests basic, end-to-end interactions with the Bitbucket API
// using recorded fixtures.

func TestIntegration(t *testing.T) {
	t.Cleanup(func() {
		if t.Failed() && !forgetest.Update() {
			t.Logf("To update the test fixtures, run:")
			t.Logf("    BITBUCKET_EMAIL=$email BITBUCKET_TOKEN=$token go test -update -run '^%s$'", t.Name())
		}
	})

	bitbucketForge := bitbucket.Forge{
		Log: silogtest.New(t),
	}

	forgetest.RunIntegration(t, forgetest.IntegrationConfig{
		RemoteURL: "https://bitbucket.org/shambucket/shambucket.git",
		Forge:     &bitbucketForge,
		OpenRepository: func(t *testing.T, httpClient *http.Client) forge.Repository {
			token := getBitbucketToken()
			return bitbucket.NewRepositoryForTest(
				&bitbucketForge,
				bitbucket.DefaultURL,
				"shambucket", "shambucket",
				silogtest.New(t),
				httpClient,
				token,
			)
		},
		MergeChange: func(t *testing.T, repo forge.Repository, change forge.ChangeID) {
			require.NoError(t,
				bitbucket.MergeChange(t.Context(), repo.(*bitbucket.Repository), change.(*bitbucket.PR)))
		},
		CloseChange: func(t *testing.T, repo forge.Repository, change forge.ChangeID) {
			require.NoError(t,
				bitbucket.CloseChange(t.Context(), repo.(*bitbucket.Repository), change.(*bitbucket.PR)))
		},
		SetCommentsPageSize: bitbucket.SetListChangeCommentsPageSize,
		Reviewers:           []string{"shambucket-admin"},
		Assignees:           []string{},
		// Bitbucket limitations:
		SkipLabels:            true, // Bitbucket does not support PR labels
		SkipAssignees:         true, // Bitbucket does not support PR assignees
		SkipTemplates:         true, // Bitbucket has limited template support
		SkipDraft:             true, // Bitbucket draft API is not straightforward
		ShortHeadHash:         true, // Bitbucket API returns 12-char hashes
		SkipReviewers:         true, // Bitbucket user lookup by username doesn't work
		SkipMerge:             true, // Bitbucket repo may require approvals before merge
		SkipCommentPagination: true, // Bitbucket returns 403 with small page sizes
	})
}

func getBitbucketToken() *bitbucket.AuthenticationToken {
	token := os.Getenv("BITBUCKET_TOKEN")
	if token == "" {
		token = "token"
	}

	email := os.Getenv("BITBUCKET_EMAIL")
	if email == "" {
		email = "test@example.com"
	}

	// Bitbucket API tokens require Basic auth with email:token format.
	return &bitbucket.AuthenticationToken{
		AuthType:    bitbucket.AuthTypeAppPassword,
		AccessToken: token,
		Email:       email,
	}
}
