// Copyright 2022 Harness, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package converthelpers

import (
	"reflect"
	"testing"

	v0 "github.com/drone/go-convert/convert/harness/yaml"
	"github.com/drone/go-convert/internal/flexible"
)

// v0 expressed "deploy everywhere" with a deployToAll flag. v1 expresses it with boolean
// markers, and the backend only honours a literal true, so an expression cannot defer the
// choice to runtime:
//   - v0 deployToAll: true        -> v1 all-infra: true, emitted alongside deploy-to
//   - v0 deployToAll: false       -> v1 deploy-to only
//   - v0 deployToAll unspecified  -> v1 deploy-to only
//   - v0 deployToAll: <expression> -> v1 deploy-to only (falls back to the listed infras)
//
// The marker never replaces the infrastructure sibling: v0's non-GitOps runtime ignores
// deployToAll and fans out over infrastructureDefinitions, so dropping the sibling would
// widen the deployment. A resolved deploy-to wins over the marker in the v1 backend, which
// leaves the marker inert exactly as deployToAll was.
//
// At group level the same flag maps to all-env: true, which is emitted alongside items so
// the per-environment config authored in v0 is not lost.

func infraDefs(ids ...string) *flexible.Field[[]*v0.InfrastructureDefinition] {
	defs := make([]*v0.InfrastructureDefinition, 0, len(ids))
	for _, id := range ids {
		defs = append(defs, &v0.InfrastructureDefinition{Identifier: id})
	}
	field := &flexible.Field[[]*v0.InfrastructureDefinition]{}
	field.Set(defs)
	return field
}

func exprField(expr string) *flexible.Field[bool] {
	field := &flexible.Field[bool]{}
	field.SetExpression(expr)
	return field
}

func TestResolveDeployTo_AllInfraMarker(t *testing.T) {
	tests := []struct {
		name         string
		deployToAll  *flexible.Field[bool]
		infraDefs    *flexible.Field[[]*v0.InfrastructureDefinition]
		wantDeployTo interface{}
		wantAllInfra bool
	}{
		{
			name:         "deployToAll true -> marker alongside the infra list",
			deployToAll:  &flexible.Field[bool]{Value: true},
			infraDefs:    infraDefs("infra1", "infra2"),
			wantDeployTo: []string{"infra1", "infra2"},
			wantAllInfra: true,
		},
		{
			name:         "deployToAll true with a single infra -> marker alongside the infra",
			deployToAll:  &flexible.Field[bool]{Value: true},
			infraDefs:    infraDefs("infra1"),
			wantDeployTo: "infra1",
			wantAllInfra: true,
		},
		{
			name:         "deployToAll true with no infra list -> marker only",
			deployToAll:  &flexible.Field[bool]{Value: true},
			infraDefs:    nil,
			wantDeployTo: nil,
			wantAllInfra: true,
		},
		{
			name:         "deployToAll false -> deploy-to only",
			deployToAll:  &flexible.Field[bool]{Value: false},
			infraDefs:    infraDefs("infra1", "infra2"),
			wantDeployTo: []string{"infra1", "infra2"},
			wantAllInfra: false,
		},
		{
			name:         "deployToAll unspecified -> deploy-to only",
			deployToAll:  nil,
			infraDefs:    infraDefs("infra1"),
			wantDeployTo: "infra1",
			wantAllInfra: false,
		},
		{
			name:         "deployToAll <+input> -> deploy-to only",
			deployToAll:  exprField("<+input>"),
			infraDefs:    infraDefs("infra1", "infra2"),
			wantDeployTo: []string{"infra1", "infra2"},
			wantAllInfra: false,
		},
		{
			name:         "deployToAll other expression -> falls back to the listed infras",
			deployToAll:  exprField("<+pipeline.variables.everywhere>"),
			infraDefs:    infraDefs("infra1"),
			wantDeployTo: "infra1",
			wantAllInfra: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployTo, allInfra := resolveDeployTo(tt.deployToAll, tt.infraDefs)
			if allInfra != tt.wantAllInfra {
				t.Fatalf("expected all-infra=%v, got all-infra=%v", tt.wantAllInfra, allInfra)
			}
			if !reflect.DeepEqual(deployTo, tt.wantDeployTo) {
				t.Fatalf("expected deploy-to=%#v, got deploy-to=%#v", tt.wantDeployTo, deployTo)
			}
		})
	}
}

func TestConvertEnvironment_AllInfraMarker(t *testing.T) {
	tests := []struct {
		name         string
		deployToAll  *flexible.Field[bool]
		wantDeployTo interface{}
		wantAllInfra bool
	}{
		{
			name:         "deployToAll true -> marker alongside deploy-to",
			deployToAll:  &flexible.Field[bool]{Value: true},
			wantDeployTo: []string{"infra1", "infra2"},
			wantAllInfra: true,
		},
		{
			name:         "deployToAll false -> deploy-to only",
			deployToAll:  &flexible.Field[bool]{Value: false},
			wantDeployTo: []string{"infra1", "infra2"},
			wantAllInfra: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &v0.Environment{
				EnvironmentRef:            "env1",
				DeployToAll:               tt.deployToAll,
				InfrastructureDefinitions: infraDefs("infra1", "infra2"),
			}
			got := ConvertEnvironment(src, NewStageConversionContext())
			items := got.ItemList()
			if len(items) != 1 {
				t.Fatalf("expected 1 environment item, got %d", len(items))
			}
			if items[0].AllInfra != tt.wantAllInfra {
				t.Fatalf("expected all-infra=%v, got all-infra=%v", tt.wantAllInfra, items[0].AllInfra)
			}
			if !reflect.DeepEqual(items[0].DeployTo, tt.wantDeployTo) {
				t.Fatalf("expected deploy-to=%#v, got deploy-to=%#v", tt.wantDeployTo, items[0].DeployTo)
			}
		})
	}
}

func TestConvertEnvironmentGroup_AllEnvMarker(t *testing.T) {
	tests := []struct {
		name        string
		deployToAll *flexible.Field[bool]
		wantAllEnv  bool
	}{
		{
			name:        "group deployToAll true -> all-env marker",
			deployToAll: &flexible.Field[bool]{Value: true},
			wantAllEnv:  true,
		},
		{
			name:        "group deployToAll false -> no marker",
			deployToAll: &flexible.Field[bool]{Value: false},
			wantAllEnv:  false,
		},
		{
			name:        "group deployToAll unspecified -> no marker",
			deployToAll: nil,
			wantAllEnv:  false,
		},
		{
			name:        "group deployToAll expression -> no marker",
			deployToAll: exprField("<+input>"),
			wantAllEnv:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			environments := &flexible.Field[[]*v0.Environment]{}
			environments.Set([]*v0.Environment{
				{
					EnvironmentRef:            "env1",
					InfrastructureDefinitions: infraDefs("infra1"),
				},
			})
			src := &v0.EnvironmentGroup{
				EnvGroupRef:  "group1",
				DeployToAll:  tt.deployToAll,
				Environments: environments,
			}
			got := ConvertEnvironmentGroup(src, NewStageConversionContext())
			if got == nil {
				t.Fatalf("expected an EnvironmentRef, got nil")
			}
			groupConfig, ok := got.Group.(map[string]interface{})
			if !ok {
				t.Fatalf("expected a group config map, got %#v", got.Group)
			}
			allEnv, present := groupConfig["all-env"]
			if tt.wantAllEnv {
				if !present || allEnv != true {
					t.Fatalf("expected all-env=true, got %#v", groupConfig)
				}
			} else if present {
				t.Fatalf("expected no all-env key, got %#v", groupConfig)
			}
			// The marker never replaces items: the per-environment config must survive.
			if _, present := groupConfig["items"]; !present {
				t.Fatalf("expected items alongside the group config, got %#v", groupConfig)
			}
		})
	}
}

// A group ref with no environments still carries the marker, so the server resolves the
// full environment list at execution time.
func TestConvertEnvironmentGroup_AllEnvMarkerWithoutEnvironments(t *testing.T) {
	src := &v0.EnvironmentGroup{
		EnvGroupRef: "group1",
		DeployToAll: &flexible.Field[bool]{Value: true},
	}
	got := ConvertEnvironmentGroup(src, NewStageConversionContext())
	if got == nil {
		t.Fatalf("expected an EnvironmentRef, got nil")
	}
	want := map[string]interface{}{"id": "group1", "all-env": true}
	if !reflect.DeepEqual(got.Group, want) {
		t.Fatalf("expected group=%#v, got group=%#v", want, got.Group)
	}
}

func TestConvertDeploymentInfrastructure_AllInfraMarker(t *testing.T) {
	tests := []struct {
		name         string
		infraDef     v0.InfrastructureDefinition
		wantDeployTo interface{}
		wantAllInfra bool
	}{
		{
			name:         "no infrastructure definition -> marker only",
			infraDef:     v0.InfrastructureDefinition{},
			wantDeployTo: nil,
			wantAllInfra: true,
		},
		{
			name:         "explicit infrastructure definition -> deploy-to only",
			infraDef:     v0.InfrastructureDefinition{Identifier: "infra1"},
			wantDeployTo: "infra1",
			wantAllInfra: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &v0.DeploymentInfrastructure{
				EnvironmentRef:           "env1",
				InfrastructureDefinition: tt.infraDef,
			}
			got := ConvertDeploymentInfrastructure(src)
			items := got.ItemList()
			if len(items) != 1 {
				t.Fatalf("expected 1 environment item, got %d", len(items))
			}
			if items[0].AllInfra != tt.wantAllInfra {
				t.Fatalf("expected all-infra=%v, got all-infra=%v", tt.wantAllInfra, items[0].AllInfra)
			}
			if !reflect.DeepEqual(items[0].DeployTo, tt.wantDeployTo) {
				t.Fatalf("expected deploy-to=%#v, got deploy-to=%#v", tt.wantDeployTo, items[0].DeployTo)
			}
		})
	}
}
