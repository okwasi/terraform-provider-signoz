// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/ksoviero/terraform-provider-signoz/internal/provider"
)

// SigNoz returns the whole receiver (name included) as `data`, so a change to
// `name` or `config` changes `data`. The plan must leave `data` unknown in that
// case; planning the prior value makes Terraform reject the apply with
// ".data: inconsistent values for sensitive attribute".
func TestNotificationChannelPlan_data(t *testing.T) {
	t.Parallel()

	prior := map[string]string{
		"id":         "11111111-1111-7111-8111-111111111111",
		"name":       "alerts",
		"config":     `{"slack_configs":[{"channel":"#a"}]}`,
		"type":       "slack",
		"data":       `{"name":"alerts","slack_configs":[{"channel":"#a","send_resolved":false}]}`,
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z",
	}

	tests := []struct {
		name            string
		isCreate        bool
		configName      string
		configJSON      string
		wantDataUnknown bool
	}{
		{name: "create", isCreate: true, configName: "alerts", configJSON: prior["config"], wantDataUnknown: true},
		{name: "config changed", configName: "alerts", configJSON: `{"slack_configs":[{"channel":"#b"}]}`, wantDataUnknown: true},
		{name: "name changed", configName: "alerts-renamed", configJSON: prior["config"], wantDataUnknown: true},
		{name: "nothing changed", configName: "alerts", configJSON: prior["config"], wantDataUnknown: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := map[string]string{"name": tt.configName, "config": tt.configJSON}
			proposed := map[string]string{}
			for k, v := range prior {
				proposed[k] = v
			}
			proposed["name"] = tt.configName
			proposed["config"] = tt.configJSON

			var priorState map[string]string
			if tt.isCreate {
				proposed = config
			} else {
				priorState = prior
			}

			planned := planNotificationChannel(t, priorState, proposed, config)

			data := planned["data"]
			if tt.wantDataUnknown {
				if data.IsKnown() {
					t.Fatalf("planned data = %s, want unknown", data)
				}
				return
			}
			if !data.Equal(tftypes.NewValue(tftypes.String, prior["data"])) {
				t.Fatalf("planned data = %s, want the prior value", data)
			}
		})
	}
}

// planNotificationChannel runs PlanResourceChange for signoz_notification_channel
// and returns the planned attributes. A nil priorState plans a create.
func planNotificationChannel(t *testing.T, priorState, proposed, config map[string]string) map[string]tftypes.Value {
	t.Helper()
	ctx := context.Background()

	server, err := providerserver.NewProtocol6WithError(provider.New("test")())()
	if err != nil {
		t.Fatal(err)
	}

	schemaResp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	resourceSchema, ok := schemaResp.ResourceSchemas["signoz_notification_channel"]
	if !ok {
		t.Fatal("signoz_notification_channel schema not found")
	}
	objectType, ok := resourceSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatal("resource schema is not an object")
	}

	priorValue := tftypes.NewValue(objectType, nil)
	if priorState != nil {
		priorValue = channelValue(objectType, priorState)
	}

	resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "signoz_notification_channel",
		PriorState:       dynamicValue(t, objectType, priorValue),
		ProposedNewState: dynamicValue(t, objectType, channelValue(objectType, proposed)),
		Config:           dynamicValue(t, objectType, channelValue(objectType, config)),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("plan error: %s: %s", d.Summary, d.Detail)
		}
	}

	plannedValue, err := resp.PlannedState.Unmarshal(objectType)
	if err != nil {
		t.Fatal(err)
	}
	var planned map[string]tftypes.Value
	if err := plannedValue.As(&planned); err != nil {
		t.Fatal(err)
	}
	return planned
}

// channelValue builds a resource object; attributes missing from values are null.
func channelValue(objectType tftypes.Object, values map[string]string) tftypes.Value {
	attrs := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name := range objectType.AttributeTypes {
		if v, ok := values[name]; ok {
			attrs[name] = tftypes.NewValue(tftypes.String, v)
		} else {
			attrs[name] = tftypes.NewValue(tftypes.String, nil)
		}
	}
	return tftypes.NewValue(objectType, attrs)
}

func dynamicValue(t *testing.T, objectType tftypes.Object, value tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	dv, err := tfprotov6.NewDynamicValue(objectType, value)
	if err != nil {
		t.Fatal(err)
	}
	return &dv
}
