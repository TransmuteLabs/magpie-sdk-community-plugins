package devin

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type NativeModel struct {
	UID           string
	Name          string
	Family        string
	FamilyName    string
	Context       uint64
	MaxOutput     uint64
	Images        bool
	Tools         bool
	Thinking      bool
	Free          bool
	Price         *NativePrice
	Efforts       []string
	DefaultEffort string
	Virtual       bool
}

type NativePrice struct {
	Input, Output, CacheRead float64
}

func nativeAgentRouter(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "adaptive", "fusion":
		return true
	default:
		return false
	}
}

func nativePrice(fields message) (*NativePrice, error) {
	values := map[string]float64{}
	for _, entry := range fields[32] {
		price, err := projectedMessage(entry.b, map[int]int{1: textField, 2: 5, 3: textField})
		if err != nil {
			return nil, errors.New("invalid native price declaration")
		}
		label := price.text(1)
		if label != "Input" && label != "Output" && label != "Cached input" {
			continue
		}
		if price.text(3) != "1M tokens" {
			continue
		}
		if _, exists := values[label]; exists || len(price.data(2)) != 4 {
			return nil, errors.New("ambiguous or missing native token price")
		}
		value := float64(math.Float32frombits(binary.LittleEndian.Uint32(price.data(2))))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, errors.New("invalid native token price")
		}
		value, err = strconv.ParseFloat(strconv.FormatFloat(value, 'g', 6, 64), 64)
		if err != nil {
			return nil, errors.New("unreadable native token price")
		}
		values[label] = value
	}
	input, hasInput := values["Input"]
	output, hasOutput := values["Output"]
	if !hasInput || !hasOutput {
		return nil, nil
	}
	return &NativePrice{Input: input, Output: output, CacheRead: values["Cached input"]}, nil
}

func DecodeNativeModels(payload []byte) ([]NativeModel, error) {
	root, err := projectedMessage(payload, map[int]int{1: 2 | repeated})
	if err != nil || len(root[1]) == 0 || len(root[1]) > 8192 {
		return nil, errors.New("invalid or empty native model catalog")
	}
	var models []NativeModel
	seen := map[string]bool{}
	for _, entry := range root[1] {
		fields, err := projectedMessage(entry.b, map[int]int{1: textField, 4: 0, 5: 0, 18: 0, 22: textField, 23: 2, 24: 0, 30: 2, 32: 2 | repeated})
		if err != nil {
			return nil, errors.New("invalid native model configuration")
		}
		id := fields.text(22)
		if id == "" || len(id) > 256 || strings.TrimSpace(id) != id || strings.ContainsFunc(id, unicode.IsControl) || seen[id] ||
			fields.number(4) > 1 || fields.number(5) > 1 || fields.number(18) > math.MaxInt32 {
			return nil, errors.New("invalid or duplicate native model identity or limit")
		}
		seen[id] = true
		info, err := projectedMessage(fields.data(23), map[int]int{4: 0, 6: 2, 13: 0, 23: textField, 25: 0})
		if err != nil || info.number(4) > math.MaxInt32 || info.number(13) > math.MaxInt32 || info.number(25) > 1 {
			return nil, errors.New("invalid native model information")
		}
		features, err := projectedMessage(info.data(6), map[int]int{12: 0, 15: 0})
		if err != nil || features.number(12) > 1 || features.number(15) > 1 {
			return nil, errors.New("invalid native model features")
		}
		if fields.number(4) == 1 || info.number(25) == 1 || nativeAgentRouter(id) || nativeAgentRouter(info.text(23)) {
			continue
		}
		family, err := projectedMessage(fields.data(30), map[int]int{1: textField})
		if err != nil || len(info.text(23)) > 256 || strings.TrimSpace(info.text(23)) != info.text(23) ||
			strings.ContainsFunc(info.text(23), unicode.IsControl) {
			return nil, errors.New("invalid native model family")
		}
		model := NativeModel{
			UID: id, Name: fields.text(1), Family: info.text(23), FamilyName: family.text(1),
			Context: fields.number(18), MaxOutput: info.number(13),
			Images: fields.number(5) == 1, Tools: features.number(12) == 1,
			Thinking: features.number(15) == 1, Free: fields.number(24) == 4,
		}
		model.Price, err = nativePrice(fields)
		if err != nil {
			return nil, err
		}
		if model.Free {
			model.Price = &NativePrice{}
		}
		if model.Name == "" {
			model.Name = model.UID
		}
		if model.Context == 0 {
			model.Context = info.number(4)
		}
		if model.Context > 0 && model.MaxOutput > model.Context {
			return nil, errors.New("native model output exceeds its context limit")
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, errors.New("native catalog contains no available concrete models")
	}
	return models, nil
}
