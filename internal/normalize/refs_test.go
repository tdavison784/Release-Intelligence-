package normalize

import (
	"reflect"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestExtractReferences(t *testing.T) {
	tests := []struct {
		name string
		text string
		repo string
		want []domain.Reference
	}{
		{name: "empty", text: "", want: nil},
		{name: "nothing", text: "just words, v1.2.3 and #hashtag", repo: "o/r", want: nil},
		{name: "cve", text: "fixes CVE-2025-27144 and cve-2024-1234567", want: []domain.Reference{
			{Type: "cve", ID: "CVE-2025-27144", URL: "https://www.cve.org/CVERecord?id=CVE-2025-27144"},
			{Type: "cve", ID: "CVE-2024-1234567", URL: "https://www.cve.org/CVERecord?id=CVE-2024-1234567"},
		}},
		{name: "cve needs 4+ digits", text: "CVE-2025-123 is not an id", want: nil},
		{name: "ghsa", text: "see GHSA-hcg3-q754-cr77 and ghsa-8rvj-mm4h-c258.", want: []domain.Reference{
			{Type: "ghsa", ID: "GHSA-hcg3-q754-cr77", URL: "https://github.com/advisories/GHSA-hcg3-q754-cr77"},
			{Type: "ghsa", ID: "GHSA-8rvj-mm4h-c258", URL: "https://github.com/advisories/GHSA-8rvj-mm4h-c258"},
		}},
		{name: "ghsa inside advisory url", text: "https://github.com/istio/istio/security/advisories/GHSA-qm8v-g4f9-qhjx", want: []domain.Reference{
			{Type: "ghsa", ID: "GHSA-qm8v-g4f9-qhjx", URL: "https://github.com/advisories/GHSA-qm8v-g4f9-qhjx"},
		}},
		{name: "pull and issue urls", text: "https://github.com/cert-manager/cert-manager/pull/7810 and (https://github.com/istio/istio/issues/60122).", want: []domain.Reference{
			{Type: "pull-request", ID: "cert-manager/cert-manager#7810", URL: "https://github.com/cert-manager/cert-manager/pull/7810"},
			{Type: "issue", ID: "istio/istio#60122", URL: "https://github.com/istio/istio/issues/60122"},
		}},
		{name: "bare refs need a repository", text: "fixed in #1234", want: nil},
		{name: "bare ref", text: "fixed in #1234, see [#77] and (#5).", repo: "o/r", want: []domain.Reference{
			{Type: "github-ref", ID: "o/r#1234", URL: "https://github.com/o/r/issues/1234"},
			{Type: "github-ref", ID: "o/r#77", URL: "https://github.com/o/r/issues/77"},
			{Type: "github-ref", ID: "o/r#5", URL: "https://github.com/o/r/issues/5"},
		}},
		{name: "bare ref in start of text and backticks", text: "#42 `#43`", repo: "o/r", want: []domain.Reference{
			{Type: "github-ref", ID: "o/r#42", URL: "https://github.com/o/r/issues/42"},
			{Type: "github-ref", ID: "o/r#43", URL: "https://github.com/o/r/issues/43"},
		}},
		{name: "repository forms", text: "#9", repo: "github.com/o/r", want: []domain.Reference{{Type: "github-ref", ID: "o/r#9", URL: "https://github.com/o/r/issues/9"}}},
		{name: "repository url form", text: "#9", repo: "https://github.com/o/r.git", want: []domain.Reference{{Type: "github-ref", ID: "o/r#9", URL: "https://github.com/o/r/issues/9"}}},
		{name: "non-github repository", text: "#9", repo: "gitlab.com/o/r", want: nil},
		{name: "not refs", text: "&#123; foo.md#12 a#13 /x/#14 color #12abc #1a", repo: "o/r", want: nil},
		{name: "pr url folds the bare number of the same repo", text: "[`#7810`](https://github.com/o/r/pull/7810) and [#7811]", repo: "o/r", want: []domain.Reference{
			{Type: "pull-request", ID: "o/r#7810", URL: "https://github.com/o/r/pull/7810"},
			{Type: "github-ref", ID: "o/r#7811", URL: "https://github.com/o/r/issues/7811"},
		}},
		{name: "other repository keeps both", text: "[#7810](https://github.com/other/repo/pull/7810)", repo: "o/r", want: []domain.Reference{
			{Type: "github-ref", ID: "o/r#7810", URL: "https://github.com/o/r/issues/7810"},
			{Type: "pull-request", ID: "other/repo#7810", URL: "https://github.com/other/repo/pull/7810"},
		}},
		{name: "cross repository", text: "see kubernetes/ingress-nginx#11176 and cert-manager/cert-manager#3851", want: []domain.Reference{
			{Type: "github-ref", ID: "kubernetes/ingress-nginx#11176", URL: "https://github.com/kubernetes/ingress-nginx/issues/11176"},
			{Type: "github-ref", ID: "cert-manager/cert-manager#3851", URL: "https://github.com/cert-manager/cert-manager/issues/3851"},
		}},
		{name: "de-duplication and order of appearance", text: "GHSA-hcg3-q754-cr77 CVE-2025-1111 CVE-2025-1111 ghsa-hcg3-q754-cr77 #3 #3", repo: "o/r", want: []domain.Reference{
			{Type: "ghsa", ID: "GHSA-hcg3-q754-cr77", URL: "https://github.com/advisories/GHSA-hcg3-q754-cr77"},
			{Type: "cve", ID: "CVE-2025-1111", URL: "https://www.cve.org/CVERecord?id=CVE-2025-1111"},
			{Type: "github-ref", ID: "o/r#3", URL: "https://github.com/o/r/issues/3"},
		}},

		// commit URLs (git-log fallback bullets)
		{name: "git-log bullet: PR number and commit", repo: "o/r",
			text: "feat(appset): add foo (#1234) ([abc1234](https://github.com/o/r/commit/abc1234def5678abc1234def5678abc1234def56))",
			want: []domain.Reference{
				{Type: "github-ref", ID: "o/r#1234", URL: "https://github.com/o/r/issues/1234"},
				{Type: "commit", ID: "abc1234", URL: "https://github.com/o/r/commit/abc1234def5678abc1234def5678abc1234def56"},
			}},
		{name: "commit url without a repository", text: "[abc1234](https://github.com/argoproj/argo-cd/commit/abc1234def5678)", want: []domain.Reference{
			{Type: "commit", ID: "abc1234", URL: "https://github.com/argoproj/argo-cd/commit/abc1234def5678"},
		}},
		{name: "commit url with a 7-character sha", text: "https://github.com/o/r/commit/abc1234.", want: []domain.Reference{
			{Type: "commit", ID: "abc1234", URL: "https://github.com/o/r/commit/abc1234"},
		}},
		{name: "commit id is lower-case and 7 characters", text: "https://github.com/o/r/commit/ABCDEF1234567890", want: []domain.Reference{
			{Type: "commit", ID: "abcdef1", URL: "https://github.com/o/r/commit/ABCDEF1234567890"},
		}},
		{name: "gitlab commit url", text: "([abc1234](https://gitlab.com/group/proj/-/commit/abc1234def5678abc1234def5678abc1234def56))", want: []domain.Reference{
			{Type: "commit", ID: "abc1234", URL: "https://gitlab.com/group/proj/-/commit/abc1234def5678abc1234def5678abc1234def56"},
		}},
		{name: "gitlab commit url with subgroups on a self-hosted instance", text: "https://gitlab.example.com/a/b/c/proj/-/commit/0123456789abcdef", want: []domain.Reference{
			{Type: "commit", ID: "0123456", URL: "https://gitlab.example.com/a/b/c/proj/-/commit/0123456789abcdef"},
		}},
		{name: "gitlab commit url without /-/", text: "https://gitlab.com/group/proj/commit/0123456789abcdef", want: []domain.Reference{
			{Type: "commit", ID: "0123456", URL: "https://gitlab.com/group/proj/commit/0123456789abcdef"},
		}},
		{name: "commit urls are de-duplicated and keep the order of appearance", repo: "o/r",
			text: "CVE-2025-1111 [a](https://github.com/o/r/commit/1111111aaaa) #5 [b](https://github.com/o/r/commit/2222222bbbb) [a again](https://github.com/o/r/commit/1111111aaaa)",
			want: []domain.Reference{
				{Type: "cve", ID: "CVE-2025-1111", URL: "https://www.cve.org/CVERecord?id=CVE-2025-1111"},
				{Type: "commit", ID: "1111111", URL: "https://github.com/o/r/commit/1111111aaaa"},
				{Type: "github-ref", ID: "o/r#5", URL: "https://github.com/o/r/issues/5"},
				{Type: "commit", ID: "2222222", URL: "https://github.com/o/r/commit/2222222bbbb"},
			}},
		{name: "commit and pull request urls", text: "https://github.com/o/r/pull/7 https://github.com/o/r/commit/abcdef0", want: []domain.Reference{
			{Type: "pull-request", ID: "o/r#7", URL: "https://github.com/o/r/pull/7"},
			{Type: "commit", ID: "abcdef0", URL: "https://github.com/o/r/commit/abcdef0"},
		}},
		{name: "a commit inside a pull request is a pull request reference", text: "https://github.com/o/r/pull/7/commits/abcdef0123", want: []domain.Reference{
			{Type: "pull-request", ID: "o/r#7", URL: "https://github.com/o/r/pull/7"},
		}},
		{name: "not commit urls", text: "https://github.com/o/r/commit/abc12 https://github.com/o/r/commit/ghijklm https://github.com/o/r/commits/abcdef0 " +
			"https://github.com/o/r/compare/v1...v2 https://bitbucket.org/o/r/commit/abcdef0 https://example.com/o/r/commit/abcdef0 https://github.com/o/commit/abcdef0", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.text, tt.repo)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
