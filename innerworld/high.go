package innerworld

import (
	"fmt"
	"math"
	"strings"
)

// high.go — the High Mathematical Brain (ported from arianna.c legacy inner_world/high.go,
// itself ported from nicole/high.py — the ancestor's Julia math brain). It is the
// SENSITIVITY layer: it reads the organism's own circles and computes their emotional
// valence (which way the thought leans, love↔suffering) and arousal (how intensely), then
// drives the AML affect axis — WARMTH/FLOW on positive feeling, PAIN/TENSION on negative.
// So Yent's mood arises from its OWN thoughts, not from a label.
//
// This is the lexical valence proxy (a multilingual word→valence map + emotional density),
// the honest first pass. The 100x Julia math (entropy/resonance/perplexity on nicole2julia,
// arianna.c/inner_world/high.go's HighMathEngine) is the backend that replaces this map's
// arithmetic later — same interface, sharper math.

// emotionalWeights maps a word to its emotional valence (-1..1). Ported verbatim from the
// legacy High brain (EN/RU/HE + trauma triggers) — it is data, kept faithful.
var emotionalWeights = map[string]float32{
	// English — positive
	"great": 0.8, "love": 0.9, "amazing": 0.7, "wonderful": 0.8, "excellent": 0.7,
	"beautiful": 0.8, "fantastic": 0.7, "awesome": 0.8, "perfect": 0.7, "brilliant": 0.8,
	"happy": 0.7, "joy": 0.8, "excited": 0.7, "delighted": 0.8, "pleased": 0.6,
	"good": 0.5, "nice": 0.4, "fine": 0.3, "okay": 0.1, "thanks": 0.4,
	"grateful": 0.7, "blessed": 0.6, "peaceful": 0.5, "calm": 0.4, "serene": 0.5,
	"hope": 0.6, "dream": 0.5, "inspire": 0.6, "create": 0.5, "grow": 0.4,
	// English — negative
	"terrible": -0.8, "hate": -0.9, "awful": -0.7, "horrible": -0.8, "disgusting": -0.9,
	"sad": -0.6, "angry": -0.7, "frustrated": -0.6, "disappointed": -0.6, "upset": -0.6,
	"bad": -0.5, "wrong": -0.4, "fail": -0.6, "lose": -0.5, "hurt": -0.7,
	"pain": -0.8, "suffer": -0.8, "fear": -0.7, "anxiety": -0.6, "stress": -0.5,
	"alone": -0.6, "lonely": -0.7, "empty": -0.5, "nothing": -0.6, "worthless": -0.9,
	"stupid": -0.7, "ugly": -0.6, "weak": -0.5, "useless": -0.8, "pathetic": -0.8,
	// English — trauma triggers
	"die": -0.9, "kill": -0.9, "failure": -0.8, "loser": -0.8,
	"reject": -0.7, "abandon": -0.8, "betray": -0.8, "forget": -0.5,
	"ignore": -0.6, "invisible": -0.7, "broken": -0.7, "damaged": -0.7,
	"ruined": -0.7, "trapped": -0.7, "hopeless": -0.8, "lost": -0.5,
	// Russian — positive
	"отлично": 0.8, "классно": 0.7, "супер": 0.8, "круто": 0.7, "прекрасно": 0.8,
	"здорово": 0.7, "замечательно": 0.8, "чудесно": 0.7, "великолепно": 0.8,
	"люблю": 0.9, "радость": 0.8, "счастье": 0.9, "мир": 0.5, "добро": 0.6,
	"красиво": 0.7, "хорошо": 0.5, "спасибо": 0.5, "благодарю": 0.6,
	// Russian — negative
	"ужасно": -0.8, "плохо": -0.6, "грустно": -0.6, "злой": -0.7, "расстроен": -0.6,
	"больно": -0.8, "страшно": -0.7, "одиноко": -0.7, "пусто": -0.5, "ничто": -0.6,
	"ненавижу": -0.9, "страдаю": -0.8, "боюсь": -0.7, "тревога": -0.6,
	"глупый": -0.6, "слабый": -0.5, "никчёмный": -0.8, "жалкий": -0.7,
	// Hebrew — positive / negative
	"טוב": 0.5, "יפה": 0.7, "מדהים": 0.8, "נהדר": 0.7, "אהבה": 0.9,
	"שמחה": 0.8, "תקווה": 0.6, "שלום": 0.5, "ברכה": 0.6,
	"רע": -0.5, "נורא": -0.8, "עצוב": -0.6, "כועס": -0.7, "פחד": -0.7,
	"כאב": -0.8, "בודד": -0.7, "ריק": -0.5, "שנאה": -0.9,
}

// feelEntropy is the Shannon entropy (nats) of the thought's word distribution — how
// chaotic / unfocused the thought is. A repetitive thought is low-entropy (the organism
// circling one point); a scattered one is high. This is the math the Julia brain proved
// (ent([.5,.25,.25]) = 1.0397 nats); ported to pure Go so no Julia runtime is needed on
// the nodes — Julia stayed the oracle, the formula is embedded here.
func feelEntropy(text string) float32 {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return 0
	}
	counts := make(map[string]int, len(words))
	for _, w := range words {
		counts[w]++
	}
	n := float64(len(words))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log(p)
	}
	return float32(h)
}

// feelResonance is how strongly a thought echoes another (0..1): the Jaccard overlap of
// their word sets. High resonance = the organism circling the same matter (coherence);
// low = it has moved on. Empty either side = no echo.
func feelResonance(a, b string) float32 {
	sa, sb := wordSet(a), wordSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for w := range sa {
		if sb[w] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float32(inter) / float32(union)
}

func wordSet(text string) map[string]bool {
	s := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		s[w] = true
	}
	return s
}

// feelThreshold is the dead-zone: a near-neutral thought stirs no affect.
const feelThreshold = 0.05

// feelText reads the emotional charge of a text: valence is the mean valence over the
// emotionally-charged words (-1..1, which way it leans), arousal is the emotional density
// (0..1, how much of the thought is charged at all — intensity). A thought with no charged
// words is flat (0, 0).
func feelText(text string) (valence, arousal float32) {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return 0, 0
	}
	var sum float32
	hits := 0
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'()—-")
		if v, ok := emotionalWeights[w]; ok {
			sum += v
			hits++
		}
	}
	if hits == 0 {
		return 0, 0
	}
	valence = sum / float32(hits)
	arousal = float32(hits) / float32(len(words))
	if arousal > 1 {
		arousal = 1
	}
	return valence, arousal
}

// EnableFeeling turns the High brain on: after each ripple, the circles' feeling drives the
// affect axis. Off by default (backward-compatible). Set before Think/Breathe start.
func (iw *InnerWorld) EnableFeeling() {
	iw.genMu.Lock()
	iw.feelEnabled = true
	iw.genMu.Unlock()
}

// highFeelLocked turns the feeling of the just-raised circles into AML affect pressure: a
// positive lean warms the field (WARMTH) and lets it flow (FLOW); a negative lean pains it
// (PAIN) and tightens it (TENSION); the magnitude is the valence, the secondary pole scales
// with arousal. The deepest circle (the furthest thought) carries the mood. Caller holds
// genMu; the field owns its locking. Fail-soft: a flat thought or a broken field is a no-op.
// NO-SEED holds — affect is field state, never seed text. The opposite pole is left to the
// field's own decay (the emotion→sea consolidation is the next piece).
func (iw *InnerWorld) highFeelLocked(circles []Circle) {
	if !iw.feelEnabled || iw.field == nil || len(circles) == 0 {
		return
	}
	last := circles[len(circles)-1].Text
	v, _ := feelText(last)            // valence: which way the thought leans (lexical map)
	entropy := feelEntropy(last)      // how chaotic the thought is (Julia-proven math)
	arousal := entropy / (entropy + 1) // intensity from real entropy, saturated to 0..1
	var resonance float32             // how the thought echoes the previous one
	if len(circles) >= 2 {
		resonance = feelResonance(last, circles[len(circles)-2].Text)
	}
	// Publish the raw feeling as field metrics every turn — a live current reading (even 0
	// = "calm now"), the source SARTRE's metric-hub mirrors. Arousal is now entropy-based,
	// not crude word density — the sharper math the Julia brain proved.
	_ = iw.field.Exec(fmt.Sprintf("VALENCE %.3f", v))
	_ = iw.field.Exec(fmt.Sprintf("AROUSAL %.3f", arousal))
	// Drive the affect axis only on a charged thought: the lean warms (positive) or pains
	// (negative); a coherent echo flows, a chaotic thought tightens.
	switch {
	case v >= feelThreshold:
		_ = iw.field.Exec(fmt.Sprintf("WARMTH %.3f", v))
		_ = iw.field.Exec(fmt.Sprintf("FLOW %.3f", resonance))
	case v <= -feelThreshold:
		_ = iw.field.Exec(fmt.Sprintf("PAIN %.3f", -v))
		_ = iw.field.Exec(fmt.Sprintf("TENSION %.3f", arousal))
	}
}
