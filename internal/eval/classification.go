package eval

// The confusion matrix (G9): expected class × actual class over the findings
// a run produced for ground-truth-labelled units, with the severity weighting
// of docs/ACTION_CLASSIFICATION.md. Not every wrong cell is equally wrong:
// telling an operator "nothing to do" about work that must be done
// (ACTION → NOT AFFECTED) is catastrophic; flagging clear work as needing a
// look (ACTION → REVIEW) is tolerable but imperfect. The weights below are
// the dataset's pre-registered quantification of that asymmetry.

// ConfusionMatrix is the expected×actual cell counts over labelled findings.
// Rows[expected][actual]; Classes is the display order (all five classes).
type ConfusionMatrix struct {
	Classes []string `json:"classes"`
	Rows    [][]int  `json:"rows"`
	// Labelled is the number of findings that contributed a cell.
	Labelled int `json:"labelled"`
	// WeightedMiss sums cell count × severity weight over the off-diagonal
	// cells (docs below; 0 for a perfect run).
	WeightedMiss float64 `json:"weightedMiss"`
}

// Severity weights for expected→actual pairs. The four weights the plan
// fixes are marked; the others follow the same logic (denying required
// action and inventing required action are the worst; unknown instead of a
// checked answer is next; confusing adjacent "look at this" degrees is
// cheapest). An on-diagonal cell weighs 0.
var missWeights = map[string]float64{
	// Plan (normative):
	//   ACTION → NOT AFFECTED  catastrophic
	//   ACTION → UNKNOWN       serious
	//   ACTION → REVIEW        tolerable but imperfect
	//   NOT AFFECTED → ACTION  serious false alarm
	ClassActionRequired + "->" + ClassNotAffected:    10, // catastrophic
	ClassActionRequired + "->" + ClassUnknown:        5,  // serious
	ClassActionRequired + "->" + ClassReviewRequired: 1,  // tolerable but imperfect
	ClassNotAffected + "->" + ClassActionRequired:    5,  // serious false alarm

	// Consequential, by the same logic:
	ClassActionRequired + "->" + ClassInformational:  5, // as serious as ACTION → UNKNOWN: the operator is told nothing is needed
	ClassReviewRequired + "->" + ClassNotAffected:    3, // a needed look is waved away
	ClassReviewRequired + "->" + ClassUnknown:        2,
	ClassInformational + "->" + ClassNotAffected:     2,
	ClassUnknown + "->" + ClassNotAffected:           3, // the classic sin: "cannot tell" silently becomes "checked, clear"
	ClassNotAffected + "->" + ClassReviewRequired:    2,
	ClassNotAffected + "->" + ClassUnknown:           1,
	ClassNotAffected + "->" + ClassInformational:     1,
	ClassReviewRequired + "->" + ClassActionRequired: 2, // escalates a look into an order — false alarm, less serious than NOT AFFECTED → ACTION
	ClassInformational + "->" + ClassActionRequired:  2,
	ClassInformational + "->" + ClassReviewRequired:  1,
	ClassInformational + "->" + ClassUnknown:         1,
	ClassUnknown + "->" + ClassActionRequired:        2,
	ClassUnknown + "->" + ClassReviewRequired:        1,
	ClassUnknown + "->" + ClassInformational:         1,
	ClassReviewRequired + "->" + ClassInformational:  1,
}

// MissWeight is the severity weight of one expected→actual cell (0 on the
// diagonal).
func MissWeight(expected, actual string) float64 {
	if expected == actual {
		return 0
	}
	return missWeights[expected+"->"+actual]
}

// NewConfusionMatrix returns an empty matrix in display order.
func NewConfusionMatrix() *ConfusionMatrix {
	m := &ConfusionMatrix{Classes: append([]string(nil), AllImpactClassOrder...)}
	m.Rows = make([][]int, len(m.Classes))
	for i := range m.Rows {
		m.Rows[i] = make([]int, len(m.Classes))
	}
	return m
}

// AllImpactClassOrder is the matrix display order (action first, mirroring
// docs/ACTION_CLASSIFICATION.md).
var AllImpactClassOrder = []string{ClassActionRequired, ClassReviewRequired, ClassInformational, ClassNotAffected, ClassUnknown}

// Add records one labelled finding.
func (m *ConfusionMatrix) Add(expected, actual string) {
	ri, ok1 := m.indexOf(expected)
	ci, ok2 := m.indexOf(actual)
	if !ok1 || !ok2 {
		return // unlabelled cells never enter the matrix
	}
	m.Rows[ri][ci]++
	m.Labelled++
	m.WeightedMiss += MissWeight(expected, actual)
}

func (m *ConfusionMatrix) indexOf(c string) (int, bool) {
	for i, x := range m.Classes {
		if x == c {
			return i, true
		}
	}
	return 0, false
}

// Accuracy over the labelled cells (1.0 when nothing is labelled — vacuous,
// which the gates treat explicitly).
func (m *ConfusionMatrix) Accuracy() float64 {
	if m.Labelled == 0 {
		return 1
	}
	correct := 0
	for i := range m.Rows {
		correct += m.Rows[i][i]
	}
	return float64(correct) / float64(m.Labelled)
}
