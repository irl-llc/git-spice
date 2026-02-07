package bitbucket_test

import (
	"net/http"
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
	const remoteURL = "https://bitbucket.org/shambucket/shambucket.git"

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
		RemoteURL: remoteURL,
		Forge:     &bitbucketForge,
		OpenRepository: func(t *testing.T, httpClient *http.Client) forge.Repository {
			_, token, source := forgetest.Credential(
				t, remoteURL, "BITBUCKET_EMAIL", "BITBUCKET_TOKEN",
			)
			authType := bitbucket.AuthTypeAPIToken
			if source == forgetest.CredentialSourceGCM {
				authType = bitbucket.AuthTypeGCM
			}
			return bitbucket.NewRepositoryForTest(
				&bitbucketForge,
				bitbucket.DefaultURL,
				"shambucket", "shambucket",
				silogtest.New(t),
				httpClient,
				&bitbucket.AuthenticationToken{
					AuthType:    authType,
					AccessToken: token,
				},
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
		Reviewers:           []string{"Ed IRL Kohlwey"},
		Assignees:           []string{},
		// Bitbucket limitations:
		SkipLabels:            true, // Bitbucket does not support PR labels
		SkipAssignees:         true, // Bitbucket does not support PR assignees
		SkipTemplates:         true, // Bitbucket has limited template support
		ShortHeadHash:         true, // Bitbucket API returns 12-char hashes
		SkipMerge:             true, // Merge requires repository-specific branch permissions
		SkipCommentPagination: true, // Bitbucket returns 403 with small page sizes
	})
}
