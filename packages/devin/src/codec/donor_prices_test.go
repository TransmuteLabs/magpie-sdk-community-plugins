package devin

import (
	"encoding/binary"
	"math"
	"testing"
)

func fixturePrice(label, unit string, value float32) []byte {
	payload := fixtureText(1, label)
	payload = append(payload, byte(2<<3|5))
	payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(value))
	return append(payload, fixtureText(3, unit)...)
}

func TestDonorNativePricesKeepUnitsPrecisionAndFreeVariantsDistinct(t *testing.T) {
	price := func(label string, value float32) []byte {
		return fixtureText(32, string(fixturePrice(label, "1M tokens", value)))
	}
	paid := join(fixtureText(22, "swe-2-high-lightning"), fixtureNumber(24, 3),
		price("Input", 1.4), price("Output", 4.4), price("Cached input", 0.26))
	free := join(fixtureText(22, "swe-2-high"), fixtureNumber(24, 4))
	unknown := join(fixtureText(22, "unknown"), fixtureText(32, string(fixturePrice("Input", "1K tokens", 2))))
	payload := join(fixtureText(1, string(paid)), fixtureText(1, string(free)), fixtureText(1, string(unknown)))
	models, err := DecodeNativeModels(payload)
	if err != nil || len(models) != 3 {
		t.Fatal(models, err)
	}
	if models[0].Free || models[0].Price == nil || *models[0].Price != (NativePrice{Input: 1.4, Output: 4.4, CacheRead: 0.26}) {
		t.Fatal("native float32 prices or paid Lightning tier were lost")
	}
	if !models[1].Free || models[1].Price == nil || *models[1].Price != (NativePrice{}) || models[2].Price != nil {
		t.Fatal("explicit Free, unknown units and unknown prices were conflated")
	}
}

func TestDonorNativePricesRefuseAmbiguousOrInvalidKnownQuotes(t *testing.T) {
	for _, prices := range [][]byte{
		join(fixtureText(32, string(fixturePrice("Input", "1M tokens", 1))), fixtureText(32, string(fixturePrice("Input", "1M tokens", 2)))),
		fixtureText(32, string(fixturePrice("Input", "1M tokens", -1))),
		fixtureText(32, string(fixturePrice("Input", "1M tokens", float32(math.Inf(1))))),
		fixtureText(32, string(fixturePrice("Input", "1M tokens", float32(math.NaN())))),
	} {
		fields, err := projectedMessage(prices, map[int]int{32: 2 | repeated})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := nativePrice(fields); err == nil {
			t.Fatal("invalid known native price became trustworthy")
		}
	}
}

func TestDonorNativeCatalogExcludesAgentOnlyRouters(t *testing.T) {
	var payload []byte
	for _, source := range []struct{ id, family string }{
		{"Adaptive", ""}, {"fusion", ""}, {"opaque-router", " ADAPTIVE "},
		{"another-router", "Fusion"}, {"swe-2-high", "swe-2"}, {"fusion-research", ""},
	} {
		model := fixtureText(22, source.id)
		model = append(model, fixtureText(23, string(fixtureText(23, source.family)))...)
		payload = append(payload, fixtureText(1, string(model))...)
	}
	models, err := DecodeNativeModels(payload)
	if err != nil || len(models) != 2 || models[0].UID != "swe-2-high" || models[1].UID != "fusion-research" {
		t.Fatalf("agent-only routers became effective API picks: %+v err=%v", models, err)
	}
	onlyRouter := fixtureText(1, string(fixtureText(22, "Adaptive")))
	if _, err := DecodeNativeModels(onlyRouter); err == nil {
		t.Fatal("an agent-only catalog became a usable empty API catalog")
	}
}
