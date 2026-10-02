package eval

// Transfer environments (MISSION G12): a transfer case (Case.TransferOf) is a
// second, independently authored environment for the release transition of
// a base case. Its edge is the base case's edge, which the base entry already
// scores, so a transfer entry contributes only its environment: links,
// findings, the confusion cells and the action/unknown accounting of its own
// impact report. Counting its edge again would double every recall,
// precision and duplicate number of the base case.

// adjudicationKey is the case whose adjudication file applies: verdicts are
// about changes of the edge, which a transfer case shares with its base.
func (c *Case) adjudicationKey() string {
	if c.TransferOf != "" {
		return c.TransferOf
	}
	return c.ID
}

// environmentOnly strips the edge-level scoring from a transfer entry (after
// adjudications were applied, so adjudicated-false changes still turn the
// entry's own ACTION findings into false actions).
func (res *EntryResult) environmentOnly() {
	m := &res.Metrics
	m.Expected, m.Found = 0, 0
	m.ExpectedCritical, m.ExpectedImportant = 0, 0
	m.MissedCritical, m.MissedImportant, m.MissedMinor = 0, 0, 0
	m.Changes, m.MatchedChanges, m.FalsePositives, m.DuplicateGroups = 0, 0, 0, 0
	m.ClassificationScored, m.ClassificationMatched = 0, 0
	m.EvidenceCovered = 0
	var findings []UnsupportedAudit
	for _, u := range res.UnsupportedConclusions {
		if u.Kind == "finding" {
			findings = append(findings, u)
		}
	}
	res.UnsupportedConclusions = findings
	m.Unsupported = len(findings)
	res.Matches = nil
	res.FalsePos = nil
	res.Duplicates = nil
	res.Adjudicated = AdjudicationStats{}
	res.FPChangeIDs = nil
	res.AllChangeIDs = nil
}
