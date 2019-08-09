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
	"github.com/racker/telemetry-envoy/telemetry_edge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseInfluxLineProtocolMetrics(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantErr  bool
	}{
		{name: "float_d.d", content: "cpu value=1.2 1", wantName: "cpu"},
		{name: "float_.d", content: "cpu value=.2 1", wantName: "cpu"},
		{name: "float_d", content: "cpu value=5 1", wantName: "cpu"},
		{name: "float_d.e", content: "cpu value=1.e+78 1", wantName: "cpu"},
		{name: "float_d.de", content: "cpu value=1.2e+78 1", wantName: "cpu"},
		{name: "float_d.E", content: "cpu value=1.E+78 1", wantName: "cpu"},
		{name: "int", content: "cpu value=5i 1", wantName: "cpu"},
		{name: "simple string", content: "disk value=rw 1", wantName: "disk"},
		{name: "quoted string", content: `disk str="some string",ival=3i,fval=1.3 1`, wantName: "disk"},
		{name: "tags", content: `disk,fstype=devfs,path=/dev free=0i,inodes_used=628i,used_percent=100 1453832006274137913`, wantName: "disk"},
		{name: "bool and int tagset value", content: `net_response,port=8080,protocol=udp,result=read_failed,server=localhost result_code=3i,result_type="read_failed",string_found=false 1525820088000000000`, wantName: "net_response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := ParseInfluxLineProtocolMetrics([]byte(tt.content))
			if !tt.wantErr {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			assertHasMetricNamed(t, results, tt.wantName)
		})
	}
}

func assertHasMetricNamed(t *testing.T, metrics []*telemetry_edge.NameTagValueMetric, wantName string) {
	for _, metric := range metrics {
		if metric.Name == wantName {
			return
		}
	}
	assert.Failf(t, "missing named metric", "wanted %s", wantName)
}
