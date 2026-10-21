package catalog

import (
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"testing"
)

func TestRouteCoverageIgnoresParameterLists(t *testing.T) {
	want := commerceTestCapability()
	have := want
	want.Parameters = append(want.Parameters, "quality")
	want.InputTypes = []string{"text"}
	have.Parameters = nil
	have.RequiredParameters = append(append([]string{}, have.RequiredParameters...), "quality")
	have.InputTypes = []string{"text", "image"}
	have.OutputLimitParameter = "different_output_parameter"
	if !coversCapability([]modelruntime.Capability{have}, want) {
		t.Fatal("parameter names blocked a route that can forward the request")
	}
	if constraintCovers(modelruntime.ParameterConstraint{Min: "2", Max: "5"}, modelruntime.ParameterConstraint{Min: "1", Max: "10"}) {
		t.Fatal("wider public numeric range admitted")
	}
	if !constraintCovers(modelruntime.ParameterConstraint{Min: "2", Max: "5"}, modelruntime.ParameterConstraint{Min: "3", Max: "4"}) {
		t.Fatal("verified numeric subset rejected")
	}
}
