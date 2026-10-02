package semantic

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Vocabulary renders the typed vocabulary of the given aspects (subject
// families, change types, condition ops and states, consequence kinds): the
// same text the proposers were shown. The proxy reviewer (internal/proxyreview)
// shows it so a correction speaks the proposers' language.
func Vocabulary(aspects []domain.Aspect) string {
	var b strings.Builder
	renderVocabulary(&b, aspects)
	return b.String()
}
