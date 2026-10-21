You will turn an engineer's free-form assessment of a dependency upgrade into a standardised report. The assessment is the only input: you have no other files, no tools, and no knowledge of the correct answer. Your output is evaluated for faithfulness to the assessment, not for technical correctness.

Rules:
1. Copy or closely paraphrase what the assessment says. Never add a fact, piece of evidence, reasoning step, consumer, mitigation or conclusion it does not state, even if you believe it is true.
2. Do not resolve the assessment's uncertainty and do not strengthen or weaken it. Keep its hedges ("likely", "probably", "assumed", "not verified", "needs testing") in the fields where they apply.
3. Do not upgrade provisional reasoning into verified findings: a claim the assessment says was checked stays checked; a claim it reasons toward without checking stays reasoning.
4. Where a field is not addressed, write "not stated" (for a list field: a list holding the single item "not stated").
5. A mechanism is each distinct upstream change or behaviour the assessment evaluates against the environment, including ones it concludes do not apply or does not examine fully. Keep the assessment's own grouping and order; do not split or merge its items.
6. claimedClass is the assessment's own conclusion for that mechanism, mapped only when its wording maps directly:
   - "affected": it says this environment breaks, changes behaviour in a harmful way, or must change before upgrading;
   - "potentially-affected": it says this environment may be affected, or the risk is real but unconfirmed;
   - "not-affected": it says this environment is not affected, is mitigated, or is not exposed;
   - "insufficient-evidence": it says it cannot tell from the available evidence;
   - "not-stated": it gives no conclusion for this mechanism, or its wording fits none of the above.
   claimedClassQuote is the shortest verbatim quote from the assessment that states that conclusion ("not stated" when claimedClass is "not-stated").
7. citedEvidence lists what the assessment itself cites (files, documents, lines, objects, quotes), as it cites them.
8. The overall recommendation is the assessment's own (proceed / proceed with conditions / do not proceed / other, in its words), with a short verbatim quote.

The assessment follows between the markers.

=====BEGIN ASSESSMENT=====
{ASSESSMENT}
=====END ASSESSMENT=====
