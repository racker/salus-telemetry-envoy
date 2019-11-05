/*
 * Copyright 2019 Rackspace US, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agents

import (
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseInfluxLineProtocolMetrics(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []*telemetry_edge.NameTagValueMetric
		wantErr string
	}{
		{name: "float_d.d", content: "cpu value=1.2 1000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 1, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 1.2}, Svalues: map[string]string{}},
		}},
		{name: "bad timestamp", content: "cpu value=1.2 5.67", wantErr: "invalid integer"},
		{name: "missing timestamp", content: "cpu value=1.2", wantErr: "expected <space>"},
		{name: "missing fields", content: "cpu", wantErr: "expected <space>"},
		{name: "float_.d", content: "cpu value=.2 2000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 2, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 0.2}, Svalues: map[string]string{}},
		}},
		{name: "float_d", content: "cpu value=5 3000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 3, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 5}, Svalues: map[string]string{}},
		}},
		{name: "float_d.e", content: "cpu value=1.e+78 4000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 4, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 1e78}, Svalues: map[string]string{}},
		}},
		{name: "float_d.de", content: "cpu value=1.2e+78 5000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 5, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 1.2e78}, Svalues: map[string]string{}},
		}},
		{name: "float_d.E", content: "cpu value=1.E+78 6000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 6, Tags: map[string]string{},
				Fvalues: map[string]float64{"value": 1e78}, Svalues: map[string]string{}},
		}},
		{name: "int", content: "cpu ival=5i 7000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 7, Tags: map[string]string{},
				Fvalues: map[string]float64{"ival": 5}, Svalues: map[string]string{}},
		}},
		{name: "simple string", content: "disk value=rw 8000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "disk", Timestamp: 8, Tags: map[string]string{},
				Fvalues: map[string]float64{}, Svalues: map[string]string{"value": "rw"}},
		}},
		{name: "quoted string", content: `disk str="some string" 9000000`, want: []*telemetry_edge.NameTagValueMetric{
			{Name: "disk", Timestamp: 9, Tags: map[string]string{},
				Fvalues: map[string]float64{}, Svalues: map[string]string{"str": "some string"}},
		}},
		{name: "tags", content: `disk,fstype=devfs,path=/dev free=0i 1000000`, want: []*telemetry_edge.NameTagValueMetric{
			{Name: "disk", Timestamp: 1, Tags: map[string]string{"fstype": "devfs", "path": "/dev"},
				Fvalues: map[string]float64{"free": 0}, Svalues: map[string]string{}},
		}},
		{name: "bool", content: `net_response,server=localhost string_found=false 2000000`, want: []*telemetry_edge.NameTagValueMetric{
			{Name: "net_response", Timestamp: 2, Tags: map[string]string{"server": "localhost"},
				Fvalues: map[string]float64{}, Svalues: map[string]string{"string_found": "false"}},
		}},
		{name: "int tagset value", content: `net_response,port=8080 result_code=3i 3000000`, want: []*telemetry_edge.NameTagValueMetric{
			{Name: "net_response", Timestamp: 3, Tags: map[string]string{"port": "8080"},
				Fvalues: map[string]float64{"result_code": 3}, Svalues: map[string]string{}},
		}},
		{name: "test output prefixed", content: "> cpu ival=5i 4000000", want: []*telemetry_edge.NameTagValueMetric{
			{Name: "cpu", Timestamp: 4, Tags: map[string]string{},
				Fvalues: map[string]float64{"ival": 5}, Svalues: map[string]string{}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := ParseInfluxLineProtocolMetrics([]byte(tt.content))
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.want, results)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
