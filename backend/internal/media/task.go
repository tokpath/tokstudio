package media

import (
	"fmt"
	"strings"
)

// 文档 D3.2 确认的 P0 任务模式。独立音频/转写/视频理解/复杂时间线仍是 P1。
const (
	TaskT2V            = "t2v"
	TaskI2V            = "i2v"
	TaskFirstFrame     = "first_frame"
	TaskFirstLastFrame = "first_last_frame"
	TaskReference      = "reference"
	TaskExtend         = "extend"
	TaskEdit           = "edit"
	TaskGenerate       = "generate"
)

type RequestError struct {
	Msg string
}

func (e *RequestError) Error() string { return e.Msg }
func (e *RequestError) Unwrap() error { return ErrInvalidRequest }

func Invalid(msg string) error {
	return &RequestError{Msg: msg}
}

func NormalizeTaskType(kind, raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	switch raw {
	case "", "text", "txt2video", "text2video", "text_to_video":
		if kind == KindImage {
			return TaskGenerate, nil
		}
		return TaskT2V, nil
	case TaskT2V:
		return TaskT2V, nil
	case TaskI2V, "img2video", "image2video", "image_to_video":
		return TaskI2V, nil
	case TaskFirstFrame:
		return TaskFirstFrame, nil
	case TaskFirstLastFrame, "first_last", "flf":
		return TaskFirstLastFrame, nil
	case TaskReference, "ref":
		return TaskReference, nil
	case TaskExtend:
		return TaskExtend, nil
	case TaskEdit:
		return TaskEdit, nil
	case TaskGenerate:
		return TaskGenerate, nil
	default:
		return "", Invalid(fmt.Sprintf("不支持的生成模式 %s", raw))
	}
}

func ValidateCreate(in CreateInput) error {
	task, err := NormalizeTaskType(in.Kind, in.TaskType)
	if err != nil {
		return err
	}
	in.TaskType = task
	if err := validateTaskKind(in.Kind, task); err != nil {
		return err
	}
	hasImage := len(in.Images) > 0 || strings.TrimSpace(in.FirstFrame) != ""
	hasLast := strings.TrimSpace(in.LastFrame) != "" || len(in.Images) >= 2
	hasRef := len(in.Images) > 0 || strings.TrimSpace(in.FirstFrame) != "" ||
		strings.TrimSpace(in.ReferenceVideo) != "" || strings.TrimSpace(in.ReferenceAudio) != ""
	switch {
	case in.Kind == KindVideo && (task == TaskI2V || task == TaskFirstFrame) && !hasImage:
		return Invalid("图生/首帧模式需要 images 或 first_frame")
	case in.Kind == KindVideo && task == TaskFirstLastFrame && (!hasImage || !hasLast):
		return Invalid("首尾帧模式需要 first_frame 与 last_frame（或至少两张 images）")
	case in.Kind == KindVideo && task == TaskReference && !hasRef:
		return Invalid("参考模式需要参考图、参考视频或参考音频至少一种")
	case in.Kind == KindVideo && (task == TaskExtend || task == TaskEdit) && strings.TrimSpace(in.SourceJobID) == "":
		return Invalid("延长/编辑需要已完成的本用户视频任务 source_job_id")
	case in.Kind == KindImage && task == TaskEdit && len(in.Images) == 0:
		return Invalid("图像编辑需要 images")
	}
	if in.Duration < 0 || in.Duration > 60 {
		return Invalid("时长必须在 1–60 秒")
	}
	if in.FPS < 0 || in.FPS > 60 {
		return Invalid("帧率必须在 0–60")
	}
	return nil
}

func validateTaskKind(kind, task string) error {
	switch kind {
	case KindVideo:
		switch task {
		case TaskT2V, TaskI2V, TaskFirstFrame, TaskFirstLastFrame, TaskReference, TaskExtend, TaskEdit:
			return nil
		case TaskGenerate:
			return Invalid("图像 generate 不能用于视频任务")
		}
	case KindImage:
		switch task {
		case TaskGenerate, TaskEdit:
			return nil
		}
		return Invalid("视频模式不能用于图像任务")
	}
	return Invalid("不支持的任务类型")
}

func adapterImages(in CreateInput) []string {
	if len(in.Images) > 0 {
		return in.Images
	}
	out := make([]string, 0, 2)
	if in.FirstFrame != "" {
		out = append(out, in.FirstFrame)
	}
	if in.LastFrame != "" {
		out = append(out, in.LastFrame)
	}
	return out
}
