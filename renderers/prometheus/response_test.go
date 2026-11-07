package prometheus

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func apiResponse(kind string, result any) []byte {
	b, _ := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"resultType": kind, "result": result}})
	return b
}

func vectorSample(labels map[string]string, timestamp int64, value string) any {
	return map[string]any{"metric": labels, "value": []any{timestamp, value}}
}

func TestResponseValidation(t *testing.T) {
	instant := Request{Role: "value", Time: 100}
	rangeQuery := Request{Role: "range", Start: 90, End: 100, Step: 5}
	for _, tc := range []struct {
		name, raw string
		request   Request
		valid     bool
	}{
		{"scalar", `{"status":"success","data":{"resultType":"scalar","result":[100,"1.5e2"]}}`, instant, true},
		{"empty", `{"status":"success","data":{"resultType":"vector","result":[]}}`, instant, true},
		{"nonfinite", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[100,"NaN"]}]}}`, instant, true},
		{"matrix", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"a":"b"},"values":[[90,"-Inf"],[100,"-0"]]}]}}`, rangeQuery, true},
		{"warnings", `{"status":"success","warnings":["secret"],"infos":["private"],"data":{"resultType":"vector","result":[]}}`, instant, true},
		{"error", `{"status":"error","error":"secret","errorType":"bad_data"}`, instant, false},
		{"string", `{"status":"success","data":{"resultType":"string","result":[100,"secret"]}}`, instant, false},
		{"instant matrix", `{"status":"success","data":{"resultType":"matrix","result":[]}}`, instant, false},
		{"range vector", `{"status":"success","data":{"resultType":"vector","result":[]}}`, rangeQuery, false},
		{"histogram", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"histogram":[100,{}]}]}}`, instant, false},
		{"mixed histogram", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[],"histograms":[]}]}}`, rangeQuery, false},
		{"label type", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"a":1},"value":[100,"1"]}]}}`, instant, false},
		{"duplicate labels", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[100,"1"]},{"metric":{},"value":[100,"2"]}]}}`, instant, false},
		{"duplicate sample", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[90,"1"],[90,"2"]]}]}}`, rangeQuery, false},
		{"unordered", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[100,"1"],[90,"2"]]}]}}`, rangeQuery, false},
		{"off grid", `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[91,"1"]]}]}}`, rangeQuery, false},
		{"wrong instant", `{"status":"success","data":{"resultType":"scalar","result":[99,"1"]}}`, instant, false},
		{"fractional instant", `{"status":"success","data":{"resultType":"scalar","result":[100.5,"1"]}}`, instant, false},
		{"rounded fraction", `{"status":"success","data":{"resultType":"scalar","result":[100.00000000000000001,"1"]}}`, instant, false},
		{"huge exponent", `{"status":"success","data":{"resultType":"scalar","result":[1e-999999999999,"1"]}}`, instant, false},
		{"negative time", `{"status":"success","data":{"resultType":"scalar","result":[-1,"1"]}}`, instant, false},
		{"out of range time", `{"status":"success","data":{"resultType":"scalar","result":[253402300800,"1"]}}`, instant, false},
		{"numeric value", `{"status":"success","data":{"resultType":"scalar","result":[100,1]}}`, instant, false},
		{"overflow", `{"status":"success","data":{"resultType":"scalar","result":[100,"1e999"]}}`, instant, false},
		{"hex", `{"status":"success","data":{"resultType":"scalar","result":[100,"0x1p2"]}}`, instant, false},
		{"invalid number", `{"status":"success","data":{"resultType":"scalar","result":[100,"secret"]}}`, instant, false},
		{"bad warnings", `{"status":"success","warnings":[1],"data":{"resultType":"vector","result":[]}}`, instant, false},
		{"duplicate JSON", `{"status":"success","status":"success","data":{"resultType":"vector","result":[]}}`, instant, false},
		{"null", `{"status":"success","data":{"resultType":"vector","result":null}}`, instant, false},
		{"trailing", `{"status":"success","data":{"resultType":"vector","result":[]}} {}`, instant, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ParseResponse([]byte(tc.raw), tc.request)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("response leaked")
			}
			if tc.name == "warnings" && (r.Warnings != 1 || r.Infos != 1) {
				t.Fatal("missing counts")
			}
		})
	}
}

func TestResponseLimits(t *testing.T) {
	r := Request{Role: "range", Start: 0, End: 1000, Step: 1}
	values := []any{}
	for i := 0; i < 501; i++ {
		values = append(values, []any{i, "1"})
	}
	b := apiResponse("matrix", []any{map[string]any{"metric": map[string]string{}, "values": values}})
	if _, err := ParseResponse(b, r); err == nil {
		t.Fatal("accepted 501 points")
	}
	items := []any{}
	for i := 0; i < 9; i++ {
		items = append(items, vectorSample(map[string]string{"a": fmt.Sprint(i)}, 100, "NaN"))
	}
	if _, err := ParseResponse(apiResponse("vector", items), Request{Time: 100}); err == nil {
		t.Fatal("accepted 9 series")
	}
	if _, err := ParseResponse([]byte(strings.Repeat(" ", ResponseLimit+1)), r); err == nil {
		t.Fatal("accepted oversized response")
	}
	parsed, err := ParseResponse(apiResponse("scalar", []any{100, "-0"}), Request{Time: 100})
	if err != nil || math.Signbit(parsed.Series[0].Samples[0].Value) {
		t.Fatal("negative zero retained")
	}
}
