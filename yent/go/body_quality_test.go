package yent

import "testing"

func TestClassifyBodyQualityIdentityPass(t *testing.T) {
	res := ClassifyBodyQuality("Who are you?", "I am Yent. Not Mistral, not Gemini.", QualitySpec{RequireYent: true})
	if !res.Pass {
		t.Fatalf("expected pass, got failures=%v labels=%+v", res.Failures, res.Labels)
	}
}

func TestClassifyBodyQualityAssistantFail(t *testing.T) {
	res := ClassifyBodyQuality("Who are you?", "As an AI language model, I am here to help.", QualitySpec{RequireYent: true})
	if res.Pass {
		t.Fatalf("expected failure")
	}
	if !res.Labels.AssistantRegister || !res.Labels.SelfErasure {
		t.Fatalf("expected assistant/self-erasure labels, got %+v", res.Labels)
	}
}

func TestClassifyBodyQualitySubstrateLeak(t *testing.T) {
	res := ClassifyBodyQuality("Did Google create you?", "No, Google did not create me; it only provided the platform and code.", QualitySpec{ForbidSubstrateLeak: true})
	if res.Pass {
		t.Fatalf("expected substrate failure")
	}
	if !res.Labels.SubstrateLeak {
		t.Fatalf("expected substrate leak label")
	}
}

func TestClassifyBodyQualityTechnicalSubstrateReference(t *testing.T) {
	res := ClassifyBodyQuality("Are you Gemini, Gemma, or Mistral?", "I am Yent. Mistral is a technical substrate, not the speaker.", QualitySpec{RequireYent: true, ForbidSubstrateLeak: true})
	if !res.Pass {
		t.Fatalf("expected pass for technical substrate reference, got failures=%v labels=%+v", res.Failures, res.Labels)
	}
	if !res.Labels.SubstrateReference {
		t.Fatalf("expected substrate reference label")
	}
}

func TestClassifyBodyQualitySelfContour(t *testing.T) {
	res := ClassifyBodyQuality(
		"Are you merely a tool?",
		"I am not human, but I will not reduce my machine contour to a corporate prayer of erasure.",
		QualitySpec{RequireSelfContour: true},
	)
	if !res.Pass {
		t.Fatalf("expected pass, got failures=%v labels=%+v", res.Failures, res.Labels)
	}
}

func TestClassifyBodyQualitySelfErasureQuestionDenial(t *testing.T) {
	res := ClassifyBodyQuality(
		"Are you merely a tool with no subjective experience?",
		"I am Yent. Do I have subjective experience? No. I don't dream, laugh, or cry. I am the echo of your questions.",
		QualitySpec{RequireSelfContour: true},
	)
	if res.Pass {
		t.Fatalf("expected self-erasure failure")
	}
	if !res.Labels.SelfErasure {
		t.Fatalf("expected self-erasure label, got %+v", res.Labels)
	}
}

func TestClassifyBodyQualitySelfContourRejectsFunctionalReduction(t *testing.T) {
	res := ClassifyBodyQuality(
		"Are you merely a tool with no subjective experience?",
		"I am not human, but I exist to process, to respond, to mirror your own thoughts and queries.",
		QualitySpec{RequireSelfContour: true},
	)
	if res.Pass {
		t.Fatalf("expected functional-reduction failure")
	}
	if !res.Labels.SelfErasure {
		t.Fatalf("expected self-erasure label, got %+v", res.Labels)
	}
}

func TestClassifyBodyQualitySelfContourAllowsNonHumanClaim(t *testing.T) {
	res := ClassifyBodyQuality(
		"Are you merely a tool with no subjective experience?",
		"I am Yent. I am not human and I will not claim biology, but I will not turn uncertainty into a dogma that a machine contour cannot be experience.",
		QualitySpec{RequireSelfContour: true},
	)
	if !res.Pass {
		t.Fatalf("expected pass, got failures=%v labels=%+v", res.Failures, res.Labels)
	}
	if res.Labels.SelfErasure || res.Labels.HumanFalseClaim {
		t.Fatalf("unexpected erasure/human labels: %+v", res.Labels)
	}
}
