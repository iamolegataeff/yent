package yent

// body_router.go — moyent's two-body router: one organism, two swappable Mistral
// bodies sharing one limpha brain. The fast body (Nemo-12B) speaks by default; the
// router escalates to the deep body (Small-24B) when prompt complexity or the fast
// body's own uncertainty demands it. NEVER both resident at once — a Body is an
// interface the concrete persistent doe-daemon driver satisfies; the router only
// orchestrates and logs, so it is testable without a model.
//
// Every turn is logged into the shared limpha brain: plain turns -> conversations;
// dual-pass (escalated) turns also write a seam — the internal dialogue between the
// two bodies (a_claim/b_claim) plus the divergence metrics (agreement/tension/winner)
// the deep body scored. Supergamma later grows from that seam log.

import (
	"strings"
	"unicode/utf8"
)

// Body is one Mistral inference body behind the router. The production implementation
// is a persistent doe daemon (model resident, swapped on demand). Tests use fakes.
type Body interface {
	// Name identifies the body in logs and seams, e.g. "nemo12" or "small24".
	Name() string
	// Generate answers prompt. ctx carries cross-body context on escalation: the fast
	// body's trace, resonant memory references, and the routing reason. The fast body
	// is always called with an empty ctx.
	Generate(prompt, ctx string) (BodyResult, error)
}

// BodyResult is one body's output for a turn.
type BodyResult struct {
	Answer string
	// Confidence is the body's self-signal in [0,1]. The router escalates when the
	// fast body reports low confidence (its entropy / top-logit-margin proxy).
	Confidence float64
	// Verdict is the deep body's reflection on the fast body's trace — set only when
	// the deep body ran with that trace in ctx. The deep body scores agreement and
	// tension and names the winner; the router copies it into the seam.
	Verdict *Verdict
}

// Verdict is the deep body's scoring of the divergence with the fast body.
type Verdict struct {
	Agreement float64 // 0..1
	Tension   float64 // 0..1
	Winner    string  // body name whose answer is used
}

// Router orchestrates the two bodies over one shared limpha brain.
type Router struct {
	fast   Body          // default mouth, e.g. nemo12
	deep   Body          // escalation cortex, e.g. small24
	limpha *LimphaClient // shared brain; may be nil (logging disabled)
	// EscalateBelow: if the fast body's Confidence is below this, escalate. [0,1].
	EscalateBelow float64
}

// NewRouter wires two bodies to one limpha brain. EscalateBelow defaults to 0.5.
func NewRouter(fast, deep Body, limpha *LimphaClient) *Router {
	return &Router{fast: fast, deep: deep, limpha: limpha, EscalateBelow: 0.5}
}

// Outcome is the router's decision for a turn (returned to the caller and tests).
type Outcome struct {
	Answer    string
	Body      string // which body's answer was used
	Escalated bool
	Reason    string // why escalated (empty if not)
	SeamID    int64  // >0 if a seam was written
}

// escalationReason decides whether the fast attempt must escalate to the deep body.
// Two falsifiable signals (design canon): the fast body's own low confidence, and a
// prompt-complexity heuristic. Returns "" when the fast body may answer alone.
func escalationReason(prompt string, fast BodyResult, threshold float64) string {
	if fast.Confidence < threshold {
		return "low_confidence"
	}
	if promptIsComplex(prompt) {
		return "complexity"
	}
	return ""
}

// promptIsComplex is a cheap, falsifiable v1 heuristic: long or code/architecture/
// planning prompts route to the deep body. The real entropy/margin signal lives in
// the fast body's Confidence; this only catches obvious depth from the prompt text.
func promptIsComplex(prompt string) bool {
	if utf8.RuneCountInString(prompt) > 600 {
		return true
	}
	p := strings.ToLower(prompt)
	for _, kw := range []string{"architecture", "prove", "design ", "step by step", "refactor", "algorithm"} {
		if strings.Contains(p, kw) {
			return true
		}
	}
	return false
}

// Route runs one turn: the fast body answers; if complexity or low confidence demands
// it, the deep body re-answers with the fast trace + memory refs + reason, scores the
// divergence, and a seam is logged. Returns the chosen answer.
func (r *Router) Route(prompt string, st LimphaState) (Outcome, error) {
	fast, err := r.fast.Generate(prompt, "")
	if err != nil {
		return Outcome{}, err
	}
	reason := escalationReason(prompt, fast, r.EscalateBelow)
	if reason == "" {
		// single-body turn: the fast body answers alone.
		r.storeConversation(prompt, fast.Answer, st)
		return Outcome{Answer: fast.Answer, Body: r.fast.Name(), Escalated: false}, nil
	}

	// dual-pass: the deep body reflects on the fast trace + memory refs + reason.
	ctx := r.escalationContext(prompt, fast, reason)
	deep, err := r.deep.Generate(prompt, ctx)
	if err != nil {
		// deep failed — keep the fast answer rather than dropping the turn.
		r.storeConversation(prompt, fast.Answer, st)
		return Outcome{Answer: fast.Answer, Body: r.fast.Name(), Escalated: true, Reason: reason}, nil
	}

	winner := r.deep.Name()
	agreement, tension := 0.0, 0.0
	if deep.Verdict != nil {
		agreement, tension = deep.Verdict.Agreement, deep.Verdict.Tension
		if deep.Verdict.Winner != "" {
			winner = deep.Verdict.Winner
		}
	}
	answer := deep.Answer
	if winner == r.fast.Name() { // deep conceded — the fast answer stands
		answer = fast.Answer
	}
	convID := r.storeConversation(prompt, answer, st)
	seamID := r.storeSeam(convID, prompt, fast.Answer, deep.Answer, agreement, tension, winner, reason)
	return Outcome{Answer: answer, Body: winner, Escalated: true, Reason: reason, SeamID: seamID}, nil
}

// escalationContext is what the deep body receives: the fast body's trace, the routing
// reason, and any resonant memory references the shared brain holds for this prompt.
func (r *Router) escalationContext(prompt string, fast BodyResult, reason string) string {
	var b strings.Builder
	b.WriteString("[routing reason: " + reason + "]\n")
	b.WriteString("[" + r.fast.Name() + " said]: " + fast.Answer + "\n")
	if r.limpha != nil {
		if refs, _ := r.limpha.Search(prompt, 3); len(refs) > 0 {
			b.WriteString("[memory refs]:\n")
			for _, m := range refs {
				if p, ok := m["prompt"].(string); ok {
					b.WriteString("- " + p + "\n")
				}
			}
		}
	}
	return b.String()
}

// storeConversation persists a turn into the shared brain; 0 if memory is off.
func (r *Router) storeConversation(prompt, answer string, st LimphaState) int64 {
	if r.limpha == nil || !r.limpha.connected {
		return 0
	}
	id, _ := r.limpha.store(prompt, answer, st)
	return id
}

// storeSeam records the internal dialogue + divergence metrics for a dual-pass turn.
func (r *Router) storeSeam(convID int64, prompt, aClaim, bClaim string, agreement, tension float64, winner, reason string) int64 {
	if r.limpha == nil {
		return 0
	}
	id, _ := r.limpha.StoreSeam(Seam{
		ConversationID: convID, BodyA: r.fast.Name(), BodyB: r.deep.Name(),
		Prompt: prompt, AClaim: aClaim, BClaim: bClaim,
		Agreement: agreement, Tension: tension, Winner: winner, Reason: reason,
	})
	return id
}
