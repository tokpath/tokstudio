package catalog

import (
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"testing"
)

func TestPublicCapabilityCannotExceedUpstreamParameterContract(t *testing.T) {
	want := commerceTestCapability()
	want.Parameters = append(want.Parameters, "quality")
	want.ParameterConstraints = map[string]modelruntime.ParameterConstraint{"quality": {Values: []string{"low", "high"}}}
	have := want
	have.ParameterConstraints = map[string]modelruntime.ParameterConstraint{"quality": {Values: []string{"low"}}}
	if coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("upstream accepts only low quality but public model advertised high")
	}
	want.ParameterConstraints = map[string]modelruntime.ParameterConstraint{"quality": {Values: []string{"low"}}}
	if !coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("compatible explicit public subset rejected")
	}
	have.RequiredParameters = append(append([]string{}, have.RequiredParameters...), "quality")
	if coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("upstream-only required parameter was hidden from public contract")
	}
	want.RequiredParameters = append(append([]string{}, want.RequiredParameters...), "quality")
	if !coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("matched required parameter rejected")
	}
	have.OutputLimitParameter = "different_output_parameter"
	if coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("different output bound parameter treated as equivalent")
	}
	if constraintCovers(modelruntime.ParameterConstraint{Min: "2", Max: "5"}, modelruntime.ParameterConstraint{Min: "1", Max: "10"}) {
		t.Fatal("wider public numeric range admitted")
	}
	if !constraintCovers(modelruntime.ParameterConstraint{Min: "2", Max: "5"}, modelruntime.ParameterConstraint{Min: "3", Max: "4"}) {
		t.Fatal("verified numeric subset rejected")
	}
}
