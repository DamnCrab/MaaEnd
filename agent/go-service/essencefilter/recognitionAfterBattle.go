package essencefilter

import (
	"encoding/json"
	"image"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

type essenceAfterBattleNthParams struct {
	RecognitionNodeName string `json:"recognitionNodeName"`
}

// EssenceFilterAfterBattleNthRecognition 在战斗结算后按行序依次返回精英识别结果中的第 N 个框。
// 该识别器会在运行状态中缓存全屏识别节点的结果（RowBoxes），通过递增 RowIndex 逐个吐出框；
// 若缓存已消费完，则重新调用指定的 RecognitionNodeName 进行识别并刷新缓存。
type EssenceFilterAfterBattleNthRecognition struct{}

var _ maa.CustomRecognitionRunner = &EssenceFilterAfterBattleNthRecognition{}

// filterAfterBattleRowBoxesByColor 仅对缩略图下半区域做 EssenceColorMatch，剔除不符合当前档位颜色的候选（含全屏模板误检）。
// 保留顺序与全屏识别结果遍历顺序一致（不做 y/x 重排）。
func filterAfterBattleRowBoxesByColor(ctx *maa.Context, img image.Image, st *RunState, raw [][4]int) [][4]int {
	out := make([][4]int, 0, len(raw))
	for _, boxArr := range raw {
		colorMatchROIW := boxArr[2]
		colorMatchROIH := boxArr[3] - 90
		if colorMatchROIW <= 0 || colorMatchROIH <= 0 {
			continue
		}
		roi := maa.Rect{boxArr[0], boxArr[1] + 90, colorMatchROIW, colorMatchROIH}

		colorMatched := false
		for _, et := range st.EssenceTypes {
			cDetail, err := ctx.RunRecognition("EssenceColorMatch", img, map[string]any{
				"EssenceColorMatch": map[string]any{"roi": roi, "lower": et.Range.Lower, "upper": et.Range.Upper},
			})
			if err != nil {
				continue
			}
			if cDetail != nil && cDetail.Hit {
				colorMatched = true
				break
			}
		}
		if !colorMatched {
			continue
		}
		out = append(out, boxArr)
	}

	return out
}

func (r *EssenceFilterAfterBattleNthRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	st := getRunState()
	if st == nil {
		return nil, false
	}
	if arg == nil || arg.Img == nil {
		log.Error().Str("component", "EssenceFilter").Str("recognition", "AfterBattleNthEssence").Msg("arg.Img nil")
		return nil, false
	}

	params := essenceAfterBattleNthParams{
		RecognitionNodeName: "EssenceFullScreenDetectAll",
	}
	if strings.TrimSpace(arg.CustomRecognitionParam) != "" {
		if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
			log.Error().Err(err).Str("component", "EssenceFilter").Str("recognition", "AfterBattleNthEssence").Msg("CustomRecognitionParam parse failed")
			return nil, false
		}
	}
	if strings.TrimSpace(params.RecognitionNodeName) == "" {
		return nil, false
	}

	if st.RowIndex < len(st.RowBoxes) {
		box := st.RowBoxes[st.RowIndex]
		st.RowIndex++
		return &maa.CustomRecognitionResult{
			Box:    maa.Rect{box[0], box[1], box[2], box[3]},
			Detail: "",
		}, true
	}

	detail, err := ctx.RunRecognition(params.RecognitionNodeName, arg.Img, nil)
	if err != nil || detail == nil || !detail.Hit || detail.Results == nil || detail.Results.Filtered == nil {
		return nil, false
	}

	raw := make([][4]int, 0, len(detail.Results.Filtered))
	for _, res := range detail.Results.Filtered {
		tm, ok := res.AsTemplateMatch()
		if !ok {
			continue
		}
		b := tm.Box
		raw = append(raw, [4]int{b.X(), b.Y(), b.Width(), b.Height()})
	}

	st.RowBoxes = filterAfterBattleRowBoxesByColor(ctx, arg.Img, st, raw)
	log.Info().Str("component", "EssenceFilter").Str("recognition", "AfterBattleNthEssence").
		Int("raw_boxes", len(raw)).Int("after_color_filter", len(st.RowBoxes)).Msg("after battle row boxes")

	if st.RowIndex >= len(st.RowBoxes) {
		return nil, false
	}

	box := st.RowBoxes[st.RowIndex]
	st.RowIndex++
	return &maa.CustomRecognitionResult{
		Box:    maa.Rect{box[0], box[1], box[2], box[3]},
		Detail: "",
	}, true
}
