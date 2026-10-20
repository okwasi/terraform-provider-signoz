// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/ksoviero/terraform-provider-signoz/internal/client"
)

// alertRuleSpecType is a custom string type that normalizes alert rule spec JSON
// (stripping API-default fields like disabled:false) before comparing for semantic
// equality. This prevents phantom diffs when users include default-value fields
// in their config that the API omits on read.
type alertRuleSpecType struct {
	basetypes.StringType
}

var alertRuleSpecTypeInstance = alertRuleSpecType{}

func (t alertRuleSpecType) String() string {
	return "alertRuleSpecType"
}

func (t alertRuleSpecType) Equal(o attr.Type) bool {
	_, ok := o.(alertRuleSpecType)
	return ok
}

func (t alertRuleSpecType) ValueType(_ context.Context) attr.Value {
	return alertRuleSpecValue{}
}

func (t alertRuleSpecType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return alertRuleSpecValue{StringValue: in}, nil
}

func (t alertRuleSpecType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}
	v, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue: %v", diags)
	}
	return v, nil
}

// alertRuleSpecValue is the value type for alertRuleSpecType.
type alertRuleSpecValue struct {
	basetypes.StringValue
}

func (v alertRuleSpecValue) Type(_ context.Context) attr.Type {
	return alertRuleSpecTypeInstance
}

func (v alertRuleSpecValue) Equal(o attr.Value) bool {
	other, ok := o.(alertRuleSpecValue)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals normalizes both JSON strings via NormalizeRuleSpecJSON
// before comparing. This means config values with disabled:false (and other
// API-default fields) are considered equal to the normalized form the API returns.
func (v alertRuleSpecValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(alertRuleSpecValue)
	if !ok {
		// Fall back to raw string equality for unexpected types.
		return v.StringValue.Equal(newValuable.(basetypes.StringValue)), diags
	}

	normA, err := client.NormalizeRuleSpecJSON(v.ValueString())
	if err != nil {
		diags.AddError("Semantic Equality Check Error", "Failed to normalize alert rule spec: "+err.Error())
		return false, diags
	}
	normB, err := client.NormalizeRuleSpecJSON(newValue.ValueString())
	if err != nil {
		diags.AddError("Semantic Equality Check Error", "Failed to normalize alert rule spec: "+err.Error())
		return false, diags
	}
	return normA == normB, diags
}

// ToStringValue implements basetypes.StringValuable.
func (v alertRuleSpecValue) ToStringValue(_ context.Context) (basetypes.StringValue, diag.Diagnostics) {
	return v.StringValue, nil
}

// normalizedJSONAttribute is required JSON with semantic equality (whitespace and key order ignored).
func normalizedJSONAttribute(markdown string, required bool) schema.StringAttribute {
	return normalizedJSONAttributeSensitive(markdown, required, false)
}

func normalizedJSONAttributeSensitive(markdown string, required bool, sensitive bool) schema.StringAttribute {
	attr := schema.StringAttribute{
		MarkdownDescription: markdown,
		CustomType:          jsontypes.NormalizedType{},
		Sensitive:           sensitive,
	}
	if required {
		attr.Required = true
	} else {
		attr.Optional = true
	}
	return attr
}

// normalizedJSONOptional is optional JSON (optionally sensitive) with semantic equality.
func normalizedJSONOptional(markdown string, sensitive bool, planModifiers ...planmodifier.String) schema.StringAttribute {
	attr := schema.StringAttribute{
		MarkdownDescription: markdown,
		CustomType:          jsontypes.NormalizedType{},
		Optional:            true,
		Sensitive:           sensitive,
	}
	if len(planModifiers) > 0 {
		attr.PlanModifiers = planModifiers
	}
	return attr
}

// normalizedJSONOptionalWithUnknown copies prior state when credentials/config are not in config.
func normalizedJSONOptionalWithUnknown(markdown string, sensitive bool) schema.StringAttribute {
	return normalizedJSONOptional(markdown, sensitive, stringplanmodifier.UseStateForUnknown())
}

func newNormalizedJSON(value string) jsontypes.Normalized {
	return jsontypes.NewNormalizedValue(value)
}

// normalizedJSONComputedSensitive is computed JSON with semantic equality (for API payloads).
// No UseStateForUnknown: the payload changes whenever the resource's arguments change,
// so planning the prior value fails apply with "inconsistent result after apply".
func normalizedJSONComputedSensitive(markdown string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: markdown,
		CustomType:          jsontypes.NormalizedType{},
		Computed:            true,
		Sensitive:           true,
	}
}
