package media

import "testing"

func TestNormalizeTaskType(t *testing.T) {
	got, err := NormalizeTaskType(KindVideo, "")
	if err != nil || got != TaskT2V {
		t.Fatalf("video default: %q %v", got, err)
	}
	got, err = NormalizeTaskType(KindImage, "")
	if err != nil || got != TaskGenerate {
		t.Fatalf("image default: %q %v", got, err)
	}
	got, err = NormalizeTaskType(KindVideo, "image-to-video")
	if err != nil || got != TaskI2V {
		t.Fatalf("alias: %q %v", got, err)
	}
	if _, err := NormalizeTaskType(KindVideo, "timeline"); err == nil {
		t.Fatal("P1 timeline must be rejected")
	}
}

func TestValidateCreate(t *testing.T) {
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskT2V, Prompt: "a"}); err != nil {
		t.Fatalf("t2v: %v", err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskI2V, Prompt: "a"}); err == nil {
		t.Fatal("i2v without image should fail")
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskI2V, Prompt: "a", Images: []string{"img://1"}}); err != nil {
		t.Fatalf("i2v: %v", err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskFirstLastFrame, Prompt: "a", FirstFrame: "a"}); err == nil {
		t.Fatal("first_last_frame needs both frames")
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskFirstLastFrame, Prompt: "a", FirstFrame: "a", LastFrame: "b"}); err != nil {
		t.Fatalf("first_last_frame: %v", err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskReference, Prompt: "a"}); err == nil {
		t.Fatal("reference needs an asset")
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskReference, Prompt: "a", ReferenceAudio: "aud://1"}); err != nil {
		t.Fatalf("reference audio: %v", err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskExtend, Prompt: "a"}); err == nil {
		t.Fatal("extend needs source_job_id")
	}
	if err := ValidateCreate(CreateInput{Kind: KindImage, TaskType: TaskEdit, Prompt: "a"}); err == nil {
		t.Fatal("image edit needs images")
	}
	if err := ValidateCreate(CreateInput{Kind: KindImage, TaskType: TaskT2V, Prompt: "a"}); err == nil {
		t.Fatal("video mode on image should fail")
	}
	if err := ValidateCreate(CreateInput{Kind: KindVideo, TaskType: TaskT2V, Prompt: "a", Duration: 99}); err == nil {
		t.Fatal("duration 99 should fail")
	}
}
