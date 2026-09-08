package main

import (
	"strings"
	"testing"
)

func TestReadRecordsSkipsHeader(t *testing.T) {
	input := "id,length,width,height,weight\nbox-1,12,10,8,4\n"
	recs, err := readRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	got := recs[0]
	want := record{id: "box-1", length: 12, width: 10, height: 8, actualWeight: 4}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadRecordsNoHeader(t *testing.T) {
	input := "box-1,12,10,8,4\nbox-2,24,18,18,6\n"
	recs, err := readRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].id != "box-1" || recs[1].id != "box-2" {
		t.Errorf("got ids %q, %q", recs[0].id, recs[1].id)
	}
}

func TestReadRecordsSkipsMalformedAndShortRows(t *testing.T) {
	input := "id,length,width,height,weight\n" +
		"box-1,12,10,8,4\n" +
		"box-2,not-a-number,18,18,6\n" +
		"box-3,1,2,3\n" +
		"box-4,5,6,7,8\n"
	recs, err := readRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(recs), recs)
	}
	if recs[0].id != "box-1" || recs[1].id != "box-4" {
		t.Errorf("got ids %q, %q, want box-1, box-4", recs[0].id, recs[1].id)
	}
}

func TestReadRecordsTrimsWhitespace(t *testing.T) {
	input := " box-1 , 12 , 10 , 8 , 4 \n"
	recs, err := readRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readRecords: %v", err)
	}
	if len(recs) != 1 || recs[0].id != "box-1" {
		t.Fatalf("got %+v", recs)
	}
}

func TestReadJSONRecords(t *testing.T) {
	input := `[
		{"id": "box-1", "length": 12, "width": 10, "height": 8, "weight": 4},
		{"id": "box-2", "length": 24, "width": 18, "height": 18, "weight": 6}
	]`
	recs, err := readJSONRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readJSONRecords: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	want := record{id: "box-1", length: 12, width: 10, height: 8, actualWeight: 4}
	if recs[0] != want {
		t.Errorf("got %+v, want %+v", recs[0], want)
	}
	if recs[1].id != "box-2" {
		t.Errorf("got id %q, want box-2", recs[1].id)
	}
}

func TestReadJSONRecordsEmptyArray(t *testing.T) {
	recs, err := readJSONRecords(strings.NewReader(`[]`))
	if err != nil {
		t.Fatalf("readJSONRecords: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("got %d records, want 0", len(recs))
	}
}

func TestReadJSONRecordsInvalidJSON(t *testing.T) {
	_, err := readJSONRecords(strings.NewReader(`not json`))
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
}

func TestBillableWeightDimExceedsActual(t *testing.T) {
	r := record{length: 24, width: 18, height: 18, actualWeight: 6}
	dimWeight, actual, billable, applies := billableWeight(r, imperialDivisor)
	if dimWeight != 57 {
		t.Errorf("dimWeight = %v, want 57", dimWeight)
	}
	if actual != 6 {
		t.Errorf("actual = %v, want 6", actual)
	}
	if billable != 57 {
		t.Errorf("billable = %v, want 57", billable)
	}
	if !applies {
		t.Error("applies = false, want true")
	}
}

func TestBillableWeightActualExceedsDim(t *testing.T) {
	r := record{length: 4, width: 4, height: 4, actualWeight: 10}
	dimWeight, actual, billable, applies := billableWeight(r, imperialDivisor)
	if dimWeight != 1 {
		t.Errorf("dimWeight = %v, want 1", dimWeight)
	}
	if actual != 10 {
		t.Errorf("actual = %v, want 10", actual)
	}
	if billable != 10 {
		t.Errorf("billable = %v, want 10", billable)
	}
	if applies {
		t.Error("applies = true, want false")
	}
}

func TestBillableWeightRoundsUp(t *testing.T) {
	r := record{length: 1, width: 1, height: 1, actualWeight: 0.1}
	dimWeight, actual, billable, applies := billableWeight(r, imperialDivisor)
	if dimWeight != 1 {
		t.Errorf("dimWeight = %v, want 1 (ceil of a tiny fraction)", dimWeight)
	}
	if actual != 1 {
		t.Errorf("actual = %v, want 1 (ceil of 0.1)", actual)
	}
	if billable != 1 {
		t.Errorf("billable = %v, want 1", billable)
	}
	if applies {
		t.Error("applies = true, want false (tie goes to actual)")
	}
}

func TestDivisorForOverrideWinsOverCarrier(t *testing.T) {
	d, err := divisorFor("usps", "in", 200)
	if err != nil {
		t.Fatalf("divisorFor: %v", err)
	}
	if d != 200 {
		t.Errorf("d = %v, want 200 (explicit override)", d)
	}
}

func TestDivisorForKnownCarrier(t *testing.T) {
	d, err := divisorFor("usps", "in", 0)
	if err != nil {
		t.Fatalf("divisorFor: %v", err)
	}
	if d != 166 {
		t.Errorf("d = %v, want 166", d)
	}
}

func TestDivisorForCarrierIsCaseInsensitive(t *testing.T) {
	d, err := divisorFor("UPS", "cm", 0)
	if err != nil {
		t.Fatalf("divisorFor: %v", err)
	}
	if d != metricDivisor {
		t.Errorf("d = %v, want %v", d, metricDivisor)
	}
}

func TestDivisorForUnknownCarrier(t *testing.T) {
	_, err := divisorFor("dhl", "in", 0)
	if err == nil {
		t.Fatal("expected an error for an unknown carrier, got nil")
	}
}

func TestDivisorForNoCarrierFallsBackToUnit(t *testing.T) {
	d, err := divisorFor("", "cm", 0)
	if err != nil {
		t.Fatalf("divisorFor: %v", err)
	}
	if d != metricDivisor {
		t.Errorf("d = %v, want %v", d, metricDivisor)
	}
}

func TestSummarizeTotalsAcrossBatch(t *testing.T) {
	recs := []record{
		{id: "box-1", length: 10, width: 10, height: 10, actualWeight: 5},
		{id: "box-2", length: 2, width: 2, height: 2, actualWeight: 15},
	}
	s := summarize(recs, imperialDivisor)
	if s.packages != 2 {
		t.Errorf("packages = %d, want 2", s.packages)
	}
	if s.dimApplies != 1 {
		t.Errorf("dimApplies = %d, want 1", s.dimApplies)
	}
	if s.totalActualWeight != 20 {
		t.Errorf("totalActualWeight = %v, want 20", s.totalActualWeight)
	}
	if s.totalDimWeight != 9 {
		t.Errorf("totalDimWeight = %v, want 9", s.totalDimWeight)
	}
	if s.totalBillable != 23 {
		t.Errorf("totalBillable = %v, want 23", s.totalBillable)
	}
}

func TestSummarizeEmptyBatch(t *testing.T) {
	s := summarize(nil, imperialDivisor)
	if s.packages != 0 || s.dimApplies != 0 || s.totalActualWeight != 0 || s.totalDimWeight != 0 || s.totalBillable != 0 {
		t.Errorf("got %+v, want zero value", s)
	}
}

func TestPrintSummary(t *testing.T) {
	var buf strings.Builder
	printSummary(&buf, batchSummary{packages: 2, dimApplies: 1, totalActualWeight: 16, totalDimWeight: 58, totalBillable: 67})
	got := buf.String()
	for _, want := range []string{"packages: 2", "dim weight applied: 1", "total actual weight: 16", "total dim weight: 58", "total billable weight: 67"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
}

func TestBillableWeightMetricDivisor(t *testing.T) {
	r := record{length: 50, width: 40, height: 30, actualWeight: 10}
	dimWeight, _, billable, applies := billableWeight(r, metricDivisor)
	if dimWeight != 12 {
		t.Errorf("dimWeight = %v, want 12", dimWeight)
	}
	if billable != 12 {
		t.Errorf("billable = %v, want 12", billable)
	}
	if !applies {
		t.Error("applies = false, want true")
	}
}
