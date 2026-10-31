package security

import (
	"testing"

	"printmarshal/internal/model"
)

func TestSummarizeCountsUnknownAndNotAssessed(t *testing.T) {
	as := []*model.Assessment{
		{DeviceID: "a", Score: 80, Compliant: false, Unknown: 3, Items: []model.ItemResult{{ItemID: "x", Status: model.ItemFail}}},
		{DeviceID: "b", Score: 100, Compliant: true, Unknown: 1},
	}
	sum := SummarizeFor(as, 5)
	if sum.Devices != 2 || sum.Compliant != 1 || sum.NonCompliant != 1 || sum.Unknown != 4 || sum.NotAssessed != 3 || sum.AvgScore != 90 || sum.FailingItems["x"] != 1 {
		t.Fatalf("summary: %+v", sum)
	}
	if s := Summarize(as); s.NotAssessed != 0 || s.Unknown != 4 {
		t.Fatalf("Summarize without a view: %+v", s)
	}
}
